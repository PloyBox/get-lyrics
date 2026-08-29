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
	if !strings.Contains(string(b), "TEST_SONG") {
		t.Fatalf("file content = %q", string(b))
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
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("stderr missing already-exists message: %q", stderr.String())
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

// TestRun_OverwriteKeepsExistingContentOnFetchFailure: with
// --overwrite, an existing output file is still only truncated after a
// successful fetch — every failed run leaves the original content
// intact, regardless of which failure path exits (unknown source, fetch
// failure, missing required parameter).
func TestRun_OverwriteKeepsExistingContentOnFetchFailure(t *testing.T) {
	cases := []struct {
		name string
		argv func(path string) []string
		want int
	}{
		{
			"unknown source",
			func(path string) []string {
				return []string{"--source", "nope", "--overwrite", "--output", path, "TEST_SONG"}
			},
			exitUnknownSrc,
		},
		{
			"fetch failure",
			func(path string) []string {
				return []string{"--source", "mock-fail", "--author", "X", "--overwrite", "--output", path, "TEST_SONG"}
			},
			exitFetchFailed,
		},
		{
			"required param missing",
			func(path string) []string {
				return []string{"--source", "mock-require", "--overwrite", "--output", path, "TEST_SONG"}
			},
			exitRequired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.txt")
			const original = "keep me\n"
			if err := os.WriteFile(path, []byte(original), 0644); err != nil {
				t.Fatalf("write seed file: %v", err)
			}

			var stdout, stderr bytes.Buffer
			code := Run(tc.argv(path), &stdout, &stderr)
			if code != tc.want {
				t.Fatalf("code = %d; want %d (stderr=%q)", code, tc.want, stderr.String())
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			if string(got) != original {
				t.Fatalf("file content = %q; want %q", string(got), original)
			}
		})
	}
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
	want := "[mock-success] lyrics for: TEST_SONG\n"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("file content = %q; want %q", string(got), want)
	}
}

// TestRun_FailedFetchDoesNotCreateOutputFile is the defect regression:
// a failed run must not leave a freshly created empty --output file
// behind, regardless of which failure path exits (unknown source,
// fetch failure, missing required parameter).
func TestRun_FailedFetchDoesNotCreateOutputFile(t *testing.T) {
	cases := []struct {
		name string
		argv func(path string) []string
		want int
	}{
		{
			"unknown source",
			func(path string) []string { return []string{"--source", "nope", "--output", path, "TEST_SONG"} },
			exitUnknownSrc,
		},
		{
			"fetch failure",
			func(path string) []string {
				return []string{"--source", "mock-fail", "--author", "X", "--output", path, "TEST_SONG"}
			},
			exitFetchFailed,
		},
		{
			"required param missing",
			func(path string) []string { return []string{"--source", "mock-require", "--output", path, "TEST_SONG"} },
			exitRequired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.txt")

			var stdout, stderr bytes.Buffer
			code := Run(tc.argv(path), &stdout, &stderr)
			if code != tc.want {
				t.Fatalf("code = %d; want %d (stderr=%q)", code, tc.want, stderr.String())
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("output file %q exists after failed run (err=%v)", path, err)
			}
		})
	}
}

// TestRun_OverwriteShortFlagWorks proves the -O short form of
// --overwrite is accepted.
func TestRun_OverwriteShortFlagWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "-O", "--output", path, "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	want := "[mock-success] lyrics for: TEST_SONG\n"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("file content = %q; want %q", string(got), want)
	}
}
