package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

// TestRenderWarning checks the behavior contract of the warning renderer:
// every warning kind gets its tag, names the source, and carries the
// parameter/field/cause the warning is about. Exact wording is deliberately
// not asserted, so rewording a message does not break the test.
func TestRenderWarning(t *testing.T) {
	cases := []struct {
		name   string
		w      fetch.Warning
		tokens []string
	}{
		{
			"unsupported typed param",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", Param: source.ParamAuthor},
			[]string{"warning[unsupported]", `source "x"`, "--author"},
		},
		{
			"unsupported custom key",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", ParamName: "FOO"},
			[]string{"warning[unsupported]", `source "x"`, "--env FOO"},
		},
		{
			"downgraded line",
			fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncLine},
			[]string{"warning[downgraded]", `source "x"`, "synced"},
		},
		{
			"downgraded word",
			fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncWord},
			[]string{"warning[downgraded]", `source "x"`, "word"},
		},
		{
			"downgraded only synced",
			fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncNone},
			[]string{"warning[downgraded]", `source "x"`, "only"},
		},
		{
			"precheck duplicate",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x"},
			[]string{"warning[precheck]", `source "x"`, "duplicate"},
		},
		{
			"precheck not found",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", Err: errors.New("source: not found")},
			[]string{"warning[precheck]", `source "x"`, "not found"},
		},
		{
			"precheck requires typed",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", Param: source.ParamAuthor},
			[]string{"warning[precheck]", `source "x"`, "--author"},
		},
		{
			"precheck requires custom",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", ParamName: "LANG"},
			[]string{"warning[precheck]", `source "x"`, "--env LANG"},
		},
		{
			"precheck-mismatch invalid key",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", ParamName: "BAD"},
			[]string{"warning[precheck-mismatch]", `source "x"`, "BAD"},
		},
		{
			"precheck-mismatch fetch-time typed",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", Param: source.ParamAuthor, Err: errors.New("bug")},
			[]string{"warning[precheck-mismatch]", `source "x"`, "--author"},
		},
		{
			"precheck-mismatch fetch-time custom",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", ParamName: "LANG", Err: errors.New("bug")},
			[]string{"warning[precheck-mismatch]", `source "x"`, "--env LANG"},
		},
		{
			"fetch failed carries cause",
			fetch.Warning{Kind: fetch.FetchFailed, Source: "x", Err: errors.New("boom")},
			[]string{"warning[fetch]", `source "x"`, "boom"},
		},
		{
			"result declared but empty",
			fetch.Warning{Kind: fetch.ResultMismatch, Source: "x", Field: source.FieldLyrics, Declared: true},
			[]string{"warning[result]", `source "x"`, "Lyrics"},
		},
		{
			"result filled but undeclared",
			fetch.Warning{Kind: fetch.ResultMismatch, Source: "x", Field: source.FieldSubSource},
			[]string{"warning[result]", `source "x"`, "SubSource"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderWarning(tc.w)
			for _, token := range tc.tokens {
				if !strings.Contains(got, token) {
					t.Fatalf("renderWarning = %q; missing %q", got, token)
				}
			}
		})
	}
}

// TestRenderWarningDistinguishesDowngradeDirections checks that the three
// downgrade directions render differently, without pinning the wording.
func TestRenderWarningDistinguishesDowngradeDirections(t *testing.T) {
	line := renderWarning(fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncLine})
	word := renderWarning(fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncWord})
	none := renderWarning(fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncNone})
	if line == word || line == none || word == none {
		t.Fatalf("downgrade directions must render differently: line=%q word=%q none=%q", line, word, none)
	}
}

// TestRenderRequiredError checks that a required parameter is spelled as
// its CLI flag or --env key, without pinning the sentence.
func TestRenderRequiredError(t *testing.T) {
	typed := renderRequiredError(fetch.RequiredParamError{Source: "x", Param: source.ParamAuthor})
	for _, token := range []string{`source "x"`, "--author"} {
		if !strings.Contains(typed, token) {
			t.Fatalf("typed = %q; missing %q", typed, token)
		}
	}
	custom := renderRequiredError(fetch.RequiredParamError{Source: "x", ParamName: "LANG"})
	for _, token := range []string{`source "x"`, "--env LANG"} {
		if !strings.Contains(custom, token) {
			t.Fatalf("custom = %q; missing %q", custom, token)
		}
	}
}
