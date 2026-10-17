package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
)

type ctxKey int

const (
	infoKey ctxKey = iota
	identityKey
)

// reqInfo collects what the request log line needs from inside the
// handler chain: the route pattern and the user.
type reqInfo struct {
	route  string
	userID int64
}

func infoFrom(ctx context.Context) *reqInfo {
	info, _ := ctx.Value(infoKey).(*reqInfo)
	return info
}

// identity returns the signed-in user of the request.
func identity(r *http.Request) (auth.Identity, bool) {
	id, ok := r.Context().Value(identityKey).(auth.Identity)
	return id, ok
}

// statusWriter records the status and size of a response for the log.
// Unwrap lets http.ResponseController reach the Flusher and the write
// deadline of the real writer, which the test stream needs.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// observe logs 1 line per request and counts it in the metrics.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		info := &reqInfo{}
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), infoKey, info)))
		d := s.now().Sub(start)
		route := info.route
		if route == "" {
			route = "unmatched"
		}
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		s.metrics.httpRequest(route, r.Method, status, d)
		level := slog.LevelInfo
		// Probes and static files would drown the lines that matter.
		if route == "GET /healthz" || route == "GET /readyz" || route == "GET /static/" {
			level = slog.LevelDebug
		}
		s.log.Log(r.Context(), level, "http request",
			"method", r.Method, "route", route, "status", status,
			"duration_ms", d.Milliseconds(), "bytes", sw.bytes, "user_id", info.userID)
	})
}

// responseTimeout bounds the time to write a response. The server has no
// WriteTimeout, because a test stream lasts as long as its run; the
// stream moves the deadline before each event instead. Every other
// response gets this one, so a client that stops reading cannot hold a
// connection and its goroutine forever. It is above the longest wait for
// the runner (a check: 60 s).
const responseTimeout = 2 * time.Minute

// writeDeadline gives every response d to be written.
func writeDeadline(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Not every ResponseWriter supports deadlines (httptest's recorder
		// does not).
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
		next.ServeHTTP(w, r)
	})
}

// recoverPanics turns a panic in a handler into a 500 and a log line with
// the stack, instead of a dropped connection.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v) // net/http's way to abort a response; it must reach the server
			}
			s.log.Error("panic", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
			if sw, ok := w.(*statusWriter); ok && sw.status != 0 {
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal", "Something went wrong on our side.")
		}()
		next.ServeHTTP(w, r)
	})
}

// csp is the Content-Security-Policy of docs/DESIGN.md. Monaco loads its
// AMD modules from /static, which 'self' covers. Beyond that it needs
// 'unsafe-inline' styles (it sets element styles), blob: workers (its
// loader wraps each worker script in a blob that calls importScripts) and
// data: fonts (its codicon icon font is inlined in the CSS as a data URL).
const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"font-src 'self' data:; worker-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; " +
	"form-action 'self'"

// pageCSP is the policy of an HTML page: csp plus the hash of the page's
// import map, the only inline script. A hash, not a nonce: the map is the
// same on every page of a deploy.
func pageCSP(importMapHash string) string {
	return strings.Replace(csp, "script-src 'self'", "script-src 'self' "+importMapHash, 1)
}

// securityHeaders sets the headers every response carries.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// Links carry tokens (/join?invite=...): no other site may see the
		// URL in a Referer.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if !s.cfg.Dev {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// csrf guards every state-changing request: it must carry X-GPW-CSRF: 1,
// which a cross-site form cannot send and a cross-site fetch cannot send
// without a CORS preflight the site never answers, and its Origin must be
// the site's. SameSite=Lax on the cookie is the second layer.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-GPW-CSRF") != "1" {
			writeJSONError(w, http.StatusForbidden, "csrf", "This request is missing its CSRF header. Reload the page.")
			return
		}
		if r.Header.Get("Origin") != s.origin {
			writeJSONError(w, http.StatusForbidden, "origin", "This request comes from another site.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loadSession puts the signed-in user into the request context. Static
// files and probes skip it: they need no user and would cost a query each.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/static/") || p == "/healthz" || p == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		id, err := s.cfg.Auth.Authenticate(r.Context(), w, r)
		switch {
		case err == nil:
			if info := infoFrom(r.Context()); info != nil {
				info.userID = id.User.ID
			}
			r = r.WithContext(context.WithValue(r.Context(), identityKey, id))
		case !errors.Is(err, auth.ErrNoSession):
			s.log.Error("load session", "err", err)
			writeJSONError(w, http.StatusServiceUnavailable, "unavailable", "The site cannot reach its database. Try again in a minute.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireUserAPI answers 401 without a session.
func (s *Server) requireUserAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := identity(r); !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthenticated", "Sign in first.")
			return
		}
		next(w, r)
	}
}

// requireUserPage sends a visitor without a session to the login page,
// which comes back here afterwards.
func (s *Server) requireUserPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := identity(r); !ok {
			http.Redirect(w, r, "/login?"+url.Values{"next": {r.URL.RequestURI()}}.Encode(), http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// requireVerified is the email gate: a user whose address is not verified
// does not reach the course, the certificates or the admin pages. The
// address is the only way back in after a lost passkey, so the site does
// not let anyone build up work on an account they cannot recover. Pages
// go to /check-email; APIs answer 403 email_unverified. It runs after a
// require-user wrapper.
func (s *Server) requireVerified(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, ok := identity(r); ok && id.User.EmailVerified() {
			next(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusForbidden, "email_unverified",
				"Verify your email address first. Open the link in the email we sent you.")
			return
		}
		http.Redirect(w, r, "/check-email", http.StatusSeeOther)
	}
}

// requireAdmin answers 403 to everyone but admins. It runs after a
// require-user wrapper.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, ok := identity(r); !ok || !id.User.IsAdmin {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSONError(w, http.StatusForbidden, "forbidden", "Only an admin can do this.")
				return
			}
			s.errorPage(w, r, http.StatusForbidden, "You do not have access to this page.")
			return
		}
		next(w, r)
	}
}

// client describes the request for rate limits and the audit log.
func (s *Server) client(r *http.Request) auth.Client {
	return auth.Client{IP: s.cfg.IPs.ClientIP(r), UserAgent: r.UserAgent()}
}

// safeNext returns a local path to go to after login; anything that could
// leave the site becomes /learn.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/learn"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/learn"
	}
	return u.RequestURI()
}

// retryAfter formats a Retry-After header in whole seconds, at least 1.
func retryAfter(d time.Duration) string {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return itoa(secs)
}
