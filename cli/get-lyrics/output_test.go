//go:build test

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dirNames lists the entries of dir in name order.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %q: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// requireNoTempFiles fails when a temp file is left in dir.
func requireNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	for _, name := range dirNames(t, dir) {
		if strings.Contains(name, ".tmp-") {
			t.Errorf("leftover temp file %q in %q", name, dir)
		}
	}
}

// seedFile writes content to path with mode.
func seedFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("seed %q: %v", path, err)
	}
}

// requireContent fails unless path holds want.
func requireContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != want {
		t.Errorf("content = %q, want %q", content, want)
	}
}

// requireMode fails unless path has want permissions; Windows is skipped.
func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if fi.Mode().Perm() != want {
		t.Errorf("mode = %o, want %o", fi.Mode().Perm(), want)
	}
}

// TestOutput locks file output, overwrite, failure cleanup and JSON sinks.
func TestOutput(t *testing.T) {
	t.Run("writes file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lyrics.txt")
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-plain", "--output", path)...)
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0 and empty",
				code, stdout, stderr)
		}
		requireContent(t, path, mockPlainLyrics)
		requireMode(t, path, 0o644)
		requireNoTempFiles(t, dir)
		if names := dirNames(t, dir); len(names) != 1 ||
			names[0] != "lyrics.txt" {
			t.Errorf("directory entries = %v, want [lyrics.txt]", names)
		}
	})

	t.Run("refuses to overwrite", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lyrics.txt")
		seedFile(t, path, "original", 0o644)
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-plain", "--output", path)...)
		if code != 7 || stdout != "" {
			t.Errorf("code/stdout = %d/%q, want 7 and empty", code, stdout)
		}
		if !strings.Contains(stderr, "error[output]") ||
			!strings.Contains(stderr, "already exists") {
			t.Errorf("stderr = %q, want an output-exists error", stderr)
		}
		requireContent(t, path, "original")
		requireNoTempFiles(t, dir)
	})

	t.Run("overwrite preserves mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lyrics.txt")
		seedFile(t, path, "original", 0o600)
		code, _, stderr := run(t, nil,
			stdArgs("mock-plain", "--output", path, "--overwrite")...)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr)
		}
		requireContent(t, path, mockPlainLyrics)
		requireMode(t, path, 0o600)
		requireNoTempFiles(t, dir)
	})

	for _, tc := range []struct {
		name string
		seed string
	}{
		{"leaves no file", ""},
		{"keeps existing file", "original"},
	} {
		t.Run("fetch failure "+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lyrics.txt")
			argv := stdArgs("mock-fail", "--output", path)
			if tc.seed != "" {
				seedFile(t, path, tc.seed, 0o644)
				argv = append(argv, "--overwrite")
			}
			code, stdout, stderr := run(t, nil, argv...)
			if code != 4 || stdout != "" {
				t.Errorf("code/stdout = %d/%q, want 4 and empty", code, stdout)
			}
			if !strings.Contains(stderr, "warning[fetch]") {
				t.Errorf("stderr = %q, want warning[fetch]", stderr)
			}
			if tc.seed != "" {
				requireContent(t, path, tc.seed)
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("output file exists after a failed fetch (err %v)", err)
			}
			requireNoTempFiles(t, dir)
		})
	}

	t.Run("missing parent directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "missing", "lyrics.txt")
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-plain", "--output", path)...)
		if code != 5 || stdout != "" {
			t.Errorf("code/stdout = %d/%q, want 5 and empty", code, stdout)
		}
		if !strings.Contains(stderr, "error[output]") {
			t.Errorf("stderr = %q, want error[output]", stderr)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("output file exists despite a missing parent (err %v)", err)
		}
		if names := dirNames(t, dir); len(names) != 0 {
			t.Errorf("directory entries = %v, want none", names)
		}
	})

	for _, tc := range []struct {
		name      string
		overwrite bool
		wantCode  int
	}{
		{"without overwrite", false, 7},
		{"with overwrite", true, 5},
	} {
		t.Run("target is a directory "+tc.name, func(t *testing.T) {
			if tc.overwrite && runtime.GOOS == "windows" {
				t.Skip("rename onto a directory differs on Windows")
			}
			dir := t.TempDir()
			target := filepath.Join(dir, "adir")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatalf("create directory: %v", err)
			}
			argv := stdArgs("mock-plain", "--output", target)
			if tc.overwrite {
				argv = append(argv, "--overwrite")
			}
			code, _, stderr := run(t, nil, argv...)
			if code != tc.wantCode ||
				!strings.Contains(stderr, "error[output]") {
				t.Errorf("code/stderr = %d/%q, want %d and error[output]",
					code, stderr, tc.wantCode)
			}
			if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
				t.Errorf("target is no longer a directory (err %v)", err)
			}
			requireNoTempFiles(t, dir)
		})
	}

	t.Run("json output", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lyrics.json")
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-plain", "--json", "--output", path)...)
		if code != 0 || stdout != "" {
			t.Fatalf("code/stdout = %d/%q, want 0 and empty (stderr %q)",
				code, stdout, stderr)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if !strings.HasSuffix(string(data), "\n") {
			t.Errorf("output = %q, want a trailing newline", data)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(data, &keys); err != nil {
			t.Fatalf("output is not JSON: %v", err)
		}
		if string(keys["FormatVersion"]) != "3" {
			t.Errorf("FormatVersion = %s, want 3", keys["FormatVersion"])
		}
		requireNoTempFiles(t, dir)
	})
}
