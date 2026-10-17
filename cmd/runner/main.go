// Command runner serves the go-practice runner API: it formats, vets and
// tests a learner's Go code for the web server. See the Runner section of
// docs/DESIGN.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/yornik/go-practice-web/internal/runner"
)

const (
	// shutdownExtra is added to GPR_TEST_TIMEOUT to give the grace period
	// after SIGTERM: long enough for a run that just started to finish and
	// send its result. The Deployment's terminationGracePeriodSeconds must
	// be above the sum plus killWait.
	shutdownExtra = 5 * time.Second
	// killWait bounds the wait for killed jobs to end their responses.
	killWait = 5 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := serve(log); err != nil {
		log.Error("runner stopped", "err", err)
		os.Exit(1)
	}
}

func serve(log *slog.Logger) error {
	cfg, err := loadConfig(os.Environ())
	if err != nil {
		return err
	}
	if err := harden(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	srv, err := newServer(ctx, log, cfg, runner.NewMetrics(reg))
	if err != nil {
		return err
	}

	api := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Bodies are at most a few MiB from inside the cluster.
		ReadTimeout: 30 * time.Second,
		// No WriteTimeout: a test stream lasts for the queue wait plus the
		// run. The stream bounds each write itself.
		IdleTimeout: 2 * time.Minute,
		ErrorLog:    slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	metrics := &http.Server{
		Handler:           promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// Listen before logging "ready", so that a busy port fails the start.
	var lc net.ListenConfig
	apiLn, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on GPR_ADDR: %w", err)
	}
	metricsLn, err := lc.Listen(ctx, "tcp", cfg.MetricsAddr)
	if err != nil {
		_ = apiLn.Close()
		return fmt.Errorf("listen on GPR_METRICS_ADDR: %w", err)
	}
	serveErr := make(chan error, 2)
	go func() { serveErr <- api.Serve(apiLn) }()
	go func() { serveErr <- metrics.Serve(metricsLn) }()
	log.Info("runner listening", "addr", cfg.Addr, "metrics_addr", cfg.MetricsAddr)

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	log.Info("shutting down: draining running jobs")
	stop() // a second signal kills the process the default way
	srv.Drain()

	grace := cfg.TestTimeout + shutdownExtra
	graceCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := api.Shutdown(graceCtx); err != nil {
		log.Warn("grace period over: killing running jobs", "grace", grace.String())
		killCtx, cancelKill := context.WithTimeout(context.Background(), killWait)
		defer cancelKill()
		if err := srv.Close(killCtx); err != nil {
			log.Error("jobs did not end", "err", err)
		}
		_ = api.Close()
	}
	_ = metrics.Close()
	log.Info("runner stopped")
	return nil
}

// newServer prepares the directories and the toolchain and returns the
// server. Every problem here stops the start: a runner that cannot run go
// must not report ready.
func newServer(ctx context.Context, log *slog.Logger, cfg config, metrics *runner.Metrics) (*runner.Server, error) {
	course, err := runner.LoadCourse(cfg.CourseDir)
	if err != nil {
		return nil, err
	}
	log.Info("course loaded", "dir", course.Dir(), "exercises", len(course.Exercises()))

	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("find the go command: %w", err)
	}
	// staticcheck is optional: without it the check reports vet only.
	staticcheck, err := exec.LookPath("staticcheck")
	if err != nil {
		log.Info("staticcheck not found; checks run go vet only")
		staticcheck = ""
	}

	if err := os.MkdirAll(cfg.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("create GPR_WORK_DIR: %w", err)
	}
	// Workspaces of a previous process are garbage: their requests are
	// gone.
	if err := runner.RemoveStaleWorkspaces(cfg.WorkDir); err != nil {
		return nil, fmt.Errorf("clear GPR_WORK_DIR: %w", err)
	}

	start := time.Now()
	seeded, err := runner.SeedCache(ctx, cfg.GoCache, cfg.GoCacheSeed)
	if err != nil {
		return nil, err
	}
	if seeded {
		log.Info("build cache seeded", "from", cfg.GoCacheSeed, "duration_ms", time.Since(start).Milliseconds())
	}
	if err := os.MkdirAll(cfg.GoCache, 0o700); err != nil {
		return nil, fmt.Errorf("create GPR_GOCACHE: %w", err)
	}
	// staticcheck keeps its own cache next to the build cache, on the same
	// volume, so that it survives requests like GOCACHE does.
	staticcheckCache := filepath.Join(filepath.Dir(cfg.GoCache), "staticcheck")
	if err := os.MkdirAll(staticcheckCache, 0o700); err != nil {
		return nil, fmt.Errorf("create staticcheck cache: %w", err)
	}

	srv, err := runner.New(runner.Options{
		Token:            cfg.Token,
		Course:           course,
		WorkDir:          cfg.WorkDir,
		GoCache:          cfg.GoCache,
		Go:               goPath,
		Staticcheck:      staticcheck,
		StaticcheckCache: staticcheckCache,
		MaxConcurrent:    cfg.MaxConcurrent,
		MaxQueue:         cfg.MaxQueue,
		TestTimeout:      cfg.TestTimeout,
		CheckTimeout:     cfg.CheckTimeout,
		Logger:           log,
		Metrics:          metrics,
	})
	if err != nil {
		return nil, err
	}
	version, err := srv.GoVersion(ctx)
	if err != nil {
		return nil, err
	}
	log.Info("toolchain ready", "go", goPath, "version", version, "staticcheck", staticcheck)
	return srv, nil
}
