package main

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSiteServerLimits(t *testing.T) {
	srv := siteServer(http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	if srv.ReadHeaderTimeout <= 0 || srv.ReadHeaderTimeout > 30*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want a few seconds (slowloris)", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Errorf("ReadTimeout = %v, IdleTimeout = %v, want both set", srv.ReadTimeout, srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes <= 0 || srv.MaxHeaderBytes > 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want a small limit", srv.MaxHeaderBytes)
	}

	// A request with headers over the limit is refused.
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	big := "GET / HTTP/1.1\r\nHost: x\r\nX-Big: " + strings.Repeat("a", 2*srv.MaxHeaderBytes) + "\r\n\r\n"
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "431") {
		t.Errorf("status line %q, want 431", status)
	}
}
