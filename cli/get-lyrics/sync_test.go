package main

import (
	"bytes"
	"strings"
	"testing"
)

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
