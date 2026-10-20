package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_UserAgentDefaultReachesAdapter asserts the default header shape
// (version prefix + project URL) via mock-useragent, which echoes the
// User-Agent it received; calling defaultUserAgent back would make the
// assertion self-referential, and the version string itself is covered by
// TestRun_VersionFlagExitsZero.
func TestRun_UserAgentDefaultReachesAdapter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-useragent", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "get-lyrics/") || !strings.Contains(stdout.String(), "https://github.com/PloyBox/get-lyrics") {
		t.Fatalf("stdout should echo the default user-agent shape get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics): %q", stdout.String())
	}
}
