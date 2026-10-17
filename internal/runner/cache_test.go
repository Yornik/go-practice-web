package runner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestSeedCache(t *testing.T) {
	seed := t.TempDir()
	writeTestFile(t, filepath.Join(seed, "README"), "cache")
	writeTestFile(t, filepath.Join(seed, "0a/0a1b-a"), "entry")
	writeTestFile(t, filepath.Join(seed, "0a/0a1b-d"), "output")
	if err := os.Chmod(filepath.Join(seed, "0a/0a1b-d"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(seed, "0a"), 0o750); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		prepare    func(t *testing.T, dir string)
		seed       string
		wantSeeded bool
	}{
		{name: "missing dir and parent", prepare: func(*testing.T, string) {}, seed: seed, wantSeeded: true},
		{name: "empty dir", prepare: func(t *testing.T, dir string) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}, seed: seed, wantSeeded: true},
		{name: "dir with entries", prepare: func(t *testing.T, dir string) {
			writeTestFile(t, filepath.Join(dir, "own"), "x")
		}, seed: seed, wantSeeded: false},
		{name: "no seed", prepare: func(*testing.T, string) {}, seed: filepath.Join(seed, "missing"), wantSeeded: false},
		{name: "seeding off", prepare: func(*testing.T, string) {}, seed: "", wantSeeded: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "cache", "go-build")
			tt.prepare(t, dir)
			seeded, err := SeedCache(context.Background(), dir, tt.seed)
			if err != nil {
				t.Fatalf("SeedCache: %v", err)
			}
			if seeded != tt.wantSeeded {
				t.Fatalf("seeded = %v, want %v", seeded, tt.wantSeeded)
			}
			if !tt.wantSeeded {
				return
			}
			assertMode(t, filepath.Join(dir, "0a/0a1b-d"), 0o640)
			assertMode(t, filepath.Join(dir, "0a"), 0o750|fs.ModeDir)
			got, err := os.ReadFile(filepath.Join(dir, "0a/0a1b-a"))
			if err != nil || string(got) != "entry" {
				t.Errorf("copied entry = %q, %v", got, err)
			}
			if _, err := os.Stat(dir + ".seeding"); !os.IsNotExist(err) {
				t.Errorf("temporary copy left behind: %v", err)
			}
		})
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode() & (fs.ModePerm | fs.ModeDir); got != want {
		t.Errorf("mode of %s = %v, want %v", filepath.Base(path), got, want)
	}
}
