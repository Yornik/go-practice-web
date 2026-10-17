// Package runnerapi holds the wire types between the web server and the
// runner. Both sides import it, so a change here changes the contract; keep
// docs/DESIGN.md in step.
package runnerapi

// Paths of the runner's HTTP API.
const (
	PathCheck  = "/v1/check"
	PathFormat = "/v1/format"
	PathTest   = "/v1/test"
)

// Limits the runner enforces on every request. The web server checks them
// too, so that a learner gets a clear message before a round trip.
const (
	MaxFileBytes  = 256 << 10
	MaxTotalBytes = 1 << 20
	MaxOutput     = 1 << 20
)

// CheckRequest asks for gofmt, go vet and staticcheck diagnostics.
// Files maps stub file names (for example "retry.go") to their content.
type CheckRequest struct {
	Exercise string            `json:"exercise"`
	Files    map[string]string `json:"files"`
}

// CheckResponse lists every diagnostic found.
type CheckResponse struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	DurationMs  int64        `json:"durationMs"`
}

// Diagnostic is one finding at a position. Line and Column are 1-based;
// EndLine and EndColumn are 0 when the tool gives only a start position.
type Diagnostic struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
	Severity  string `json:"severity"` // SeverityError, SeverityWarning or SeverityInfo
	Source    string `json:"source"`   // SourceGofmt, SourceCompile, SourceVet or SourceStaticcheck
	// Code names the check: the vet analyzer (for example "printf", from
	// go vet -json) or the staticcheck code (for example "SA4006"). The
	// explain catalog matches on it, so the runner must fill it for those 2
	// sources.
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// Values of Diagnostic.Severity.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Values of Diagnostic.Source.
const (
	SourceGofmt       = "gofmt"
	SourceCompile     = "compile"
	SourceVet         = "vet"
	SourceStaticcheck = "staticcheck"
)

// FormatRequest asks for one file to be formatted with gofmt.
type FormatRequest struct {
	Exercise string `json:"exercise"`
	File     string `json:"file"`
	Content  string `json:"content"`
}

// FormatResponse holds the formatted content. On a syntax error Content is
// the input unchanged and Diagnostics holds the error.
type FormatResponse struct {
	Content     string       `json:"content"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// TestRequest runs go test -json on the exercise. Run is a -run regular
// expression; empty means all tests.
type TestRequest struct {
	Exercise string            `json:"exercise"`
	Files    map[string]string `json:"files"`
	Run      string            `json:"run,omitempty"`
	Race     bool              `json:"race"`
}

// Event is one line of the NDJSON stream that answers a TestRequest.
// Type decides which other fields are set.
type Event struct {
	Type string `json:"type"` // EventQueued, EventStart, EventBuild, EventTest or EventDone

	// EventQueued: the number of requests ahead of this one.
	Position int `json:"position,omitempty"`

	// EventBuild and EventTest: text output.
	Output string `json:"output,omitempty"`

	// EventTest: the go test -json fields. Test is empty for package output.
	Action  string  `json:"action,omitempty"`
	Test    string  `json:"test,omitempty"`
	Elapsed float64 `json:"elapsed,omitempty"`

	// EventDone: the summary.
	ExitCode    int   `json:"exitCode"`
	Passed      int   `json:"passed,omitempty"`
	Failed      int   `json:"failed,omitempty"`
	Skipped     int   `json:"skipped,omitempty"`
	DurationMs  int64 `json:"durationMs,omitempty"`
	TimedOut    bool  `json:"timedOut,omitempty"`
	Truncated   bool  `json:"truncated,omitempty"`
	BuildFailed bool  `json:"buildFailed,omitempty"`
}

// Values of Event.Type.
const (
	EventQueued = "queued"
	EventStart  = "start"
	EventBuild  = "build"
	EventTest   = "test"
	EventDone   = "done"
)

// Error is the body of every 4xx and 5xx response.
type Error struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}
