package runner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// SeedCache fills an empty or missing build cache dir from seed, the
// pre-warmed cache baked into the image, so that the first runs after a pod
// start do not compile the standard library with -race. It does nothing and
// returns false when dir already has entries or seed does not exist.
//
// The copy goes to a sibling directory first and is renamed into place, so
// a runner killed half way leaves no half-seeded cache behind: the next
// start sees an empty dir and seeds again.
func SeedCache(ctx context.Context, dir, seed string) (bool, error) {
	if seed == "" {
		return false, nil
	}
	if ok, err := isDir(seed); err != nil || !ok {
		return false, err
	}
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, fmt.Errorf("read cache dir: %w", err)
	case len(entries) > 0:
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return false, fmt.Errorf("create cache parent dir: %w", err)
	}
	tmp := dir + ".seeding"
	if err := os.RemoveAll(tmp); err != nil {
		return false, fmt.Errorf("remove old seed copy: %w", err)
	}
	if err := copyTree(ctx, seed, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return false, fmt.Errorf("seed cache from %s: %w", seed, err)
	}
	// dir is empty or missing; Remove fails only if something wrote to it
	// in the meantime, and then that content wins.
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		_ = os.RemoveAll(tmp)
		return false, fmt.Errorf("replace cache dir: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return false, fmt.Errorf("move seeded cache into place: %w", err)
	}
	return true, nil
}

// copyTree copies the directory tree src to dst, keeping file and directory
// modes. The build cache is thousands of small files, so the files are
// copied by a few goroutines in parallel; the disk, not 1 goroutine's
// syscalls, should be the limit.
func copyTree(ctx context.Context, src, dst string) error {
	type job struct {
		src, dst string
		mode     fs.FileMode
	}
	jobs := make(chan job)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	for range min(runtime.GOMAXPROCS(0)*2, 16) {
		wg.Go(func() {
			for j := range jobs {
				if err := copyFile(j.src, j.dst, j.mode); err != nil {
					setErr(err)
				}
			}
		})
	}

	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
			// The final mode comes after the walk: a read-only source dir
			// must stay writable until its files are in.
			return nil
		case d.Type().IsRegular():
			jobs <- job{src: path, dst: target, mode: writable(info.Mode().Perm())}
		}
		// Symlinks and other special files have no place in a build cache.
		return nil
	})
	close(jobs)
	wg.Wait()
	if walkErr != nil {
		return walkErr
	}
	if firstErr != nil {
		return firstErr
	}
	return copyDirModes(src, dst)
}

// copyDirModes gives every directory under dst the mode of its source.
func copyDirModes(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		return os.Chmod(filepath.Join(dst, rel), writable(info.Mode().Perm()|0o100))
	})
}
