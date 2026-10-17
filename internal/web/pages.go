package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/godoc"
	"github.com/yornik/go-practice-web/internal/store"
)

func (s *Server) landingPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "landing", view{Title: "Learn Go by solving real problems", Nav: "home"})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if _, ok := identity(r); ok {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "login", view{
		Title: "Sign in", Nav: "login", Script: "pages/login.js",
		Data: map[string]any{"Next": next},
	})
}

// signUpView is the data of the sign-up form, shared by /join and /setup.
type signUpView struct {
	Setup     bool   // the first account, made admin
	Token     string // the invite or setup token from the link
	Error     string // why the link does not work, if it does not
	InvitedBy string
	Note      string
	Email     string // the address the invite went to, as a default
	Expires   time.Time
}

func (s *Server) joinPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("invite")
	v := signUpView{Token: token}
	status := http.StatusOK
	inv, err := s.cfg.Auth.CheckInvite(r.Context(), token)
	if err != nil {
		status, v.Error = s.linkError(r, err)
	} else {
		v.Note, v.Email, v.Expires = inv.Note, inv.Email, inv.ExpiresAt
		if inv.CreatedBy != 0 {
			if u, err := s.cfg.Store.User(r.Context(), inv.CreatedBy); err == nil {
				v.InvitedBy = u.DisplayName
			}
		}
	}
	s.render(w, r, status, "join", view{Title: "Join Go Practice", Script: "pages/join.js", Data: v})
}

func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	v := signUpView{Setup: true, Token: token}
	status := http.StatusOK
	if err := s.cfg.Auth.CheckSetupToken(r.Context(), token); err != nil {
		status, v.Error = s.linkError(r, err)
	}
	s.render(w, r, status, "join", view{Title: "Set up Go Practice", Script: "pages/join.js", Data: v})
}

// linkError turns the error of a token check into a status and a message
// for the page.
func (s *Server) linkError(r *http.Request, err error) (int, string) {
	if e, ok := auth.AsError(err); ok {
		return e.Status, e.Message
	}
	s.log.Error("check link", "path", r.URL.Path, "err", err)
	return http.StatusInternalServerError, "Something went wrong on our side. Try the link again later."
}

func (s *Server) recoverPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "recover", view{Title: "Recover your account", Script: "pages/recover.js"})
}

func (s *Server) recoverConfirmPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	data := map[string]any{"Token": token}
	status := http.StatusOK
	u, err := s.cfg.Auth.RecoveryUser(r.Context(), token)
	if err != nil {
		var msg string
		status, msg = s.linkError(r, err)
		data["Error"] = msg
	} else {
		data["Name"], data["Username"] = u.DisplayName, u.Username
	}
	s.render(w, r, status, "recover-confirm", view{Title: "Add a new passkey", Script: "pages/recover.js", Data: data})
}

func (s *Server) verifyEmailPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "verify-email", view{
		Title: "Verify your email", Script: "pages/verify-email.js",
		Data: map[string]any{"Token": r.URL.Query().Get("token")},
	})
}

func (s *Server) guidePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "guide", view{Title: "How the course works", Nav: "guide"})
}

// exerciseRow is 1 exercise on the dashboard with the user's progress.
type exerciseRow struct {
	*course.Exercise
	Status        string // "", "started" or "passed"
	ActiveSeconds int64
	Runs          int
	Hints         int // hint levels 1 to 3 revealed
	Levels        int // highest level revealed, 0 to 6
	PassedAt      time.Time
}

type tierRow struct {
	*course.Tier
	Exercises []exerciseRow
	Passed    int
}

type dashboard struct {
	Tiers         []tierRow
	Rows          []exerciseRow // every exercise, for the learning log
	Passed, Total int
	Started       int
	ActiveSeconds int64
	Runs, Hints   int
	Next          *course.Exercise // the first exercise not passed
}

