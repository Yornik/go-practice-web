package web

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWriteDeadlineFreesSlowReaders opens a connection, asks for a large
// response and never reads it: the handler must give up at the deadline
// instead of blocking on a full socket forever.
func TestWriteDeadlineFreesSlowReaders(t *testing.T) {
	done := make(chan error, 1)
	big := bytes.Repeat([]byte("x"), 64<<20)
	h := writeDeadline(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(big)
		done <- err
	}), 300*time.Millisecond)
	srv := httptest.NewServer(h)
	defer srv.Close()

	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("64 MiB were written to a client that reads nothing")
		}
	case <-ctx.Done():
		t.Fatal("the handler is still blocked writing to a client that reads nothing")
	}
}
