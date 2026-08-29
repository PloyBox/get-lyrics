package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_EnvMissingRequiredExitsSix drives the custom required-param
// precheck through mock-custom: without LANG the run exits 6 and the
// message spells the custom key as --env LANG.
func TestRun_EnvMissingRequiredExitsSix(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_EnvConditionalCountryExitsSix drives mock-custom's
// conditional requirement: once LANG is present, COUNTRY joins the
// required list and its absence exits 6.
func TestRun_EnvConditionalCountryExitsSix(t *testing.T) {
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "--env", "LANG=en", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env COUNTRY`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
}

// TestRun_EnvCountryWithoutLangExitsSix: COUNTRY alone is not
// recognized (conditional), so LANG stays the missing required key; the
// strict precheck abort carries no unsupported warning for COUNTRY.
func TestRun_EnvCountryWithoutLangExitsSix(t *testing.T) {
	t.Setenv("LANG", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "--env", "COUNTRY=cn", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
	wantSub := `error[required]: source "mock-custom" requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
	if strings.Contains(stderr.String(), "warning[") {
		t.Fatalf("stderr should carry no warnings on this strict abort: %q", stderr.String())
	}
}

// TestRun_EnvLenientSkipsMissing mirrors the typed lenient path: a
// missing required custom key skips the source with a PreCheck warning.
func TestRun_EnvLenientSkipsMissing(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--lenient", "--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitFetchFailed {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitFetchFailed, stderr.String())
	}
	wantSub := `warning[precheck]: source "mock-custom" skipped: requires --env LANG`
	if !strings.Contains(stderr.String(), wantSub) {
		t.Fatalf("stderr missing %q; got %q", wantSub, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[no-result]") {
		t.Fatalf("stderr missing no-result error: %q", stderr.String())
	}
}

// TestRun_EnvProvidedSucceeds drives the happy path: supplied keys reach
// the adapter's Request.Custom.
func TestRun_EnvProvidedSucceeds(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing lang=en, mock-custom must read Request.Custom: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "does not support") {
		t.Fatalf("stderr should carry no unsupported warnings: %q", stderr.String())
	}
}

// TestRun_EnvUnrecognizedKeyWarns: a user-supplied key the source does
// not recognize produces one unsupported warning, success intact.
func TestRun_EnvUnrecognizedKeyWarns(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "--env", "FOO=bar", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning[unsupported]: source "mock-custom" does not support --env FOO`) {
		t.Fatalf("stderr missing FOO unsupported warning: %q", stderr.String())
	}
}

// TestRun_EnvMultipleUnrecognizedWarnsSet: multiple unrecognized keys
// warn with unspecified order — assert the warning set, not the order.
func TestRun_EnvMultipleUnrecognizedWarnsSet(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "--env", "FOO=bar", "--env", "BAR=baz", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.Count(stderr.String(), "does not support --env"); got != 2 {
		t.Fatalf("unsupported warnings = %d; want 2 (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--env FOO") || !strings.Contains(stderr.String(), "--env BAR") {
		t.Fatalf("stderr missing FOO/BAR warnings: %q", stderr.String())
	}
}

// TestRun_EnvInvalidInputsExitTwo locks the CLI hard gate: bad key
// syntax, empty/whitespace/missing values and duplicate keys are usage
// errors (exit 2), independent of --lenient and never reaching a source.
func TestRun_EnvInvalidInputsExitTwo(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{"lowercase key", []string{"--env", "lang=en", "TEST_SONG"}},
		{"digit-start key", []string{"--env", "1LANG=en", "TEST_SONG"}},
		{"hyphen key", []string{"--env", "LANG-X=en", "TEST_SONG"}},
		{"empty value", []string{"--env", "LANG=", "TEST_SONG"}},
		{"whitespace value", []string{"--env", "LANG=  ", "TEST_SONG"}},
		{"missing value", []string{"--env", "LANG", "TEST_SONG"}},
		{"duplicate key", []string{"--env", "LANG=en", "--env", "LANG=fr", "TEST_SONG"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.argv, &stdout, &stderr)
			if code != exitUsage {
				t.Fatalf("code = %d; want %d (stderr=%q)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "error[usage]") {
				t.Fatalf("stderr missing usage tag: %q", stderr.String())
			}
		})
	}
}

// TestRun_EnvShortAndEqualsForms: -e and --env= both collect values.
func TestRun_EnvShortAndEqualsForms(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"-e", "LANG=en", "--env=COUNTRY=cn", "--source", "mock-custom", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing lang=en: %q", stdout.String())
	}
}

// TestRun_EnvFallbackFromProcessEnv: a declared key the user did not
// pass via -e is filled from the process environment before the fetch.
func TestRun_EnvFallbackFromProcessEnv(t *testing.T) {
	t.Setenv("LANG", "en")
	t.Setenv("COUNTRY", "cn")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=en)") {
		t.Fatalf("stdout missing env-injected lang=en: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "does not support") {
		t.Fatalf("stderr should carry no unsupported warnings: %q", stderr.String())
	}
}

// TestRun_EnvFlagBeatsProcessEnv: -e wins over the environment.
func TestRun_EnvFlagBeatsProcessEnv(t *testing.T) {
	t.Setenv("LANG", "envvalue")
	t.Setenv("COUNTRY", "cn")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=flagvalue", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(lang=flagvalue)") {
		t.Fatalf("stdout should carry the -e value, got %q", stdout.String())
	}
}

// TestRun_EnvEmptyProcessEnvTreatedMissing: an environment variable that
// exists but is empty (LANG=) counts as missing and is not injected, so
// the required-key semantics kick in.
func TestRun_EnvEmptyProcessEnvTreatedMissing(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--source", "mock-custom", "TEST_SONG"}, &stdout, &stderr)
	if code != exitRequired {
		t.Fatalf("code = %d; want %d (stderr=%q)", code, exitRequired, stderr.String())
	}
}

// TestRun_EnvInjectedToUndeclaringSourceWarns: env-injected keys are
// treated like user-provided ones — mock-nosupport does not declare
// LANG, so the injected value produces an unsupported warning there,
// while mock-custom (which declared LANG) is skipped under --lenient
// for the missing conditional COUNTRY.
func TestRun_EnvInjectedToUndeclaringSourceWarns(t *testing.T) {
	t.Setenv("LANG", "en")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--lenient", "--source", "mock-custom,mock-nosupport", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[mock-nosupport]") {
		t.Fatalf("stdout missing nosupport lyrics: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `warning[unsupported]: source "mock-nosupport" does not support --env LANG`) {
		t.Fatalf("stderr missing injected-LANG unsupported warning: %q", stderr.String())
	}
}
