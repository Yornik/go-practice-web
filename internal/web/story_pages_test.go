package web_test

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yornik/go-practice-web/internal/web"
)

// The story's page data (internal/web/story.go) goes through the real
// handlers and the store. The site's templates do not show all of it yet,
// so these probe templates print it.
var storyProbes = map[string]string{
	"dashboard": `{{define "content"}}<p id="probe">{{with .Story}}oncall={{with .OnCall}}{{.Number}}{{end}};key={{.KeyOnRing}};stamped={{.Stamped}};` +
		`month={{.Readout.MonthLine}};rotation={{.Readout.Rotation}};slab={{.Slab.Button}};said={{.Ledger.Said}};calm={{.Calm.Score}};count={{.Ring.Count}}{{end}};` +
		`badge={{.Badge.TagText}}{{range .Data.Tiers}};T{{.N}}={{.Tag.State}}{{end}}{{range .Data.Rows}}{{if .OnCall}};row={{.Number}}{{end}}{{end}}</p>` +
		`{{with .Story}}{{.Ring.SVG}}{{end}}{{end}}`,
	"ide":          `{{define "content"}}<p id="probe">oncall={{.Data.OnCall}};tier={{.Data.TierValue}};first={{.Data.FirstPassLine}};badge={{.Badge.TagText}}</p>{{end}}`,
	"certificates": `{{define "content"}}<p id="probe">said={{.Data.Head.Said}};tags={{.Data.Head.Tags}};newest={{.Data.Head.Newest}}</p>{{end}}`,
	"account":      `{{define "content"}}<p id="probe">label={{.Badge.Label}};ledger={{with .Story}}{{.Readout.Ledger}}{{end}}</p>{{end}}`,
	"error":        `{{define "content"}}<p id="probe">error={{.Data.Message}}</p>{{end}}`,
}

var probeLine = regexp.MustCompile(`<p id="probe">([^<]*)</p>`)

// probe renders path with the probe templates and returns the probe line.
func (h *harness) probe(path string, as *user) string {
	h.t.Helper()
	w := h.do(req{method: "GET", path: path, as: as})
	wantStatus(h.t, w, http.StatusOK)
	m := probeLine.FindStringSubmatch(w.Body.String())
	if m == nil {
		h.t.Fatalf("GET %s has no probe: %s", path, w.Body)
	}
	return strings.ReplaceAll(m[1], "&#39;", "'")
}

