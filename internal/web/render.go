package web

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/store"
)

// renderer executes the page templates. Each page is parsed together with
// the layout into its own set, because every page defines the same block
// names. In dev mode the files are parsed on every render, so an edit
// shows on reload.
type renderer struct {
	fsys   fs.FS
	dev    bool
	funcs  template.FuncMap
	mu     sync.Mutex
	parsed map[string]*template.Template
}

func newRenderer(fsys fs.FS, dev bool, static func(string) string) (*renderer, error) {
	r := &renderer{fsys: fsys, dev: dev, funcs: templateFuncs(static), parsed: map[string]*template.Template{}}
	names, err := fs.Glob(fsys, "pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: list templates: %w", err)
	}
	if len(names) == 0 {
		return nil, errors.New("web: no templates in pages/")
	}
	// Parse every page at start, also in dev mode, so a broken template
	// stops the start instead of 1 page.
	for _, n := range names {
		name := strings.TrimSuffix(path.Base(n), ".html")
		t, err := r.parse(name)
		if err != nil {
			return nil, err
		}
		r.parsed[name] = t
	}
	return r, nil
}

func (r *renderer) parse(name string) (*template.Template, error) {
	t, err := template.New(name).Funcs(r.funcs).ParseFS(r.fsys, "layout.html", "partials/*.html", "pages/"+name+".html")
	if err != nil {
		return nil, fmt.Errorf("web: parse template %s: %w", name, err)
	}
	return t, nil
}

