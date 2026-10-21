package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// RunnerClient calls the runner's API (docs/DESIGN.md, "Runner") with the
// bearer token.
type RunnerClient struct {
	base    string
	token   string
	hc      *http.Client
	metrics *metrics
}

// NewRunnerClient returns a client for the runner at baseURL. hc nil uses
// a client without an overall timeout: a test stream lasts as long as the
// run, and every call has a context deadline instead.
func NewRunnerClient(baseURL, token string, hc *http.Client) *RunnerClient {
	if hc == nil {
		hc = &http.Client{Transport: &http.Transport{
			Proxy:                 nil, // the runner is inside the cluster
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 0, // a queued test answers only when it starts
		}}
	}
	return &RunnerClient{base: strings.TrimSuffix(baseURL, "/"), token: token, hc: hc}
}

func (c *RunnerClient) setMetrics(m *metrics) { c.metrics = m }

// maxRunnerJSON bounds a check or format answer: diagnostics from at most
// 1 MiB of tool output, or 1 formatted file, with JSON escaping.
const maxRunnerJSON = 16 << 20

// Deadlines of the calls. A check queues behind at most a few runs; a test
// may wait in the queue for GPR_MAX_QUEUE_WAIT and then run for
// GPR_TEST_TIMEOUT. The runner answers info from memory.
const (
	checkTimeout  = 60 * time.Second
	formatTimeout = 15 * time.Second
	testTimeout   = 5 * time.Minute
	infoTimeout   = 5 * time.Second
)

// RunnerError is a failed runner call, ready for the learner: the status
// and code to answer with and a message that says what to do.
type RunnerError struct {
	Status     int
	Code       string
	Message    string
	RetryAfter string
	Err        error
}

func (e *RunnerError) Error() string {
	if e.Err != nil {
		return "runner: " + e.Code + ": " + e.Err.Error()
	}
	return "runner: " + e.Code + ": " + e.Message
}

func (e *RunnerError) Unwrap() error { return e.Err }

// Check runs the gofmt syntax check, go vet and staticcheck.
func (c *RunnerClient) Check(ctx context.Context, req runnerapi.CheckRequest) (runnerapi.CheckResponse, error) {
	var out runnerapi.CheckResponse
	err := c.callJSON(ctx, "check", runnerapi.PathCheck, checkTimeout, req, &out)
	return out, err
}

// Info asks the runner what it can do: its Go release and whether its
// race detector works.
func (c *RunnerClient) Info(ctx context.Context) (runnerapi.Info, error) {
	ctx, cancel := context.WithTimeout(ctx, infoTimeout)
	defer cancel()
	start := time.Now()
	var out runnerapi.Info
	resp, err := c.do(ctx, http.MethodGet, runnerapi.PathInfo, nil)
	if err == nil {
		err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out)
		_ = resp.Body.Close()
		if err != nil {
			err = &RunnerError{Status: http.StatusBadGateway, Code: "runner_error", Message: "The runner sent an answer the site cannot read.", Err: err}
		}
	}
	c.observe("info", err, start)
	return out, err
}

// Format formats 1 file.
func (c *RunnerClient) Format(ctx context.Context, req runnerapi.FormatRequest) (runnerapi.FormatResponse, error) {
	var out runnerapi.FormatResponse
	err := c.callJSON(ctx, "format", runnerapi.PathFormat, formatTimeout, req, &out)
	return out, err
}

func (c *RunnerClient) callJSON(ctx context.Context, kind, path string, timeout time.Duration, req, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	resp, err := c.post(ctx, path, req)
	if err != nil {
		c.observe(kind, err, start)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// The runner's answers carry what learner code printed; the runner caps
	// that, and this cap keeps a broken runner from exhausting memory here.
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRunnerJSON)).Decode(out); err != nil {
		err = &RunnerError{Status: http.StatusBadGateway, Code: "runner_error", Message: "The runner sent an answer the site cannot read.", Err: err}
		c.observe(kind, err, start)
		return err
	}
	c.observe(kind, nil, start)
	return nil
}

func (c *RunnerClient) observe(kind string, err error, start time.Time) {
	if c.metrics == nil {
		return
	}
	result := "ok"
	if err != nil {
		result = "error"
		var re *RunnerError
		if errors.As(err, &re) {
			result = re.Code
		}
	}
	c.metrics.runnerCall(kind, result, time.Since(start))
}

