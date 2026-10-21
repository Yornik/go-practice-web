package web

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/runnerapi"
	"github.com/yornik/go-practice-web/internal/store"
)

// Soft gates of the hint ladder, from rule 3 of the course: try for 25
// minutes before hint 1 and wait 10 minutes between hints.
const (
	firstHintMinutes   = 25
	betweenHintMinutes = 10
)

// Activity accounting: the browser sends a heartbeat every 15 s while the
// tab is visible and the user typed or clicked in the last 60 s. The
// server credits the time since the previous heartbeat, at most 1 beat,
// so extra tabs or a fast client cannot inflate it.
const heartbeatCredit = 15 * time.Second

// maxNotes bounds the notes of 1 exercise.
const maxNotes = 64 << 10

// exercise returns the exercise of the path or answers 404.
func (s *Server) exercise(w http.ResponseWriter, r *http.Request) (*course.Exercise, bool) {
	ex, ok := s.cfg.Course.Exercise(r.PathValue("exercise"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown_exercise", "This exercise does not exist.")
	}
	return ex, ok
}

// fileJSON is a file in the IDE bootstrap.
type fileJSON struct {
	course.File
	Saved     bool      `json:"saved"`              // the content is the user's, not the stub
	UpdatedAt time.Time `json:"updatedAt,omitzero"` // the last save
}

// progressJSON is the user's progress in API responses.
type progressJSON struct {
	Status        string          `json:"status"` // "started" or "passed"
	StartedAt     time.Time       `json:"startedAt,omitzero"`
	PassedAt      time.Time       `json:"passedAt,omitzero"`
	ActiveSeconds int64           `json:"activeSeconds"`
	Runs          int             `json:"runs"`
	LastRunAt     time.Time       `json:"lastRunAt,omitzero"`
	LastResult    json.RawMessage `json:"lastResult,omitempty"`
	Notes         string          `json:"notes"`
}

func toProgressJSON(p store.Progress) progressJSON {
	return progressJSON{
		Status: string(p.Status), StartedAt: p.StartedAt, PassedAt: p.PassedAt, ActiveSeconds: p.ActiveSeconds,
		Runs: p.Runs, LastRunAt: p.LastRunAt, LastResult: p.LastResult, Notes: p.Notes,
	}
}

// stepJSON is a step in the IDE bootstrap, with its label as safe HTML:
// the names in it are marked as code.
type stepJSON struct {
	course.Step
	LabelHTML template.HTML `json:"labelHTML"`
}

func stepsJSON(ex *course.Exercise) []stepJSON {
	out := make([]stepJSON, len(ex.Steps))
	for i, st := range ex.Steps {
		out[i] = stepJSON{Step: st, LabelHTML: ex.StepLabel(st)}
	}
	return out
}

// revealJSON is a revealed level of the hint ladder.
type revealJSON struct {
	Level      int           `json:"level"`
	Title      string        `json:"title"`
	HTML       template.HTML `json:"html"`
	RevealedAt time.Time     `json:"revealedAt"`
}

func (s *Server) exerciseBootstrap(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	id, _ := identity(r)
	ctx := r.Context()
	// Opening an exercise starts it (docs/DESIGN.md, "Progress").
	p, err := s.cfg.Store.StartExercise(ctx, id.User.ID, ex.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	files, err := s.userFiles(ctx, id.User.ID, ex)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reveals, err := s.cfg.Store.Reveals(ctx, id.User.ID, ex.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	revealed := make([]revealJSON, 0, len(reveals))
	for _, rv := range reveals {
		html, err := ex.Hidden(course.Level(rv.Level))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		revealed = append(revealed, revealJSON{Level: rv.Level, Title: course.Level(rv.Level).String(), HTML: html, RevealedAt: rv.RevealedAt})
	}
	ladder := make([]map[string]any, 0, int(course.MaxLevel))
	for l := course.Hint1; l <= course.MaxLevel; l++ {
		ladder = append(ladder, map[string]any{"level": int(l), "title": l.String()})
	}
	prev, next := s.cfg.Course.Neighbours(ex.Slug)
	writeJSON(w, http.StatusOK, map[string]any{
		"exercise": map[string]any{
			"slug": ex.Slug, "number": ex.Number, "title": ex.Title, "tier": ex.Tier, "time": ex.Time,
			"package": ex.Package, "prev": slugOf(prev), "next": slugOf(next),
		},
		"steps":    stepsJSON(ex),
		"files":    files,
		"progress": toProgressJSON(p),
		"revealed": revealed,
		"ladder":   ladder,
		"gates":    map[string]int{"firstHintMinutes": firstHintMinutes, "betweenHintMinutes": betweenHintMinutes},
		// The -race toggle is right before the first run.
		"raceUnavailable": s.knownRaceUnavailable(),
	})
}

func slugOf(e *course.Exercise) string {
	if e == nil {
		return ""
	}
	return e.Slug
}

// userFiles returns the stub files with the user's saved content, then
// the read-only test files.
func (s *Server) userFiles(ctx context.Context, userID int64, ex *course.Exercise) ([]fileJSON, error) {
	saved, err := s.cfg.Store.Code(ctx, userID, ex.Slug)
	if err != nil {
		return nil, err
	}
	out := make([]fileJSON, 0, len(ex.Files))
	for _, f := range ex.Files {
		fj := fileJSON{File: f}
		if c, ok := saved[f.Name]; ok && f.Kind == course.FileStub {
			fj.Content, fj.Saved, fj.UpdatedAt = c.Content, true, c.UpdatedAt
		}
		out = append(out, fj)
	}
	return out, nil
}

// stubContents returns the content of every stub file as the runner wants
// it: the user's saved version or the stub.
func (s *Server) stubContents(ctx context.Context, userID int64, ex *course.Exercise) (map[string]string, error) {
	saved, err := s.cfg.Store.Code(ctx, userID, ex.Slug)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(ex.StubFiles()))
	for _, f := range ex.StubFiles() {
		files[f.Name] = f.Content
		if c, ok := saved[f.Name]; ok {
			files[f.Name] = c.Content
		}
	}
	return files, nil
}

// stubFile returns the stub of the path, or answers 404: only stubs can be
// saved, and the name must be exactly 1 of them.
func (s *Server) stubFile(w http.ResponseWriter, r *http.Request, ex *course.Exercise) (course.File, bool) {
	f, ok := ex.File(r.PathValue("file"))
	if !ok || f.Kind != course.FileStub {
		writeJSONError(w, http.StatusNotFound, "unknown_file", "This file is not 1 you can edit.")
		return course.File{}, false
	}
	return f, true
}

// checkContent applies the runner's per-file limit before the round trip.
func checkContent(name, content string) error {
	if len(content) > runnerapi.MaxFileBytes {
		return errorf(http.StatusRequestEntityTooLarge, "file_too_large", name+" is larger than 256 KiB.")
	}
	return nil
}

func (s *Server) saveFile(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	f, ok := s.stubFile(w, r, ex)
	if !ok {
		return
	}
	id, _ := identity(r)
	if err := limit(s.writeLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	// JSON escaping can grow content; the decoded size is checked below.
	if err := decode(w, r, 4*runnerapi.MaxFileBytes, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := checkContent(f.Name, in.Content); err != nil {
		s.fail(w, r, err)
		return
	}
	at, err := s.cfg.Store.SaveCode(r.Context(), id.User.ID, ex.Slug, f.Name, in.Content)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"savedAt": at})
}

func (s *Server) resetFile(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	f, ok := s.stubFile(w, r, ex)
	if !ok {
		return
	}
	id, _ := identity(r)
	if err := limit(s.writeLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Store.DeleteCode(r.Context(), id.User.ID, ex.Slug, f.Name); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": f.Content})
}

// limit applies a per-user limiter.
func limit(l interface {
	Allow(string) (bool, time.Duration)
}, userID int64) error {
	if ok, retry := l.Allow(itoa(int(userID))); !ok {
		return &apiError{status: http.StatusTooManyRequests, code: "rate_limited",
			msg: "Slow down a little: too many requests. Wait a moment and try again.", retry: retryAfter(retry)}
	}
	return nil
}

// diagnosticJSON is a diagnostic with the explanation that teaches it.
type diagnosticJSON struct {
	runnerapi.Diagnostic
	Explain explain.Explanation `json:"explain"`
}

func explainAll(ex *course.Exercise, ds []runnerapi.Diagnostic) []diagnosticJSON {
	out := make([]diagnosticJSON, 0, len(ds))
	for _, d := range ds {
		e := []explain.Explanation{explain.Explain(d)}
		markOpenable(ex, e)
		out = append(out, diagnosticJSON{Diagnostic: d, Explain: e[0]})
	}
	return out
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var in struct {
		Files map[string]string `json:"files"`
	}
	if err := decode(w, r, 4*runnerapi.MaxTotalBytes, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	files, err := s.mergeFiles(r.Context(), id.User.ID, ex, in.Files)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := limit(s.checkLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, done := s.claimCheck(r.Context(), id.User.ID)
	defer done()
	res, err := s.cfg.Runner.Check(ctx, runnerapi.CheckRequest{Exercise: ex.Slug, Files: files})
	if errors.Is(context.Cause(ctx), errSuperseded) {
		s.fail(w, r, errorf(http.StatusConflict, "superseded", "A newer check of your code replaced this one."))
		return
	}
	if err != nil {
		s.runnerFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": explainAll(ex, res.Diagnostics), "durationMs": res.DurationMs})
}

// errSuperseded cancels a check when the same user starts another one.
var errSuperseded = errors.New("a newer check replaced this one")

// checkSlot is a user's check in flight.
type checkSlot struct {
	cancel context.CancelCauseFunc
}

// claimCheck makes the request the user's only check in flight: it
// cancels the one before, whose runner job then ends too. Lint as you type
// sends a check after every pause, and the runner's slots and queue are
// shared by everyone; without this, 1 user (or a script with their
// session) could fill them and lock the others out. Call done when the
// check is over.
func (s *Server) claimCheck(parent context.Context, userID int64) (ctx context.Context, done func()) {
	ctx, cancel := context.WithCancelCause(parent)
	slot := &checkSlot{cancel: cancel}
	s.mu.Lock()
	if old := s.checks[userID]; old != nil {
		old.cancel(errSuperseded)
	}
	s.checks[userID] = slot
	s.mu.Unlock()
	return ctx, func() {
		s.mu.Lock()
		if s.checks[userID] == slot {
			delete(s.checks, userID)
		}
		s.mu.Unlock()
		cancel(context.Canceled)
	}
}

// mergeFiles completes the files of a check request with the saved or
// stub content of the stubs the browser did not send, and rejects any
// name that is not a stub: the runner would too, but the learner gets a
// clearer message here.
func (s *Server) mergeFiles(ctx context.Context, userID int64, ex *course.Exercise, sent map[string]string) (map[string]string, error) {
	total := 0
	for name, content := range sent {
		if !ex.IsStub(name) {
			return nil, errorf(http.StatusBadRequest, "unknown_file", name+" is not 1 of the files you can edit.")
		}
		if err := checkContent(name, content); err != nil {
			return nil, err
		}
		total += len(content)
	}
	if total > runnerapi.MaxTotalBytes {
		return nil, errorf(http.StatusRequestEntityTooLarge, "too_large", "Your files are larger than 1 MiB together.")
	}
	files, err := s.stubContents(ctx, userID, ex)
	if err != nil {
		return nil, err
	}
	for name, content := range sent {
		files[name] = content
	}
	return files, nil
}

// runnerFailed answers a failed runner call; a request the browser
// cancelled needs no answer.
func (s *Server) runnerFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		return
	}
	var re *RunnerError
	if errors.As(err, &re) && re.Err != nil && re.Status >= 500 {
		s.log.Warn("runner call failed", "route", r.Pattern, "code", re.Code, "err", re.Err)
	}
	s.fail(w, r, err)
}

func (s *Server) format(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var in struct {
		File    string `json:"file"`
		Content string `json:"content"`
	}
	if err := decode(w, r, 4*runnerapi.MaxFileBytes, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if !ex.IsStub(in.File) {
		s.fail(w, r, errorf(http.StatusBadRequest, "unknown_file", "Only the files you can edit can be formatted."))
		return
	}
	if err := checkContent(in.File, in.Content); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	if err := limit(s.formatLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.cfg.Runner.Format(r.Context(), runnerapi.FormatRequest{Exercise: ex.Slug, File: in.File, Content: in.Content})
	if err != nil {
		s.runnerFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": res.Content, "diagnostics": explainAll(ex, res.Diagnostics)})
}

// gateError is the 409 of a soft time gate: the UI shows the numbers and
// asks the learner to confirm.
type gateError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Gate    string `json:"gate"`    // "time"
	Kind    string `json:"kind"`    // "active": time on the exercise; "since": time since the last reveal
	Level   int    `json:"level"`   // the level asked for
	Minutes int    `json:"minutes"` // what the learner has
	Needed  int    `json:"needed"`  // what the course asks for
}

func (s *Server) reveal(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var in struct {
		Level   int  `json:"level"`
		Confirm bool `json:"confirm"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	level := course.Level(in.Level)
	if !level.Valid() {
		s.fail(w, r, errorf(http.StatusBadRequest, "bad_level", "Levels go from 1 (hint 1) to 6 (the solution)."))
		return
	}
	id, _ := identity(r)
	ctx := r.Context()
	reveals, err := s.cfg.Store.Reveals(ctx, id.User.ID, ex.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var prev *store.Reveal
	for i := range reveals {
		if reveals[i].Level == in.Level {
			// Revealed before: show it again, no gate.
			s.writeReveal(w, r, ex, reveals[i])
			return
		}
		if reveals[i].Level == in.Level-1 {
			prev = &reveals[i]
		}
	}
	if in.Level > 1 && prev == nil {
		s.fail(w, r, errorf(http.StatusConflict, "reveal_order", "Reveal "+course.Level(in.Level-1).String()+" first."))
		return
	}
	if !in.Confirm {
		gate, err := s.timeGate(ctx, id.User.ID, ex.Slug, in.Level, prev)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if gate != nil {
			writeJSON(w, http.StatusConflict, gate)
			return
		}
	}
	rv, _, err := s.cfg.Store.RecordReveal(ctx, id.User.ID, ex.Slug, in.Level)
	if errors.Is(err, store.ErrRevealOrder) {
		s.fail(w, r, errorf(http.StatusConflict, "reveal_order", "Reveal "+course.Level(in.Level-1).String()+" first."))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeReveal(w, r, ex, rv)
}

// timeGate returns the gate that applies, or nil. Once the exercise is
// passed there is none: rule 5 asks learners to read the explanation and
// the walkthrough afterwards.
func (s *Server) timeGate(ctx context.Context, userID int64, slug string, level int, prev *store.Reveal) (*gateError, error) {
	p, err := s.cfg.Store.Progress(ctx, userID, slug)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if p.Passed() {
		return nil, nil
	}
	if level == 1 {
		minutes := int(p.ActiveSeconds / 60)
		if minutes >= firstHintMinutes {
			return nil, nil
		}
		return &gateError{
			Error: "The course asks you to try for 25 minutes before hint 1.",
			Code:  "gate", Gate: "time", Kind: "active", Level: level, Minutes: minutes, Needed: firstHintMinutes,
		}, nil
	}
	minutes := int(s.now().Sub(prev.RevealedAt) / time.Minute)
	if minutes >= betweenHintMinutes {
		return nil, nil
	}
	return &gateError{
		Error: "The course asks you to wait 10 minutes between hints.",
		Code:  "gate", Gate: "time", Kind: "since", Level: level, Minutes: minutes, Needed: betweenHintMinutes,
	}, nil
}

func (s *Server) writeReveal(w http.ResponseWriter, r *http.Request, ex *course.Exercise, rv store.Reveal) {
	l := course.Level(rv.Level)
	html, err := ex.Hidden(l)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, revealJSON{Level: rv.Level, Title: l.String(), HTML: html, RevealedAt: rv.RevealedAt})
}

func (s *Server) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	id, _ := identity(r)
	if err := limit(s.writeLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	now := s.now()
	s.mu.Lock()
	last, seen := s.heartbeat[id.User.ID]
	s.heartbeat[id.User.ID] = now
	s.mu.Unlock()
	credit := heartbeatCredit
	if seen {
		credit = min(max(now.Sub(last), 0), heartbeatCredit)
	}
	if err := s.cfg.Store.AddActiveTime(r.Context(), id.User.ID, ex.Slug, credit); err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.cfg.Store.Progress(r.Context(), id.User.ID, ex.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"activeSeconds": p.ActiveSeconds})
}

func (s *Server) saveNotes(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var in struct {
		Notes string `json:"notes"`
	}
	if err := decode(w, r, 4*maxNotes, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(in.Notes) > maxNotes {
		s.fail(w, r, errorf(http.StatusRequestEntityTooLarge, "too_large", "Notes are limited to 64 KiB per exercise."))
		return
	}
	id, _ := identity(r)
	if err := limit(s.writeLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Store.SetNotes(r.Context(), id.User.ID, ex.Slug, in.Notes); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
