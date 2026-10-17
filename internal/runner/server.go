// Package runner is the code execution side of go-practice-web. It serves
// the runner API (see internal/runnerapi and docs/DESIGN.md): it formats Go
// in process, and runs go vet, staticcheck and go test on a learner's files
// in a fresh workspace, in a child process group with a minimal environment,
// a wall clock limit, an output cap and a bounded queue.
package runner

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// errShuttingDown is the cause of every job context when the server closes.
var errShuttingDown = errors.New("runner shutting down")

// Options configures a Server. Course, Token, WorkDir, GoCache and Go are
// required.
type Options struct {
	// Token is the expected bearer token.
	Token string
	// Course is the indexed course checkout.
	Course *Course
	// WorkDir holds the per-request workspaces. It must exist.
	WorkDir string
	// GoCache is the shared GOCACHE of every go invocation.
	GoCache string
	// Go is the path of the go command.
	Go string
	// Staticcheck is the path of staticcheck. Empty skips staticcheck.
	Staticcheck string
	// StaticcheckCache is staticcheck's cache directory. Without a shared
	// one every check analyses the standard library again.
	StaticcheckCache string
	// Path is the PATH of the child processes; go needs it to find the C
	// compiler for -race. Empty means the runner's own PATH.
	Path string
	// MaxConcurrent is the number of concurrent go invocations (at least 1).
	MaxConcurrent int
	// MaxQueue is the number of requests that may wait for a slot.
	MaxQueue int
	// MaxQueueWait bounds how long a test request waits for a slot. A
	// request that waited longer ends with a busy done event, so that the
	// wait and the run together stay under the proxy timeout in front of
	// the web server. 0 means no bound.
	MaxQueueWait time.Duration
	// TestTimeout is the wall clock limit of go test. go test itself gets
	// a -timeout a little below it; see goTestTimeout.
	TestTimeout time.Duration
	// CheckTimeout is the wall clock limit of go vet and staticcheck
	// together.
	CheckTimeout time.Duration
	// Logger receives 1 line per request. Nil discards.
	Logger *slog.Logger
	// Metrics receives the request metrics. Nil registers them on a private
	// registry.
	Metrics *Metrics

	// testTimeoutMargin replaces the const of the same name, so that a
	// test can make go test's alarm fire long before the runner's kill
	// without waiting 35 s. 0 means the const.
	testTimeoutMargin time.Duration
}

// Server serves the runner API. Create it with New.
type Server struct {
	opts      Options
	tokenHash [sha256.Size]byte
	limiter   *Limiter
	log       *slog.Logger
	metrics   *Metrics
	sandbox   *sandbox

	// stopSweeps ends the periodic sweep for stray processes.
	stopSweeps func()

	// ctx is the parent of every job; Close cancels it, which kills every
	// running process group.
	ctx    context.Context
	cancel context.CancelCauseFunc

	mu       sync.Mutex
	closed   bool           // guarded by mu
	jobs     sync.WaitGroup // Add only under mu while !closed
	draining atomic.Bool

	// info is what Probe found; nil until it ran.
	info atomic.Pointer[runnerapi.Info]
}

// New validates opts and returns a Server.
func New(opts Options) (*Server, error) {
	switch {
	case opts.Token == "":
		return nil, errors.New("runner: token is required")
	case opts.Course == nil:
		return nil, errors.New("runner: course is required")
	case opts.WorkDir == "", opts.GoCache == "", opts.Go == "":
		return nil, errors.New("runner: work dir, GOCACHE and the go command are required")
	case opts.TestTimeout <= 0, opts.CheckTimeout <= 0:
		return nil, errors.New("runner: timeouts must be positive")
	case opts.MaxQueueWait < 0:
		return nil, errors.New("runner: the queue wait must not be negative")
	}
	if ok, err := isDir(opts.WorkDir); err != nil {
		return nil, fmt.Errorf("runner: work dir: %w", err)
	} else if !ok {
		return nil, fmt.Errorf("runner: work dir %s does not exist", opts.WorkDir)
	}
	if opts.Path == "" {
		opts.Path = os.Getenv("PATH")
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Metrics == nil {
		opts.Metrics = NewMetrics(prometheus.NewRegistry())
	}
	if opts.testTimeoutMargin == 0 {
		opts.testTimeoutMargin = testTimeoutMargin
	}
	// The sandbox can only grant access to directories that exist; a cache
	// that go or staticcheck created later would be out of reach.
	caches := []string{opts.GoCache}
	if opts.Staticcheck != "" && opts.StaticcheckCache != "" {
		caches = append(caches, opts.StaticcheckCache)
	}
	for _, dir := range caches {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("runner: create cache: %w", err)
		}
	}
	sb, err := newSandbox(opts)
	if err != nil {
		return nil, err
	}
	if err := enableSubreaper(); err != nil {
		return nil, fmt.Errorf("runner: become a child subreaper: %w", err)
	}

	s := &Server{
		opts:      opts,
		tokenHash: sha256.Sum256([]byte(opts.Token)),
		limiter:   NewLimiter(opts.MaxConcurrent, opts.MaxQueue),
		log:       opts.Logger,
		metrics:   opts.Metrics,
		sandbox:   sb,
	}
	s.limiter.onChange = s.metrics.limiterChanged
	s.ctx, s.cancel = context.WithCancelCause(context.Background())
	// "level" would clash with the log level key of the JSON handler.
	if enforced, ok := sb.describe(); ok {
		s.log.Info("sandbox for runs", "sandbox", enforced)
	} else {
		s.log.Warn("sandbox for runs", "sandbox", enforced)
	}
	s.stopSweeps = s.sweepEvery(sweepInterval)
	return s, nil
}

