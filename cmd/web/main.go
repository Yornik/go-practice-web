// Command web serves the go-practice web IDE: the pages, the JSON API and
// the proxy to the runner. See docs/DESIGN.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
	"github.com/yornik/go-practice-web/internal/web"
	webfiles "github.com/yornik/go-practice-web/web"
)

const (
	// shutdownGrace lets running requests end after SIGTERM. A test stream
	// can last longer; it is cut, and the runner kills its run. It stays
	// under the default terminationGracePeriodSeconds of 30.
	shutdownGrace = 25 * time.Second
	backupEvery   = 24 * time.Hour
	// firstBackup waits a little after start, so a crash loop does not
	// rewrite the backup on every start.
	firstBackup  = 10 * time.Minute
	cleanupEvery = time.Hour
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := serve(log); err != nil {
		log.Error("web server stopped", "err", err)
		os.Exit(1)
	}
}

func serve(log *slog.Logger) error {
	cfg, err := loadConfig(os.Environ())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		return fmt.Errorf("create the database directory: %w", err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Error("close database", "err", err)
		}
	}()

	start := time.Now()
	crs, err := course.Load(cfg.CourseDir)
	if err != nil {
		return err
	}
	log.Info("course loaded", "dir", crs.Dir, "version", crs.Version(), "exercises", len(crs.Exercises),
		"duration_ms", time.Since(start).Milliseconds())
	if err := explain.Check(); err != nil {
		// A broken catalog entry is left out; the rest still teaches.
		log.Warn("explain catalog has broken entries", "err", err)
	}

	signer, err := cert.LoadSigner(ctx, st)
	if err != nil {
		return err
	}
	log.Info("certificate signing key ready", "fingerprint", signer.Fingerprint())

	sender, err := mail.NewSender(cfg.SMTP, log)
	if err != nil {
		return err
	}
	accounts, err := auth.New(st, mail.NewMailer(sender, cfg.PublicURL), auth.Config{
		PublicURL: cfg.PublicURL, Dev: cfg.Dev, Logger: log,
	})
	if err != nil {
		return err
	}
	defer accounts.Wait()
	link, err := accounts.EnsureSetupLink(ctx)
	if err != nil {
		return err
	}
	if link != "" {
		// The only way to create the first account; see docs/DESIGN.md,
		// "Authentication and accounts", flow 1.
		log.Warn("setup link: " + link)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		web.NewStoreCollector(st),
	)
	templates, static := assets(cfg.Dev, log)
	site, err := web.New(web.Config{
		PublicURL: cfg.PublicURL,
		Dev:       cfg.Dev,
		Store:     st,
		Course:    crs,
		Auth:      accounts,
		Runner:    web.NewRunnerClient(cfg.RunnerURL, cfg.RunnerToken, nil),
		Signer:    signer,
		IPs:       auth.NewIPResolver(cfg.TrustedProxies),
		Templates: templates,
		Static:    static,
		Logger:    log,
		Metrics:   reg,
	})
	if err != nil {
		return err
	}

	api := siteServer(site.Handler(), log)
	metrics := &http.Server{
		Handler:           promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	var lc net.ListenConfig
	apiLn, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on GPW_ADDR: %w", err)
	}
	metricsLn, err := lc.Listen(ctx, "tcp", cfg.MetricsAddr)
	if err != nil {
		_ = apiLn.Close()
		return fmt.Errorf("listen on GPW_METRICS_ADDR: %w", err)
	}
	serveErr := make(chan error, 2)
	go func() { serveErr <- api.Serve(apiLn) }()
	go func() { serveErr <- metrics.Serve(metricsLn) }()
	log.Info("web server listening", "addr", cfg.Addr, "metrics_addr", cfg.MetricsAddr,
		"public_url", cfg.PublicURL, "dev", cfg.Dev)

	var loops sync.WaitGroup
	loops.Go(func() { every(ctx, firstBackup, backupEvery, func() { backup(ctx, log, st) }) })
	loops.Go(func() { every(ctx, cleanupEvery, cleanupEvery, func() { cleanup(ctx, log, st) }) })

	select {
	case err := <-serveErr:
		stop()
		loops.Wait()
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	log.Info("shutting down")
	stop() // a second signal kills the process the default way
	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := api.Shutdown(shutCtx); err != nil {
		log.Warn("requests still running at the end of the grace period", "err", err)
		_ = api.Close()
	}
	_ = metrics.Close()
	loops.Wait()
	log.Info("web server stopped")
	return nil
}

// siteServer is the public HTTP server. Its limits stop slow or oversized
// requests; slow readers are handled by the site, which gives every
// response a write deadline.
func siteServer(h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// No WriteTimeout: a test run streams for as long as it runs. The
		// site sets the deadlines itself.
		IdleTimeout: 2 * time.Minute,
		// A request carries a session cookie and a few small headers; the
		// default of 1 MiB per request only helps a flood.
		MaxHeaderBytes: 64 << 10,
		ErrorLog:       slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// assets returns the templates and static files: from the repository's
// web/ directory in dev mode when it is there, so an edit shows on reload,
// and the embedded copy otherwise.
func assets(dev bool, log *slog.Logger) (templates, static fs.FS) {
	if dev {
		if st, err := os.Stat(filepath.Join("web", "templates")); err == nil && st.IsDir() {
			log.Info("dev mode: templates and static files are read from ./web")
			return os.DirFS(filepath.Join("web", "templates")), os.DirFS(filepath.Join("web", "static"))
		}
	}
	return webfiles.Templates(), webfiles.Static()
}

// every runs fn after first and then every period until ctx ends.
func every(ctx context.Context, first, period time.Duration, fn func()) {
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
			t.Reset(period)
		}
	}
}

// backup writes the daily copy next to the database.
func backup(ctx context.Context, log *slog.Logger, st *store.Store) {
	start := time.Now()
	path, err := st.Backup(ctx, filepath.Join(filepath.Dir(st.Path()), "backups"))
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error("backup failed", "err", err)
		}
		return
	}
	log.Info("backup written", "path", path, "duration_ms", time.Since(start).Milliseconds())
}

// cleanup drops expired sessions, ceremonies and old email tokens.
func cleanup(ctx context.Context, log *slog.Logger, st *store.Store) {
	res, err := st.Cleanup(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error("cleanup failed", "err", err)
		}
		return
	}
	log.Info("cleanup done", "sessions", res.Sessions, "ceremonies", res.Ceremonies, "email_tokens", res.EmailTokens)
}
