package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// Mock/test-only sources are registered in main_loadmock.go (build tag
// "test"), not here.

// buggyCustomSource is a test-only fixture registered into the shared
// registry to exercise gate 2 at the CLI level. Its dynamic
// RequiredCustom is invalid while its static declaration is empty, so
// gate 1 (registration) passes and gate 2 (precheck) flags it.
type buggyCustomSource struct{ name string }

func (b *buggyCustomSource) Name() string { return b.name }
func (b *buggyCustomSource) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{
		Required:       source.ParamAuthor, // would abort exit 6 if the missing-check ran
		RequiredCustom: []string{"LANG-X"}, // gate-2 violation
	}
}
func (b *buggyCustomSource) CustomParams() []source.ParamSpec { return nil }
func (b *buggyCustomSource) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{}, nil
}

// userAgentSource is a test-only fixture that echoes the incoming
// Request.UserAgent so a CLI run can prove --user-agent reaches the
// adapter. It registers no custom params and requires nothing.
type userAgentSource struct{ name string }

func (u *userAgentSource) Name() string { return u.name }
func (u *userAgentSource) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}
func (u *userAgentSource) CustomParams() []source.ParamSpec { return nil }
func (u *userAgentSource) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[user-agent] " + req.UserAgent + "\n",
		Title:  req.Song,
		Filled: source.FieldLyrics | source.FieldTitle,
	}, nil
}

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
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--sync-level", "line", "TEST_SONG"}, &stdout, &stderr)
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

// TestRun_MockMismatchMissingAuthorWarnsAndExitsFour drives the
// precheck-vs-requirement mismatch path through mock-mismatch: it
// declares nothing required, so precheck lets it through, but its Fetch
// raises a RequiredParamMismatchError. As the only source the run ends
// in exit 4 with the mismatch warning on stderr.
func TestRun_MockMismatchMissingAuthorWarnsAndExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck-mismatch]: source "mock-mismatch" requires --author`) {
		t.Fatalf("stderr missing precheck-mismatch warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (no lyrics fetched): %q", stdout.String())
	}
}

// TestRun_MockMismatchFailsOverToNextSource verifies that a
// RequiredParamMismatchError does not abort the run: mock-nosupport
// (listed second) is fetched successfully, with the mismatch warning
// still printed.
func TestRun_MockMismatchFailsOverToNextSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch,mock-nosupport", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-nosupport]") {
		t.Fatalf("stdout missing nosupport lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[precheck-mismatch]") {
		t.Fatalf("stderr missing precheck-mismatch warning: %q", stderr.String())
	}
}

// TestRun_MockMismatchWithAuthorSucceeds covers the happy path of
// mock-mismatch: with --author supplied its Fetch succeeds and no
// mismatch warning is emitted.
func TestRun_MockMismatchWithAuthorSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-mismatch]") {
		t.Fatalf("stdout missing mismatch lyrics: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "warning[precheck-mismatch]") {
		t.Fatalf("stderr should have no mismatch warning: %q", stderr.String())
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

// TestRun_InvalidSyncLevelValueExitsTwo: any comma-separated value other
// than line/none is rejected at parse time with a usage error.
func TestRun_InvalidSyncLevelValueExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", "karaoke", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), `invalid sync level value "karaoke"`) {
		t.Fatalf("stderr missing invalid sync level message: %q", stderr.String())
	}
}

// TestRun_SyncLevelLineWritesSyncedLyrics drives the synced output path
// via mock-lrc, which returns LRC-style SyncedLyrics when --sync-level
// line is set.
func TestRun_SyncLevelLineWritesSyncedLyrics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing synced LRC line: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr not empty: %q", stderr.String())
	}
}

// TestRun_SyncLevelLineNoSyncedExitsFour drives the "synced requested,
// source returned only plain, no plain iteration to fall back on" path
// via mock-nosync: exit 4 with the downgrade warning and no-result error.
func TestRun_SyncLevelLineNoSyncedExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosync", "--sync-level", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "returned no synced lyrics") {
		t.Fatalf("stderr missing fallback warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_SyncedOnlySourceWithNoneExitsFour is the empty-success
// regression end to end: mock-synconly never fills plain Lyrics, so a
// lone "none" iteration cannot match — exit 4 with the symmetric
// downgrade warning printed before the no-result error and empty
// stdout.
func TestRun_SyncedOnlySourceWithNoneExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-synconly", "--sync-level", "none", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `warning[downgraded]: source "mock-synconly" returned only synced lyrics`) {
		t.Fatalf("stderr missing symmetric downgrade warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
	stderrStr := stderr.String()
	wi := strings.Index(stderrStr, "warning[downgraded]")
	ei := strings.Index(stderrStr, "error[no-result]")
	if wi == -1 || ei == -1 || wi > ei {
		t.Fatalf("expected downgrade warning before error; stderr=%q", stderrStr)
	}
}

// TestRun_SyncedOnlySourceNoneThenLineReusesCache drives the cache-reuse
// path: "none,line" on mock-synconly — the first (none) round stores the
// synced track with one downgrade warning, the second (line) round
// matches it from the cache and prints the LRC content.
func TestRun_SyncedOnlySourceNoneThenLineReusesCache(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-synconly", "--sync-level", "none,line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing synced lyrics from the cache round: %q", stdout.String())
	}
	if got := strings.Count(stderr.String(), "warning[downgraded]"); got != 1 {
		t.Fatalf("downgraded warnings = %d; want exactly 1 (stderr=%q)", got, stderr.String())
	}
}

// TestRun_SyncLevelNoneBeforeLinePrefersPlain proves the user-given
// sync level order is the priority: "none,line" returns plain lyrics
// from the first iteration, no warning, exit 0.
func TestRun_SyncLevelNoneBeforeLinePrefersPlain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", "none,line", "TEST_SONG"}, &stdout, &stderr)
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

func TestRun_SyncLevelValueIsTrimmed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", " line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing synced lyrics: %q", stdout.String())
	}
}

func TestRun_SyncLevelOnlyEmptyEntriesExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", ",", "TEST_SONG"}, &stdout, &stderr)
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

// TestRun_EnvMissingRequiredExitsSix drives the custom required-param
// precheck through mock-custom: without LANG the run exits 6 and the
// message spells the custom key as --env LANG.
func TestRun_EnvMissingRequiredExitsSix(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_EnvConditionalCountryExitsSix drives mock-custom's
// conditional requirement: once LANG is present, COUNTRY joins the
// required list and its absence exits 6.
func TestRun_EnvConditionalCountryExitsSix(t *testing.T) {
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "--env", "LANG=en", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env COUNTRY`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_EnvCountryWithoutLangExitsSix: COUNTRY alone is not
// recognized (conditional), so LANG stays the missing required key; the
// strict precheck abort carries no unsupported warning for COUNTRY.
func TestRun_EnvCountryWithoutLangExitsSix(t *testing.T) {
	t.Setenv("LANG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "--env", "COUNTRY=cn", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
	if strings.Contains(stderr.String(), "warning[") {
		t.Fatalf("stderr should carry no warnings on this strict abort: %q", stderr.String())
	}
}

// TestRun_EnvLenientSkipsMissing mirrors the typed lenient path: a
// missing required custom key skips the source with a PreCheck warning.
func TestRun_EnvLenientSkipsMissing(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	wantSub := `warning[precheck]: source "mock-custom" skipped: requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_EnvProvidedSucceeds drives the happy path: supplied keys reach
// the adapter's Request.Custom.
func TestRun_EnvProvidedSucceeds(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing lang=en, mock-custom must read Request.Custom: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "does not support") {
		t.Fatalf("stderr should carry no unsupported warnings: %q", stderr.String())
	}
}

// TestRun_EnvUnrecognizedKeyWarns: a user-supplied key the source does
// not recognize produces one unsupported warning, success intact.
func TestRun_EnvUnrecognizedKeyWarns(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "--env", "FOO=bar", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[unsupported]: source "mock-custom" does not support --env FOO`) {
		t.Fatalf("stderr missing FOO unsupported warning: %q", stderr.String())
	}
}

