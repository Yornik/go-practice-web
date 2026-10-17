// Package web is the HTTP side of the site: the server-rendered pages, the
// JSON API under /api/, the proxy to the runner and the middleware that
// guards them (security headers and CSP, CSRF, sessions, logging and
// metrics). See "HTTP routes" in docs/DESIGN.md.
package web

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/godoc"
	"github.com/yornik/go-practice-web/internal/store"
)

// Config holds the dependencies of a Server.
type Config struct {
	PublicURL string // GPW_PUBLIC_URL; mutations must come from this origin
	Dev       bool   // GPW_DEV: templates and static files are read again on each request

	Store  *store.Store
	Course *course.Course
	Auth   *auth.Service
	Runner *RunnerClient
	Signer *cert.Signer
	IPs    *auth.IPResolver // nil trusts no proxy

	// Templates holds layout.html and pages/*.html; Static is served at
	// /static/. Both are web/ of the repository: embedded, or the disk in
	// dev mode.
	Templates fs.FS
	Static    fs.FS

	// Docs serves /api/docs/; nil uses the embedded standard library index.
	Docs http.Handler

	Logger  *slog.Logger          // nil discards
	Metrics prometheus.Registerer // nil registers nowhere
	Now     func() time.Time      // nil is time.Now
}

// Server is the site. Create it with New and serve Handler.
type Server struct {
	cfg     Config
	origin  string
	log     *slog.Logger
	now     func() time.Time
	pages   *renderer
	static  *staticFiles
	metrics *metrics
	handler http.Handler

	checkLimit  *auth.Limiter
	formatLimit *auth.Limiter
	testLimit   *auth.Limiter
	pdfLimit    *auth.Limiter
	writeLimit  *auth.Limiter
	claimLimit  *auth.Limiter

	mu        sync.Mutex
	running   map[int64]bool       // users with a test run in flight
	checks    map[int64]*checkSlot // users with a check in flight
	heartbeat map[int64]time.Time  // last heartbeat per user

	// info caches the runner's GET /v1/info; see runnerInfo.
	info runnerInfoCache

	// readiness caches the last database probe; see probe.
	readiness struct {
		mu  sync.Mutex
		at  time.Time
		err error
	}
}

// New checks the configuration, parses the templates and builds the
// routes.
func New(cfg Config) (*Server, error) {
	u, err := url.Parse(cfg.PublicURL)
	if err != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("web: public URL %q is not an http(s) origin", cfg.PublicURL)
	}
	if cfg.Store == nil || cfg.Course == nil || cfg.Auth == nil || cfg.Runner == nil || cfg.Signer == nil {
		return nil, errors.New("web: Store, Course, Auth, Runner and Signer are required")
	}
	if cfg.Templates == nil || cfg.Static == nil {
		return nil, errors.New("web: Templates and Static are required")
	}
	s := &Server{
		cfg:       cfg,
		origin:    u.Scheme + "://" + u.Host,
		log:       cfg.Logger,
		now:       cfg.Now,
		running:   map[int64]bool{},
		checks:    map[int64]*checkSlot{},
		heartbeat: map[int64]time.Time{},
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.cfg.IPs == nil {
		s.cfg.IPs = auth.NewIPResolver(nil)
	}
	if s.cfg.Docs == nil {
		s.cfg.Docs = godoc.Handler()
	}
	// The limits of docs/DESIGN.md, "Protection". Format has none there;
	// it is cheap, but a loop in a broken client should not hammer the
	// runner.
	s.checkLimit = auth.NewLimiter(30, time.Minute, s.now)
	s.formatLimit = auth.NewLimiter(60, time.Minute, s.now)
	s.testLimit = auth.NewLimiter(10, time.Minute, s.now)
	s.pdfLimit = auth.NewLimiter(30, time.Minute, s.now)
	// Every save is a synced write on the NFS volume, which all users
	// share. Autosave, notes and heartbeats stay far below this.
	s.writeLimit = auth.NewLimiter(120, time.Minute, s.now)
	// Each claim adds a certificate row; nobody needs more than a few.
	s.claimLimit = auth.NewLimiter(10, time.Hour, s.now)

	if s.metrics, err = newMetrics(cfg.Metrics); err != nil {
		return nil, err
	}
	cfg.Runner.setMetrics(s.metrics)
	if s.static, err = newStaticFiles(cfg.Static, cfg.Dev); err != nil {
		return nil, err
	}
	if s.pages, err = newRenderer(cfg.Templates, cfg.Dev, s.static.url); err != nil {
		return nil, err
	}
	s.handler = s.routes()
	return s, nil
}

