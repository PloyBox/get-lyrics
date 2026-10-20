package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_EnvRequiredPrecheck drives the custom required-param precheck
// through mock-custom end to end: a missing required key exits 6 with the
// key spelled as --env, the conditional COUNTRY requirement only kicks in
// once LANG is present, COUNTRY alone does not satisfy LANG, and --lenient
// skips the source instead of aborting.
func TestRun_EnvRequiredPrecheck(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		argv       []string
		wantCode   int
		wantStderr []string
		wantAbsent []string
	}{
		{
			name:       "missing LANG",
			env:        map[string]string{"LANG": "", "COUNTRY": ""},
			argv:       []string{"--source", "mock-custom", "TEST_SONG"},
			wantCode:   exitRequired,
			wantStderr: []string{"error[required]", `source "mock-custom"`, "--env LANG"},
			wantAbsent: []string{"warning["},
		},
		{
			name:       "conditional COUNTRY once LANG is present",
			env:        map[string]string{"COUNTRY": ""},
			argv:       []string{"--source", "mock-custom", "--env", "LANG=en", "TEST_SONG"},
			wantCode:   exitRequired,
			wantStderr: []string{"error[required]", "--env COUNTRY"},
		},
		{
			name:       "COUNTRY alone does not satisfy LANG",
			env:        map[string]string{"LANG": ""},
			argv:       []string{"--source", "mock-custom", "--env", "COUNTRY=cn", "TEST_SONG"},
			wantCode:   exitRequired,
			wantStderr: []string{"error[required]", "--env LANG"},
			wantAbsent: []string{"warning["},
		},
		{
			name:       "lenient skips the source",
			env:        map[string]string{"LANG": "", "COUNTRY": ""},
			argv:       []string{"--lenient", "--source", "mock-custom", "TEST_SONG"},
			wantCode:   exitFetchFailed,
			wantStderr: []string{"warning[precheck]", `source "mock-custom"`, "--env LANG", "error[no-result]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			var stdout, stderr bytes.Buffer
			code := Run(tc.argv, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("code = %d; want %d (stderr=%q)", code, tc.wantCode, stderr.String())
			}
			for _, want := range tc.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr missing %q; got %q", want, stderr.String())
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(stderr.String(), absent) {
					t.Fatalf("stderr should not contain %q: %q", absent, stderr.String())
				}
			}
		})
	}
}

// TestRun_EnvProvidedSucceeds drives the happy path: supplied keys reach
// the adapter's Request.Custom.
func TestRun_EnvProvidedSucceeds(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("COUNTRY", "")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"--source", "mock-custom", "--env", "LANG=en", "--env", "COUNTRY=cn", "--sync-level", "none", "TEST_SONG"},
		&stdout, &stderr,
	)
	if code != exitOK {
		t.Fatalf("code = %d; want 0 (stderr=%q)", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "TEST_SONG") || !strings.Contains(got, "(lang=en)") {
		t.Fatalf("stdout = %q; want lyrics carrying Request.Custom (lang=en)", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should carry no warnings: %q", stderr.String())
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
	for _, want := range []string{"warning[unsupported]", `source "mock-custom"`, "--env FOO"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
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
	if got := strings.Count(stderr.String(), "warning[unsupported]"); got != 2 {
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
		want []string // tokens identifying the validation that fired
	}{
		{"lowercase key", []string{"--env", "lang=en", "TEST_SONG"}, []string{"lang"}},
		{"digit-start key", []string{"--env", "1LANG=en", "TEST_SONG"}, []string{"1LANG"}},
		{"hyphen key", []string{"--env", "LANG-X=en", "TEST_SONG"}, []string{"LANG-X"}},
		{"empty value", []string{"--env", "LANG=", "TEST_SONG"}, []string{"LANG="}},
		{"whitespace value", []string{"--env", "LANG=  ", "TEST_SONG"}, []string{"LANG=  "}},
		{"missing value", []string{"--env", "LANG", "TEST_SONG"}, []string{"LANG"}},
		{"duplicate key", []string{"--env", "LANG=en", "--env", "LANG=fr", "TEST_SONG"}, []string{"duplicate", "LANG"}},
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
			for _, want := range tc.want {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr missing %q; got %q", want, stderr.String())
				}
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
	if strings.Contains(stderr.String(), "warning[unsupported]") {
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
	for _, want := range []string{"warning[unsupported]", `source "mock-nosupport"`, "--env LANG"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}
