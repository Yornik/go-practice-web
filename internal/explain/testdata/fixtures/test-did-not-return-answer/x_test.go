package fixture

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStatusAfterReset(t *testing.T) {
	srv := httptest.NewServer(&Status{})
	client := &http.Client{Timeout: 100 * time.Millisecond}
	for _, url := range []string{srv.URL + "/?reset=1", srv.URL + "/"} {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET %s: no answer within %v", url, client.Timeout)
		}
		resp.Body.Close()
	}
}
