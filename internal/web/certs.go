package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Exercises []string `json:"exercises"`
	Passed    int      `json:"passed"`
	Total     int      `json:"total"`
	Eligible  bool     `json:"eligible"`
	// Certificate is the held certificate when it covers what the kind
	// covers now.
	Certificate *certJSON `json:"certificate,omitempty"`
	// Earlier is a held certificate of the kind that covers what the kind
	// covered when it was issued (the course grew). It stays valid; a claim
	// of the kind replaces it.
	Earlier *certJSON `json:"earlier,omitempty"`
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
	return s.certificateStatusFrom(passed, held), nil
}

// certificateStatusFrom is certificateStatus for rows already read: the
// story reads the progress anyway.
func (s *Server) certificateStatusFrom(passed []string, held []store.Certificate) []certStatus {
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
			j := &certJSON{
				ID: c.ID, Name: c.NameOnCert, IssuedAt: c.IssuedAt, ShowStats: c.ShowStats,
				URL: verifyPath(c.ID), PDF: verifyPath(c.ID) + ".pdf",
			}
			if s.currentCertificate(e.Kind, c) {
				st.Certificate = j
			} else {
				st.Earlier = j
			}
		}
		out = append(out, st)
	}
	return out
}

// currentCertificate reports whether a held certificate covers what its
// kind covers in the loaded course. The stored payload is what was signed;
// the verify page checks the signature, this only reads the exercises.
func (s *Server) currentCertificate(k cert.Kind, c store.Certificate) bool {
	p, err := cert.ParsePayload(c.Payload)
	if err != nil {
		s.log.Warn("certificate payload does not parse", "id", c.ID, "err", err)
		return false
	}
	return cert.Current(s.cfg.Course, k, cert.Interpret(s.cfg.Course, p))
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
	story, err := s.loadStory(r.Context(), id.User, "")
	if err != nil {
		s.log.Error("certificates", "err", err)
		s.errorPage(w, r, http.StatusInternalServerError, "The certificates could not load.")
		return
	}
	// The Key's rail shows the learner's own skyline at the rail's size.
	sky := story.Skyline
	sky.Class = skyKinds["dash"] + " " + skyKinds["key"]
	keyCaption := ""
	if oc := story.OnCall; oc != nil {
		keyCaption = fmt.Sprintf("%02d is next", oc.Number)
		if story.KeyOnRing {
			keyCaption = fmt.Sprintf("%02d on call", oc.Number)
		}
	}
	s.render(w, r, http.StatusOK, "certificates", view{
		Title: "Certificates", Nav: "certificates", Script: "pages/certificates.js",
		Data: map[string]any{
			"Fingerprint": s.cfg.Signer.Fingerprint(),
			// The head, the strip and the tag rail (VESPER.md 8.7); the
			// tiers and the Key are in .Story.
			"Head":       story.CertsHead,
			"KeySky":     sky,
			"KeyCaption": keyCaption,
			"Earlier":    s.earlierNotes(r.Context(), story),
		},
		Story: story,
	})
}

// earlierCert is what the certificates page says about an earlier
// certificate of a kind: what it covers, and what it was issued as.
type earlierCert struct {
	Covers string // "The complete course, 25 exercises in 5 tiers"
	Note   string // cert.Coverage.IssuedAs
}

// earlierNotes reads each earlier certificate's payload: its PDF of then
// shows the first course's numbers. A certificate that cannot be read
// gets no note; the page still loads.
func (s *Server) earlierNotes(ctx context.Context, st *story) map[string]earlierCert {
	out := map[string]earlierCert{}
	for _, k := range st.certs {
		if k.Earlier == nil {
			continue
		}
		row, err := s.cfg.Store.Certificate(ctx, k.Earlier.ID)
		if err != nil {
			s.log.Warn("earlier certificate", "id", k.Earlier.ID, "err", err)
			continue
		}
		p, err := cert.ParsePayload(row.Payload)
		if err != nil {
			s.log.Warn("earlier certificate payload", "id", k.Earlier.ID, "err", err)
			continue
		}
		cv := cert.Interpret(s.cfg.Course, p)
		out[k.Kind] = earlierCert{Covers: cv.Title() + ", " + cv.Summary(), Note: cv.IssuedAs()}
	}
	return out
}