// buildDashboard joins the course with the user's progress and reveals.
func (s *Server) buildDashboard(ctx context.Context, userID int64) (dashboard, error) {
	progress, err := s.cfg.Store.UserProgress(ctx, userID)
	if err != nil {
		return dashboard{}, err
	}
	reveals, err := s.cfg.Store.UserReveals(ctx, userID)
	if err != nil {
		return dashboard{}, err
	}
	bySlug := make(map[string]store.Progress, len(progress))
	for _, p := range progress {
		bySlug[p.Exercise] = p
	}
	hints, levels := map[string]int{}, map[string]int{}
	for _, rv := range reveals {
		if course.Level(rv.Level).IsHint() {
			hints[rv.Exercise]++
		}
		levels[rv.Exercise] = max(levels[rv.Exercise], rv.Level)
	}
	d := dashboard{Total: len(s.cfg.Course.Exercises)}
	for _, t := range s.cfg.Course.Tiers {
		tr := tierRow{Tier: t}
		for _, e := range t.Exercises {
			p := bySlug[e.Slug]
			row := exerciseRow{
				Exercise: e, Status: string(p.Status), ActiveSeconds: p.ActiveSeconds, Runs: p.Runs,
				Hints: hints[e.Slug], Levels: levels[e.Slug], PassedAt: p.PassedAt,
			}
			if p.Passed() {
				tr.Passed++
				d.Passed++
			} else if d.Next == nil {
				d.Next = e
			}
			if p.Status != "" {
				d.Started++
			}
			d.ActiveSeconds += p.ActiveSeconds
			d.Runs += p.Runs
			d.Hints += row.Hints
			tr.Exercises = append(tr.Exercises, row)
			d.Rows = append(d.Rows, row)
		}
		d.Tiers = append(d.Tiers, tr)
	}
	return d, nil
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	d, err := s.buildDashboard(r.Context(), id.User.ID)
	if err != nil {
		s.log.Error("dashboard", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The dashboard could not load. Try again.")
		return
	}
	s.render(w, r, http.StatusOK, "dashboard", view{Title: "Your progress", Nav: "learn", Data: d})
}

func (s *Server) idePage(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.cfg.Course.Exercise(r.PathValue("exercise"))
	if !ok {
		s.errorPage(w, r, http.StatusNotFound, "This exercise does not exist.")
		return
	}
	prev, next := s.cfg.Course.Neighbours(ex.Slug)
	levels := make([]course.Level, 0, int(course.MaxLevel))
	for l := course.Hint1; l <= course.MaxLevel; l++ {
		levels = append(levels, l)
	}
	s.render(w, r, http.StatusOK, "ide", view{
		Title: fmt.Sprintf("%02d · %s", ex.Number, ex.Title), Nav: "learn", Script: "ide/main.js", Wide: true,
		Data: map[string]any{
			"Exercise":  ex,
			"Prev":      prev,
			"Next":      next,
			"Levels":    levels,
			"GoVersion": goVersion(),
		},
	})
}

// goVersion is the Go release of the runner image, from the docs index
// that was built with it.
func goVersion() string {
	ix, err := godoc.Default()
	if err != nil {
		return ""
	}
	return ix.GoVersion
}

func (s *Server) accountPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	ctx := r.Context()
	passkeys, err := s.cfg.Auth.Passkeys(ctx, id.User)
	if err != nil {
		s.log.Error("account", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The account page could not load. Try again.")
		return
	}
	pending, err := s.cfg.Auth.PendingEmail(ctx, id.User)
	if err != nil {
		s.log.Error("account", "err", err)
	}
	sessions, err := s.cfg.Store.Sessions(ctx, id.User.ID)
	if err != nil {
		s.log.Error("account", "err", err)
	}
	s.render(w, r, http.StatusOK, "account", view{
		Title: "Your account", Nav: "account", Script: "pages/account.js",
		Data: map[string]any{
			"Passkeys": passkeys, "PendingEmail": pending, "Sessions": len(sessions),
		},
	})
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	invites, err := s.cfg.Store.Invites(ctx)
	if err != nil {
		s.log.Error("admin", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The admin page could not load. Try again.")
		return
	}
	users, err := s.cfg.Store.UserSummaries(ctx)
	if err != nil {
		s.log.Error("admin", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The admin page could not load. Try again.")
		return
	}
	names := map[int64]string{}
	for _, u := range users {
		names[u.ID] = u.Username
	}
	s.render(w, r, http.StatusOK, "admin", view{
		Title: "Admin", Nav: "admin", Script: "pages/admin.js",
		Data: map[string]any{"Invites": invites, "Users": users, "Names": names, "Now": s.now()},
	})
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// readyz checks that the database takes a write and a read. The runner is
// not part of it: the site works without the runner, and the IDE says so.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.Course == nil || len(s.cfg.Course.Exercises) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("course not loaded\n"))
		return
	}
	if err := s.probe(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("database not writable\n"))
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

// probeFor is how long a database probe answers /readyz. The path is
// public, and each probe is a synced write on the NFS volume: without the
// cache, anyone could keep the database's write lock busy.
const probeFor = 2 * time.Second

// probe runs the database probe at most once per probeFor; requests in
// between, and those that arrive while it runs, get its result.
func (s *Server) probe(ctx context.Context) error {
	s.readiness.mu.Lock()
	defer s.readiness.mu.Unlock()
	now := s.now()
	if at := s.readiness.at; !at.IsZero() && !now.Before(at) && now.Sub(at) < probeFor {
		return s.readiness.err
	}
	// Detached from the request: a client that hangs up must not make the
	// cached result a failure that the kubelet's next probe then sees.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err := s.cfg.Store.Probe(pctx)
	if err != nil {
		s.log.Error("readiness", "err", err)
	}
	s.readiness.at, s.readiness.err = now, err
	return err
}
