//go:build test

package main_test

import (
	"strings"
	"testing"
	"time"
)

// TestTimeout locks global/per-source budgets and huge timeout handling.
func TestTimeout(t *testing.T) {
	requireBounded := func(t *testing.T, start time.Time, limit time.Duration) {
		t.Helper()
		if elapsed := time.Since(start); elapsed > limit {
			t.Errorf("elapsed = %v, want under %v", elapsed, limit)
		}
	}

	t.Run("global budget stops the fetch", func(t *testing.T) {
		start := time.Now()
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-block", "--timeout-global", "1")...)
		requireBounded(t, start, 5*time.Second)
		if code != 9 || stdout != "" {
			t.Errorf("code/stdout = %d/%q, want 9 and empty", code, stdout)
		}
		if !strings.Contains(stderr, "error[timeout]:") {
			t.Errorf("stderr = %q, want error[timeout]", stderr)
		}
	})
	t.Run("per-source budget fails over", func(t *testing.T) {
		start := time.Now()
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-block,mock-plain", "--timeout", "1")...)
		requireBounded(t, start, 5*time.Second)
		if code != 0 || stdout != mockPlainLyrics {
			t.Errorf("code/stdout = %d/%q, want 0/%q",
				code, stdout, mockPlainLyrics)
		}
		if !strings.Contains(stderr, `warning[fetch]: source "mock-block"`) {
			t.Errorf("stderr = %q, want a mock-block fetch warning", stderr)
		}
	})
	t.Run("maximum timeout does not overflow", func(t *testing.T) {
		start := time.Now()
		code, stdout, stderr := run(t, nil,
			stdArgs("mock-plain", "--timeout", "9223372036")...)
		requireBounded(t, start, 3*time.Second)
		if code != 0 || stdout != mockPlainLyrics {
			t.Errorf("code/stdout = %d/%q, want 0/%q (stderr %q)",
				code, stdout, mockPlainLyrics, stderr)
		}
	})
}
