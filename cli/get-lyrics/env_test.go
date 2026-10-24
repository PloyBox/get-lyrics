//go:build test

package main_test

import (
	"strings"
	"testing"
)

// TestEnv locks --env injection, environment fallback and precedence.
func TestEnv(t *testing.T) {
	base := stdArgs("mock-customparams")
	withEnv := func(extra ...string) []string {
		return append(append([]string{}, base...), extra...)
	}

	t.Run("flag injection", func(t *testing.T) {
		code, stdout, stderr := run(t, nil,
			withEnv("--env", "MOCK_KEY=abc")...)
		if code != 0 || stdout != "MOCK_KEY=abc\nMOCK_OPT=" || stderr != "" {
			t.Errorf("code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
		}
	})
	t.Run("environment fallback", func(t *testing.T) {
		code, stdout, stderr := run(t, map[string]string{"MOCK_KEY": "abc"},
			withEnv()...)
		if code != 0 || stdout != "MOCK_KEY=abc\nMOCK_OPT=" || stderr != "" {
			t.Errorf("code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
		}
	})
	t.Run("flag wins over environment", func(t *testing.T) {
		code, stdout, _ := run(t, map[string]string{"MOCK_KEY": "from-env"},
			withEnv("--env", "MOCK_KEY=from-flag")...)
		if code != 0 || stdout != "MOCK_KEY=from-flag\nMOCK_OPT=" {
			t.Errorf("code/stdout = %d/%q", code, stdout)
		}
	})
	t.Run("empty environment value is missing", func(t *testing.T) {
		code, stdout, stderr := run(t, map[string]string{"MOCK_KEY": ""},
			withEnv()...)
		const want = `error[required]: source "mock-customparams" requires ` +
			"--env MOCK_KEY\n"
		if code != 6 || stdout != "" || stderr != want {
			t.Errorf("code/stdout/stderr = %d/%q/%q, want 6/empty/%q",
				code, stdout, stderr, want)
		}
	})
	t.Run("optional key is injected", func(t *testing.T) {
		code, stdout, _ := run(t, map[string]string{"MOCK_OPT": "opt"},
			withEnv("--env", "MOCK_KEY=abc")...)
		if code != 0 || stdout != "MOCK_KEY=abc\nMOCK_OPT=opt" {
			t.Errorf("code/stdout = %d/%q", code, stdout)
		}
	})
	t.Run("injected key on a non-declaring source", func(t *testing.T) {
		code, stdout, stderr := run(t, map[string]string{"MOCK_KEY": "abc"},
			srcArgs("mock-plain,mock-customparams", "--sync-level", "none")...)
		if code != 0 || stdout != mockPlainLyrics {
			t.Errorf("code/stdout = %d/%q, want 0/%q",
				code, stdout, mockPlainLyrics)
		}
		if !strings.Contains(stderr, "warning[unsupported]") ||
			!strings.Contains(stderr, "mock-plain") {
			t.Errorf("stderr = %q, want unsupported warning for mock-plain",
				stderr)
		}
	})
}
