package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// outputExistsError reports that --output points to an existing file
// while --overwrite was not given. Exit code 7.
type outputExistsError struct{ path string }

func (e outputExistsError) Error() string {
	return fmt.Sprintf("file %q already exists (use --overwrite to replace it)", e.path)
}

// outputSink writes to stdout or to a temp file renamed by commit.
type outputSink struct {
	io.Writer
	commit func() error
	abort  func() error
}

// openOutput returns the lyrics sink. File output goes to a temp file
// in the target directory and is renamed into place by commit.
func openOutput(path string, overwrite bool, fallback io.Writer) (*outputSink, error) {
	if path == "" {
		return &outputSink{
			Writer: fallback,
			commit: func() error { return nil },
			abort:  func() error { return nil },
		}, nil
	}
	if !overwrite {
		if _, err := os.Lstat(path); err == nil {
			return nil, outputExistsError{path}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return nil, fmt.Errorf("create output for %q: %w", path, err)
	}
	tmp := f.Name()
	mode := os.FileMode(0644)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return nil, err
	}
	done := false
	commit := func() error {
		if done {
			return nil
		}
		done = true
		if err := f.Sync(); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if !overwrite {
			if _, err := os.Lstat(path); err == nil {
				_ = os.Remove(tmp)
				return outputExistsError{path}
			} else if !errors.Is(err, fs.ErrNotExist) {
				_ = os.Remove(tmp)
				return err
			}
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	}
	abort := func() error {
		if done {
			return nil
		}
		done = true
		cerr := f.Close()
		rerr := os.Remove(tmp)
		if errors.Is(rerr, fs.ErrNotExist) {
			rerr = nil
		}
		return errors.Join(cerr, rerr)
	}
	return &outputSink{Writer: f, commit: commit, abort: abort}, nil
}
