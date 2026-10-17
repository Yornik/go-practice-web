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
	t, err := template.New(name).Funcs(r.funcs).ParseFS(r.fsys, "layout.html", "pages/"+name+".html")
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
	Title  string      // the <title> before " · Go Practice"
	User   *store.User // nil for a visitor
	Nav    string      // the current item of the main navigation
	Script string      // the page's ES module below /static/js/, or ""
	Wide   bool        // the IDE: full width and height, no footer
	Data   any         // the page's own data
	Course *course.Course
	Dev    bool
}

// render executes a page into a buffer first, so that a template error
// becomes a clean 500 instead of half a page.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, v view) {
	if id, ok := identity(r); ok && v.User == nil {
		u := id.User
		v.User = &u
	}
	v.Course, v.Dev = s.cfg.Course, s.cfg.Dev
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
	// Pages show the user's own data; no shared cache may keep them.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// errorPage renders the error page with a message for people.
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.render(w, r, status, "error", view{
		Title: http.StatusText(status),
		Data:  map[string]any{"Status": status, "Message": msg},
	})
}

func (s *Server) notFoundPage(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusNotFound, "This page does not exist.")
}

func templateFuncs(static func(string) string) template.FuncMap {
	return template.FuncMap{
		"static":   static,
		"duration": formatDuration,
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format("2 January 2006")
		},
		"datetime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format("2 Jan 2006 15:04 UTC")
		},
		"iso": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format(time.RFC3339)
		},
		"hours":  func(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) + " h" },
		"b64url": base64.RawURLEncoding.EncodeToString,
		"hex":    hex.EncodeToString,
		"pct": func(n, total int) int {
			if total <= 0 {
				return 0
			}
			return n * 100 / total
		},
		"level": func(l int) string { return course.Level(l).String() },
		"int":   func(l course.Level) int { return int(l) },
		"pad2":  func(n int) string { return fmt.Sprintf("%02d", n) },
	}
}

// formatDuration writes active time the way the dashboard shows it:
// "0 min", "25 min", "3 h 05 min".
func formatDuration(secs int64) string {
	m := secs / 60
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d h %02d min", m/60, m%60)
}

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
