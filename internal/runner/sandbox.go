package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// sandbox describes what the processes of a run may touch besides their
// own workspace. On Linux it is enforced with Landlock (sandbox_linux.go):
// learner code runs as the runner's user, so without it a test could read
// the workspaces of other learners' runs, the runner's token in the
// environment of PID 1, or signal the runner. Paths that do not exist are
// left out when a run starts.
type sandbox struct {
	// exec may be read and executed: the toolchain, the C compiler that
	// -race needs, and the system libraries.
	exec []string
	// read may be read only: configuration and the kernel's views of the
	// process, which the Go runtime and the race detector read.
	read []string
	// dev holds the devices (/dev/null, /dev/urandom).
	dev []string
	// write may be read, written and executed: the shared caches. The
	// run's workspace is added per run.
	write []string
}

// systemExecDirs hold the C toolchain and the shared libraries of a
// Debian-like image. Missing ones are skipped.
var systemExecDirs = []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32"}

// newSandbox derives the sandbox of the runner's children from its
// options: the go command's GOROOT, the directories of PATH (go finds gcc
// there) and the caches.
func newSandbox(opts Options) (*sandbox, error) {
	goroot, err := findGOROOT(opts.Go, opts.Path)
	if err != nil {
		return nil, err
	}
	sb := &sandbox{
		exec:  slices.Clone(systemExecDirs),
		read:  []string{"/etc", "/proc", "/sys"},
		dev:   []string{"/dev"},
		write: []string{opts.GoCache},
	}
	sb.exec = append(sb.exec, goroot, binDir(opts.Go))
	if opts.Staticcheck != "" {
		sb.exec = append(sb.exec, binDir(opts.Staticcheck))
		if opts.StaticcheckCache != "" {
			sb.write = append(sb.write, opts.StaticcheckCache)
		}
	}
	for _, dir := range filepath.SplitList(opts.Path) {
		// A relative PATH entry would resolve against the workspace, which
		// is writable anyway.
		if filepath.IsAbs(dir) {
			sb.exec = append(sb.exec, dir)
		}
	}
	slices.Sort(sb.exec)
	sb.exec = slices.Compact(sb.exec)
	return sb, nil
}

// binDir returns the directory that holds the binary at path, with
// symbolic links resolved: Landlock checks the file that is executed, not
// the link to it.
func binDir(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Dir(path)
}

// findGOROOT returns the GOROOT of the go command at goPath. go finds it
// the same way: the parent of the directory that holds the binary, if it
// has pkg/tool. Otherwise the go command knows it from its build, so it is
// asked.
func findGOROOT(goPath, pathEnv string) (string, error) {
	if root := filepath.Dir(binDir(goPath)); isGOROOT(root) {
		return root, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, goPath, "env", "GOROOT")
	cmd.Env = []string{"PATH=" + pathEnv, "GOTOOLCHAIN=local", "HOME=" + os.TempDir()}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("runner: find GOROOT of %s: %w", goPath, err)
	}
	root := strings.TrimSpace(out.String())
	if !filepath.IsAbs(root) {
		return "", errors.New("runner: go env GOROOT printed no absolute path")
	}
	return root, nil
}

func isGOROOT(dir string) bool {
	ok, err := isDir(filepath.Join(dir, "pkg", "tool"))
	return err == nil && ok
}
