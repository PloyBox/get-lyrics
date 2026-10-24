//go:build test

package main_test

import "testing"

// TestQuiet locks full stderr suppression without changing exit codes.
func TestQuiet(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"usage error", []string{"--quiet", "--name", "", "--sync-level", "none"},
			2, ""},
		{"fetch error", stdArgs("mock-fail", "--quiet"), 4, ""},
		{"success with warning", stdArgs("mock-fail,mock-plain", "--quiet"), 0,
			mockPlainLyrics},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != tc.wantCode || stdout != tc.wantOut || stderr != "" {
				t.Errorf("code/stdout/stderr = %d/%q/%q, want %d/%q/empty",
					code, stdout, stderr, tc.wantCode, tc.wantOut)
			}
		})
	}
}
