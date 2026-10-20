package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_QuietSuppressesWarnings locks that --quiet silences stderr
// warnings while lyrics still reach stdout.
func TestRun_QuietSuppressesWarnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--quiet", "--source", "mock-success", "--author", "TEST_AUTHOR", "--album", "TEST_ALBUM", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "TEST_SONG") || !strings.Contains(got, "lyrics") {
		t.Fatalf("stdout = %q; want lyrics", got)
	}
}

// TestRun_QuietShortFlagSuppressesFetchWarningsAndErrors locks the -q
// alias and that fetch-failure warnings and errors are silenced.
func TestRun_QuietShortFlagSuppressesFetchWarningsAndErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-q", "--source", "mock-fail", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
}

// TestRun_QuietSuppressesUsageErrors locks that the missing-song usage
// error prints nothing under --quiet but still exits 2.
func TestRun_QuietSuppressesUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--quiet"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d", code, exitUsage)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}
