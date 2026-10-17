package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

const (
	// maxBodyBytes bounds a request body. JSON escaping can grow content
	// up to 6 times (a control byte becomes \u00XX), so the limit is well
	// above MaxTotalBytes; the decoded files are checked exactly after.
	maxBodyBytes = 6*runnerapi.MaxTotalBytes + 64<<10
	// maxRunPattern bounds the -run regular expression.
	maxRunPattern = 1024
	// retryAfterSeconds is the hint sent with 429. A typical run takes a
	// few seconds, so this is when a slot is likely to be free.
	retryAfterSeconds = 5
	// streamWriteTimeout bounds 1 write of the test stream, so that a
	// client that stops reading cannot hold a run (and its slot) forever.
	streamWriteTimeout = 15 * time.Second
)

// requestError is a 4xx answer: a status, a machine code and a message.
type requestError struct {
	status int
	code   string
	msg    string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(code, format string, args ...any) *requestError {
	return &requestError{status: http.StatusBadRequest, code: code, msg: fmt.Sprintf(format, args...)}
}

// decode reads a JSON body of at most maxBodyBytes into v.
func decode(w http.ResponseWriter, r *http.Request, v any) *requestError {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &requestError{status: http.StatusRequestEntityTooLarge, code: "too_large", msg: "request body too large"}
		}
		return badRequest("bad_json", "invalid JSON body: %v", err)
	}
	return nil
}

// exercise looks up an exercise by name. The name is only ever a map key,
// never part of a path.
func (s *Server) exercise(name string) (*Exercise, *requestError) {
	ex, ok := s.opts.Course.Exercise(name)
	if !ok {
		return nil, badRequest("unknown_exercise", "unknown exercise %q", name)
	}
	return ex, nil
}

// validateFiles checks that files holds exactly the exercise's stub files
// within the size limits. Accepting only the known stub names is what
// keeps a request from writing outside its workspace or replacing a test.
func validateFiles(ex *Exercise, files map[string]string) *requestError {
	total := 0
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if !ex.IsStub(name) {
			return badRequest("bad_file", "%q is not a file of %s; send only %v", name, ex.Name, ex.Stubs)
		}
		if err := checkFileSize(name, files[name]); err != nil {
			return err
		}
		total += len(files[name])
	}
	if total > runnerapi.MaxTotalBytes {
		return badRequest("too_large", "files total %d bytes, the limit is %d", total, runnerapi.MaxTotalBytes)
	}
	for _, name := range ex.Stubs {
		if _, ok := files[name]; !ok {
			return badRequest("missing_file", "missing file %q of %s", name, ex.Name)
		}
	}
	return nil
}

func checkFileSize(name, content string) *requestError {
	if len(content) > runnerapi.MaxFileBytes {
		return badRequest("file_too_large", "%s has %d bytes, the limit is %d", name, len(content), runnerapi.MaxFileBytes)
	}
	return nil
}

// requestLog collects what the request log line and the metrics need.
type requestLog struct {
	kind     string
	exercise string
	result   string
	waited   bool // the request went through the limiter
	wait     time.Duration
	start    time.Time // of the work, after the queue
}

func (s *Server) finish(ctx context.Context, l *requestLog) {
	d := time.Duration(0)
	if !l.start.IsZero() {
		d = time.Since(l.start)
	}
	s.metrics.observe(l.kind, l.result, d)
	if l.waited {
		s.metrics.wait.Observe(l.wait.Seconds())
	}
	s.log.InfoContext(ctx, "runner request",
		"kind", l.kind, "exercise", l.exercise, "result", l.result,
		"wait_ms", l.wait.Milliseconds(), "duration_ms", d.Milliseconds())
}

// fail writes a request error and records the result.
func (s *Server) fail(w http.ResponseWriter, l *requestLog, err *requestError) {
	l.result = err.code
	writeError(w, err.status, err.code, err.msg)
}

// join takes a ticket at the limiter or answers 429.
func (s *Server) join(w http.ResponseWriter, l *requestLog) (*Ticket, bool) {
	t, err := s.limiter.Join()
	if err != nil {
		l.result = "queue_full"
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
		writeError(w, http.StatusTooManyRequests, "queue_full", "the runner is busy; try again in a few seconds")
		return nil, false
	}
	return t, true
}