// post sends a JSON request and returns a 200 response; any other status
// becomes a *RunnerError.
func (c *RunnerClient) post(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// do sends a request, with body as JSON unless it is nil, and returns a
// 200 response; any other status becomes a *RunnerError.
func (c *RunnerClient) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("runner: encode request: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, context.DeadlineExceeded) {
			return nil, &RunnerError{Status: http.StatusGatewayTimeout, Code: "runner_timeout",
				Message: "The runner took too long to answer. Try again.", Err: err}
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, &RunnerError{Status: http.StatusServiceUnavailable, Code: "runner_unavailable",
			Message: "The test runner is not reachable right now. Your code is saved; try again in a minute.", Err: err}
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	return nil, runnerStatusError(resp)
}

// runnerStatusError turns a runner error response into what the learner
// sees. Problems with the request itself are the site's bugs or limits the
// learner hit; the runner's message says which.
func runnerStatusError(resp *http.Response) error {
	var body runnerapi.Error
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
	e := &RunnerError{Status: resp.StatusCode, Code: body.Code, Message: body.Error,
		Err: fmt.Errorf("runner answered %s: %s", resp.Status, body.Error)}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// The tokens of web and runner differ: an operator problem.
		e.Status, e.Code, e.Message = http.StatusBadGateway, "runner_auth", "The site cannot use the test runner right now. Try again later."
	case resp.StatusCode == http.StatusTooManyRequests:
		e.Code, e.Message = "runner_busy", "The test runner is busy. Try again in a few seconds."
		e.RetryAfter = resp.Header.Get("Retry-After")
	case resp.StatusCode == http.StatusServiceUnavailable:
		e.Code, e.Message = "runner_unavailable", "The test runner is restarting. Try again in a minute."
	case resp.StatusCode == http.StatusBadRequest && body.Code == "unknown_exercise":
		// The site only asks for exercises of its own course, so this
		// runner serves another version of it. Web and runner roll out
		// one after the other, which leaves a short window like this.
		e.Status, e.Code, e.Message = http.StatusServiceUnavailable, "runner_updating", "The test runner is being updated to this version of the course. Try again in a minute."
		e.RetryAfter = "30"
	case resp.StatusCode == http.StatusGatewayTimeout:
		e.Code, e.Message = "runner_timeout", "The checks took too long. Try again."
	case resp.StatusCode >= 500:
		e.Status, e.Code, e.Message = http.StatusBadGateway, "runner_error", "The test runner failed. Try again."
	case e.Message == "":
		e.Message = "The test runner refused the request."
	}
	if e.Code == "" {
		e.Code = "runner_error"
	}
	return e
}

// TestStream reads the NDJSON events of a test run.
type TestStream struct {
	body  io.ReadCloser
	sc    *bufio.Scanner
	ev    runnerapi.Event
	err   error
	stop  context.CancelFunc
	start time.Time
	c     *RunnerClient
	done  bool
}

// maxEventLine bounds 1 NDJSON line. The runner splits long output lines,
// so a line this long is a broken stream.
const maxEventLine = 4 << 20

// Test starts a test run. The caller reads the events with Next and must
// Close the stream.
func (c *RunnerClient) Test(ctx context.Context, req runnerapi.TestRequest) (*TestStream, error) {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	start := time.Now()
	resp, err := c.post(ctx, runnerapi.PathTest, req) //nolint:bodyclose // TestStream.Close closes it
	if err != nil {
		cancel()
		c.observe("test", err, start)
		return nil, err
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxEventLine)
	return &TestStream{body: resp.Body, sc: sc, stop: cancel, start: start, c: c}, nil
}

// Next reads the next event. It returns false at the end of the stream or
// on an error; Err tells which.
func (s *TestStream) Next() bool {
	if s.err != nil || s.done {
		return false
	}
	for s.sc.Scan() {
		line := bytes.TrimSpace(s.sc.Bytes())
		if len(line) == 0 {
			continue
		}
		s.ev = runnerapi.Event{}
		if err := json.Unmarshal(line, &s.ev); err != nil {
			s.err = fmt.Errorf("runner: bad event %q: %w", truncate(string(line), 200), err)
			return false
		}
		if s.ev.Type == runnerapi.EventDone {
			s.done = true
		}
		return true
	}
	s.err = s.sc.Err()
	if s.err == nil && !s.done {
		s.err = errors.New("runner: the stream ended before the run was done")
	}
	return false
}

// Event returns the event Next read.
func (s *TestStream) Event() runnerapi.Event { return s.ev }

// Err returns the error that ended the stream, or nil after a done event.
func (s *TestStream) Err() error { return s.err }

// Close ends the stream; a run still going is cancelled.
func (s *TestStream) Close() error {
	s.stop()
	err := s.body.Close()
	result := "ok"
	if s.err != nil {
		result = "stream_error"
	}
	if s.c.metrics != nil {
		s.c.metrics.runnerCall("test", result, time.Since(s.start))
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
