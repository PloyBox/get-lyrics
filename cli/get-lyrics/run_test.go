package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// versionLine is the user-visible shape of --version output; the stamped
// version itself is a release-build concern the test cannot observe.
var versionLine = regexp.MustCompile(`^get-lyrics \S+\n$`)

// Mock/test-only sources are registered in loadmock.go (build tag "test"),
// not here.

func TestRun_MissingSongExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
	}
}

// TestRun_HelpRendersSourcesAndParameters locks the help contract in one
// place: every registered source is listed (derived from the registry, so a
// new source cannot drift out of the test), every flag is documented, and
// the source-parameter section carries the static --env declarations.
func TestRun_HelpRendersSourcesAndParameters(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0", code)
	}
	help := stdout.String()
	if !strings.Contains(help, "Usage:") {
		t.Fatalf("stdout missing Usage header: %q", help)
	}
	for _, name := range registry.Names() {
		if !strings.Contains(help, "  "+name+"\n") {
			t.Fatalf("stdout missing registered source %q: %q", name, help)
		}
	}
	for _, flag := range []string{"--source", "--author", "--album", "--isrc", "--duration", "--timeout", "--timeout-global", "--output", "--overwrite", "--json", "--sync-level", "--user-agent", "--env", "--lenient", "--quiet", "--version"} {
		if !strings.Contains(help, flag) {
			t.Fatalf("stdout missing %s documentation: %q", flag, help)
		}
	}
	for _, want := range []string{"Source parameters:", "mock-custom:", "--env LANG", "--env COUNTRY"} {
		if !strings.Contains(help, want) {
			t.Fatalf("stdout missing %q: %q", want, help)
		}
	}
}

func TestRun_VersionFlagExitsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--version"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if got := stdout.String(); !versionLine.MatchString(got) {
		t.Fatalf("stdout = %q; want \"get-lyrics <version>\n\"", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

// TestRun_WritesLyricsToStdoutByDefault locks the default output sink:
// with no --output the lyrics go to stdout, and the plain result answering
// a default "line" request still carries a downgrade warning on stderr.
func TestRun_WritesLyricsToStdoutByDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") || !strings.Contains(stdout.String(), "lyrics") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing downgrade warning: %q", stderr.String())
	}
}

// TestRun_JSONResultEnvelope locks the whole --json envelope: formatVersion,
// the Level field, and every fetch.Result field -- empty metadata stays present
// instead of being omitted.
func TestRun_JSONResultEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--json", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("JSON output invalid: %v (%q)", err, stdout.String())
	}
	if got["formatVersion"] != float64(2) {
		t.Fatalf("JSON formatVersion = %v; want 2", got["formatVersion"])
	}
	for _, key := range []string{"Lyrics", "Title", "Artist", "Album", "ISRC", "Source", "SubSource", "Level"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("JSON missing %q: %q", key, stdout.String())
		}
	}
	for _, key := range []string{"Title", "Artist", "Album", "ISRC", "SubSource"} {
		if got[key] != "" {
			t.Fatalf("JSON %q = %v; want empty string", key, got[key])
		}
	}
	if got["Source"] != "mock-success" || got["Level"] != "none" {
		t.Fatalf("JSON result metadata = %#v", got)
	}
}

// closeSpy records whether Run closes the writer it is handed.
type closeSpy struct {
	bytes.Buffer
	closed bool
}

func (s *closeSpy) Close() error {
	s.closed = true
	return nil
}

// TestRun_DoesNotCloseStdoutWriter covers the one property a plain
// bytes.Buffer cannot: Run must not close the stdout writer it is handed.
func TestRun_DoesNotCloseStdoutWriter(t *testing.T) {
	var out closeSpy
	var stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "TEST_SONG"}, &out, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if out.closed {
		t.Fatal("Run must not close the stdout writer")
	}
	if got := out.String(); !strings.Contains(got, "TEST_SONG") ||
		!strings.Contains(got, "lyrics") {
		t.Fatalf("stdout = %q; want lyrics", got)
	}
}

// TestRun_MultiplePositionalsExitTwo locks strict positional arity: only a
// single positional (the song title) is accepted, so extra positionals are
// a usage error instead of being joined into one title.
func TestRun_MultiplePositionalsExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "Bohemian", "Rhapsody"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on usage error: %q", stdout.String())
	}
}

// TestRun_WritesUnsupportedParamWarningsToStderr exercises the
// "source does not support parameter" branch via mock-success: mock-success
// advertises only ParamAuthor, so passing --isrc (and --album) trips one
// warning each. --author is included so the fetch itself succeeds.
func TestRun_WritesUnsupportedParamWarningsToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{
			"--source", "mock-success",
			"--author", "TEST_AUTHOR",
			"--album", "TEST_ALBUM",
			"--isrc", "TEST_ISRC",
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
	for _, flag := range []string{"--album", "--isrc"} {
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
	for _, want := range []string{"error[unknown]", "nope"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

// TestRun_FetchFailureWarnsBeforeNoResultError locks the whole failure path:
// exit 4, the adapter cause and the fetch-failed warning on stderr, and the
// warnings printed before the no-result error.
func TestRun_FetchFailureWarnsBeforeNoResultError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-fail", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	stderrStr := stderr.String()
	for _, want := range []string{"warning[fetch]", "error[no-result]", "mock-fail: intentional fetch failure"} {
		if !strings.Contains(stderrStr, want) {
			t.Fatalf("stderr missing %q: %q", want, stderrStr)
		}
	}
	wi := strings.Index(stderrStr, "warning[fetch]")
	ei := strings.Index(stderrStr, "error[no-result]")
	if wi > ei {
		t.Fatalf("expected warning before error; stderr=%q", stderrStr)
	}
}

// TestRun_WhitespaceAuthorNotTreatedAsProvided locks the "empty means not
// provided" normalization: a whitespace-only --author counts as absent, so
// a source without the Author filter has nothing to warn about.
func TestRun_WhitespaceAuthorNotTreatedAsProvided(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport", "--author", " ", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "warning[unsupported]") {
		t.Fatalf("stderr should not report whitespace author as unsupported: %q", stderr.String())
	}
}
