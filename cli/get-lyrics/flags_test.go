package main

import (
	"bytes"
	"strings"
	"testing"
)

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