func (r *renderer) template(name string) (*template.Template, error) {
	if r.dev {
		return r.parse(name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.parsed[name]
	if !ok {
		return nil, fmt.Errorf("web: no template %q", name)
	}
	return t, nil
}

// view is what every page template gets.
type view struct {
	Title  string      // the <title> before ", Vesper Labs Go Practice"; "" for the landing page
	User   *store.User // nil for a visitor
	Nav    string      // the current item of the main navigation
	Script string      // the page's ES module below /static/js/, or ""
	Wide   bool        // the IDE: full width and height, its own top bar, no footer
	Mono   bool        // the first screen shows code; no longer preloads a font (VESPER.md 4.3)
	Data   any         // the page's own data
	Course *course.Course
	Dev    bool
	// ImportMap maps every ES module path to its versioned URL.
	ImportMap template.HTML
	// Page and AppHeader are set by render: the template's name, and the
	// app header for a signed-in user whose address is verified, outside
	// the gate's own pages.
	Page      string
	AppHeader bool
	// Story is the signed-in learner's place in the story (story.go): the
	// readout strip, the staff badge, the ring and the slab. nil for a
	// visitor, on a 5xx page, and when it could not load.
	Story *story
	// Badge is the signed-in user's staff badge, also when Story is nil;
	// zero for a visitor.
	Badge staffBadge
}

// render executes a page into a buffer first, so that a template error
// becomes a clean 500 instead of half a page.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, v view) {
	if id, ok := identity(r); ok && v.User == nil {
		u := id.User
		v.User = &u
	}
	v.Course, v.Dev, v.Page = s.cfg.Course, s.cfg.Dev, name
	if v.User != nil {
		// A user the email gate turns away gets the public header with
		// "Sign out" on every page: the app header's links lead only back
		// to /check-email.
		v.AppHeader = !gatePages[name] && v.User.EmailVerified()
		// Every app page's header shows the badge and the readout strip.
		// A 5xx page skips the queries: the store can be its cause.
		if v.Story == nil && status < http.StatusInternalServerError {
			st, err := s.loadStory(r.Context(), *v.User, "")
			if err != nil {
				s.log.Error("story", "page", name, "err", err)
			}
			v.Story = st
		}
		v.Badge = unknownBadge(v.User.DisplayName)
		if v.Story != nil {
			v.Badge = v.Story.Badge
		}
	}
	assets := s.static.current()
	v.ImportMap = assets.importMap
	t, err := s.pages.template(name)
	if err != nil {
		s.log.Error("template", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", v); err != nil {
		s.log.Error("render", "page", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP(assets.cspHash, v.Wide))
	// Pages show the user's own data; no shared cache may keep them.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// errorView is the error page (VESPER.md 8.12): the status as a numeral,
// what happened as the heading, then what to do. A 404 adds the roll call.
type errorView struct {
	Status  int
	Message string // the heading: what happened
	Detail  string // what to do
	// A 404's roll call: the time of the check and its rows, the asked-for
	// path last.
	At   time.Time
	Rows []rollRow
}

// rollRow is 1 line of the 404's roll call.
type rollRow struct {
	State, Path, Beat, Hush string
	Miss                    bool // the asked-for path: no page answers it
}

// rollPaths are the pages the roll call finds healthy: pages that answer
// every visitor.
var rollPaths = []string{"/learn", "/guide", "/verify", "/certificates"}

// maxPathRunes cuts the asked-for path: it is user text, and a long one
// would push the roll call off a phone.
const maxPathRunes = 60

// errorPage renders the error page. msg says what happened; a 5xx page
// adds the time to look for in the server log.
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, msg string) {
	now := s.now().UTC()
	v := errorView{Status: status, Message: msg}
	switch {
	case status == http.StatusNotFound:
		// The story's 404 (VESPER.md 10.7); the roll call names the path.
		v.Message = "This page is not in the inventory."
		v.Detail = "If it starts sending heartbeats, tell Noor. Until then, check the address, or go to 1 of these pages."
		v.At = now
		for i, p := range rollPaths {
			// The heartbeats are a moment apart, as a real roll call's are.
			beat := now.Add(-time.Duration(1+i*2/3) * time.Second)
			v.Rows = append(v.Rows, rollRow{State: "healthy", Path: p, Beat: beat.Format("15:04:05"), Hush: "calm"})
		}
		v.Rows = append(v.Rows, rollRow{State: "unknown", Path: cutRunes(r.URL.Path, maxPathRunes), Beat: "never", Hush: "RESTING", Miss: true})
	case status == http.StatusForbidden:
		v.Detail = "Admin pages need an admin account."
	case status >= 500:
		v.Message = "The server could not finish this request."
		v.Detail = msg + " Try again in a minute. If it fails again, the site owner can find the cause in the server log with the time " +
			now.Format("15:04:05") + " UTC."
	}
	s.render(w, r, status, "error", view{Title: errorTitle(status), Data: v})
}

// errorTitle is the error page's title in sentence case (VESPER.md 10.2):
// http.StatusText is in title case ("Not Found").
func errorTitle(status int) string {
	switch status {
	case http.StatusNotFound:
		return "Page not found"
	case http.StatusForbidden:
		return "No access"
	}
	if status >= 500 {
		return "Server error"
	}
	t := http.StatusText(status)
	if t == "" {
		return "Error"
	}
	return t[:1] + strings.ToLower(t[1:])
}

// cutRunes cuts s to at most n runes, the last of them an ellipsis.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (s *Server) notFoundPage(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusNotFound, "This page does not exist.")
}

func templateFuncs(static func(string) string) template.FuncMap {
	return template.FuncMap{
		"static":   static,
		"duration": formatDuration,
		// date is for sentences and the certificate, shortdate for tables.
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format("2" + nbsp + "January 2006")
		},
		"shortdate": shortDate,
		// daymonth is a date in a tag's line or a Ledger row: "22 Sep".
		"daymonth": dayMonth,
		"datetime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format("2 Jan 2006, 15:04 UTC")
		},
		"iso": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format(time.RFC3339)
		},
		"hours":  formatHours,
		"b64url": base64.RawURLEncoding.EncodeToString,
		"hex":    hex.EncodeToString,
		"pct": func(n, total int) int {
			if total <= 0 {
				return 0
			}
			return n * 100 / total
		},
		"level":    func(l int) string { return course.Level(l).String() },
		"int":      func(l course.Level) int { return int(l) },
		"pad2":     func(n int) string { return fmt.Sprintf("%02d", n) },
		"initials": initials,
		// catalogHTML marks the HTML of an explanation as safe: the catalog
		// renders its Markdown without raw HTML (internal/explain).
		"catalogHTML": func(s string) template.HTML { return template.HTML(s) }, //nolint:gosec // see above
		"seq": func(n int) []int {
			out := make([]int, n)
			for i := range out {
				out[i] = i + 1
			}
			return out
		},
		"lower":      strings.ToLower,
		"trimPrefix": strings.TrimPrefix,
		// json is for data attributes; html/template escapes the quotes.
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"join":       strings.Join,
		"sub":        func(a, b int) int { return a - b },
		"inc":        func(n int) int { return n + 1 },
		"helpLevels": func() []string { return []string{"1", "2", "3", "E", "W", "S"} },
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return "1 " + one
			}
			return itoa(n) + " " + many
		},
		// The helpers of the Vesper Labs design (design.go).
		"zero":      zero,
		"dict":      dict,
		"skyline":   skylineFunc,
		"upper":     strings.ToUpper,
		"hasSuffix": strings.HasSuffix,
	}
}