func (s *Server) handleFormat(w http.ResponseWriter, r *http.Request) {
	l := &requestLog{kind: kindFormat, start: time.Now()}
	defer s.finish(r.Context(), l)

	var req runnerapi.FormatRequest
	if err := decode(w, r, &req); err != nil {
		s.fail(w, l, err)
		return
	}
	l.exercise = req.Exercise
	ex, rerr := s.exercise(req.Exercise)
	if rerr != nil {
		s.fail(w, l, rerr)
		return
	}
	if !ex.IsStub(req.File) {
		s.fail(w, l, badRequest("bad_file", "%q is not a file of %s; send only %v", req.File, ex.Name, ex.Stubs))
		return
	}
	if rerr := checkFileSize(req.File, req.Content); rerr != nil {
		s.fail(w, l, rerr)
		return
	}

	content, diags, err := formatFile(req.File, req.Content)
	if err != nil {
		s.log.ErrorContext(r.Context(), "format failed", "err", err)
		l.result = "error"
		writeError(w, http.StatusInternalServerError, "internal", "formatting failed")
		return
	}
	l.result = "ok"
	if len(diags) > 0 {
		l.result = "syntax_error"
	}
	writeJSON(w, http.StatusOK, runnerapi.FormatResponse{Content: content, Diagnostics: nonNil(diags)})
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	l := &requestLog{kind: kindCheck}
	defer s.finish(r.Context(), l)

	var req runnerapi.CheckRequest
	if err := decode(w, r, &req); err != nil {
		s.fail(w, l, err)
		return
	}
	l.exercise = req.Exercise
	ex, rerr := s.exercise(req.Exercise)
	if rerr == nil {
		rerr = validateFiles(ex, req.Files)
	}
	if rerr != nil {
		s.fail(w, l, rerr)
		return
	}

	// A file that does not parse makes vet report the same error less
	// clearly, so the in-process check answers alone and takes no slot.
	start := time.Now()
	if diags := syntaxCheck(req.Files); len(diags) > 0 {
		l.start, l.result = start, "syntax_error"
		writeJSON(w, http.StatusOK, runnerapi.CheckResponse{Diagnostics: diags, DurationMs: time.Since(start).Milliseconds()})
		return
	}

	ticket, ok := s.join(w, l)
	if !ok {
		return
	}
	defer ticket.Release()
	ctx, cancel := s.jobContext(r.Context())
	defer cancel()
	err := ticket.Wait(ctx, nil)
	l.waited, l.wait = true, time.Since(start)
	if err != nil {
		l.result = "canceled"
		writeError(w, http.StatusServiceUnavailable, "canceled", "the check was canceled")
		return
	}
	l.start = time.Now()

	diags, err := s.runCheck(ctx, ex, req.Files)
	switch {
	case errors.Is(err, errCheckTimeout):
		l.result = "timeout"
		writeError(w, http.StatusGatewayTimeout, "timeout", "the check took too long")
	case err != nil && ctx.Err() != nil:
		l.result = "canceled"
		writeError(w, http.StatusServiceUnavailable, "canceled", "the check was canceled")
	case err != nil:
		s.log.ErrorContext(ctx, "check failed", "exercise", ex.Name, "err", err)
		l.result = "error"
		writeError(w, http.StatusInternalServerError, "internal", "the check failed")
	default:
		l.result = "ok"
		writeJSON(w, http.StatusOK, runnerapi.CheckResponse{
			Diagnostics: nonNil(diags),
			DurationMs:  time.Since(l.start).Milliseconds(),
		})
	}
}

