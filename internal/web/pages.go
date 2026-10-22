package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/godoc"
	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

// landingView is the landing page (VESPER.md 8.1). Every count on it
// comes from the course, so the page cannot disagree with it.
type landingView struct {
	First     *course.Exercise // where you start: Figure 1, the lit tower, the board's amber cell
	FirstTier string           // its tier as a readout value: "1, Badge In"
	Anatomy   landingAnatomy
	Sky       skyline
	Years     []landingYear
}

// landingYear is 1 year of the departures board.
type landingYear struct {
	N     int
	Label string // "September to 1 January. Tiers 1 to 5, about 35.5"; the template adds the unit
	Hours string // "35.5"
	Tiers []landingTier
}

// landingTier is 1 tier of the departures board.
type landingTier struct {
	*course.Tier
	State string // the tag's state: "cur" for the tier you start in, else "off"
	Head  string // its months and hours: "September, about 7"; the template adds the unit
}

// yearSpans are the months of each year of the story (the bible's
// timeline): the tiers' own months do not say where a year starts.
var yearSpans = map[int]string{1: "September to 1 January", 2: "2 January to the night the clocks go back"}

// number writes hours without the unit, so a capitals label can set the
// unit in lower case: "ABOUT 7 h".
func number(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) }

func newLandingView(c *course.Course) landingView {
	v := landingView{Anatomy: newLandingAnatomy()}
	if len(c.Exercises) > 0 {
		v.First = c.Exercises[0]
		v.FirstTier = tierValueOf(c, v.First)
	}
	for _, t := range c.Tiers {
		if n := len(v.Years); n == 0 || v.Years[n-1].N != t.Year() {
			v.Years = append(v.Years, landingYear{N: t.Year()})
		}
		y := &v.Years[len(v.Years)-1]
		lt := landingTier{Tier: t, State: "off", Head: "about " + number(t.Hours())}
		if v.First != nil && t.N == v.First.Tier {
			lt.State = "cur"
		}
		if w := t.When(); w != "" {
			lt.Head = w + ", " + lt.Head
		}
		y.Tiers = append(y.Tiers, lt)
	}
	var years []string
	for i := range v.Years {
		y := &v.Years[i]
		var h float64
		for _, t := range y.Tiers {
			h += t.Hours()
		}
		first, last := y.Tiers[0].N, y.Tiers[len(y.Tiers)-1].N
		y.Hours = number(h)
		y.Label = fmt.Sprintf("Tiers %d to %d, about %s", first, last, y.Hours)
		if span := yearSpans[y.N]; span != "" {
			y.Label = span + ". " + y.Label
		}
		years = append(years, fmt.Sprintf("year %d holds tiers %d to %d, about %s", y.N, first, last, formatHours(h)))
	}
	label := fmt.Sprintf("Skyline of the course: %d towers, 1 per exercise, as tall as its estimate.", len(c.Exercises))
	if len(years) > 0 {
		label += " " + strings.ToUpper(years[0][:1]) + strings.Join(years, "; ")[1:] + "."
	}
	states := map[string]string{}
	if v.First != nil {
		states[v.First.Slug] = "cur"
		label += fmt.Sprintf(" Tower %02d is lit: you start there.", v.First.Number)
	}
	v.Sky, _ = skylineFunc(c, states, "landing", label)
	return v
}

func (s *Server) landingPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "landing", view{Nav: "home", Mono: true, Data: newLandingView(s.cfg.Course)})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if _, ok := identity(r); ok {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "login", view{
		Title: "Sign in", Nav: "login", Script: "pages/login.js",
		Data: map[string]any{"Next": next, "Host": s.host(), "MovedFrom": s.cfg.MovedFrom},
	})
}

// emailAddr is an address in 2 halves that never break inside: the page
// breaks a long address only after its "@" (VESPER.md 8.3).
type emailAddr struct{ At, Domain string }