// TestRun_EnvMultipleUnrecognizedWarnsSet: multiple unrecognized keys
// warn with unspecified order — assert the warning set, not the order.
func TestRun_EnvMultipleUnrecognizedWarnsSet(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "--env", "FOO=bar", "--env", "BAR=baz", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.Count(stderr.String(), "does not support --env"); got != 2 {
		t.Fatalf("unsupported warnings = %d; want 2 (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--env FOO") || !strings.Contains(stderr.String(), "--env BAR") {
		t.Fatalf("stderr missing FOO/BAR warnings: %q", stderr.String())
	}
}

// TestRun_EnvInvalidInputsExitTwo locks the CLI hard gate: bad key
// syntax, empty/whitespace/missing values and duplicate keys are usage
// errors (exit 2), independent of --lenient and never reaching a source.
func TestRun_EnvInvalidInputsExitTwo(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{"lowercase key", []string{"--env", "lang=en", "TEST_SONG"}},
		{"digit-start key", []string{"--env", "1LANG=en", "TEST_SONG"}},
		{"hyphen key", []string{"--env", "LANG-X=en", "TEST_SONG"}},
		{"empty value", []string{"--env", "LANG=", "TEST_SONG"}},
		{"whitespace value", []string{"--env", "LANG=  ", "TEST_SONG"}},
		{"missing value", []string{"--env", "LANG", "TEST_SONG"}},
		{"duplicate key", []string{"--env", "LANG=en", "--env", "LANG=fr", "TEST_SONG"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.argv, &stdout, &stderr)
			if code != exitUsage {
				t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "error[usage]") {
				t.Fatalf("stderr missing usage tag: %q", stderr.String())
			}
		})
	}
}