// highlightCSS serves the chroma classes of the rendered course Markdown.
// Pages link it with the course version, which changes when it does.
func (s *Server) highlightCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(course.HighlightCSS()))
}

// Handler returns the site's HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// routes builds the mux and wraps it in the middleware. Every route that
// needs a user says so here, so a reader sees the access rule next to the
// path.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	page := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }
	userPage := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireUserPage(h)) }
	api := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }
	userAPI := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireUserAPI(h)) }
	adminAPI := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireUserAPI(s.requireAdmin(h))) }

	// Pages.
	page("GET /{$}", s.landingPage)
	page("GET /login", s.loginPage)
	page("GET /join", s.joinPage)
	page("GET /setup", s.setupPage)
	page("GET /recover", s.recoverPage)
	page("GET /recover/confirm", s.recoverConfirmPage)
	page("GET /verify-email", s.verifyEmailPage)
	page("GET /guide", s.guidePage)
	userPage("GET /learn", s.dashboardPage)
	userPage("GET /learn/{exercise}", s.idePage)
	userPage("GET /account", s.accountPage)
	userPage("GET /certificates", s.certificatesPage)
	userPage("GET /admin", s.requireAdmin(s.adminPage))
	page("GET /verify", s.verifyLookup)
	page("GET /verify/{id}", s.verifyPage) // also /verify/<id>.pdf
	page("GET /.well-known/go-practice-certificates.pub", s.publicKey)
	page("GET /healthz", s.healthz)
	page("GET /readyz", s.readyz)
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	page("GET /highlight.css", s.highlightCSS)
	page("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.static.url("img/favicon.svg"), http.StatusMovedPermanently)
	})

	// Accounts.
	api("POST /api/auth/register/begin", s.registerBegin)
	api("POST /api/auth/register/finish", s.registerFinish)
	api("POST /api/auth/login/begin", s.loginBegin)
	api("POST /api/auth/login/finish", s.loginFinish)
	userAPI("POST /api/auth/logout", s.logout)
	userAPI("POST /api/auth/passkeys/begin", s.passkeyBegin)
	userAPI("POST /api/auth/passkeys/finish", s.passkeyFinish)
	userAPI("PUT /api/auth/passkeys/{id}", s.passkeyRename)
	userAPI("DELETE /api/auth/passkeys/{id}", s.passkeyDelete)
	api("POST /api/auth/recover", s.recoverRequest)
	api("POST /api/auth/recover/begin", s.recoverBegin)
	api("POST /api/auth/recover/finish", s.recoverFinish)
	api("POST /api/auth/verify-email", s.verifyEmail)
	userAPI("POST /api/auth/verify-email/resend", s.resendVerification)
	userAPI("PUT /api/account", s.updateAccount)

	// Learning.
	userAPI("GET /api/exercises/{exercise}", s.exerciseBootstrap)
	userAPI("PUT /api/exercises/{exercise}/files/{file}", s.saveFile)
	userAPI("POST /api/exercises/{exercise}/files/{file}/reset", s.resetFile)
	userAPI("POST /api/exercises/{exercise}/check", s.check)
	userAPI("POST /api/exercises/{exercise}/format", s.format)
	userAPI("POST /api/exercises/{exercise}/test", s.runTests)
	userAPI("POST /api/exercises/{exercise}/reveal", s.reveal)
	userAPI("POST /api/exercises/{exercise}/heartbeat", s.heartbeatHandler)
	userAPI("PUT /api/exercises/{exercise}/notes", s.saveNotes)
	mux.Handle("GET /api/docs/", s.requireUserAPI(s.cfg.Docs.ServeHTTP))

	// Certificates.
	userAPI("GET /api/certificates", s.listCertificates)
	userAPI("POST /api/certificates", s.claimCertificate)

	// Admin.
	adminAPI("POST /api/admin/invites", s.createInvite)
	adminAPI("DELETE /api/admin/invites/{hash}", s.revokeInvite)
	adminAPI("PUT /api/admin/users/{id}", s.updateUser)

	// Anything else under /api/ is JSON, not the HTML 404 page.
	api("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONError(w, http.StatusNotFound, "not_found", "No such API endpoint.")
	})
	page("/", s.notFoundPage)

	// The mux sets r.Pattern on the request it is given; the middleware
	// outside sees an earlier copy, so the route travels back in reqInfo.
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if info := infoFrom(r.Context()); info != nil {
			info.route = r.Pattern
		}
	})
	h = s.loadSession(h)
	h = s.csrf(h)
	h = s.securityHeaders(h)
	h = s.recoverPanics(h)
	h = writeDeadline(h, responseTimeout)
	h = s.observe(h)
	return h
}