func newEmailAddr(email string) emailAddr {
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return emailAddr{At: email}
	}
	return emailAddr{At: email[:i+1], Domain: email[i+1:]}
}

// signUpView is the data of the sign-up form, shared by /join and /setup.
type signUpView struct {
	Setup     bool   // the first account, made admin
	Token     string // the invite or setup token from the link
	Error     string // why the link does not work, if it does not
	InvitedBy string
	Note      string
	Email     string // the address the invite went to, as a default
	// Mailed is true when the invite was mailed to Email: a sign-up with
	// that address is verified at once.
	Mailed  bool
	Expires time.Time
	Host    string    // the site, for the passkey's own words
	Addr    emailAddr // Email, for the help that names it
}

func (s *Server) joinPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("invite")
	v := signUpView{Token: token, Host: s.host()}
	status := http.StatusOK
	inv, err := s.cfg.Auth.CheckInvite(r.Context(), token)
	if err != nil {
		status, v.Error = s.linkError(r, err)
		if e, ok := auth.AsError(err); ok && e.Code == "token_invalid" {
			v.Error = s.deadInviteMessage(r, token)
		}
	} else {
		v.Note, v.Email, v.Expires, v.Mailed = inv.Note, inv.Email, inv.ExpiresAt, inv.Mailed()
		v.Addr = newEmailAddr(inv.Email)
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
	v := signUpView{Setup: true, Token: token, Host: s.host()}
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
	s.render(w, r, http.StatusOK, "recover", view{Title: "Recover your account", Script: "pages/recover.js", Data: map[string]any{
		"Host": s.host(), "MovedFrom": s.cfg.MovedFrom, "Lifetime": mail.Lifetime(auth.RecoveryTTL),
	}})
}

func (s *Server) recoverConfirmPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	data := map[string]any{"Token": token, "Host": s.host()}
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

// checkEmailView is the page a user waits on until the address is
// verified.
type checkEmailView struct {
	Email   string // where the link went; "" when the account has no address
	Pending bool   // a link that still works is out
	// ValidFor is how long a link works, for a sentence: "48 hours".
	ValidFor string
	// MailLogged is true when the server has no mail server: the link
	// is in the server log, and the page tells the operator so.
	MailLogged bool
	Addr       emailAddr // Email in its 2 halves
	// From is the sender's address, FromName its display name: the
	// learner looks for them in the inbox.
	From     emailAddr
	FromAddr string
	FromName string
	Origin   string // where the link in the mail starts
}

// checkEmailPage is where the email gate sends a user whose address is not
// verified. A verified user has nothing to wait for and goes on.
func (s *Server) checkEmailPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	if id.User.EmailVerified() {
		http.Redirect(w, r, "/learn", http.StatusSeeOther)
		return
	}
	pending, err := s.cfg.Auth.PendingEmail(r.Context(), id.User)
	if err != nil {
		// The page still works: the user can send a new link.
		s.log.Error("check email", "err", err)
	}
	v := checkEmailView{
		Email:      id.User.Email,
		Pending:    pending != "",
		ValidFor:   mail.Lifetime(auth.VerifyTTL),
		MailLogged: !s.cfg.Auth.MailDelivers(),
	}
	if pending != "" {
		v.Email = pending
	}
	v.Addr, v.Origin = newEmailAddr(v.Email), s.origin
	from := s.cfg.MailFrom
	if from == "" {
		from = mail.DefaultFrom
	}
	if a, err := mail.ParseFrom(from); err == nil {
		v.FromName, v.FromAddr, v.From = a.Name, a.Address, newEmailAddr(a.Address)
	}
	s.render(w, r, http.StatusOK, "check-email", view{Title: "Check your email", Script: "pages/check-email.js", Data: v})
}

