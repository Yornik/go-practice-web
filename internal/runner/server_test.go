package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestAuthorization(t *testing.T) {
	env := newTestEnv(t)
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{name: "no header", header: "", want: http.StatusUnauthorized},
		{name: "wrong token", header: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "token prefix", header: "Bearer " + testToken[:10], want: http.StatusUnauthorized},
		{name: "token with suffix", header: "Bearer " + testToken + "x", want: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic " + testToken, want: http.StatusUnauthorized},
		{name: "no scheme", header: testToken, want: http.StatusUnauthorized},
		// The right token gets past the check to the body validation.
		{name: "right token", header: "Bearer " + testToken, want: http.StatusBadRequest},
		{name: "scheme in lower case", header: "bearer " + testToken, want: http.StatusBadRequest},
	}
	for _, path := range []string{runnerapi.PathCheck, runnerapi.PathFormat, runnerapi.PathTest} {
		for _, tt := range tests {
			t.Run(path+"/"+tt.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.url+path, strings.NewReader("{}"))
				if err != nil {
					t.Fatal(err)
				}
				if tt.header != "" {
					req.Header.Set("Authorization", tt.header)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != tt.want {
					t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
				}
				if tt.want == http.StatusUnauthorized && len(body) != 0 {
					t.Errorf("401 with a body: %q", body)
				}
			})
		}
	}
}

func TestHealthAndReadiness(t *testing.T) {
	env := newTestEnv(t)
	get := func(path string) int {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.url+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := get("/healthz"); got != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", got)
	}
	if got := get("/readyz"); got != http.StatusOK {
		t.Errorf("/readyz = %d, want 200", got)
	}
	// go version runs in a scratch HOME; nothing of it may stay, not even
	// go's telemetry files.
	assertWorkDirEmpty(t, env.workDir)

	env.srv.Drain()
	if got := get("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("/readyz while draining = %d, want 503", got)
	}
	var apiErr runnerapi.Error
	env.postJSON(t, runnerapi.PathFormat, runnerapi.FormatRequest{Exercise: "26-logfmt", File: "logfmt.go"},
		&apiErr, http.StatusServiceUnavailable)
	if apiErr.Code != "shutting_down" {
		t.Errorf("code = %q, want shutting_down", apiErr.Code)
	}
}

func TestRequestValidation(t *testing.T) {
	env := newTestEnv(t)
	stub := stubFiles(t, "26-logfmt")
	with := func(name, content string) map[string]string {
		files := map[string]string{"logfmt.go": stub["logfmt.go"]}
		files[name] = content
		return files
	}
	big := strings.Repeat("x", runnerapi.MaxFileBytes+1)

	tests := []struct {
		name       string
		body       any
		wantStatus int
		wantCode   string
	}{
		{name: "unknown exercise", body: runnerapi.CheckRequest{Exercise: "99-nope", Files: stub}, wantCode: "unknown_exercise"},
		{name: "path as exercise", body: runnerapi.CheckRequest{Exercise: "../26-logfmt", Files: stub}, wantCode: "unknown_exercise"},
		{name: "parent traversal", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("../go.mod", "module x")}, wantCode: "bad_file"},
		{name: "deep traversal", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("../../../../etc/passwd", "x")}, wantCode: "bad_file"},
		{name: "absolute path", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("/etc/passwd", "x")}, wantCode: "bad_file"},
		{name: "subdirectory", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("sub/logfmt.go", "x")}, wantCode: "bad_file"},
		{name: "test replacement", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("logfmt_test.go", "package logfmt_test")}, wantCode: "bad_file"},
		{name: "new test file", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("extra_test.go", "package logfmt")}, wantCode: "bad_file"},
		{name: "other exercise's stub", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("retry.go", "package logfmt")}, wantCode: "bad_file"},
		{name: "go.mod", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("go.mod", "module x")}, wantCode: "bad_file"},
		{name: "NUL in name", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: with("logfmt.go\x00", "x")}, wantCode: "bad_file"},
		{name: "missing stub", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: map[string]string{}}, wantCode: "missing_file"},
		{name: "oversized file", body: runnerapi.CheckRequest{Exercise: "26-logfmt", Files: map[string]string{"logfmt.go": big}}, wantCode: "file_too_large"},
		{name: "bad JSON", body: `{"exercise": `, wantCode: "bad_json"},
		{name: "body over the limit", body: `{"x":"` + strings.Repeat("a", maxBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge, wantCode: "too_large"},
	}
	for _, path := range []string{runnerapi.PathCheck, runnerapi.PathTest} {
		for _, tt := range tests {
			t.Run(path+"/"+tt.name, func(t *testing.T) {
				want := tt.wantStatus
				if want == 0 {
					want = http.StatusBadRequest
				}
				var apiErr runnerapi.Error
				env.postJSON(t, path, tt.body, &apiErr, want)
				if apiErr.Code != tt.wantCode || apiErr.Error == "" {
					t.Errorf("error = %+v, want code %q and a message", apiErr, tt.wantCode)
				}
			})
		}
	}

	t.Run("37-workerpool needs both stubs", func(t *testing.T) {
		files := stubFiles(t, "37-workerpool")
		delete(files, "group.go")
		var apiErr runnerapi.Error
		env.postJSON(t, runnerapi.PathCheck, runnerapi.CheckRequest{Exercise: "37-workerpool", Files: files}, &apiErr, http.StatusBadRequest)
		if apiErr.Code != "missing_file" {
			t.Errorf("code = %q, want missing_file", apiErr.Code)
		}
	})
	t.Run("total over the limit", func(t *testing.T) {
		// 4 files of 255 KiB: each is within the file limit, the sum is not.
		ex := &Exercise{Name: "x", Stubs: []string{"a.go", "b.go", "c.go", "d.go", "e.go"}}
		files := map[string]string{}
		for _, name := range ex.Stubs {
			files[name] = strings.Repeat("x", runnerapi.MaxFileBytes-1024)
		}
		if err := validateFiles(ex, files); err == nil || err.code != "too_large" {
			t.Errorf("validateFiles = %v, want too_large", err)
		}
	})
	t.Run("bad run pattern", func(t *testing.T) {
		var apiErr runnerapi.Error
		req := runnerapi.TestRequest{Exercise: "26-logfmt", Files: stub, Run: "Test("}
		env.postJSON(t, runnerapi.PathTest, req, &apiErr, http.StatusBadRequest)
		if apiErr.Code != "bad_run" {
			t.Errorf("code = %q, want bad_run", apiErr.Code)
		}
	})
	t.Run("format with a bad file", func(t *testing.T) {
		var apiErr runnerapi.Error
		req := runnerapi.FormatRequest{Exercise: "26-logfmt", File: "../logfmt.go", Content: "package logfmt\n"}
		env.postJSON(t, runnerapi.PathFormat, req, &apiErr, http.StatusBadRequest)
		if apiErr.Code != "bad_file" {
			t.Errorf("code = %q, want bad_file", apiErr.Code)
		}
	})
	t.Run("format with an oversized file", func(t *testing.T) {
		var apiErr runnerapi.Error
		req := runnerapi.FormatRequest{Exercise: "26-logfmt", File: "logfmt.go", Content: big}
		env.postJSON(t, runnerapi.PathFormat, req, &apiErr, http.StatusBadRequest)
		if apiErr.Code != "file_too_large" {
			t.Errorf("code = %q, want file_too_large", apiErr.Code)
		}
	})
	if entries, _ := readDirNames(env.workDir); len(entries) != 0 {
		t.Errorf("rejected requests left files in the work dir: %v", entries)
	}
}

