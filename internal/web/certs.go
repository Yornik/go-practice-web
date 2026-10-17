package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/store"
)

// certJSON is an issued certificate in the API.
type certJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IssuedAt  time.Time `json:"issuedAt"`
	ShowStats bool      `json:"showStats"`
	URL       string    `json:"url"`
	PDF       string    `json:"pdf"`
}

// certStatus is 1 kind of certificate for a user: how far they are and
// what they hold.
type certStatus struct {
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Exercises   []string  `json:"exercises"`
	Passed      int       `json:"passed"`
	Total       int       `json:"total"`
	Eligible    bool      `json:"eligible"`
	Certificate *certJSON `json:"certificate,omitempty"`
}

func verifyPath(id string) string { return "/verify/" + id }

func (s *Server) certificateStatus(ctx context.Context, userID int64) ([]certStatus, error) {
	passed, err := s.cfg.Store.PassedExercises(ctx, userID)
	if err != nil {
		return nil, err
	}
	held, err := s.cfg.Store.UserCertificates(ctx, userID)
	if err != nil {
		return nil, err
	}
	byKind := map[string]store.Certificate{}
	for _, c := range held {
		byKind[c.Kind] = c
	}
	var out []certStatus
	for _, e := range cert.Eligibilities(s.cfg.Course, passed) {
		st := certStatus{
			Kind: string(e.Kind), Title: e.Title, Exercises: e.Exercises,
			Passed: e.Passed, Total: len(e.Exercises), Eligible: e.Eligible,
		}
		if c, ok := byKind[string(e.Kind)]; ok {
			st.Certificate = &certJSON{
				ID: c.ID, Name: c.NameOnCert, IssuedAt: c.IssuedAt, ShowStats: c.ShowStats,
				URL: verifyPath(c.ID), PDF: verifyPath(c.ID) + ".pdf",
			}
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	st, err := s.certificateStatus(r.Context(), id.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kinds": st})
}

func (s *Server) certificatesPage(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	st, err := s.certificateStatus(r.Context(), id.User.ID)
	if err != nil {
		s.log.Error("certificates", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The certificates could not load.")
		return
	}
	passed, err := s.cfg.Store.PassedExercises(r.Context(), id.User.ID)
	if err != nil {
		s.log.Error("certificates", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The certificates could not load.")
		return
	}
	done := map[string]bool{}
	for _, slug := range passed {
		done[slug] = true
	}
	cards := make([]certCard, 0, len(st))
	for _, k := range st {
		c := certCard{certStatus: k}
		if kind, err := cert.ParseKind(s.cfg.Course, k.Kind); err == nil {
			if n, ok := kind.Tier(); ok {
				if t, ok := s.cfg.Course.Tier(n); ok {
					c.Name, c.Theme = t.Title(), t.Theme
				}
			} else {
				c.Name, c.Theme = "The course", fmt.Sprintf("All %d exercises", len(s.cfg.Course.Exercises))
			}
		}
		for _, slug := range k.Exercises {
			if e, ok := s.cfg.Course.Exercise(slug); ok && !done[slug] {
				c.Missing = append(c.Missing, fmt.Sprintf("%02d", e.Number))
			}
		}
		cards = append(cards, c)
	}
	s.render(w, r, http.StatusOK, "certificates", view{
		Title: "Certificates", Nav: "certificates", Script: "pages/certificates.js",
		Data: map[string]any{"Cards": cards, "Fingerprint": s.cfg.Signer.Fingerprint()},
	})
}

// certCard is 1 certificate on the certificates page.
type certCard struct {
	certStatus
	Name    string   // "Tier 2" or "The course"
	Theme   string   // the tier's theme
	Missing []string // the numbers of the exercises that do not pass yet
}

// issueAttempts bounds the retries on an ID collision, which at 60 bits
// of randomness means something is broken if it repeats.
const issueAttempts = 3

func (s *Server) claimCertificate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		ShowStats bool   `json:"showStats"`
	}
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	ctx := r.Context()
	if err := limit(s.claimLimit, id.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	kind, err := cert.ParseKind(s.cfg.Course, in.Kind)
	if err != nil {
		s.fail(w, r, errorf(http.StatusBadRequest, "bad_kind", "There is no certificate "+in.Kind+"."))
		return
	}
	passed, err := s.cfg.Store.PassedExercises(ctx, id.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ok, err := cert.Eligible(s.cfg.Course, kind, passed); err != nil || !ok {
		s.fail(w, r, errorf(http.StatusForbidden, "not_eligible", "Pass every exercise this certificate covers first."))
		return
	}
	name := in.Name
	if strings.TrimSpace(name) == "" {
		name = id.User.DisplayName
	}
	if name, err = cert.CleanName(name); err != nil {
		msg := "Use letters, spaces and punctuation."
		var ne *cert.NameError
		if errors.As(err, &ne) {
			msg = ne.Message()
		}
		s.fail(w, r, errorf(http.StatusBadRequest, "bad_name", msg))
		return
	}
	exs, err := cert.Exercises(s.cfg.Course, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	slugs := make([]string, len(exs))
	for i, e := range exs {
		slugs[i] = e.Slug
	}
	var stats *cert.Stats
	if in.ShowStats {
		if stats, err = s.activityStats(ctx, id.User.ID, slugs); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	for range issueAttempts {
		c, replaced, err := s.issue(ctx, id.User.ID, kind, name, slugs, stats)
		var conflict *store.ConflictError
		if errors.As(err, &conflict) && conflict.Column == "certificates.id" {
			continue
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.cfg.Store.Audit(ctx, id.User.ID, "certificate.issued",
			map[string]string{"id": c.ID, "kind": string(kind), "replaced": replaced}, s.client(r).IP); err != nil {
			s.log.Error("audit", "err", err)
		}
		writeJSON(w, http.StatusCreated, map[string]string{
			"id": c.ID, "url": verifyPath(c.ID), "pdf": verifyPath(c.ID) + ".pdf", "replaced": replaced,
		})
		return
	}
	s.fail(w, r, errors.New("certificate ID collided on every attempt"))
}

// issue signs and stores 1 certificate.
func (s *Server) issue(ctx context.Context, userID int64, kind cert.Kind, name string, slugs []string, stats *cert.Stats) (store.Certificate, string, error) {
	c, err := s.cfg.Signer.Issue(cert.Request{Kind: kind, Name: name, Exercises: slugs, Stats: stats, IssuedAt: s.now()})
	if err != nil {
		return store.Certificate{}, "", err
	}
	issued, err := c.Payload.Issued()
	if err != nil {
		return store.Certificate{}, "", err
	}
	row := store.Certificate{
		ID: c.Payload.ID, UserID: userID, Kind: string(kind), NameOnCert: c.Payload.Name, IssuedAt: issued,
		ShowStats: stats != nil, Payload: c.Canonical, Signature: c.Signature,
	}
	if stats != nil {
		if row.Stats, err = json.Marshal(stats); err != nil {
			return store.Certificate{}, "", err
		}
	}
	replaced, err := s.cfg.Store.IssueCertificate(ctx, row)
	return row, replaced, err
}

// activityStats sums the user's activity on the given exercises.
func (s *Server) activityStats(ctx context.Context, userID int64, slugs []string) (*cert.Stats, error) {
	progress, err := s.cfg.Store.UserProgress(ctx, userID)
	if err != nil {
		return nil, err
	}
	reveals, err := s.cfg.Store.UserReveals(ctx, userID)
	if err != nil {
		return nil, err
	}
	activity := map[string]cert.Activity{}
	for _, p := range progress {
		activity[p.Exercise] = cert.Activity{ActiveSeconds: p.ActiveSeconds, Runs: p.Runs}
	}
	for _, rv := range reveals {
		a := activity[rv.Exercise]
		a.Levels = append(a.Levels, rv.Level)
		activity[rv.Exercise] = a
	}
	return cert.BuildStats(slugs, activity), nil
}

// verifyView is the data of the public verify page.
type verifyView struct {
	ID          string
	Status      string // "lookup", "valid", "revoked", "unknown" or "invalid"
	Error       string // for a lookup that failed
	Name        string
	Title       string
	IssuedAt    time.Time
	RevokedAt   time.Time
	ReplacedBy  string
	Exercises   []*course.Exercise
	Stats       *cert.Stats
	StatsBySlug map[string]cert.ExerciseStats
	Fingerprint string
	PDF         string
	URL         string // the absolute link to this page, to share
	// The drawing area: a tier's exercises, or for the course the 5 tiers
	// and the course strip.
	IsCourse bool
	Tiers    []*course.Tier
	Strip    strip
	Range    string // the exercises as a range, "04 to 09"
}

// verifyLookup answers the "enter an ID" form.
func (s *Server) verifyLookup(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("id"))
	v := verifyView{Status: "lookup", Fingerprint: s.cfg.Signer.Fingerprint()}
	if raw != "" {
		id, err := cert.ParseID(raw)
		if err == nil {
			http.Redirect(w, r, verifyPath(id), http.StatusSeeOther)
			return
		}
		v.Error = "This is not a certificate ID. IDs look like GP-7Q2M-K9XD-3HFA."
	}
	s.render(w, r, http.StatusOK, "verify", view{Title: "Verify a certificate", Data: v})
}

func (s *Server) verifyPage(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	if base, ok := strings.CutSuffix(raw, ".pdf"); ok {
		s.verifyPDF(w, r, base)
		return
	}
	id, err := cert.ParseID(raw)
	if err != nil {
		s.render(w, r, http.StatusNotFound, "verify", view{Title: "Unknown certificate",
			Data: verifyView{ID: raw, Status: "unknown", Fingerprint: s.cfg.Signer.Fingerprint()}})
		return
	}
	if id != raw {
		// 1 URL per certificate, whatever was typed or scanned.
		http.Redirect(w, r, verifyPath(id), http.StatusMovedPermanently)
		return
	}
	v, _, _, err := s.loadCertificate(r.Context(), id)
	if err != nil {
		s.log.Error("verify", "id", id, "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The server could not check this certificate.")
		return
	}
	status := http.StatusOK
	title := "Certificate " + id
	if v.Status == "unknown" {
		status, title = http.StatusNotFound, "Unknown certificate"
	}
	s.render(w, r, status, "verify", view{Title: title, Data: v})
}

// loadCertificate reads a certificate and checks its signature. The page
// shows only what the signature covers: the payload, not the columns
// beside it.
func (s *Server) loadCertificate(ctx context.Context, id string) (verifyView, store.Certificate, cert.Payload, error) {
	v := verifyView{ID: id, Fingerprint: s.cfg.Signer.Fingerprint(), PDF: verifyPath(id) + ".pdf", URL: s.verifyURL(id)}
	row, err := s.cfg.Store.Certificate(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		v.Status = "unknown"
		return v, row, cert.Payload{}, nil
	}
	if err != nil {
		return v, row, cert.Payload{}, err
	}
	p, err := s.cfg.Signer.Verify(row.Payload, row.Signature)
	if err != nil || p.ID != id {
		// A signature that does not match is what the page reports, not a
		// failure to load it.
		s.log.Warn("certificate signature does not verify", "id", id, "err", err)
		v.Status = "invalid"
		return v, row, cert.Payload{}, nil //nolint:nilerr // see above
	}
	v.Status = "valid"
	if row.Revoked() {
		v.Status, v.RevokedAt, v.ReplacedBy = "revoked", row.RevokedAt, row.ReplacedBy
	}
	v.Name = p.Name
	if v.Title, err = cert.Title(s.cfg.Course, p.Kind); err != nil {
		v.Title = string(p.Kind)
	}
	v.IssuedAt, _ = p.Issued()
	for _, slug := range p.Exercises {
		if e, ok := s.cfg.Course.Exercise(slug); ok {
			v.Exercises = append(v.Exercises, e)
		}
	}
	if n := len(v.Exercises); n > 0 {
		v.Range = fmt.Sprintf("%02d to %02d", v.Exercises[0].Number, v.Exercises[n-1].Number)
	}
	if p.Kind == cert.KindCourse {
		v.IsCourse, v.Tiers = true, s.cfg.Course.Tiers
		states := map[string]string{}
		for _, e := range v.Exercises {
			states[e.Slug] = "passed"
		}
		v.Strip = courseStrip(s.cfg.Course, states)
		v.Strip.Label = fmt.Sprintf("The course to scale: all %d exercises pass.", len(v.Exercises))
	}
	if p.Stats != nil {
		v.Stats = p.Stats
		v.StatsBySlug = map[string]cert.ExerciseStats{}
		for _, e := range p.Stats.Exercises {
			v.StatsBySlug[e.Slug] = e
		}
	}
	return v, row, p, nil
}

func (s *Server) verifyPDF(w http.ResponseWriter, r *http.Request, raw string) {
	id, err := cert.ParseID(raw)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if id != raw {
		http.Redirect(w, r, verifyPath(id)+".pdf", http.StatusMovedPermanently)
		return
	}
	// Rendering takes tens of milliseconds of CPU; a public endpoint must
	// not be a cheap way to burn it.
	if ok, retry := s.pdfLimit.Allow(s.client(r).RateKey()); !ok {
		w.Header().Set("Retry-After", retryAfter(retry))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	v, row, p, err := s.loadCertificate(r.Context(), id)
	if err != nil {
		s.log.Error("verify pdf", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if v.Status != "valid" && v.Status != "revoked" {
		http.NotFound(w, r)
		return
	}
	doc, err := cert.NewDocument(s.cfg.Course, p, s.origin, s.cfg.Signer.Fingerprint())
	if err != nil {
		s.log.Error("verify pdf", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	doc.Revoked, doc.ReplacedBy = row.Revoked(), row.ReplacedBy
	var buf bytes.Buffer
	if err := cert.Render(&buf, doc); err != nil {
		s.log.Error("render pdf", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	// The browser's PDF viewer runs under the page's CSP in some browsers;
	// the PDF has no scripts of ours to protect.
	h.Del("Content-Security-Policy")
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="go-practice-%s.pdf"`, id))
	// Short: a revocation must show up soon.
	h.Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) publicKey(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(s.cfg.Signer.PublicKeyBase64() + "\n"))
}

// verifyURL is the absolute verify link, for pages that show it.
func (s *Server) verifyURL(id string) string {
	return s.origin + verifyPath(url.PathEscape(id))
}
