package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/bootstrap"
)

// init registers mock/test-only sources so they are available during tests.
func init() {
	if err := bootstrap.RegisterAllMock(registry); err != nil {
		panic(err)
	}
}

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
	if !strings.Contains(stdout.String(), "mock-success") {
		t.Fatalf("stdout missing registered source 'mock-success': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "lrclib") {
		t.Fatalf("stdout missing registered source 'lrclib': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "lyricsovh") {
		t.Fatalf("stdout missing registered source 'lyricsovh': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "lrccx") {
		t.Fatalf("stdout missing registered source 'lrccx': %q", stdout.String())
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
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics, got %q", stdout.String())
	}
}

// TestRun_JoinsMultiplePositionalArgsAsSong verifies that all positional
// arguments are joined with spaces into a single song title instead of
// only the first one being used.
func TestRun_JoinsMultiplePositionalArgsAsSong(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "Bohemian", "Rhapsody", "by", "Queen"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lyrics for: Bohemian Rhapsody by Queen") {
		t.Fatalf("stdout missing joined song title: %q", stdout.String())
	}
}

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

// TestRun_WritesUnsupportedParamWarningsToStderr exercises the
// "source does not support parameter" branch via mock-success: mock-success
// advertises only ParamAuthor, so passing --iswc (and --album) trips one
// warning each. --author is included so the fetch itself succeeds.
func TestRun_WritesUnsupportedParamWarningsToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{
			"--source", "mock-success",
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

func TestRun_AcceptsSingleDashLongForm(t *testing.T) {
	// Per plan: we do not intercept; flag accepts -source too.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-source", "mock-success", "-author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

func TestRun_TimestampOnUnsupportedSourceFallsBackToPlain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--timestamp", "line", "TEST_SONG"}, &stdout, &stderr)
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
// path through mock-require: mock-require requires --author, so a
// fetch without it must surface exit code 6 and the canonical stderr
// message.
func TestRun_SourceRequiresAuthorExitsSix(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error: source "mock-require" requires --author`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

func TestRun_FetchFailureExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-fail", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mock-fail: intentional fetch failure") {
		t.Fatalf("stderr missing fetch error: %q", stderr.String())
	}
}

func TestRun_MockRequireWithAuthorSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-require", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

func TestRun_MockNosupportNoAuthorNeeded(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

// TestRun_UnknownFlagExitsTwo drives the parseFlags error branch: an
// unrecognized flag makes flag.Parse fail, which Run maps to exit 2.
func TestRun_UnknownFlagExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--bogus", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr missing flag parse error: %q", stderr.String())
	}
}

// TestRun_TimestampWritesSyncedLyrics drives the ModeSynced output path
// via mock-lrc, which supports ParamTimestamp and returns LRC-style
// SyncedLyrics when --timestamp is set.
func TestRun_TimestampWritesSyncedLyrics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--timestamp", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing timestamped LRC line: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr not empty: %q", stderr.String())
	}
}

// TestRun_TimestampSupportedButNoSyncedLyricsFallsBackToPlain drives the
// "source honors --timestamp but had no synced lyrics" branch via
// mock-nosync, which advertises ParamTimestamp yet always returns empty
// SyncedLyrics: output falls back to plain text with a stderr warning.
func TestRun_TimestampSupportedButNoSyncedLyricsFallsBackToPlain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosync", "--timestamp", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing plain lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "returned no timestamped lyrics") {
		t.Fatalf("stderr missing fallback warning: %q", stderr.String())
	}
}
