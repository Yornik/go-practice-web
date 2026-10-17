package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/godoc"
	"github.com/yornik/go-practice-web/internal/store"
)

func (s *Server) landingPage(w http.ResponseWriter, r *http.Request) {
	// Figure 1 is the retry loop of exercise 07; its bar is the blue one.
	st := courseStrip(s.cfg.Course, map[string]string{"07-retry": "here"})
	st.Class = "strip-lg"
	st.Label = fmt.Sprintf("The course to scale: %d exercises in %d tiers, about %s. The blue bar is exercise 07.",
		len(s.cfg.Course.Exercises), len(s.cfg.Course.Tiers), formatHours(s.cfg.Course.Hours()))
	s.render(w, r, http.StatusOK, "landing", view{Nav: "home", Mono: true, Script: "pages/landing.js", Data: map[string]any{
		"Strip":  st,
		"Figure": figureTwo(),
	}})
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
		if e, ok := auth.AsError(err); ok && e.Code == "token_invalid" {
			v.Error = s.deadInviteMessage(r, token)
		}
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
		if e, ok := auth.AsError(err); ok && e.Code == "token_invalid" {
			v.Error = "This setup link does not work any more. Someone used it, or the server logged a newer one."
		}
	}
	s.render(w, r, status, "join", view{Title: "Set up Go Practice", Script: "pages/join.js", Data: v})
}

// deadInviteMessage says why an invite link does not work, with its date.
func (s *Server) deadInviteMessage(r *http.Request, token string) string {
	inv, ok := s.cfg.Auth.DeadInvite(r.Context(), token)
	switch {
	case !ok:
		return "This invite link does not work. Check that you copied all of it."
	case inv.Used():
		return fmt.Sprintf("Someone used this invite link on %s. An invite link works once.", shortDate(inv.UsedAt))
	default:
		return fmt.Sprintf("This invite link expired on %s.", shortDate(inv.ExpiresAt))
	}
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
		if e, ok := auth.AsError(err); ok && e.Code == "token_invalid" {
			msg = "This recovery link does not work any more. It works once, for 15 minutes."
		}
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
	var v guideView
	for _, sec := range s.cfg.Course.Guide {
		// "Start here" sets up Go on your own machine; the site needs none of it.
		if sec.Title != "Start here" {
			v.Sections = append(v.Sections, sec)
		}
	}
	for _, id := range []string{explain.GuideErrorMessage, explain.GuideStackTrace, explain.GuideRaceReport, explain.GuideFailingTest} {
		if g, ok := explain.Guide(id); ok {
			v.Guides = append(v.Guides, g)
		}
	}
	s.render(w, r, http.StatusOK, "guide", view{Title: "The guide", Nav: "guide", Script: "pages/guide.js", Data: v})
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

// Revealed reports whether ladder level l (1 to 6) is revealed. The ladder
// opens in order, so that is every level up to the highest.
func (e exerciseRow) Revealed(l int) bool { return l <= e.Levels }

type tierRow struct {
	*course.Tier
	Exercises []exerciseRow
	Passed    int
	Started   int
	Cert      *certStatus // the tier's certificate
}

type dashboard struct {
	Tiers         []tierRow
	Rows          []exerciseRow // every exercise, for the learning log
	Passed, Total int
	Started       int
	ActiveSeconds int64
	Runs, Hints   int
	Next          *course.Exercise // the first exercise not passed
	NextStarted   bool             // Next is started
	Strip         strip
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
				d.Next, d.NextStarted = e, p.Status != ""
			}
			if p.Status != "" {
				d.Started++
				tr.Started++
			}
			d.ActiveSeconds += p.ActiveSeconds
			d.Runs += p.Runs
			d.Hints += row.Hints
			tr.Exercises = append(tr.Exercises, row)
			d.Rows = append(d.Rows, row)
		}
		d.Tiers = append(d.Tiers, tr)
	}
	states := map[string]string{}
	for _, row := range d.Rows {
		states[row.Slug] = row.Status
	}
	d.Strip = courseStrip(s.cfg.Course, states)
	// A picture, not 25 more links: the tables below link every exercise.
	d.Strip.Label = fmt.Sprintf("The course to scale: %d of %d exercises pass, %d started.", d.Passed, d.Total, d.Started)
	return d, nil
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	d, err := s.buildDashboard(r.Context(), id.User.ID)
	if err == nil {
		var certs []certStatus
		if certs, err = s.certificateStatus(r.Context(), id.User.ID); err == nil {
			for i := range d.Tiers {
				for j := range certs {
					if certs[j].Kind == fmt.Sprintf("tier%d", d.Tiers[i].N) {
						d.Tiers[i].Cert = &certs[j]
					}
				}
			}
		}
	}
	if err != nil {
		s.log.Error("dashboard", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "Your course could not load.")
		return
	}
	s.render(w, r, http.StatusOK, "dashboard", view{Title: "Your course", Nav: "learn", Script: "pages/dashboard.js", Data: d})
}

