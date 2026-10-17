package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const goodToken = "GPR_TOKEN=0123456789abcdef0123456789abcdef"

func TestLoadConfigDefaults(t *testing.T) {
	got, err := loadConfig([]string{goodToken, "PATH=/usr/bin", "GPW_ADDR=:1"})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := config{
		Addr:          ":8081",
		MetricsAddr:   ":9091",
		Token:         "0123456789abcdef0123456789abcdef",
		CourseDir:     "/course",
		WorkDir:       "/tmp/runs",
		GoCache:       "/cache/go-build",
		GoCacheSeed:   "/opt/gocache-seed",
		MaxConcurrent: 2,
		MaxQueue:      8,
		TestTimeout:   45 * time.Second,
		CheckTimeout:  20 * time.Second,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	got, err := loadConfig([]string{
		goodToken,
		"GPR_ADDR=127.0.0.1:1", "GPR_METRICS_ADDR=127.0.0.1:2",
		"GPR_COURSE_DIR=/c", "GPR_WORK_DIR=/w", "GPR_GOCACHE=/g", "GPR_GOCACHE_SEED=",
		"GPR_MAX_CONCURRENT=4", "GPR_MAX_QUEUE=0", "GPR_TEST_TIMEOUT=1m", "GPR_CHECK_TIMEOUT=500ms",
	})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.Addr != "127.0.0.1:1" || got.MetricsAddr != "127.0.0.1:2" || got.CourseDir != "/c" ||
		got.WorkDir != "/w" || got.GoCache != "/g" || got.GoCacheSeed != "" ||
		got.MaxConcurrent != 4 || got.MaxQueue != 0 ||
		got.TestTimeout != time.Minute || got.CheckTimeout != 500*time.Millisecond {
		t.Errorf("config = %+v", got)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		wantErr string
	}{
		{name: "no token", env: nil, wantErr: "GPR_TOKEN"},
		{name: "short token", env: []string{"GPR_TOKEN=REPLACE_ME"}, wantErr: "GPR_TOKEN"},
		{name: "unknown variable", env: []string{goodToken, "GPR_TIMEOUT=1s"}, wantErr: "GPR_TIMEOUT: unknown variable"},
		{name: "bad integer", env: []string{goodToken, "GPR_MAX_QUEUE=many"}, wantErr: "GPR_MAX_QUEUE"},
		{name: "no slots", env: []string{goodToken, "GPR_MAX_CONCURRENT=0"}, wantErr: "GPR_MAX_CONCURRENT"},
		{name: "negative queue", env: []string{goodToken, "GPR_MAX_QUEUE=-1"}, wantErr: "GPR_MAX_QUEUE"},
		{name: "bad duration", env: []string{goodToken, "GPR_TEST_TIMEOUT=45"}, wantErr: "GPR_TEST_TIMEOUT"},
		{name: "zero duration", env: []string{goodToken, "GPR_CHECK_TIMEOUT=0s"}, wantErr: "GPR_CHECK_TIMEOUT"},
		{name: "relative cache", env: []string{goodToken, "GPR_GOCACHE=cache"}, wantErr: "GPR_GOCACHE"},
		{name: "relative work dir", env: []string{goodToken, "GPR_WORK_DIR=runs"}, wantErr: "GPR_WORK_DIR"},
		{name: "bad address", env: []string{goodToken, "GPR_ADDR=8081"}, wantErr: "GPR_ADDR"},
		{name: "same addresses", env: []string{goodToken, "GPR_METRICS_ADDR=:8081"}, wantErr: "GPR_METRICS_ADDR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(tt.env)
			if err == nil {
				t.Fatal("loadConfig succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "0123456789abcdef") {
				t.Errorf("error %q shows the token", err)
			}
		})
	}
}

func TestLoadConfigReportsAllProblems(t *testing.T) {
	_, err := loadConfig([]string{"GPR_MAX_QUEUE=x", "GPR_TEST_TIMEOUT=y"})
	if err == nil {
		t.Fatal("loadConfig succeeded, want an error")
	}
	for _, key := range []string{"GPR_TOKEN", "GPR_MAX_QUEUE", "GPR_TEST_TIMEOUT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
