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

// TestRun_InvalidDurationValueExitsTwo: a --duration value that is not
// seconds or mm:ss is rejected at parse time with a usage error.
func TestRun_InvalidDurationValueExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-success", "--duration", "abc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), `invalid duration value "abc"`) {
		t.Fatalf("stderr missing invalid duration message: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
	}
}

// TestParseDuration locks the --duration value grammar: whole seconds,
// mm:ss, and whitespace-only (not provided) are accepted; anything else
// is an error.
func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want int
		err  bool
	}{
		{"225", 225, false},
		{"3:45", 225, false},
		{"0:45", 45, false},
		{" 3:45 ", 225, false},
		{"", 0, false},
		{"   ", 0, false},
		{"0", 0, true},
		{"-5", 0, true},
		{"abc", 0, true},
		{"3:75", 0, true},
		{"1:2:3", 0, true},
		{"0:00", 0, true},
	}
	for _, tc := range cases {
		got, err := parseDuration(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("parseDuration(%q) = %d, nil; want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseDuration(%q) err = %v; want nil", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseDuration(%q) = %d; want %d", tc.in, got, tc.want)
		}
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