// TestRun_EnvShortAndEqualsForms: -e and --env= both collect values.
func TestRun_EnvShortAndEqualsForms(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"-e", "LANG=en", "--env=COUNTRY=cn", "--source", "mock-custom", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing lang=en: %q", stdout.String())
	}
}

// TestRun_EnvFallbackFromProcessEnv: a declared key the user did not
// pass via -e is filled from the process environment before the fetch.
func TestRun_EnvFallbackFromProcessEnv(t *testing.T) {
	t.Setenv("LANG", "en")
	t.Setenv("COUNTRY", "cn")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing env-injected lang=en: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "does not support") {
		t.Fatalf("stderr should carry no unsupported warnings: %q", stderr.String())
	}
}

// TestRun_EnvFlagBeatsProcessEnv: -e wins over the environment.
func TestRun_EnvFlagBeatsProcessEnv(t *testing.T) {
	t.Setenv("LANG", "envvalue")
	t.Setenv("COUNTRY", "cn")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=flagvalue", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=flagvalue)") {
		t.Fatalf("stdout should carry the -e value, got %q", stdout.String())
	}
}

// TestRun_EnvEmptyProcessEnvTreatedMissing: an environment variable that
// exists but is empty (LANG=) counts as missing and is not injected, so
// the required-key semantics kick in.
func TestRun_EnvEmptyProcessEnvTreatedMissing(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
}

// TestRun_EnvInjectedToUndeclaringSourceWarns: env-injected keys are
// treated like user-provided ones — mock-nosupport does not declare
// LANG, so the injected value produces an unsupported warning there,
// while mock-custom (which declared LANG) is skipped under --lenient
// for the missing conditional COUNTRY.
func TestRun_EnvInjectedToUndeclaringSourceWarns(t *testing.T) {
	t.Setenv("LANG", "en")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--lenient", "--source", "mock-custom,mock-nosupport", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-nosupport]") {
		t.Fatalf("stdout missing nosupport lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `warning[unsupported]: source "mock-nosupport" does not support --env LANG`) {
		t.Fatalf("stderr missing injected-LANG unsupported warning: %q", stderr.String())
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

// TestRun_Gate2BuggySourceSkippedNotExitSix: a gate-2 source bug is
// skipped with a precheck-mismatch warning in strict mode too — the run
// ends in no-result, never exit 6, even though the same source also
// requires a typed param the caller did not supply.
func TestRun_Gate2BuggySourceSkippedNotExitSix(t *testing.T) {
	if err := registry.Register(&buggyCustomSource{name: "buggy-gate2"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("buggy-gate2") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "buggy-gate2", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck-mismatch]: source "buggy-gate2" declared invalid --env key "LANG-X" (source bug)`) {
		t.Fatalf("stderr missing gate-2 warning: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "error[required]") {
		t.Fatalf("gate-2 bug must not surface as exit 6: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_Gate2WarningPrintedBeforeStrictError locks the review C.7
// decision end to end: the gate-2 warning from source A is printed
// before the strict required-param error from source B aborts the run.
func TestRun_Gate2WarningPrintedBeforeStrictError(t *testing.T) {
	if err := registry.Register(&buggyCustomSource{name: "buggy-gate2"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("buggy-gate2") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "buggy-gate2,mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	stderrStr := stderr.String()
	wi := strings.Index(stderrStr, "warning[precheck-mismatch]")
	ei := strings.Index(stderrStr, "error[required]")
	if wi == -1 || ei == -1 || wi > ei {
		t.Fatalf("expected gate-2 warning before required error; stderr=%q", stderrStr)
	}
}

// TestRun_UserAgentReachesAdapter drives the --user-agent flag end to
// end: the header value the user supplies must reach the adapter's
// Request.UserAgent.
func TestRun_UserAgentReachesAdapter(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "--user-agent", "CustomUA/1.0", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] CustomUA/1.0") {
		t.Fatalf("stdout missing user-agent value: %q", stdout.String())
	}
}

// TestRun_UserAgentShortFlagWorks proves the -u short form is accepted.
func TestRun_UserAgentShortFlagWorks(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "-u", "ShortUA/2.0", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] ShortUA/2.0") {
		t.Fatalf("stdout missing user-agent value: %q", stdout.String())
	}
}

// TestRun_UserAgentDefaultReachesAdapter proves an unset --user-agent
// falls back to the CLI's default UA (version-injected) rather than an
// empty value — the sources no longer supply their own default.
func TestRun_UserAgentDefaultReachesAdapter(t *testing.T) {
	if err := registry.Register(&userAgentSource{name: "mock-useragent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("mock-useragent") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[user-agent] "+defaultUserAgent()) {
		t.Fatalf("stdout should echo the default user-agent %q: %q", defaultUserAgent(), stdout.String())
	}
}
