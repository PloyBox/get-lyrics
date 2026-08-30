package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// Mock/test-only sources are registered in main_loadmock.go (build tag
// "test"), not here.

func TestRun_MissingSongExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "error[usage]: song title is required") {
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
	if !strings.Contains(stdout.String(), "--lenient") {
		t.Fatalf("stdout missing --lenient flag: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--overwrite") {
		t.Fatalf("stdout missing --overwrite flag: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--user-agent") {
		t.Fatalf("stdout missing --user-agent flag: %q", stdout.String())
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

// TestRun_WritesLyricsToStdoutByDefault exercises the default
// line,none sync level order on a plain-only source: the "line"
// iteration stores the plain result with a downgrade warning, and the
// "none" iteration matches the cache and returns it.
func TestRun_WritesLyricsToStdoutByDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing downgrade warning: %q", stderr.String())
	}
}

// TestRun_RealFileStdout exercises the default stdout sink when the
// writer is a real *os.File (as in production): the output file must
// never be truncated or seeked.
func TestRun_RealFileStdout(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	var stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "TEST_SONG"}, pw, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	// Close the write end so ReadAll returns; Run never closes the
	// stdout fallback itself.
	pw.Close()

	b, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics, got %q", string(b))
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
	if !strings.Contains(stderr.String(), "warning[unsupported]") {
		t.Fatalf("stderr missing unsupported warning tag: %q", stderr.String())
	}
}

func TestRun_UnknownSourceExitsThree(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "nope", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUnknownSrc {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUnknownSrc, stderr.String())
	}
	if !strings.Contains(stderr.String(), `error[unknown]: source "nope" not found`) {
		t.Fatalf("stderr missing unknown-source message: %q", stderr.String())
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

func TestRun_FetchFailureExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-fail", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mock-fail: intentional fetch failure") {
		t.Fatalf("stderr missing fetch error: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning[fetch]") {
		t.Fatalf("stderr missing fetch-failed warning tag: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_FetchFailureWarnsBeforeNoResultError locks the failure-path
// ordering: in-flight warnings are printed before the no-result error.
func TestRun_FetchFailureWarnsBeforeNoResultError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-fail", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d", code, exitFetchFailed)
	}
	stderrStr := stderr.String()
	wi := strings.Index(stderrStr, "warning[fetch]")
	ei := strings.Index(stderrStr, "error[no-result]")
	if wi == -1 || ei == -1 || wi > ei {
		t.Fatalf("expected warning before error; stderr=%q", stderrStr)
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

func TestRun_WhitespaceAuthorNotTreatedAsProvided(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "--author", " ", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "does not support --author") {
		t.Fatalf("stderr should not report whitespace author as unsupported: %q", stderr.String())
	}
}

// TestRun_DurationUnsupportedWarnsOnMockSuccess: mock-success only
// declares ParamAuthor, so a --duration filter trips one
// warning[unsupported] while the fetch itself still succeeds.
func TestRun_DurationUnsupportedWarnsOnMockSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{
			"--source", "mock-success",
			"--author", "TEST_AUTHOR",
			"--duration", "3:45",
			"TEST_SONG",
		},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lyrics") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[unsupported]") {
		t.Fatalf("stderr missing unsupported warning tag: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--duration") {
		t.Fatalf("stderr missing --duration spelling: %q", stderr.String())
	}
}

// TestRun_WhitespaceDurationNotTreatedAsProvided: whitespace-only
// --duration is treated as not provided and produces no warnings
// (mirroring the --author whitespace precedent).
func TestRun_WhitespaceDurationNotTreatedAsProvided(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "--duration", " ", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "does not support --duration") {
		t.Fatalf("stderr should not report whitespace duration as unsupported: %q", stderr.String())
	}
}

// TestRun_HelpRendersSourceParameters: --help renders the per-source
// static declarations in a "Source parameters:" section.
func TestRun_HelpRendersSourceParameters(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	for _, want := range []string{"Source parameters:", "mock-custom:", "--env LANG", "--env COUNTRY", "language hint"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %q", want, stdout.String())
		}
	}
}
