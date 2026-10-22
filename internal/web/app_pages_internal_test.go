package web

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
)

func TestSlabMarkLines(t *testing.T) {
	for label, want := range map[string][]string{
		"Breaker 0, Hall 3": {"Breaker 0,", "Hall 3"},
		"Day 1, floor 61":   {"Day 1,", "floor 61"},
		"":                  nil,
	} {
		if got := (slab{MarkLabel: label}).MarkLines(); !slices.Equal(got, want) {
			t.Errorf("MarkLines(%q) = %q, want %q", label, got, want)
		}
	}
}

// TestPlateSVG: the plate's look follows the tier's state, and solid
// amber means stamped only (VESPER.md 6.18).
func TestPlateSVG(t *testing.T) {
	tier := &course.Tier{N: 10, Name: "The Key Turns", First: 45, Last: 50}
	for _, tt := range []struct {
		tag        tierTag
		class, has string
	}{
		{tierTag{Tier: tier, State: tagStamped, Total: 6, Passed: 6}, "gpw-ct-held", `class="gpw-sv-plate"`},
		{tierTag{Tier: tier, State: tagReady, Total: 6, Passed: 6}, "gpw-ct-claim", `class="gpw-sv-claim"`},
		{tierTag{Tier: tier, State: tagOpen, Total: 6, Started: 1}, "gpw-ct-progress", `class="gpw-sv-am gpw-sv-dash"`},
		{tierTag{Tier: tier, State: tagNone, Total: 6}, "gpw-ct-blank", `<path class="gpw-sv-h2" d="M8 14H104V132L90 146H8Z"/>`},
	} {
		if got := tt.tag.CertClass(); got != tt.class {
			t.Errorf("%s: CertClass = %q, want %q", tt.tag.State, got, tt.class)
		}
		svg := string(tt.tag.PlateSVG())
		for _, want := range []string{tt.has, `>T1<tspan class="gpw-sv-z">0</tspan></text>`, ">THE KEY TURNS<", `>45 TO 5<tspan class="gpw-sv-z">0</tspan></text>`} {
			if !strings.Contains(svg, want) {
				t.Errorf("%s: the plate lacks %q:\n%s", tt.tag.State, want, svg)
			}
		}
		if tt.class != "gpw-ct-held" && strings.Contains(svg, "gpw-sv-plate") {
			t.Errorf("%s: a plate that is not stamped is solid amber", tt.tag.State)
		}
	}
}

func TestKeyArt(t *testing.T) {
	for _, tt := range []struct {
		key  keyState
		want string
	}{
		{keyState{}, `class="gpw-ct-keyg"`},
		{keyState{Ready: true}, `class="gpw-ct-keyg gpw-ct-keyg-ready"`},
		{keyState{Held: &certJSON{ID: "GP-X"}}, `class="gpw-ct-keyg gpw-ct-keyg-held"`},
	} {
		s := &story{Key: tt.key, Total: 50}
		if svg := string(s.KeyArt()); !strings.Contains(svg, tt.want) || !strings.Contains(svg, keyPath) {
			t.Errorf("KeyArt(%+v) lacks %q", tt.key, tt.want)
		}
	}
	if got := (&story{Total: 50}).KeySummary(); got != "The Key covers exercises 01 to 50. All 50 pass." {
		t.Errorf("KeySummary = %q", got)
	}
}

// TestQRSVG: the paper draws the same modules as the PDF, 1 rectangle per
// run, inside a quiet zone of 2 modules.
func TestQRSVG(t *testing.T) {
	link := "https://vesper-labs.org/verify/GP-7F3K-Q9MW-2D4X"
	m, err := cert.QRMatrix(link)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(qrSVG(link))
	n := len(m)
	if want := "viewBox=\"-2 -2 " + itoa(n+4) + " " + itoa(n+4) + "\""; !strings.Contains(svg, want) {
		t.Errorf("qrSVG lacks %s", want)
	}
	dark := 0
	for _, row := range m {
		for _, b := range row {
			if b {
				dark++
			}
		}
	}
	drawn := 0
	for _, run := range regexp.MustCompile(`M\d+ \d+h(\d+)v1`).FindAllStringSubmatch(svg, -1) {
		drawn += atoi(t, run[1])
	}
	if drawn != dark {
		t.Errorf("qrSVG draws %d modules, the code has %d", drawn, dark)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// TestPaperText: the verify page's paper says what the PDF says.
func TestPaperText(t *testing.T) {
	tier := testCourseTier(t, 1)
	v := verifyView{TierN: 1, Tier: tier, Exercises: tier.Exercises}
	if got, want := v.What(), "completed Tier 1, Badge In, of the go-practice course: you read the raw lines the dashboards miss."; got != want {
		t.Errorf("What = %q, want %q", got, want)
	}
	if got := v.Each(); got != "Each of the 5 exercises" {
		t.Errorf("Each = %q", got)
	}
	gone := verifyView{TierN: 11, Exercises: tier.Exercises[:1]}
	if got := gone.What(); got != "completed Tier 11 of the go-practice course." {
		t.Errorf("What of a tier that no longer exists = %q", got)
	}
	if got := gone.Each(); got != "The exercise" {
		t.Errorf("Each of 1 = %q", got)
	}
	course := verifyView{IsCourse: true, Exercises: tier.Exercises}
	if got, want := course.What(), "held the Key through 5 exercises and wrote every tool by hand. Every pager in Meridian still works."; got != want {
		t.Errorf("What of the course = %q, want %q", got, want)
	}
	if got := course.Each(); got != "Each of the 5 exercises of the go-practice course" {
		t.Errorf("Each of the course = %q", got)
	}
	v.StatsBySlug = map[string]cert.ExerciseStats{"a": {Explanation: true, Solution: true}}
	if got := v.AlsoRevealed("a"); got != "explanation, solution" {
		t.Errorf("AlsoRevealed = %q", got)
	}
	if got := v.AlsoRevealed("b"); got != "nothing" {
		t.Errorf("AlsoRevealed of nothing = %q", got)
	}
}

func testCourseTier(t *testing.T, n int) *course.Tier {
	t.Helper()
	c, err := course.Load(cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course")))
	if err != nil {
		t.Fatal(err)
	}
	tier, ok := c.Tier(n)
	if !ok {
		t.Fatalf("no tier %d", n)
	}
	return tier
}
