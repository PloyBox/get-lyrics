package main

import (
	"fmt"
	"io"
	"os"
)

// outputExistsError reports that --output points to an existing file
// while --overwrite was not given. Run maps it to exit code 7.
type outputExistsError struct{ path string }

func (e outputExistsError) Error() string {
	return fmt.Sprintf("file %q already exists (use --overwrite to replace it)", e.path)
}

// openOutput returns the lyrics sink: stdout when path is empty, an
// os.File when path is set. A new file is created exclusively
// (O_CREATE|O_EXCL) and reported via created so the caller can remove
// it on failure. An existing file is only opened when overwrite is set,
// and never with O_TRUNC — truncation happens only after a successful
// fetch, so a failed run leaves existing content intact. The caller
// must invoke the closer.
func openOutput(path string, overwrite bool, fallback io.Writer) (io.Writer, func() error, bool, error) {
	if path == "" {
		return fallback, func() error { return nil }, false, nil
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		// WARNING: O_EXCL is not completely safe; file races cannot be
		// fully eliminated.
		if err == nil {
			// Created by this process: on failure the caller removes it.
			return f, f.Close, true, nil
		}
		if !os.IsExist(err) {
			return nil, func() error { return nil }, false, err
		}
		if !overwrite {
			return nil, func() error { return nil }, false, outputExistsError{path}
		}
		// The file exists; reopen it without O_CREATE and without
		// O_TRUNC (the caller truncates only after a successful fetch).
		f, err = os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			return f, f.Close, false, nil
		}
		lastErr = err
		if !os.IsNotExist(err) {
			return nil, func() error { return nil }, false, err
		}
		// The file vanished between the two opens; retry the exclusive
		// create.
	}
	return nil, func() error { return nil }, false, lastErr
}