func TestStoryPages(t *testing.T) {
	h := newHarness(t)
	// The probes replace the pages; the layout and the partials are the
	// site's own.
	site := os.DirFS(filepath.Join("..", "..", "web", "templates"))
	partials, err := fs.Glob(site, "partials/*.html")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{}
	for _, name := range append(partials, "layout.html") {
		b, err := fs.ReadFile(site, name)
		if err != nil {
			t.Fatal(err)
		}
		fsys[name] = &fstest.MapFile{Data: b}
	}
	for name, src := range storyProbes {
		fsys["pages/"+name+".html"] = &fstest.MapFile{Data: []byte(src)}
	}
	cfg := h.cfg
	cfg.Templates = fsys
	srv, err := web.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.handler = srv.Handler()
	admin := h.admin()
	ada := h.learner(admin, "noor")
	ctx := t.Context()
	wantProbe := func(path string, want ...string) {
		t.Helper()
		got := h.probe(path, ada)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("GET %s: %q lacks %q", path, got, w)
			}
		}
	}

	// Day 1.
	wantProbe("/learn", "oncall=1;key=false;stamped=0;month=September, tier 1;rotation=The Key is on its hook;slab=Start exercise 01;",
		"said=0 of 50 tools are in the Ledger.;calm=99.2;count=0 of 10 tags;badge=Day 1;T1=open;T2=none")
	wantProbe("/account", "label=Noor (N), day 1, no tags yet. Account menu.;ledger=Noor, junior SRE")

	// Tier 1 passes and its tag is stamped by the claim. No exercise of
	// tier 2 is started: the Key stays on its hook beside the tag.
	for _, e := range testCourse.Tiers[0].Exercises {
		if _, err := h.store.MarkPassed(ctx, ada.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}
	wantProbe("/learn", "stamped=0;", "T1=ready;T2=open", "badge=Day 1")
	w := h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "tier2"}, as: ada})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Tier 2 needs every exercise to pass. 06 to 10 do not pass yet.") {
		t.Errorf("claim of tier 2 = %d %s", w.Code, w.Body)
	}
	wantStatus(t, h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "tier1"}, as: ada}), http.StatusCreated)
	wantProbe("/learn", "oncall=6;key=false;stamped=1;month=October, tier 2;rotation=The Key is on its hook;slab=Start exercise 06;",
		"said=5 of 50 tools are in the Ledger.;calm=99.2;count=1 of 10 tags;badge=T1;T1=stamped;T2=open")
	if body := h.do(req{method: "GET", path: "/learn", as: ada}).Body.String(); !strings.Contains(body, "translate(98 138) scale(1.6)") {
		t.Error("the dashboard's ring has no hook beside the T1 tag")
	}
	wantProbe("/certificates", "said=1 of 11 certificates is yours and current.;tags=1 of 10;newest=17 Oct 2026, tier 1")

	// The IDE counts the exercise it opens as started: 07 puts the Key on
	// the ring and is the on-call exercise.
	semver, rollcall := testCourse.Tiers[1].Exercises[0], testCourse.Tiers[1].Exercises[1]
	wantProbe("/learn/"+rollcall.Slug, "oncall=true;tier=2, The Key;first=All green, and this time it is true.;badge=T1")
	// The IDE's bootstrap request starts an exercise: 06, then 07.
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/" + semver.Slug, as: ada}), http.StatusOK)
	h.clock.Advance(time.Minute)
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/" + rollcall.Slug, as: ada}), http.StatusOK)
	wantProbe("/learn", "oncall=7;key=true;", "rotation=On call, with Wren;slab=Continue exercise 07;", "row=7")

	// A later save in 06 makes 06 the exercise worked in last.
	h.clock.Advance(time.Minute)
	wantStatus(t, h.do(req{method: "PUT", path: "/api/exercises/" + semver.Slug + "/files/" + semver.StubNames()[0],
		body: map[string]string{"content": "package semver\n"}, as: ada}), http.StatusOK)
	wantProbe("/learn", "oncall=6;key=true;", "slab=Continue exercise 06;", "row=6")
	// Whichever is open in the IDE is the one worked in now.
	wantProbe("/learn/"+rollcall.Slug, "oncall=true;")
	wantProbe("/learn/"+semver.Slug, "oncall=true;", "first=Into the Ledger. Author: you.")
}

// "Issue again" of an older tier stamps no new tag: through the store, the
// tag keeps the date of its first certificate and T2 stays in front.
func TestReissueKeepsTheFrontTag(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	ada := h.learner(admin, "wren")
	for _, tier := range testCourse.Tiers[:2] {
		for _, e := range tier.Exercises {
			if _, err := h.store.MarkPassed(t.Context(), ada.ID, e.Slug); err != nil {
				t.Fatal(err)
			}
		}
	}
	claim := func(kind string) {
		t.Helper()
		wantStatus(t, h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": kind}, as: ada}), http.StatusCreated)
		h.clock.Advance(24 * time.Hour)
	}
	claim("tier1") // 17 Oct
	claim("tier2") // 18 Oct
	claim("tier1") // 19 Oct: issued again
	text := pageText(h.do(req{method: "GET", path: "/learn", as: ada}).Body.String())
	for _, want := range []string{
		"Your ring: the stamped tag T2, The Key, hangs on it, with 1 earlier tag behind it.",
		"Tag stamped 17 Oct", "stamped 18 Oct",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the course page lacks %q", want)
		}
	}
	if strings.Contains(text, "Tag stamped 19 Oct") {
		t.Error("the re-issue moved the tag's date")
	}
}
