package runner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// copyFile copies the regular file src to the new file dst with mode.
func copyFile(src, dst string, mode fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	// A close error after a complete read changes nothing.
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("copy %s: %w", src, cerr)
		}
	}()
	// ReadFrom uses copy_file_range where the kernel offers it.
	if _, err := out.ReadFrom(in); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	// The umask may have narrowed the mode at create time.
	if err := out.Chmod(mode); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return nil
}

// removeAll is os.RemoveAll that also removes directories a test made
// read-only: a learner's test can chmod its temp directories, and the
// workspace must go away regardless.
func removeAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(path)
}

// writable adds the owner's write bit. A seed baked into an image may be
// read-only, but go must be able to add to and trim the copy.
func writable(mode fs.FileMode) fs.FileMode {
	return mode | 0o200
}

// isDir reports whether path is an existing directory. A path that exists
// but is something else is an error: the caller expected a directory there.
func isDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}
	return true, nil
}
