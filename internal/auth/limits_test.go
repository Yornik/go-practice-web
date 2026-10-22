package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
)

func TestLimiter(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	l := auth.NewLimiter(3, time.Minute, clk.Now)
	steps := []struct {
		advance time.Duration
		key     string
		ok      bool
		retry   time.Duration
	}{
		{0, "a", true, 0},
		{0, "a", true, 0},
		{0, "a", true, 0},
		{0, "a", false, 20 * time.Second}, // 1 token per 20 s
		{0, "b", true, 0},                 // keys are separate
		{10 * time.Second, "a", false, 10 * time.Second},
		{10 * time.Second, "a", true, 0},
		{0, "a", false, 20 * time.Second},
		{time.Hour, "a", true, 0}, // refills to the burst, not beyond
		{0, "a", true, 0},
		{0, "a", true, 0},
		{0, "a", false, 20 * time.Second},
	}
	for i, s := range steps {
		clk.Advance(s.advance)
		ok, retry := l.Allow(s.key)
		if ok != s.ok || retry != s.retry {
			t.Fatalf("step %d: Allow(%s) = %v, %v; want %v, %v", i, s.key, ok, retry, s.ok, s.retry)
		}
	}
}

func TestClientIP(t *testing.T) {
	trusted, err := auth.ParseTrustedProxies("10.244.0.0/16, 192.168.1.1/32")
	if err != nil {
		t.Fatal(err)
	}
	r := auth.NewIPResolver(trusted)
	tests := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{"direct client", "203.0.113.7:5000", nil, "203.0.113.7"},
		{"untrusted peer ignores the header", "203.0.113.7:5000", []string{"198.51.100.1"}, "203.0.113.7"},
		{"trusted proxy", "10.244.3.4:80", []string{"198.51.100.1"}, "198.51.100.1"},
		{"forged left entries", "10.244.3.4:80", []string{"1.1.1.1, 198.51.100.1"}, "198.51.100.1"},
		{"chain of proxies", "10.244.3.4:80", []string{"198.51.100.1, 192.168.1.1"}, "198.51.100.1"},
		{"several headers", "10.244.3.4:80", []string{"1.1.1.1", "198.51.100.1"}, "198.51.100.1"},
		{"garbage hop", "10.244.3.4:80", []string{"nonsense, 10.244.0.9"}, "10.244.0.9"},
		{"only proxies", "10.244.3.4:80", []string{"10.244.0.9"}, "10.244.0.9"},
		{"no header from proxy", "10.244.3.4:80", nil, "10.244.3.4"},
		{"IPv6 client", "[2001:db8::1]:443", nil, "2001:db8::1"},
		{"IPv4-mapped proxy", "[::ffff:10.244.1.1]:80", []string{"198.51.100.1"}, "198.51.100.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := r.ClientIP(req); got != tt.want {
				t.Fatalf("ClientIP = %q, want %q", got, tt.want)
			}
		})
	}
	if _, err := auth.ParseTrustedProxies("10.0.0.0/8,banana"); err == nil {
		t.Fatal("ParseTrustedProxies accepted banana")
	}
	if p, err := auth.ParseTrustedProxies(""); err != nil || len(p) != 0 {
		t.Fatalf("empty = %v, %v", p, err)
	}
}

func TestRateKey(t *testing.T) {
	tests := []struct{ ip, want string }{
		{"192.0.2.1", "192.0.2.1"},
		{"::ffff:192.0.2.1", "192.0.2.1"},
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::ffff", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"fe80::1%eth0", "fe80::/64"},
		{"not an address", "not an address"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := auth.RateKey(tt.ip); got != tt.want {
			t.Errorf("RateKey(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestWaitText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                       "1 second",
		400 * time.Millisecond:  "1 second",
		time.Second:             "1 second",
		1500 * time.Millisecond: "2 seconds",
		59 * time.Second:        "59 seconds",
		time.Minute:             "1 minute",
		61 * time.Second:        "2 minutes",
		6 * time.Minute:         "6 minutes",
	} {
		if got := auth.WaitText(d); got != want {
			t.Errorf("WaitText(%v) = %q, want %q", d, got, want)
		}
	}
}