func (s *Server) verifyEmailPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "verify-email", view{
		Title: "Verify your email", Script: "pages/verify-email.js",
		Data: map[string]any{"Token": r.URL.Query().Get("token"), "Host": s.host(), "ValidFor": mail.Lifetime(auth.VerifyTTL)},
	})
}

func (s *Server) guidePage(w http.ResponseWriter, r *http.Request) {
	var v guideView
	// The story comes first: the manual after it reads in its terms.
	v.Items = append(v.Items, guideItem{ID: "the-story", Title: "The story", Kind: "story"})
	for _, sec := range s.cfg.Course.Guide {
		it := guideItem{ID: sec.ID, Title: sec.Title, Kind: "section", Section: sec}
		switch sec.Title {
		case "Start here":
			// It sets up Go on your own machine; the site needs none of it.
			continue
		case "How to work an exercise":
			// The course's 10 steps run the tests on your machine; the
			// page has the site's own version of them.
			it.Kind = "work"
		case "A problem-solving checklist":
			it.Title, it.Kind = "The checklist", "checklist"
		case "Make it a tool":
			v.Items = append(v.Items,
				guideItem{ID: "read-go-messages", Title: "How to read Go messages", Kind: "messages"},
				guideItem{ID: "editor-keys", Title: "Keys in the editor", Kind: "keys"})
			it.Kind = "tools"
		}
		v.Items = append(v.Items, it)
	}
	for i := range v.Items {
		v.Items[i].N = i + 1
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
	OnCall        bool // the on-call exercise: its row has the amber rule (VESPER.md 8.6)
}

// Revealed reports whether ladder level l (1 to 6) is revealed. The ladder
// opens in order, so that is every level up to the highest.
func (e exerciseRow) Revealed(l int) bool { return l <= e.Levels }

// InLedger is the row's "In the Ledger" cell: "entered 2 Oct", "waiting
// for a pass" or "not started".
func (e exerciseRow) InLedger() string {
	switch e.Status {
	case "passed":
		return "entered " + dayMonth(e.PassedAt)
	case "started":
		return "waiting for a pass"
	}
	return "not started"
}

// RevealedText names the help revealed, for the Help cells' accessible
// text: "Nothing revealed", "Revealed: hint 1", "Revealed: hints 1 to 3
// and the explanation".
func (e exerciseRow) RevealedText() string {
	switch e.Levels {
	case 0:
		return "Nothing revealed"
	case 1:
		return "Revealed: hint 1"
	case 2:
		return "Revealed: hints 1 and 2"
	case 3:
		return "Revealed: hints 1 to 3"
	case 4:
		return "Revealed: hints 1 to 3 and the explanation"
	case 5:
		return "Revealed: hints 1 to 3, the explanation and the walkthrough"
	}
	return "Revealed: hints 1 to 3, the explanation, the walkthrough and the solution"
}

type tierRow struct {
	*course.Tier
	Exercises []exerciseRow
	Passed    int
	Started   int
	Cert      *certStatus // the tier's certificate
	Tag       *tierTag    // the tier as a tag: stamped, ready, open (story.go)
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
}

// buildDashboard joins the course with the learner's progress, as the
// story read it.
func (s *Server) buildDashboard(st *story) dashboard {
	d := dashboard{Total: len(s.cfg.Course.Exercises)}
	for i, t := range s.cfg.Course.Tiers {
		tr := tierRow{Tier: t}
		if i < len(st.Tiers) {
			tr.Tag, tr.Cert = st.Tiers[i], st.Tiers[i].Status
		}
		for _, e := range t.Exercises {
			p := st.Exercise(e.Slug)
			row := exerciseRow{
				Exercise: e, ActiveSeconds: p.ActiveSeconds, Runs: p.Runs,
				Hints: p.Hints, Levels: p.Level, PassedAt: p.PassedAt, OnCall: st.OnCall == e,
			}
			switch {
			case p.Passed:
				row.Status = "passed"
			case p.Started:
				row.Status = "started"
			}
			if p.Passed {
				tr.Passed++
				d.Passed++
			} else if d.Next == nil {
				d.Next, d.NextStarted = e, p.Started
			}
			if p.Started {
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
	return d
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	st, err := s.loadStory(r.Context(), id.User, "")
	if err != nil {
		s.log.Error("dashboard", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "Your course could not load.")
		return
	}
	s.render(w, r, http.StatusOK, "dashboard", view{Title: "Your page of the Ledger", Nav: "learn", Script: "pages/dashboard.js",
		Data: s.buildDashboard(st), Story: st})
}

func (s *Server) idePage(w http.ResponseWriter, r *http.Request) {
	ex, renamed, ok := s.cfg.Course.Resolve(r.PathValue("exercise"))
	if !ok {
		s.errorPage(w, r, http.StatusNotFound, "This exercise does not exist.")
		return
	}
	if renamed {
		// A bookmark or a link from before the course grew: the exercise
		// lives on under its new number. The API takes only current slugs.
		// 302, not 301: a browser keeps a 301 for good, and after a
		// rollback to the first course it would still send the old link
		// to a slug that build does not have.
		http.Redirect(w, r, "/learn/"+ex.Slug, http.StatusFound)
		return
	}
	prev, next := s.cfg.Course.Neighbours(ex.Slug)
	id, _ := identity(r)
	// The open exercise counts as started and worked in now: the IDE's
	// bootstrap request starts it right after this page loads.
	st, err := s.loadStory(r.Context(), id.User, ex.Slug)
	if err != nil {
		s.log.Error("ide", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The exercise could not load.")
		return
	}
	type rung struct {
		Level       int
		Mark        string // the letter or number in its marker: "1", or "E" for the explanation (VESPER.md 7.8)
		Title       string
		Description string
		After       string // the level it waits for: "after hint 2"
	}
	var rungs []rung
	for l := course.Hint1; l <= course.MaxLevel; l++ {
		rg := rung{Level: int(l), Mark: rungMark(l), Title: l.String(), Description: s.cfg.Course.LevelDescription(l)}
		if l > course.Hint1 {
			rg.After = "after " + (l - 1).InSentence()
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
			// The IDE moves what the browser kept under the old slug.
			"RenamedFrom": s.cfg.Course.RenamedFrom(ex.Slug),
			// The top bar (VESPER.md 7.2): the Tier readout, and "On call"
			// with the Key when the Key is on the ring and this is the
			// on-call exercise (16.2).
			"TierValue": tierValueOf(s.cfg.Course, ex),
			"OnCall":    st.KeyOnRing && st.OnCall == ex,
			// The story line of this exercise's first pass (16.8).
			"FirstPassLine": firstPassLine(ex.Number),
		},
		Story: st,
	})
}

// rungMark is the marker of a ladder level: the number of a hint, or the
// first letter of the explanation, the walkthrough and the solution.
func rungMark(l course.Level) string {
	if l <= course.Hint3 {
		return itoa(int(l))
	}
	return l.String()[:1]
}

// tierValueOf is the exercise's tier as a readout value: "2, The Key".
func tierValueOf(c *course.Course, ex *course.Exercise) string {
	if t, ok := c.Tier(ex.Tier); ok {
		return tierValue(t)
	}
	return itoa(ex.Tier)
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
			// A passkey is bound to this site's name (VESPER.md 8.10).
			"Host": s.host(),
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
			// The skyline at 8 px, with no plates: a picture beside the count
			// (VESPER.md 8.11).
			states := map[string]string{}
			for _, p := range progress {
				states[p.Exercise] = string(p.Status)
			}
			sky, err := skylineFunc(s.cfg.Course, states, "mini",
				fmt.Sprintf("%d of %d exercises pass", u.Passed, len(s.cfg.Course.Exercises)))
			if err == nil {
				row.Sky = &sky
			}
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
	Sky      *skyline  // nil when the progress could not load
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
