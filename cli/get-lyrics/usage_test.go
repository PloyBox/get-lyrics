//go:build test

package main_test

import (
	"strings"
	"testing"
)

// TestUsageErrors locks exit 2 plus the usage text for invalid argv.
func TestUsageErrors(t *testing.T) {
	named := func(extra ...string) []string {
		return append([]string{"--name", "x"}, extra...)
	}
	cases := []struct {
		name string
		args []string
	}{
		{"missing name", []string{"--sync-level", "none"}},
		{"empty name", []string{"--name", "", "--sync-level", "none"}},
		{"whitespace name", []string{"--name", "   ", "--sync-level", "none"}},
		{"positional argument", named("--sync-level", "none", "extra")},
		{"unknown flag", []string{"--bogus", "--name", "x"}},
		{"repeated name", named("--name", "y")},
		{"repeated source aliases", named("-s", "mock-plain", "--source", "mock-plain")},
		{"invalid sync level", named("--sync-level", "bogus")},
		{"invalid duration", named("--duration", "abc")},
		{"invalid timeout", named("--timeout", "abc")},
		{"invalid env key", named("--env", "bad_key=1")},
		{"empty env value", named("--env", "MOCK_KEY=")},
		{"duplicate env key", named("--env", "MOCK_KEY=a", "--env", "MOCK_KEY=b")},
		{"empty source", named("--source", "")},
		{"duration above max", named("--duration", "9223372037")},
		{"duration minute overflow", named("--duration", "153722867:17")},
		{"zero duration", named("--duration", "0")},
		{"zero mm:ss duration", named("--duration", "0:00")},
		{"timeout above max", named("--timeout", "9223372037")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != 2 || stdout != "" {
				t.Errorf("code/stdout = %d/%q, want 2 and empty", code, stdout)
			}
			for _, want := range []string{"error[usage]", "Usage: get-lyrics"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want substring %q", stderr, want)
				}
			}
		})
	}
}

// TestHelpVersion locks the help/version short-circuit and their precedence.
func TestHelpVersion(t *testing.T) {
	const versionOut = "get-lyrics dev\n"
	cases := []struct {
		name        string
		args        []string
		wantVersion bool
	}{
		{"short help", []string{"-h"}, false},
		{"long help", []string{"--help"}, false},
		{"help wins over version", []string{"--help", "--version"}, false},
		{"help before unknown flag", []string{"--help", "--bogus"}, false},
		{"help before repeated flag", []string{"--help", "--name", "a", "--name", "b"}, false},
		{"help before invalid value", []string{"--sync-level", "bogus", "--help"}, false},
		{"short version", []string{"-v"}, true},
		{"long version", []string{"--version"}, true},
		{"version before unknown flag", []string{"--version", "--bogus"}, true},
		{"version before repeated flag",
			[]string{"--version", "--source", "a", "--source", "b"}, true},
		{"version before invalid value",
			[]string{"--timeout", "bogus", "--version"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != 0 || stderr != "" {
				t.Errorf("code/stderr = %d/%q, want 0 and empty", code, stderr)
			}
			if tc.wantVersion {
				if stdout != versionOut {
					t.Errorf("stdout = %q, want %q", stdout, versionOut)
				}
				return
			}
			if !strings.Contains(stdout, "Usage: get-lyrics") ||
				strings.Contains(stdout, versionOut) {
				t.Errorf("stdout = %q, want usage text without version", stdout)
			}
		})
	}
	t.Run("help lists sources and parameters", func(t *testing.T) {
		code, stdout, stderr := run(t, nil, "--help")
		if code != 0 || stderr != "" {
			t.Fatalf("code/stderr = %d/%q, want 0 and empty", code, stderr)
		}
		want := []string{
			"lrclib", "lyricsovh", "lrccx", "musixmatch", "betterlyrics",
			"mock-plain", "MUSIXMATCH_API_KEY", "Source parameters:",
		}
		for _, s := range want {
			if !strings.Contains(stdout, s) {
				t.Errorf("help output missing %q", s)
			}
		}
	})
}

// TestSourceErrors locks the unknown/duplicate source exits and lenient mode.
func TestSourceErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"unknown source", stdArgs("nosuch"), 3,
			"error[unknown]: source \"nosuch\" not found\n"},
		{"duplicate source", stdArgs("mock-plain,mock-plain"), 8,
			"error[usage]: source \"mock-plain\" is listed more than once\n"},
		{"lenient unknown source",
			srcArgs("nosuch", "--lenient", "--sync-level", "none"), 4, ""},
		{"lenient duplicate with valid source",
			srcArgs("mock-plain,mock-plain", "--lenient", "--sync-level", "none"),
			0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if tc.wantErr == "" {
				return
			}
			if stdout != "" || stderr != tc.wantErr {
				t.Errorf("stdout/stderr = %q/%q, want empty/%q",
					stdout, stderr, tc.wantErr)
			}
		})
	}
}
