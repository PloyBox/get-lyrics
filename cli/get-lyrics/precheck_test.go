package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_SourceRequiresAuthorExitsSix drives the required-parameter
// precheck through mock-require: mock-require requires --author, so a
// fetch without it must surface exit code 6 and the canonical stderr
// message.
func TestRun_SourceRequiresAuthorExitsSix(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	for _, want := range []string{"error[required]", `source "mock-require"`, "--author"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q; got %q", want, stderr.String())
		}
	}
}

// TestRun_MockMismatchMissingAuthorWarnsAndExitsFour drives the fetch-time
// mismatch path: a missing --author becomes a warning, never exit 6.
func TestRun_MockMismatchMissingAuthorWarnsAndExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	for _, want := range []string{
		"warning[precheck-mismatch]", `source "mock-mismatch"`, "--author",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "LANG-X") {
		t.Fatalf("must render fetch-time mismatch, not gate 2: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (no lyrics fetched): %q", stdout.String())
	}
}

// TestRun_MockMismatchWithAuthorSucceeds is the happy half: with --author the
// declaration becomes consistent, so mock-mismatch is fetched normally and
// no mismatch warning is emitted.
func TestRun_MockMismatchWithAuthorSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TEST_SONG") || !strings.Contains(stdout.String(), "lyrics") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "warning[precheck-mismatch]") {
		t.Fatalf("stderr should have no mismatch warning: %q", stderr.String())
	}
}

// TestRun_LenientSkipsInvalidSources drives the --lenient precheck:
// mock-require lacks --author and is skipped with a PreCheck warning,
// while mock-nosupport (listed second) is fetched successfully.
func TestRun_LenientSkipsInvalidSources(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-require,mock-nosupport", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-nosupport]") || !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout = %q; want lyrics from the second source", stdout.String())
	}
	for _, want := range []string{"warning[precheck]", `source "mock-require"`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

// TestRun_LenientAllSkippedExitsFour: when --lenient skips every source,
// the run ends in the unified no-result error.
func TestRun_LenientAllSkippedExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning[precheck]") {
		t.Fatalf("stderr missing precheck warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_DuplicateSourceExitsEight locks the duplicate-source precheck: a
// repeated --source entry exits 8 (sharing the error[usage] tag) and names
// the offending source.
func TestRun_DuplicateSourceExitsEight(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitDuplicateSrc {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitDuplicateSrc, stderr.String())
	}
	for _, want := range []string{"error[usage]", "mock-lrc"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

// TestRun_LenientDuplicateSourceSkipped is the lenient half of the same
// check: the repeated entry is dropped with a precheck warning and the first
// occurrence still serves the lyrics.
func TestRun_LenientDuplicateSourceSkipped(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	for _, want := range []string{"warning[precheck]", `source "mock-lrc"`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

// TestRun_Gate2BugSkippedEndToEnd drives the CLI half of gate 2: mock-gate2
// declares an inconsistent custom requirement, so it is skipped with a
// precheck-mismatch warning in strict mode too — never exit 6, even though
// it also requires a typed param the caller did not supply — and that
// warning is printed before a later source's strict required-param abort.
func TestRun_Gate2BugSkippedEndToEnd(t *testing.T) {
	t.Run("skipped instead of exit six", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"--source", "mock-gate2", "TEST_SONG"}, &stdout, &stderr)
		if code != exitFetchFailed {
			t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
		}
		for _, want := range []string{"warning[precheck-mismatch]", `source "mock-gate2"`, "LANG-X"} {
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("stderr missing %q: %q", want, stderr.String())
			}
		}
		if strings.Contains(stderr.String(), "error[required]") {
			t.Fatalf("gate-2 bug must not surface as exit 6: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "error[no-result]") {
			t.Fatalf("stderr missing no-result error: %q", stderr.String())
		}
	})

	t.Run("warning printed before a later strict abort", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"--source", "mock-gate2,mock-require", "TEST_SONG"}, &stdout, &stderr)
		if code != exitRequired {
			t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
		}
		stderrStr := stderr.String()
		wi := strings.Index(stderrStr, "warning[precheck-mismatch]")
		ei := strings.Index(stderrStr, "error[required]")
		if wi == -1 || ei == -1 || wi > ei {
			t.Fatalf("expected gate-2 warning before required error; stderr=%q", stderrStr)
		}
	})
}
