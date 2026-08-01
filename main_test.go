package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_MissingSongExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "song title is required") {
		t.Fatalf("stderr missing song-required message: %q", stderr.String())
	}
}

func TestRun_HelpFlagExitsZeroAndListsSources(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout missing Usage header: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "stub") {
		t.Fatalf("stdout missing registered source 'stub': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "lrclib") {
		t.Fatalf("stdout missing registered source 'lrclib': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "lyricsovh") {
		t.Fatalf("stdout missing registered source 'lyricsovh': %q", stdout.String())
	}
}

func TestRun_HelpShortFlagAlsoWorks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-h"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
}

func TestRun_VersionFlagExitsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--version"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), version) {
		t.Fatalf("stdout missing version %q: %q", version, stdout.String())
	}
}

func TestRun_VersionShortFlagAlsoWorks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-v"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Fatalf("stdout missing version %q: %q", version, stdout.String())
	}
}

func TestRun_WritesLyricsToStdoutByDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "stub", "--author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics, got %q", stdout.String())
	}
}

func TestRun_WritesLyricsToFileWhenOutputSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lyrics.txt")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "stub", "--author", "TEST_AUTHOR", "--output", path, "TEST_SONG"}, &stdout, &stderr)
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

// TestRun_WritesUnsupportedParamWarningsToStderr exercises the
// "source does not support parameter" branch via stub: stub advertises
// only ParamAuthor, so passing --iswc (and --album) trips one warning
// each. --author is included so the fetch itself succeeds.
func TestRun_WritesUnsupportedParamWarningsToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{
			"--source", "stub",
			"--author", "TEST_AUTHOR",
			"--album", "TEST_ALBUM",
			"--iswc", "TEST_ISWC",
			"TEST_SONG",
		},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	if !strings.Contains(stdout.String(), "lyrics") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	for _, flag := range []string{"--album", "--iswc"} {
		if !strings.Contains(stderr.String(), flag) {
			t.Fatalf("stderr missing warning for %s: %q", flag, stderr.String())
		}
	}
}

func TestRun_UnknownSourceExitsThree(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "nope", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUnknownSrc {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUnknownSrc, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown source") {
		t.Fatalf("stderr missing 'unknown source': %q", stderr.String())
	}
}

// TestRun_OutputUnwritableExitsFive fails before fetch, so the source's
// author requirement does not matter — we deliberately omit it.
func TestRun_OutputUnwritableExitsFive(t *testing.T) {
	// Point --output at a directory that can't be created.
	bad := "/proc/this/does/not/exist/lyrics.txt"
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "stub", "--output", bad, "TEST_SONG"}, &stdout, &stderr)
	if code != exitOutputFailed {
		t.Fatalf("code = %d; want %d", code, exitOutputFailed)
	}
}

func TestRun_AcceptsSingleDashLongForm(t *testing.T) {
	// Per plan: we do not intercept; flag accepts -source too.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-source", "stub", "-author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

func TestRun_TimestampOnUnsupportedSourceFallsBackToPlain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "stub", "--author", "TEST_AUTHOR", "--timestamp", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	if !strings.Contains(stdout.String(), "lyrics") {
		t.Fatalf("stdout missing fallback plain lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--timestamp") {
		t.Fatalf("stderr missing timestamp warning: %q", stderr.String())
	}
}

// TestRun_SourceRequiresAuthorExitsSix drives the required-parameter
// path through the built-in stub: stub now requires --author, so a
// fetch without it must surface exit code 6 and the canonical stderr
// message.
func TestRun_SourceRequiresAuthorExitsSix(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "stub", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error: source "stub" requires --author`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}