func TestFormat(t *testing.T) {
	env := newTestEnv(t)
	tests := []struct {
		name      string
		content   string
		want      string
		wantDiags []runnerapi.Diagnostic
	}{
		{
			name:    "formats",
			content: "package logfmt\nfunc  F( )  int {\nreturn 1}\n",
			want:    "package logfmt\n\nfunc F() int {\n\treturn 1\n}\n",
		},
		{
			name:    "syntax error keeps the content",
			content: "package logfmt\n\nfunc F() int {\n\treturn 1 +\n}\n",
			want:    "package logfmt\n\nfunc F() int {\n\treturn 1 +\n}\n",
			wantDiags: []runnerapi.Diagnostic{{
				File: "logfmt.go", Line: 5, Column: 1, Severity: "error", Source: "gofmt",
				Message: "expected operand, found '}'",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp runnerapi.FormatResponse
			req := runnerapi.FormatRequest{Exercise: "26-logfmt", File: "logfmt.go", Content: tt.content}
			env.postJSON(t, runnerapi.PathFormat, req, &resp, http.StatusOK)
			if resp.Content != tt.want {
				t.Errorf("content:\n%s\nwant:\n%s", resp.Content, tt.want)
			}
			if resp.Diagnostics == nil {
				t.Error("diagnostics is null, want a list")
			}
			if len(resp.Diagnostics) != len(tt.wantDiags) {
				t.Fatalf("diagnostics = %+v, want %+v", resp.Diagnostics, tt.wantDiags)
			}
			for i := range tt.wantDiags {
				if resp.Diagnostics[i] != tt.wantDiags[i] {
					t.Errorf("diagnostic %d = %+v, want %+v", i, resp.Diagnostics[i], tt.wantDiags[i])
				}
			}
		})
	}
}

func TestQueueFull(t *testing.T) {
	env := newTestEnv(t, func(o *Options) { o.MaxConcurrent, o.MaxQueue = 1, 0 })
	held := mustJoin(t, env.srv.limiter)
	defer held.Release()

	stub := stubFiles(t, "26-logfmt")
	for _, tc := range []struct {
		path string
		body any
	}{
		{runnerapi.PathCheck, runnerapi.CheckRequest{Exercise: "26-logfmt", Files: stub}},
		{runnerapi.PathTest, runnerapi.TestRequest{Exercise: "26-logfmt", Files: stub}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			resp := env.post(t, ctx, tc.path, tc.body)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", resp.StatusCode)
			}
			if ra := resp.Header.Get("Retry-After"); ra == "" {
				t.Error("429 without Retry-After")
			}
		})
	}

	// Format runs in process and takes no slot.
	var resp runnerapi.FormatResponse
	env.postJSON(t, runnerapi.PathFormat,
		runnerapi.FormatRequest{Exercise: "26-logfmt", File: "logfmt.go", Content: stub["logfmt.go"]},
		&resp, http.StatusOK)
}