func (s *Server) idePage(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.cfg.Course.Exercise(r.PathValue("exercise"))
	if !ok {
		s.errorPage(w, r, http.StatusNotFound, "This exercise does not exist.")
		return
	}
	prev, next := s.cfg.Course.Neighbours(ex.Slug)
	type rung struct {
		Level       int
		Title       string
		Description string
		After       string // the level it waits for: "after hint 2"
	}
	var rungs []rung
	for l := course.Hint1; l <= course.MaxLevel; l++ {
		rg := rung{Level: int(l), Title: l.String(), Description: s.cfg.Course.LevelDescription(l)}
		if l > course.Hint1 {
			prevLevel := (l - 1).String()
			if l-1 > course.Hint3 {
				prevLevel = "the " + strings.ToLower(prevLevel)
			} else {
				prevLevel = strings.ToLower(prevLevel)
			}
			rg.After = "after " + prevLevel
		}
		rungs = append(rungs, rg)
	}
	s.render(w, r, http.StatusOK, "ide", view{
		// The exercise's name in the tab title: several exercises can be open.
		Title: fmt.Sprintf("%02d %s", ex.Number, ex.Title), Nav: "learn", Script: "ide/main.js", Wide: true,
		Data: map[string]any{
			"Exercise":  ex,
			"Prev":      prev,
			"Next":      next,
			"Rungs":     rungs,
			"GoVersion": goVersion(),
			"Count":     len(s.cfg.Course.Exercises),
			"Titles":    exerciseTitles(s.cfg.Course),
		},
	})
}

// exerciseTitles maps each slug to "01 Parse logfmt log lines", for the
// "Taught in" line of an explanation.
func exerciseTitles(c *course.Course) map[string]string {
	out := make(map[string]string, len(c.Exercises))
	for _, e := range c.Exercises {
		out[e.Slug] = fmt.Sprintf("%02d %s", e.Number, e.Title)
	}
	return out
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
		s.errorPage(w, r, http.StatusInternalServerError, "The account page could not load.")
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
		Title: "Account", Nav: "account", Script: "pages/account.js",
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
		s.errorPage(w, r, http.StatusInternalServerError, "The admin page could not load.")
		return
	}
	users, err := s.cfg.Store.UserSummaries(ctx)
	if err != nil {
		s.log.Error("admin", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The admin page could not load.")
		return
	}
	names := map[int64]string{}
	rows := make([]adminUser, 0, len(users))
	for _, u := range users {
		names[u.ID] = u.Username
		row := adminUser{UserSummary: u}
		// A few queries per user: the admin page lists the invited few.
		if progress, err := s.cfg.Store.UserProgress(ctx, u.ID); err == nil {
			states := map[string]string{}
			for _, p := range progress {
				states[p.Exercise] = string(p.Status)
			}
			row.Strip = courseStrip(s.cfg.Course, states)
			row.Strip.Class = "strip-mini"
			row.Strip.Label = fmt.Sprintf("%d of %d exercises pass", u.Passed, len(s.cfg.Course.Exercises))
		}
		if sessions, err := s.cfg.Store.Sessions(ctx, u.ID); err == nil {
			for _, se := range sessions {
				if se.LastSeenAt.After(row.LastSeen) {
					row.LastSeen = se.LastSeenAt
				}
			}
		}
		rows = append(rows, row)
	}
	s.render(w, r, http.StatusOK, "admin", view{
		Title: "Admin", Nav: "admin", Script: "pages/admin.js",
		Data: map[string]any{"Invites": invites, "Users": rows, "Names": names, "Now": s.now(), "Total": len(s.cfg.Course.Exercises)},
	})
}

// adminUser is 1 row of the admin page's user table.
type adminUser struct {
	store.UserSummary
	Strip    strip
	LastSeen time.Time // the newest session activity
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
