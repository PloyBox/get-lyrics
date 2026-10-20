package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_InvalidSyncLevelValueExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--sync-level", "karaoke", "TEST_SONG"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
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
	if !strings.Contains(stderr.String(), "error[usage]") {
		t.Fatalf("stderr missing usage tag: %q", stderr.String())
	}
}

// TestParseDuration locks the --duration value grammar on the (pure)
// parser: whole seconds, mm:ss, and whitespace-only (not provided) are
// accepted; anything else is an error. The user-visible half of the same
// contract is covered by TestRun_InvalidDurationValueExitsTwo.
func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want uint
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

// TestParseTimeout locks the --timeout / --timeout-global grammar on the
// (pure) parser: non-negative integer seconds; empty means not provided
// (0); negative or non-numeric values are errors. The user-visible half is
// covered by TestRun_InvalidTimeoutValueExitsTwo.
func TestParseTimeout(t *testing.T) {
	cases := []struct {
		in   string
		want uint
		err  bool
	}{
		{"15", 15, false},
		{"0", 0, false},
		{" 20 ", 20, false},
		{"", 0, false},
		{"   ", 0, false},
		{"-3", 0, true},
		{"abc", 0, true},
		{"1.5", 0, true},
	}
	for _, tc := range cases {
		got, err := parseTimeout(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("parseTimeout(%q) = %d, nil; want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseTimeout(%q) err = %v; want nil", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseTimeout(%q) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

// TestRun_SyncLevelAllEmptyEntriesYieldsNoResult pins the documented
// behaviour for a --sync-level value made only of separators: the empty
// entries are dropped, the level list ends up empty, so there is no
// iteration to match and the run ends in the unified no-result error
// (exit 4) rather than a usage error.
func TestRun_SyncLevelAllEmptyEntriesYieldsNoResult(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc", "--sync-level", ",", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}
