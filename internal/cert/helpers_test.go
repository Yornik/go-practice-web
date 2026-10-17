package cert

import (
	"sync"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
)

var (
	courseOnce sync.Once
	theCourse  *course.Course
	courseErr  error
)

func loadCourse(t *testing.T) *course.Course {
	t.Helper()
	courseOnce.Do(func() { theCourse, courseErr = course.Load("../../course") })
	if courseErr != nil {
		t.Fatal(courseErr)
	}
	return theCourse
}
