package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_TimeoutGlobalExits9 locks --timeout-global: the whole-fetch
// budget fires on a hung source and maps to exit 9.
func TestRun_TimeoutGlobalExits9(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-slow", "--timeout-global", "1", "TEST_SONG"}, &stdout, &stderr)
	if code != exitTimeout {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitTimeout, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[timeout]") {
		t.Fatalf("stderr missing timeout error: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on timeout: %q", stdout.String())
	}
}

// TestRun_TimeoutPerSourceFailsOver locks --timeout: the per-source budget
// expires only that source, so fetch fails over to the next one.
func TestRun_TimeoutPerSourceFailsOver(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{
			"--source", "mock-slow,mock-success",
			"--author", "TEST_AUTHOR",
			"--sync-level", "none",
			"--timeout", "1",
			"TEST_SONG",
		},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics from the next source: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[fetch]") {
		t.Fatalf("stderr missing fetch-failed warning: %q", stderr.String())
	}
}
