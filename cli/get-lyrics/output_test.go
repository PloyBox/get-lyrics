package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_WritesLyricsToFileWhenOutputSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lyrics.txt")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--output", path, "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if got := string(b); !strings.Contains(got, "TEST_SONG") || !strings.Contains(got, "lyrics") {
		t.Fatalf("file content = %q; want lyrics", got)
	}
}

// TestRun_ExistingOutputFileRefusedWithoutOverwrite locks the
// no-overwrite semantics: --output pointing at an existing file exits
// with code 7 before any fetch runs, leaves the file untouched, and
// hints at --overwrite on stderr.
func TestRun_ExistingOutputFileRefusedWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	const original = "keep me\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--output", path, "TEST_SONG"}, &stdout, &stderr)
	if code != exitFileExists {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFileExists, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "error[output]") {
		t.Fatalf("stderr missing output error: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--overwrite") {
		t.Fatalf("stderr missing --overwrite hint: %q", stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != original {
		t.Fatalf("file content = %q; want %q", string(got), original)
	}
}

// TestRun_FailedRunLeavesOutputFileAlone checks that a failed run keeps an
// existing file intact and leaves no newly created file behind.
func TestRun_FailedRunLeavesOutputFileAlone(t *testing.T) {
	const original = "keep me\n"
	argv := func(path string, overwrite bool) []string {
		a := []string{
			"--source", "mock-fail",
			"--author", "X",
			"--output", path,
			"TEST_SONG",
		}
		if overwrite {
			a = append([]string{"--overwrite"}, a...)
		}
		return a
	}

	t.Run("existing file kept", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.txt")
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatalf("write seed file: %v", err)
		}

		var stdout, stderr bytes.Buffer
		code := Run(argv(path, true), &stdout, &stderr)
		if code != exitFetchFailed {
			t.Fatalf("code = %d; want %d (stderr=%q)",
				code, exitFetchFailed, stderr.String())
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(got) != original {
			t.Fatalf("file content = %q; want %q", string(got), original)
		}
	})

	t.Run("no file created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.txt")

		var stdout, stderr bytes.Buffer
		code := Run(argv(path, false), &stdout, &stderr)
		if code != exitFetchFailed {
			t.Fatalf("code = %d; want %d (stderr=%q)",
				code, exitFetchFailed, stderr.String())
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("output file %q exists after failed run (err=%v)", path, err)
		}
	})
}

func TestRun_OutputFileTruncatedOnlyOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("this is old content longer than new"), 0644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--overwrite", "--output", path, "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.Contains(string(got), "this is old content") {
		t.Fatalf("file content = %q; want the stale longer content truncated", string(got))
	}
	if got := string(got); !strings.Contains(got, "TEST_SONG") || !strings.Contains(got, "lyrics") {
		t.Fatalf("file content = %q; want lyrics", got)
	}
}
