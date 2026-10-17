package main

import (
	"strings"
	"testing"
)

const token = "0123456789abcdef0123456789abcdef"

func TestLoadConfigDefaults(t *testing.T) {
	c, err := loadConfig([]string{
		"GPW_PUBLIC_URL=https://go-learn.party/",
		"GPW_RUNNER_URL=http://runner.go-practice-runner.svc:8081/",
		"GPW_RUNNER_TOKEN=" + token,
		"PATH=/usr/bin", // other variables are not ours
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.MetricsAddr != ":9090" || c.DBPath != "/data/go-practice.db" || c.CourseDir != "/course" {
		t.Fatalf("defaults = %+v", c)
	}
	if c.PublicURL != "https://go-learn.party" || c.RunnerURL != "http://runner.go-practice-runner.svc:8081" {
		t.Fatalf("URLs = %q %q", c.PublicURL, c.RunnerURL)
	}
	if c.SMTP.Port != 465 || c.SMTP.From != "Go Practice <go-practice@yornik.eu>" || c.SMTP.Enabled() {
		t.Fatalf("SMTP = %+v", c.SMTP)
	}
	if len(c.TrustedProxies) != 1 || c.TrustedProxies[0].String() != "10.244.0.0/16" || c.Dev {
		t.Fatalf("proxies %v dev %v", c.TrustedProxies, c.Dev)
	}
}

func TestLoadConfigDev(t *testing.T) {
	c, err := loadConfig([]string{
		"GPW_PUBLIC_URL=http://localhost:8080",
		"GPW_RUNNER_URL=http://127.0.0.1:8081",
		"GPW_RUNNER_TOKEN=" + token,
		"GPW_DEV=true",
		"GPW_TRUSTED_PROXIES=",
		"GPW_SMTP_HOST=smtp.resend.com",
		"GPW_SMTP_PASSWORD=REPLACE_ME",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Dev || c.PublicURL != "http://localhost:8080" || len(c.TrustedProxies) != 0 || c.SMTP.Enabled() {
		t.Fatalf("dev config = %+v", c)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	base := map[string]string{
		"GPW_PUBLIC_URL":   "https://go-learn.party",
		"GPW_RUNNER_URL":   "http://runner:8081",
		"GPW_RUNNER_TOKEN": token,
	}
	tests := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"unknown variable", map[string]string{"GPW_PUBLIC_UR": "x"}, "GPW_PUBLIC_UR: unknown variable"},
		{"no public URL", map[string]string{"GPW_PUBLIC_URL": ""}, "GPW_PUBLIC_URL: required"},
		{"http without dev", map[string]string{"GPW_PUBLIC_URL": "http://go-learn.party"}, "must use https"},
		{"public URL with a path", map[string]string{"GPW_PUBLIC_URL": "https://go-learn.party/app"}, "not an origin"},
		{"public URL with a query", map[string]string{"GPW_PUBLIC_URL": "https://go-learn.party?x=1"}, "not an origin"},
		{"no runner URL", map[string]string{"GPW_RUNNER_URL": ""}, "GPW_RUNNER_URL: required"},
		{"bad runner URL", map[string]string{"GPW_RUNNER_URL": "runner:8081"}, "GPW_RUNNER_URL"},
		{"short token", map[string]string{"GPW_RUNNER_TOKEN": "short"}, "GPW_RUNNER_TOKEN"},
		{"relative database", map[string]string{"GPW_DB_PATH": "data/x.db"}, "GPW_DB_PATH"},
		{"relative course", map[string]string{"GPW_COURSE_DIR": "course"}, "GPW_COURSE_DIR"},
		{"bad address", map[string]string{"GPW_ADDR": "8080"}, "GPW_ADDR"},
		{"same addresses", map[string]string{"GPW_ADDR": ":9090"}, "must differ"},
		{"bad port", map[string]string{"GPW_SMTP_PORT": "smtp"}, "GPW_SMTP_PORT"},
		{"bad sender", map[string]string{"GPW_MAIL_FROM": "nobody"}, "GPW_MAIL_FROM"},
		{"bad proxy", map[string]string{"GPW_TRUSTED_PROXIES": "10.0.0.0/8,banana"}, "GPW_TRUSTED_PROXIES"},
		{"bad dev", map[string]string{"GPW_DEV": "yes please"}, "GPW_DEV"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range base {
				env[k] = v
			}
			for k, v := range tt.set {
				env[k] = v
			}
			environ := make([]string, 0, len(env))
			for k, v := range env {
				environ = append(environ, k+"="+v)
			}
			_, err := loadConfig(environ)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadConfig = %v, want an error with %q", err, tt.want)
			}
			if strings.Contains(err.Error(), token) {
				t.Fatal("the error shows the runner token")
			}
		})
	}
}

func TestLoadConfigReportsAll(t *testing.T) {
	_, err := loadConfig([]string{"GPW_DB_PATH=x"})
	for _, want := range []string{"GPW_PUBLIC_URL", "GPW_RUNNER_URL", "GPW_RUNNER_TOKEN", "GPW_DB_PATH"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error %v lacks %s", err, want)
		}
	}
}