// sweepInterval is how often the server looks for stray processes besides
// the sweep after every command. A stray found here escaped while its run
// was still going, or survived an earlier sweep.
const sweepInterval = 5 * time.Second

// sweepEvery sweeps for stray processes every interval until the returned
// function is called; that function waits for a sweep in progress.
func (s *Server) sweepEvery(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				s.reportStrays(procs.sweep())
			}
		}
	})
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}

// reportStrays logs and counts the processes a sweep killed. Any number
// above 0 means learner code tried to outlive its run.
func (s *Server) reportStrays(killed int) {
	if killed == 0 {
		return
	}
	s.metrics.strays.Add(float64(killed))
	s.log.Warn("killed processes that outlived their run", "count", killed)
}

// Handler returns the HTTP handler of the runner API, /healthz and /readyz.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET "+runnerapi.PathInfo, s.handleInfo)
	mux.Handle("POST "+runnerapi.PathCheck, s.api(kindCheck, s.handleCheck))
	mux.Handle("POST "+runnerapi.PathFormat, s.api(kindFormat, s.handleFormat))
	mux.Handle("POST "+runnerapi.PathTest, s.api(kindTest, s.handleTest))
	return mux
}

// Drain makes /readyz fail and new API requests get 503, so that traffic
// moves away while running jobs finish.
func (s *Server) Drain() {
	s.draining.Store(true)
}

// Close kills every running and queued job and waits until their handlers
// returned or ctx is done. Use it when the shutdown grace period is over.
func (s *Server) Close(ctx context.Context) error {
	s.Drain()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel(errShuttingDown)

	done := make(chan struct{})
	go func() {
		s.jobs.Wait()
		s.stopSweeps()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("runner: jobs still running: %w", ctx.Err())
	}
}

// The request kinds, as used in logs and metrics.
const (
	kindCheck  = "check"
	kindFormat = "format"
	kindTest   = "test"
)

// api wraps an API handler with the token check, the drain check and job
// accounting.
func (s *Server) api(kind string, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			s.metrics.observe(kind, "unauthorized", 0)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if s.draining.Load() || !s.enter() {
			s.metrics.observe(kind, "unavailable", 0)
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusServiceUnavailable, "shutting_down", "the runner is shutting down")
			return
		}
		defer s.jobs.Done()
		h(w, r)
	})
}

func (s *Server) enter() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.jobs.Add(1)
	return true
}

// authorized compares the bearer token in constant time. Hashing both
// sides first makes the comparison independent of the token length too.
func (s *Server) authorized(r *http.Request) bool {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], s.tokenHash[:]) == 1
}

// jobContext returns a context that ends with the request or when the
// server closes, whichever comes first.
func (s *Server) jobContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	stop := context.AfterFunc(s.ctx, func() { cancel(errShuttingDown) })
	return ctx, func() {
		stop()
		cancel(context.Canceled)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writePlain(w, http.StatusOK, "ok\n")
}

// probeTimeout bounds the readiness probe. go version takes milliseconds;
// a probe that hangs longer means the node is in trouble.
const probeTimeout = 5 * time.Second

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writePlain(w, http.StatusServiceUnavailable, "shutting down\n")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	if err := s.probe(ctx); err != nil {
		s.log.WarnContext(ctx, "readiness probe failed", "err", err)
		writePlain(w, http.StatusServiceUnavailable, "not ready: "+err.Error()+"\n")
		return
	}
	// Ready either way: runs without -race still work. The second line
	// tells an operator who looks at the probe why the IDE says so.
	body := "ok\n"
	if info := s.info.Load(); info != nil && !info.RaceAvailable {
		body += "race detector not available: " + info.RaceError + "\n"
	}
	writePlain(w, http.StatusOK, body)
}

// probe checks that go runs and the work dir is writable. The course was
// loaded before the Server existed.
func (s *Server) probe(ctx context.Context) error {
	f, err := os.CreateTemp(s.opts.WorkDir, workspacePrefix+"probe-")
	if err != nil {
		return fmt.Errorf("work dir not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("work dir: %w", err)
	}
	if _, err := s.GoVersion(ctx); err != nil {
		return err
	}
	return nil
}

// GoVersion runs go version with the children's environment.
func (s *Server) GoVersion(ctx context.Context) (string, error) {
	var out strings.Builder
	ws, err := newScratch(s.opts.WorkDir)
	if err != nil {
		return "", fmt.Errorf("go version: %w", err)
	}
	defer func() { _ = ws.Close() }()
	code, err := run(ctx, command{
		path: s.opts.Go, args: []string{"version"}, dir: ws.root,
		env: s.env(ws), stdout: &out, stderr: &out,
		sandbox: s.sandbox, workspace: ws.root, onStrays: s.reportStrays,
	})
	if err != nil {
		return "", fmt.Errorf("go version: %w", err)
	}
	if code != 0 {
		return "", fmt.Errorf("go version: exit code %d: %s", code, strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// env is the complete environment of a child process. Nothing of the
// runner's own environment leaks in except PATH: the children run learner
// code, and the runner's token must stay out of their reach.
func (s *Server) env(ws *workspace) []string {
	const deadProxy = "http://127.0.0.1:9"
	return []string{
		"HOME=" + ws.home,
		"PATH=" + s.opts.Path,
		"GOCACHE=" + s.opts.GoCache,
		"GOPATH=" + ws.gopath,
		"TMPDIR=" + ws.tmp,
		"GOFLAGS=-mod=mod",
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
		"GOTELEMETRY=off",
		"CGO_ENABLED=1",
		"HTTP_PROXY=" + deadProxy,
		"HTTPS_PROXY=" + deadProxy,
		"ALL_PROXY=" + deadProxy,
	}
}

// writeJSON writes v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a runnerapi.Error.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, runnerapi.Error{Error: msg, Code: code})
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
