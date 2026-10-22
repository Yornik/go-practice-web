package web

import (
	"cmp"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
)

func TestFirstSentences(t *testing.T) {
	for in, want := range map[template.HTML]template.HTML{
		"<p>One. Two <code>a. b</code> two. Three.</p><p>Four.</p>": "One. Two <code>a. b</code> two.",
		"<p>Only 1 sentence.</p>":                                   "Only 1 sentence.",
		"<p>Line<br>break. Two. Three.</p>":                         "Line<br>break. Two.",
		"<h3>No paragraph</h3>":                                     "",
		"<p>A <a href=\"x. y\">link</a>. Two. Three.</p>":           "A <a href=\"x. y\">link</a>. Two.",
	} {
		if got := firstSentences(in, 2); got != want {
			t.Errorf("firstSentences(%q) = %q, want %q", in, got, want)
		}
	}
}

// The anatomy drawing takes its words from the catalog entry the IDE
// shows for the drawn message.
func TestLandingAnatomy(t *testing.T) {
	a := newLandingAnatomy()
	if !strings.Contains(string(a.Title), "<code>strings</code>") {
		t.Errorf("title %q does not set the name in code", a.Title)
	}
	if a.Means == "" || strings.Contains(string(a.Means), "<p>") || !strings.HasSuffix(string(a.Means), ".") {
		t.Errorf("meaning %q is not 1 or 2 sentences of text", a.Means)
	}
}

func TestCutRunes(t *testing.T) {
	for in, want := range map[string]string{
		"/short":                      "/short",
		"/" + strings.Repeat("é", 60): "/" + strings.Repeat("é", 58) + "…",
	} {
		if got := cutRunes(in, 60); got != want {
			t.Errorf("cutRunes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailAddr(t *testing.T) {
	for in, want := range map[string]emailAddr{
		"ada@example.org": {"ada@", "example.org"},
		"a@b@example.org": {"a@b@", "example.org"},
		"no-at-sign":      {"no-at-sign", ""},
	} {
		if got := newEmailAddr(in); got != want {
			t.Errorf("newEmailAddr(%q) = %+v, want %+v", in, got, want)
		}
	}
}

// Every count of the landing page comes from the course: the real one,
// and the first course, whose tiers have no names and no place in the
// story.
func TestLandingView(t *testing.T) {
	c, err := course.Load(cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course")))
	if err != nil {
		t.Fatal(err)
	}
	v := newLandingView(c)
	if v.First == nil || v.First.Number != 1 || v.FirstTier != "1, Badge In" {
		t.Fatalf("first = %+v, tier %q", v.First, v.FirstTier)
	}
	if len(v.Years) != 2 || v.Years[0].Label != "September to 1 January. Tiers 1 to 5, about 35.5" ||
		v.Years[1].Label != "2 January to the night the clocks go back. Tiers 6 to 10, about 73" {
		t.Errorf("years = %+v", v.Years)
	}
	if got := v.Years[0].Tiers[0]; got.State != "cur" || got.Head != "September, about 7" {
		t.Errorf("tier 1 = %q %q", got.State, got.Head)
	}
	if got := v.Years[1].Tiers[0]; got.State != "off" || got.Head != "January and February, about 4" {
		t.Errorf("tier 6 = %q %q", got.State, got.Head)
	}
	wantLabel := "Skyline of the course: 50 towers, 1 per exercise, as tall as its estimate. " +
		"Year 1 holds tiers 1 to 5, about 35.5 h; year 2 holds tiers 6 to 10, about 73 h. Tower 01 is lit: you start there."
	if v.Sky.Label != wantLabel || v.Sky.Class != "gpw-sky-refl" {
		t.Errorf("skyline = %q %q", v.Sky.Class, v.Sky.Label)
	}

	first := newLandingView(coursetest.Load(t, coursetest.First()))
	if first.First == nil || len(first.Years) == 0 {
		t.Fatalf("the first course has no landing data: %+v", first)
	}
	for _, y := range first.Years {
		for _, tr := range y.Tiers {
			if !strings.HasPrefix(tr.Head, "about ") {
				t.Errorf("tier %d of the first course has months: %q", tr.N, tr.Head)
			}
		}
	}
}

// Error pages have titles in sentence case (VESPER.md 10.2).
func TestErrorTitle(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusNotFound:            "Page not found",
		http.StatusForbidden:           "No access",
		http.StatusInternalServerError: "Server error",
		http.StatusServiceUnavailable:  "Server error",
		http.StatusBadRequest:          "Bad request",
		499:                            "Error",
	} {
		if got := errorTitle(status); got != want {
			t.Errorf("errorTitle(%d) = %q, want %q", status, got, want)
		}
	}
}
