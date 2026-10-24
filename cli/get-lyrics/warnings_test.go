//go:build test

package main_test

import "testing"

// TestWarnings locks every rendered warning line and its exit code.
func TestWarnings(t *testing.T) {
	noResult := "error[no-result]: no source returned a valid result\n"
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{
			name:     "unsupported",
			args:     stdArgs("mock-nosupport", "--author", "X"),
			wantCode: 0,
			wantStderr: `warning[unsupported]: source "mock-nosupport" ` +
				"does not support --author\n",
		},
		{
			name: "precheck requires",
			args: srcArgs("mock-require-author,mock-plain",
				"--lenient", "--sync-level", "none"),
			wantCode: 0,
			wantStderr: `warning[precheck]: source "mock-require-author" ` +
				"skipped: requires --author\n",
		},
		{
			name:     "precheck-mismatch gate2",
			args:     stdArgs("mock-gate2,mock-plain"),
			wantCode: 0,
			wantStderr: `warning[precheck-mismatch]: source "mock-gate2" ` +
				`declared invalid --env key "BAD_KEY" (source bug)` + "\n",
		},
		{
			name:     "precheck-mismatch at fetch time",
			args:     stdArgs("mock-fetchmismatch,mock-plain"),
			wantCode: 0,
			wantStderr: `warning[precheck-mismatch]: source ` +
				`"mock-fetchmismatch" requires --env MOCK_TOKEN but ` +
				"precheck did not enforce it (source bug); " +
				"trying next source\n",
		},
		{
			name:     "fetch failure",
			args:     stdArgs("mock-fail,mock-plain"),
			wantCode: 0,
			wantStderr: `warning[fetch]: source "mock-fail" failed: ` +
				"mock-fail: upstream failure; trying next source\n",
		},
		{
			name:     "result declared empty",
			args:     stdArgs("mock-result-mismatch"),
			wantCode: 0,
			wantStderr: `warning[result]: source "mock-result-mismatch" ` +
				`declares field "Title" but left it empty (source issue)` +
				"\n",
		},
		{
			name:     "not found",
			args:     stdArgs("mock-notfound,mock-plain"),
			wantCode: 0,
			wantStderr: `warning[not-found]: source "mock-notfound" ` +
				"found no match; trying next source\n",
		},
		{
			name:     "body limit",
			args:     srcArgs("mock-bodylimit", "--sync-level", "line"),
			wantCode: 4,
			wantStderr: `warning[body-limit]: source "mock-bodylimit" ` +
				"returned an oversized response; trying next source\n" +
				noResult,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := run(t, nil, tc.args...)
			if code != tc.wantCode || stderr != tc.wantStderr {
				t.Errorf("code/stderr = %d/%q, want %d/%q",
					code, stderr, tc.wantCode, tc.wantStderr)
			}
		})
	}
}

// TestRequiredParams locks exit 6 and the render of required parameters.
func TestRequiredParams(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"typed author", stdArgs("mock-require-author"),
			"error[required]: source \"mock-require-author\" requires --author\n"},
		{"custom key", stdArgs("mock-customparams"),
			"error[required]: source \"mock-customparams\" requires --env MOCK_KEY\n"},
		{"warning before required error", stdArgs("mock-gate2,mock-require-author"),
			`warning[precheck-mismatch]: source "mock-gate2" declared ` +
				`invalid --env key "BAD_KEY" (source bug)` + "\n" +
				"error[required]: source \"mock-require-author\" " +
				"requires --author\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, nil, tc.args...)
			if code != 6 || stdout != "" || stderr != tc.wantStderr {
				t.Errorf("code/stdout/stderr = %d/%q/%q, want 6/empty/%q",
					code, stdout, stderr, tc.wantStderr)
			}
		})
	}
}
