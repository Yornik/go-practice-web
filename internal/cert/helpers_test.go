package cert

import (
	"fmt"
	"os"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
)

// The tests read 2 fixture courses, not the submodule, so that they hold
// for either course the submodule pins: first is the first course (25
// exercises in 5 tiers), fifty the course of 50 in 10 named tiers.
var first, fifty *course.Course

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cert-courses-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer func() { _ = os.RemoveAll(dir) }()
		for _, f := range []struct {
			c    **course.Course
			name string
			spec coursetest.Spec
		}{{&first, "first", coursetest.First()}, {&fifty, "fifty", coursetest.Fifty()}} {
			d := dir + "/" + f.name
			if err := coursetest.Write(d, f.spec); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if *f.c, err = course.Load(d); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		return m.Run()
	}()
	os.Exit(code)
}

// loadCourse is the first course.
func loadCourse(t *testing.T) *course.Course {
	t.Helper()
	return first
}
