package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// buggyCustomSource is a test-only fixture registered into the shared
// registry to exercise gate 2 at the CLI level. Its dynamic
// RequiredCustom is invalid while its static declaration is empty, so
// gate 1 (registration) passes and gate 2 (precheck) flags it.
type buggyCustomSource struct{ name string }

func (b *buggyCustomSource) Name() string { return b.name }
func (b *buggyCustomSource) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{
		Required:       source.ParamAuthor, // would abort exit 6 if the missing-check ran
		RequiredCustom: []string{"LANG-X"}, // gate-2 violation
	}
}
func (b *buggyCustomSource) CustomParams() []source.ParamSpec { return nil }
func (b *buggyCustomSource) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{}, nil
}

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
	wantSub := `error[required]: source "mock-require" requires --author`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_StrictPrecheckFailsFastOnLaterSource verifies fail-fast
// ordering: mock-require's missing --author aborts the strict precheck
// even though mock-nosupport (listed first) is eligible, and no source
// is fetched.
func TestRun_StrictPrecheckFailsFastOnLaterSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-nosupport,mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (no fetch happened): %q", stdout.String())
	}
}

// TestRun_MockMismatchMissingAuthorWarnsAndExitsFour drives the
// precheck-vs-requirement mismatch path through mock-mismatch: it
// declares nothing required, so precheck lets it through, but its Fetch
// raises a RequiredParamMismatchError. As the only source the run ends
// in exit 4 with the mismatch warning on stderr.
func TestRun_MockMismatchMissingAuthorWarnsAndExitsFour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck-mismatch]: source "mock-mismatch" requires --author`) {
		t.Fatalf("stderr missing precheck-mismatch warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (no lyrics fetched): %q", stdout.String())
	}
}

// TestRun_MockMismatchFailsOverToNextSource verifies that a
// RequiredParamMismatchError does not abort the run: mock-nosupport
// (listed second) is fetched successfully, with the mismatch warning
// still printed.
func TestRun_MockMismatchFailsOverToNextSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch,mock-nosupport", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-nosupport]") {
		t.Fatalf("stdout missing nosupport lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning[precheck-mismatch]") {
		t.Fatalf("stderr missing precheck-mismatch warning: %q", stderr.String())
	}
}

// TestRun_MockMismatchWithAuthorSucceeds covers the happy path of
// mock-mismatch: with --author supplied its Fetch succeeds and no
// mismatch warning is emitted.
func TestRun_MockMismatchWithAuthorSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-mismatch", "--author", "X", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-mismatch]") {
		t.Fatalf("stdout missing mismatch lyrics: %q", stdout.String())
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
	if !strings.Contains(stdout.String(), "TEST_SONG") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck]: source "mock-require" skipped: requires --author`) {
		t.Fatalf("stderr missing precheck warning: %q", stderr.String())
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

func TestRun_DuplicateSourceExitsEight(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitDuplicateSrc {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitDuplicateSrc, stderr.String())
	}
	if !strings.Contains(stderr.String(), `source "mock-lrc" is listed more than once`) {
		t.Fatalf("stderr missing duplicate message: %q", stderr.String())
	}
}

func TestRun_LenientDuplicateSourceSkipped(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-lrc,mock-lrc", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck]: source "mock-lrc" skipped: duplicate`) {
		t.Fatalf("stderr missing duplicate warning: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[00:00.00]") {
		t.Fatalf("stdout missing lyrics: %q", stdout.String())
	}
}

// TestRun_Gate2BuggySourceSkippedNotExitSix: a gate-2 source bug is
// skipped with a precheck-mismatch warning in strict mode too — the run
// ends in no-result, never exit 6, even though the same source also
// requires a typed param the caller did not supply.
func TestRun_Gate2BuggySourceSkippedNotExitSix(t *testing.T) {
	if err := registry.Register(&buggyCustomSource{name: "buggy-gate2"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("buggy-gate2") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "buggy-gate2", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[precheck-mismatch]: source "buggy-gate2" declared invalid --env key "LANG-X" (source bug)`) {
		t.Fatalf("stderr missing gate-2 warning: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "error[required]") {
		t.Fatalf("gate-2 bug must not surface as exit 6: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_Gate2WarningPrintedBeforeStrictError locks the review C.7
// decision end to end: the gate-2 warning from source A is printed
// before the strict required-param error from source B aborts the run.
func TestRun_Gate2WarningPrintedBeforeStrictError(t *testing.T) {
	if err := registry.Register(&buggyCustomSource{name: "buggy-gate2"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = registry.Unregister("buggy-gate2") }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "buggy-gate2,mock-require", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	stderrStr := stderr.String()
	wi := strings.Index(stderrStr, "warning[precheck-mismatch]")
	ei := strings.Index(stderrStr, "error[required]")
	if wi == -1 || ei == -1 || wi > ei {
		t.Fatalf("expected gate-2 warning before required error; stderr=%q", stderrStr)
	}
}
