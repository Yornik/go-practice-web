package godoc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A server whose index fails to load still answers in JSON, so the editor can show "docs unavailable".
func TestHandlerIndexUnavailable(t *testing.T) {
	h := &handler{load: func() (*Index, error) { return nil, errors.New("broken") }}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/docs/pkg?path=strings", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"code":"docs_unavailable"`) {
		t.Errorf("status %d, body %s; want 503 and docs_unavailable", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "broken") {
		t.Error("the error leaks the internal cause to the browser")
	}
}
