package main

import (
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

// TestRenderWarning locks the exact stderr text for every WarningKind
// branch: the fetch layer now carries structured data only, so these
// assertions are the byte-level rendering contract (previously locked
// by the fetch layer's own message tests).
func TestRenderWarning(t *testing.T) {
	cases := []struct {
		name string
		w    fetch.Warning
		want string
	}{
		{
			"unsupported author",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", Param: source.ParamAuthor},
			`warning[unsupported]: source "x" does not support --author`,
		},
		{
			"unsupported album",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", Param: source.ParamAlbum},
			`warning[unsupported]: source "x" does not support --album`,
		},
		{
			"unsupported isrc",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", Param: source.ParamISRC},
			`warning[unsupported]: source "x" does not support --isrc`,
		},
		{
			"unsupported duration",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", Param: source.ParamDuration},
			`warning[unsupported]: source "x" does not support --duration`,
		},
		{
			"unsupported custom key",
			fetch.Warning{Kind: fetch.UnsupportedParam, Source: "x", ParamName: "FOO"},
			`warning[unsupported]: source "x" does not support --env FOO`,
		},
		{
			"downgraded no synced",
			fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncLine},
			`warning[downgraded]: source "x" returned no synced lyrics`,
		},
		{
			"downgraded only synced",
			fetch.Warning{Kind: fetch.Downgraded, Source: "x", Want: fetch.SyncNone},
			`warning[downgraded]: source "x" returned only synced lyrics`,
		},
		{
			"precheck duplicate",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x"},
			`warning[precheck]: source "x" skipped: duplicate`,
		},
		{
			"precheck not found",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", Err: errors.New("source: not found")},
			`warning[precheck]: source "x" skipped: not found`,
		},
		{
			"precheck requires typed",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", Param: source.ParamAuthor},
			`warning[precheck]: source "x" skipped: requires --author`,
		},
		{
			"precheck requires custom",
			fetch.Warning{Kind: fetch.PreCheck, Source: "x", ParamName: "LANG"},
			`warning[precheck]: source "x" skipped: requires --env LANG`,
		},
		{
			"precheck-mismatch invalid key",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", ParamName: "BAD"},
			`warning[precheck-mismatch]: source "x" declared invalid --env key "BAD" (source bug)`,
		},
		{
			"precheck-mismatch fetch-time typed",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", Param: source.ParamAuthor, Err: errors.New("bug")},
			`warning[precheck-mismatch]: source "x" requires --author but precheck did not enforce it (source bug); trying next source`,
		},
		{
			"precheck-mismatch fetch-time custom",
			fetch.Warning{Kind: fetch.PrecheckMismatch, Source: "x", ParamName: "LANG", Err: errors.New("bug")},
			`warning[precheck-mismatch]: source "x" requires --env LANG but precheck did not enforce it (source bug); trying next source`,
		},
		{
			"fetch failed",
			fetch.Warning{Kind: fetch.FetchFailed, Source: "x", Err: errors.New("boom")},
			`warning[fetch]: source "x" failed: boom; trying next source`,
		},
		{
			"result declared but empty",
			fetch.Warning{Kind: fetch.ResultMismatch, Source: "x", Field: source.FieldLyrics, Declared: true},
			`warning[result]: source "x" declares field "Lyrics" but left it empty (source issue)`,
		},
		{
			"result filled but undeclared",
			fetch.Warning{Kind: fetch.ResultMismatch, Source: "x", Field: source.FieldSubSource},
			`warning[result]: source "x" filled field "SubSource" without declaring it (source issue)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderWarning(tc.w); got != tc.want {
				t.Fatalf("renderWarning = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestRenderRequiredError locks the error[required] line body for both
// the typed and the custom-key spelling.
func TestRenderRequiredError(t *testing.T) {
	if got := renderRequiredError(fetch.RequiredParamError{Source: "x", Param: source.ParamAuthor}); got != `source "x" requires --author` {
		t.Fatalf("typed = %q", got)
	}
	if got := renderRequiredError(fetch.RequiredParamError{Source: "x", ParamName: "LANG"}); got != `source "x" requires --env LANG` {
		t.Fatalf("custom = %q", got)
	}
}
