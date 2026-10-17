package auth

import "testing"

func TestDeviceName(t *testing.T) {
	tests := []struct{ ua, want string }{
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36", "Chrome on Linux"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15", "Safari on macOS"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1", "Safari on iPhone"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0", "Edge on Windows"},
		{"Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36", "Chrome on Android"},
		{"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0", "Firefox on Linux"},
		{"curl/8.5.0", "Passkey"},
		{"", "Passkey"},
	}
	for _, tt := range tests {
		if got := deviceName(tt.ua); got != tt.want {
			t.Errorf("deviceName(%q) = %q, want %q", tt.ua, got, tt.want)
		}
	}
}

func TestHashToken(t *testing.T) {
	token, hash := newToken()
	if len(token) != 43 {
		t.Fatalf("token %q has %d characters", token, len(token))
	}
	got, ok := hashToken(token)
	if !ok || string(got) != string(hash) {
		t.Fatal("hashToken does not match newToken")
	}
	for _, bad := range []string{"", "short", token + "x", token[:42] + "!", token[:42] + "="} {
		if _, ok := hashToken(bad); ok {
			t.Errorf("hashToken(%q) accepted", bad)
		}
	}
}
