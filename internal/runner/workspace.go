package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// workspace is the per-request directory tree:
//
//	<root>/mod/go.mod                    the course go.mod
//	<root>/mod/exercises/<ex>/           tests, testdata and the learner's files
//	<root>/home, <root>/gopath, <root>/tmp
//
// The module root mirrors the course layout, so the black-box tests import
// the package under its real path. HOME, GOPATH and TMPDIR live next to the
// module, not in it, so that nothing the go command or a test writes there
// becomes part of the package, and removing root removes all of it.
type workspace struct {
	root   string
	modDir string
	pkgDir string
	home   string
	gopath string
	tmp    string
}

// workspacePrefix starts the name of every directory the runner creates in
// the work dir, so that a cleanup never touches anything else there.
const workspacePrefix = "run-"

// newWorkspace creates a fresh workspace under workDir for ex with the
// learner's files. The caller must Close it.
func newWorkspace(workDir string, c *Course, ex *Exercise, files map[string]string) (_ *workspace, err error) {
	ws, err := newScratch(workDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = ws.Close()
		}
	}()

	ws.modDir = filepath.Join(ws.root, "mod")
	ws.pkgDir = filepath.Join(ws.modDir, "exercises", ex.Name)
	if err := os.MkdirAll(ws.pkgDir, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	if err := c.copyCourseFiles(ex, ws.modDir, ws.pkgDir); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	for name, content := range files {
		// name was checked against ex.Stubs, so it is a plain file name.
		if err := os.WriteFile(filepath.Join(ws.pkgDir, name), []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}
	return ws, nil
}

// telemetryModeFile is where the go command reads its telemetry mode,
// relative to HOME. GOTELEMETRY in the environment is read-only: in the
// default "local" mode go starts a telemetry child in its own session,
// which escapes the process group kill and writes into HOME after the
// workspace is gone. The mode file is the only switch.
var telemetryModeFile = filepath.Join(".config", "go", "telemetry", "mode")

// newScratch creates a workspace with HOME, GOPATH and TMPDIR but no
// module; newWorkspace adds the module. modDir is the root until then.
func newScratch(workDir string) (_ *workspace, err error) {
	root, err := os.MkdirTemp(workDir, workspacePrefix)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	ws := &workspace{
		root:   root,
		modDir: root,
		home:   filepath.Join(root, "home"),
		gopath: filepath.Join(root, "gopath"),
		tmp:    filepath.Join(root, "tmp"),
	}
	defer func() {
		if err != nil {
			_ = ws.Close()
		}
	}()
	mode := filepath.Join(ws.home, telemetryModeFile)
	for _, dir := range []string{filepath.Dir(mode), ws.gopath, ws.tmp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create workspace: %w", err)
		}
	}
	if err := os.WriteFile(mode, []byte("off"), 0o600); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	return ws, nil
}

// Close removes the workspace.
func (ws *workspace) Close() error {
	return removeAll(ws.root)
}

// RemoveStaleWorkspaces removes the workspaces a previous runner process
// left in workDir. It touches only names the runner creates, so a work dir
// set to a directory with other content loses nothing else.
func RemoveStaleWorkspaces(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read work dir: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), workspacePrefix) {
			errs = append(errs, removeAll(filepath.Join(workDir, e.Name())))
		}
	}
	return errors.Join(errs...)
}
