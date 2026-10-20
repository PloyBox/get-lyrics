package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_SyncLevelLineOnPlainOnlySourceExitsFour drives the "no result
// matched the requested flag" path: mock-success is plain-only, so a
// lone "line" iteration stores the plain result (downgrade warning) and
// nothing matches — ending in exit 4 with the no-result error.
func TestRun_SyncLevelLineOnPlainOnlySourceExitsFour(t *testing.T) {
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
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_SyncLevelLineNoSyncedExitsFour is the same exit-4 downgrade path
// one level lower: mock-nosync declares no filters at all and always
// answers with plain lyrics, so a lone "line" iteration cannot match.
func TestRun_SyncLevelLineNoSyncedExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosync", "--sync-level", "line", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing downgrade warning: %q", stderr.String())
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
	for _, want := range []string{"warning[downgraded]", `source "mock-synconly"`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
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

// TestRun_SyncLevelNoneBeforeLinePrefersPlain proves the user-given order is
// the priority: mock-lrc could serve LRC, but "none" comes first, so the
// plain answer from the first iteration is returned without a downgrade.
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

// TestRun_SyncLevelWordWritesTTML drives the word-level output path via
// mock-word, which returns pseudo TTML when --sync-level word is set.
func TestRun_SyncLevelWordWritesTTML(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-word", "--sync-level", "word", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "<tt>") || !strings.Contains(stdout.String(), "[mock-word]") {
		t.Fatalf("stdout missing TTML payload: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr not empty: %q", stderr.String())
	}
}

// TestRun_SyncLevelWordDegradesOnLineOnlySource drives the word-side
// downgrade: mock-lrc only produces line-level LRC, so a "word,none"
// request gets the word round downgraded (warning) and the "none" round
// reuses the cached plain result — plain output, exit 0.
func TestRun_SyncLevelWordDegradesOnLineOnlySource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", "word,none", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing word downgrade warning: %q", stderr.String())
	}
	if got := strings.Count(stderr.String(), "warning[downgraded]"); got != 1 {
		t.Fatalf("downgraded warnings = %d; want exactly 1 (stderr=%q)", got, stderr.String())
	}
	if strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout should carry the plain fallback, got %q", stdout.String())
	}
}

// TestRun_SyncLevelWordOnPlainOnlySourceExitsFour: mock-success is
// plain-only, so a lone "word" iteration cannot match — exit 4 with the
// word downgrade warning and no-result error.
func TestRun_SyncLevelWordOnPlainOnlySourceExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--author", "TEST_AUTHOR", "--sync-level", "word", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[downgraded]") {
		t.Fatalf("stderr missing word downgrade warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}