// nbsp keeps a number and its unit on 1 line.
const nbsp = "\u00a0"

// formatDuration writes active time the way the copy rules want it:
// "0 min", "25 min", "3 h 5 min", "2 h".
func formatDuration(secs int64) string {
	m := secs / 60
	if m < 60 {
		return fmt.Sprintf("%d%smin", m, nbsp)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d%sh", m/60, nbsp)
	}
	return fmt.Sprintf("%d%sh %d%smin", m/60, nbsp, m%60, nbsp)
}

// formatHours writes an estimate: "2.5 h".
func formatHours(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) + nbsp + "h" }

// apiError is a client error of the web layer itself.
type apiError struct {
	status int
	code   string
	msg    string
	retry  string // Retry-After of a rate limit
}

func (e *apiError) Error() string { return e.code + ": " + e.msg }

func errorf(status int, code, msg string) error {
	return &apiError{status: status, code: code, msg: msg}
}

// errorBody is the JSON of every error response.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal error","code":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

// fail answers an API request with the error's status and message.
// Unknown errors are logged and hidden behind a 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	var re *RunnerError
	if e, ok := auth.AsError(err); ok {
		if e.RetryAfter > 0 {
			w.Header().Set("Retry-After", retryAfter(e.RetryAfter))
		}
		switch {
		case e.Status >= 500:
			s.log.Error("request failed", "route", r.Pattern, "code", e.Code, "err", e.Err)
		case e.Err != nil:
			// A refused passkey or a dead link: the cause helps support.
			s.log.Info("request refused", "route", r.Pattern, "code", e.Code, "err", e.Err)
		}
		writeJSONError(w, e.Status, e.Code, e.Message)
		return
	}
	switch {
	case errors.As(err, &ae):
		if ae.retry != "" {
			w.Header().Set("Retry-After", ae.retry)
		}
		writeJSONError(w, ae.status, ae.code, ae.msg)
	case errors.As(err, &re):
		if re.RetryAfter != "" {
			w.Header().Set("Retry-After", re.RetryAfter)
		}
		writeJSONError(w, re.Status, re.Code, re.Message)
	default:
		s.log.Error("request failed", "route", r.Pattern, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal", "Something went wrong on our side.")
	}
}

// maxJSON is the default body limit of a JSON request.
const maxJSON = 64 << 10

// decode reads a JSON body of at most limit bytes into v. Unknown fields
// are an error: they are a client bug, and silently ignoring them hides
// it.
func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	body := http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return errorf(http.StatusRequestEntityTooLarge, "too_large", "The request is too large.")
		}
		if errors.Is(err, io.EOF) {
			return errorf(http.StatusBadRequest, "bad_json", "The request needs a JSON body.")
		}
		return errorf(http.StatusBadRequest, "bad_json", "The request body is not valid JSON: "+err.Error())
	}
	if dec.More() {
		return errorf(http.StatusBadRequest, "bad_json", "The request body has more than 1 JSON value.")
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }

// shortDate is a date for tables and short sentences: "24 Oct 2026".
func shortDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2" + nbsp + "Jan 2006")
}
