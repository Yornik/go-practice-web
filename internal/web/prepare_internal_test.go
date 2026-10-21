package web

import (
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
	"github.com/yornik/go-practice-web/internal/explain"
)

// TestPrepareRenamesCourseExercise: the catalog names the exercise that
// teaches a concept by its slug of the first course; the IDE gets the slug
// the loaded course uses, in the field and in the link.
func TestPrepareRenamesCourseExercise(t *testing.T) {
	output := "fatal error: all goroutines are asleep - deadlock!\n\ngoroutine 1 [chan receive]:\nmain.main()\n"
	items := explain.ExplainTestOutput("TestRun", output)
	if len(items) == 0 || items[0].Course == "" {
		t.Fatalf("the deadlock explanation names no exercise: %+v", items)
	}
	old := items[0].Course
	renamed, ok := course.Renamed(old)
	if !ok {
		t.Fatalf("%s is not an old slug", old)
	}

	first := coursetest.Load(t, coursetest.First())
	kept := explain.ExplainTestOutput("TestRun", output)
	prepare(first, nil, kept)
	if kept[0].Course != old {
		t.Errorf("with the first course the exercise is %q, want %q", kept[0].Course, old)
	}

	fifty := coursetest.Load(t, coursetest.Fifty())
	ex, _ := fifty.Exercise(renamed)
	prepare(fifty, ex, items)
	if items[0].Course != renamed || strings.Contains(items[0].HTML, old) || !strings.Contains(items[0].HTML, "/learn/"+renamed) {
		t.Errorf("with the course of 50: course %q\n%s", items[0].Course, items[0].HTML)
	}
}
