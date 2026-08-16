package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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
// line,none timestamp order on a plain-only source: the "line"
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

// TestRun_SyncedOnlyRequestOnPlainSourceExitsFour drives the "no result
// matched the requested flag" path: mock-success is plain-only, so a
// lone "line" iteration stores the plain result (downgrade warning) and
// nothing matches — ending in exit 4 with the no-result error.
func TestRun_SyncedOnlyRequestOnPlainSourceExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--timestamp", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing downgrade warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]: no source returned a valid result") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_SourceRequiresAuthorExitsSix drives the required-parameter
// precheck through mock-require: mock-require requires --author, so a
// fetch without it must surface exit code 6 and the canonical stderr
// message.
func TestRun_SourceRequiresAuthorExitsSix(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-require" requires --author`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_StrictPrecheckFailsFastOnLaterSource verifies fail-fast
// ordering: mock-require's missing --author aborts the strict precheck
// even though mock-nosupport (listed first) is eligible, and no source
// is fetched.
func TestRun_StrictPrecheckFailsFastOnLaterSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport,mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (no fetch happened): %q", stdout.String())
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

// TestRun_LenientSkipsInvalidSources drives the --lenient precheck:
// mock-require lacks --author and is skipped with a PreCheck warning,
// while mock-nosupport (listed second) is fetched successfully.
func TestRun_LenientSkipsInvalidSources(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-require,mock-nosupport", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck]: source "mock-require" skipped: requires --author`) {
		t.Fatalf("stderr missing precheck warning: %q", stderr.String())
	}
}

// TestRun_LenientAllSkippedExitsFour: when --lenient skips every source,
// the run ends in the unified no-result error.
func TestRun_LenientAllSkippedExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning[precheck]") {
		t.Fatalf("stderr missing precheck warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
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
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr missing flag parse error: %q", stderr.String())
	}
}

// TestRun_InvalidTimestampValueExitsTwo: any comma-separated value other
// than line/none is rejected at parse time with a usage error.
func TestRun_InvalidTimestampValueExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--timestamp", "karaoke", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), `invalid timestamp value "karaoke"`) {
		t.Fatalf("stderr missing invalid-timestamp message: %q", stderr.String())
	}
}

// TestRun_TimestampWritesSyncedLyrics drives the synced output path via
// mock-lrc, which returns LRC-style SyncedLyrics when --timestamp line
// is set.
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

// TestRun_TimestampLineNoSyncedExitsFour drives the "synced requested,
// source returned only plain, no plain iteration to fall back on" path
// via mock-nosync: exit 4 with the downgrade warning and no-result error.
func TestRun_TimestampLineNoSyncedExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosync", "--timestamp", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "returned no timestamped lyrics") {
		t.Fatalf("stderr missing fallback warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_TimestampNoneBeforeLinePrefersPlain proves the user-given
// timestamp order is the priority: "none,line" returns plain lyrics
// from the first iteration, no warning, exit 0.
func TestRun_TimestampNoneBeforeLinePrefersPlain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--timestamp", "none,line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout should carry plain lyrics, got %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr not empty: %q", stderr.String())
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

func TestRun_TimestampValueIsTrimmed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--timestamp", " line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing timestamped lyrics: %q", stdout.String())
	}
}

func TestRun_TimestampOnlyEmptyEntriesExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--timestamp", ",", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

func TestRun_SourceEmptyEntriesFiltered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc,", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

func TestRun_DuplicateSourceExitsEight(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitDuplicateSrc {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitDuplicateSrc, stderr.String())
	}
	if !strings.Contains(stderr.String(), `source "mock-lrc" is listed more than once`) {
		t.Fatalf("stderr missing duplicate message: %q", stderr.String())
	}
}

func TestRun_LenientDuplicateSourceSkipped(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck]: source "mock-lrc" skipped: duplicate`) {
		t.Fatalf("stderr missing duplicate warning: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
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