// runPatternError reports why a -run pattern is unusable, or nil.
func runPatternError(run string) *requestError {
	if len(run) > maxRunPattern {
		return badRequest("bad_run", "the run pattern is longer than %d bytes", maxRunPattern)
	}
	if _, err := regexp.Compile(run); err != nil {
		return badRequest("bad_run", "invalid run pattern: %v", err)
	}
	return nil
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	l := &requestLog{kind: kindTest}
	defer s.finish(r.Context(), l)

	var req runnerapi.TestRequest
	if err := decode(w, r, &req); err != nil {
		s.fail(w, l, err)
		return
	}
	l.exercise = req.Exercise
	ex, rerr := s.exercise(req.Exercise)
	if rerr == nil {
		rerr = validateFiles(ex, req.Files)
	}
	if rerr == nil {
		rerr = runPatternError(req.Run)
	}
	if rerr != nil {
		s.fail(w, l, rerr)
		return
	}

	// A node whose race detector does not work runs the tests without it,
	// rather than fail every run; the done event says so.
	raceDropped := req.Race && !s.raceAvailable()
	if raceDropped {
		req.Race = false
	}

	queuedAt := time.Now()
	ticket, ok := s.join(w, l)
	if !ok {
		return
	}
	defer ticket.Release()
	ctx, cancel := s.jobContext(r.Context())
	defer cancel()

	stream := newEventStream(w, cancel)
	err := s.waitForSlot(ctx, ticket, func(position int) {
		stream.send(runnerapi.Event{Type: runnerapi.EventQueued, Position: position})
	})
	l.waited, l.wait = true, time.Since(queuedAt)
	switch {
	case errors.Is(err, errQueueWait):
		// The stream started with the queued events, so the answer is a done
		// event rather than a 429.
		l.result = "busy"
		stream.send(runnerapi.Event{Type: runnerapi.EventDone, ExitCode: -1, Busy: true})
		return
	case err != nil:
		l.result = "canceled"
		stream.send(runnerapi.Event{Type: runnerapi.EventDone, ExitCode: -1})
		return
	}

	l.start = time.Now()
	stream.send(runnerapi.Event{Type: runnerapi.EventStart})
	done, err := s.runTest(ctx, ex, req, stream.send)
	if err != nil {
		s.log.ErrorContext(ctx, "test run failed", "exercise", ex.Name, "err", err)
		l.result = "error"
		stream.send(runnerapi.Event{Type: runnerapi.EventDone, ExitCode: -1, DurationMs: time.Since(l.start).Milliseconds()})
		return
	}
	l.result = testResult(ctx, done)
	done.RaceUnavailable = raceDropped
	stream.send(done)
}

// errQueueWait is the cause of a test request that waited longer than
// Options.MaxQueueWait for a slot.
var errQueueWait = errors.New("waited too long for a slot")

// waitForSlot waits for the ticket's slot, at most MaxQueueWait. Behind
// the web server sits a proxy with a fixed timeout for the whole response
// (60 s for Traefik); a run that starts late would be cut off there with
// no result, so it does not start at all and the learner tries again.
func (s *Server) waitForSlot(ctx context.Context, t *Ticket, onPosition func(int)) error {
	if s.opts.MaxQueueWait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, s.opts.MaxQueueWait, errQueueWait)
		defer cancel()
	}
	return t.Wait(ctx, onPosition)
}

// testResult names the outcome of a test run for logs and metrics.
func testResult(ctx context.Context, done runnerapi.Event) string {
	switch {
	case done.TimedOut:
		return "timeout"
	case done.Truncated:
		return "truncated"
	case ctx.Err() != nil:
		return "canceled"
	case done.BuildFailed:
		return "build_failed"
	case done.ExitCode == 0:
		return "passed"
	default:
		return "failed"
	}
}

// eventStream writes NDJSON events and flushes after each, so that the
// learner sees tests finish as they finish. It is safe for concurrent use.
// When a write fails, the client is gone: it cancels the run and drops the
// remaining events.
type eventStream struct {
	mu       sync.Mutex
	w        http.ResponseWriter
	rc       *http.ResponseController
	enc      *json.Encoder
	started  bool
	err      error
	onFailed func()
}

func newEventStream(w http.ResponseWriter, onFailed func()) *eventStream {
	enc := json.NewEncoder(w)
	// Test output is full of < and >; escaping them only makes it bigger.
	enc.SetEscapeHTML(false)
	return &eventStream{w: w, rc: http.NewResponseController(w), enc: enc, onFailed: onFailed}
}

func (s *eventStream) send(ev runnerapi.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	if !s.started {
		s.started = true
		h := s.w.Header()
		h.Set("Content-Type", "application/x-ndjson")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		s.w.WriteHeader(http.StatusOK)
	}
	// Not every ResponseWriter supports deadlines (httptest's recorder does
	// not); the stream works without one.
	_ = s.rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
	err := s.enc.Encode(ev)
	if err == nil {
		err = s.rc.Flush()
	}
	if err != nil {
		s.err = err
		s.onFailed()
	}
}

// nonNil makes an empty list encode as [] instead of null.
func nonNil(diags []runnerapi.Diagnostic) []runnerapi.Diagnostic {
	if diags == nil {
		return []runnerapi.Diagnostic{}
	}
	return diags
}