// notEligible says which exercises a claim still needs (VESPER.md 10.5):
// "Tier 2 needs every exercise to pass. 08 and 09 do not pass yet."
func notEligible(c *course.Course, k cert.Kind, passed []string) string {
	done := make(map[string]bool, len(passed))
	for _, slug := range passed {
		done[slug] = true
	}
	who := "The course certificate"
	if n, ok := k.Tier(); ok {
		who = fmt.Sprintf("Tier %d", n)
	}
	exs, err := cert.Exercises(c, k)
	if err != nil {
		return who + " needs every exercise to pass."
	}
	var missing []int
	for _, e := range exs {
		if !done[e.Slug] {
			missing = append(missing, e.Number)
		}
	}
	verb := "do not pass yet."
	if len(missing) == 1 {
		verb = "does not pass yet."
	}
	return who + " needs every exercise to pass. " + numberList(missing, true) + " " + verb
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
	if err := limit(s.claimLimit, id.User.ID, limitClaims); err != nil {
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
		s.fail(w, r, errorf(http.StatusForbidden, "not_eligible", notEligible(s.cfg.Course, kind, passed)))
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
	Title       string // "Tier 6, Forensics" or "The complete course"
	IssuedAt    time.Time
	RevokedAt   time.Time
	ReplacedBy  string
	Exercises   []*course.Exercise
	Stats       *cert.Stats
	StatsBySlug map[string]cert.ExerciseStats
	Fingerprint string
	PDF         string
	URL         string // the absolute link to this page, to share
	// The paper (VESPER.md 8.9) repeats the PDF's body and title block:
	// a tier's tag and its exercises, or for the course the Key and the
	// course to scale, both of the exercises the payload covers.
	IsCourse bool
	TierN    int          // the tier a tier certificate covers, from its kind
	Tier     *course.Tier // that tier in the course; nil when it no longer exists
	Sky      skyline      // the course certificate's exercises to scale
	Range    string       // the exercises as a range, "04 to 09"
	Host     string       // the portal in the issuer line: "vesper-labs.org"
	QR       template.HTML
	// IssuedAs says what a certificate of the first course was issued as,
	// for a reader who holds its PDF of then (cert.Coverage.IssuedAs).
	IssuedAs string
	// NameRow: the name takes the title block's whole first row, as on
	// the PDF, and Issued moves into the second row (VESPER.md 13.1.1).
	NameRow bool
	// NameOther: Saira cannot set the name, so the paper sets it in
	// another face at a size that face fits, as the PDF does.
	NameOther bool
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
		// The field keeps what was typed, so it can be corrected (VESPER.md 6.3).
		v.ID, v.Error = raw, "This is not a certificate ID. IDs look like GP-7Q2M-K9XD-3HFA."
	}
	s.render(w, r, http.StatusOK, "verify", view{Title: "Verify a certificate", Nav: "verify", Data: v})
}

func (s *Server) verifyPage(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	if base, ok := strings.CutSuffix(raw, ".pdf"); ok {
		s.verifyPDF(w, r, base)
		return
	}
	id, err := cert.ParseID(raw)
	if err != nil {
		s.render(w, r, http.StatusNotFound, "verify", view{Title: "Unknown certificate", Nav: "verify",
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
	s.render(w, r, status, "verify", view{Title: title, Nav: "verify", Data: v})
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
	if v.NameRow, err = cert.NameTakesTitleRow(p.Name); err != nil {
		// The page still shows every fact; only the cells keep their place.
		s.log.Warn("measure the name for the title block", "id", id, "err", err)
	}
	if saira, err := cert.NameInSaira(p.Name); err == nil {
		v.NameOther = !saira
	}
	cv := cert.Interpret(s.cfg.Course, p)
	v.Title, v.Exercises, v.Range, v.IssuedAs = cv.Title(), cv.Exercises, cv.Range(), cv.IssuedAs()
	v.IssuedAt, _ = p.Issued()
	v.Host = s.host()
	v.QR = qrSVG(v.URL)
	// A certificate of the first course names its tier as the course reads
	// it now (cert.Interpret); its PDF of then has the old number.
	v.Tier = cv.Tier
	v.TierN, _ = p.Kind.Tier()
	if cv.Tier != nil {
		v.TierN = cv.Tier.N
	}
	if p.Kind == cert.KindCourse {
		v.IsCourse = true
		// Every tower passed, so every tower is ink, as on the PDF; only
		// the tiers the certificate covers stand.
		covered := map[string]bool{}
		for _, e := range v.Exercises {
			covered[e.Slug] = true
		}
		v.Sky = drawSkyline(&course.Course{Tiers: cv.Tiers}, func(e *course.Exercise) string {
			if covered[e.Slug] {
				return "on"
			}
			return ""
		})
		v.Sky.Class = skyKinds["key"]
		v.Sky.Label = fmt.Sprintf("The course to scale: %d towers, 1 per exercise, as tall as its estimate. All of them pass.", len(v.Exercises))
	}
	if p.Stats != nil {
		v.Stats, v.StatsBySlug = p.Stats, cv.Stats
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

// qrSVG draws the verify link's QR code as the PDF does (cert.QRMatrix),
// 1 rectangle per run of dark modules, with a quiet zone of 2 modules.
// "" when the link does not encode.
func qrSVG(link string) template.HTML {
	m, err := cert.QRMatrix(link)
	if err != nil || len(m) == 0 {
		return ""
	}
	n := len(m)
	var d strings.Builder
	for y, row := range m {
		for x := 0; x < n; {
			if !row[x] {
				x++
				continue
			}
			start := x
			for x < n && row[x] {
				x++
			}
			fmt.Fprintf(&d, "M%d %dh%dv1h-%dZ", start, y, x-start, x-start)
		}
	}
	return template.HTML(fmt.Sprintf(`<svg class="gpw-pp-qrsvg" viewBox="-2 -2 %d %d" aria-hidden="true"><rect class="gpw-pp-qrbg" x="-2" y="-2" width="%d" height="%d"/><path class="gpw-pp-qrm" d="%s"/></svg>`, //nolint:gosec // numbers only
		n+4, n+4, n+4, n+4, d.String()))
}

// verifyURL is the absolute verify link, for pages that show it.
func (s *Server) verifyURL(id string) string {
	return s.origin + verifyPath(url.PathEscape(id))
}
