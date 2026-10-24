package fetch_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

func TestFailover(t *testing.T) {
	boom := errors.New("boom")
	mismatch := source.RequiredParamMismatchError{Source: "bad", ParamName: "TOKEN"}
	cases := []struct {
		name     string
		failure  error
		wantKind fetch.WarningKind
		wantIs   error
		wantMM   *source.RequiredParamMismatchError
	}{
		{"plain error", boom, fetch.FetchFailed, boom, nil},
		{"not found", fmt.Errorf("lookup: %w", source.ErrNotFound),
			fetch.NotFound, source.ErrNotFound, nil},
		{"body too large", fmt.Errorf("read: %w", source.ErrBodyTooLarge),
			fetch.BodyTooLarge, source.ErrBodyTooLarge, nil},
		{"fetch-time requirement mismatch", mismatch,
			fetch.PrecheckMismatch, nil, &mismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := newStub("bad", source.Capabilities{},
				func(context.Context, source.Request) (source.Result, error) {
					return source.Result{}, tc.failure
				})
			good := newStub("good", source.Capabilities{}, nil)
			svc := newService(t, bad, good)
			res, warnings := mustFetch(t, svc, baseParams("bad", "good"))
			if res.Source != "good" || res.Lyrics != "plain lyrics" ||
				bad.calls != 1 || good.calls != 1 {
				t.Fatalf("result/calls = %+v/%d/%d, want good fallback",
					res, bad.calls, good.calls)
			}
			w := requireOneWarning(t, warnings, tc.wantKind)
			if w.Source != "bad" {
				t.Fatalf("warning source = %q", w.Source)
			}
			if tc.wantIs != nil && !errors.Is(w.Err, tc.wantIs) {
				t.Fatalf("warning error = %v", w.Err)
			}
			if tc.wantMM != nil {
				got := requireAs[source.RequiredParamMismatchError](t, w.Err)
				if got != *tc.wantMM || w.Param != tc.wantMM.Param ||
					w.ParamName != tc.wantMM.ParamName {
					t.Fatalf("mismatch = %+v warning = %+v", got, w)
				}
			}
		})
	}
}

func TestSyncLevels(t *testing.T) {
	cases := []struct {
		name       string
		levels     []fetch.SyncLevel
		result     source.Result
		wantResult bool
		wantLevel  fetch.SyncLevel
		wantWants  []fetch.SyncLevel
		wantCalls  int
	}{
		{"line falls back to plain and caches",
			[]fetch.SyncLevel{fetch.SyncLine, fetch.SyncNone},
			plainResult("plain lyrics"), true, fetch.SyncNone,
			[]fetch.SyncLevel{fetch.SyncLine}, 1},
		{"plain request rejects lrc only", []fetch.SyncLevel{fetch.SyncNone},
			lyricsResult(source.SyncLine, "[00:01]line"), false, 0,
			[]fetch.SyncLevel{fetch.SyncNone}, 1},
		{"none then line reuses lrc",
			[]fetch.SyncLevel{fetch.SyncNone, fetch.SyncLine},
			lyricsResult(source.SyncLine, "[00:01]line"), true, fetch.SyncLine,
			[]fetch.SyncLevel{fetch.SyncNone}, 1},
		{"word exact match returns ttml", []fetch.SyncLevel{fetch.SyncWord},
			lyricsResult(source.SyncWord, "<tt>word</tt>"), true, fetch.SyncWord,
			nil, 1},
		{"both requested levels fail",
			[]fetch.SyncLevel{fetch.SyncLine, fetch.SyncWord},
			plainResult("plain lyrics"), false, 0,
			[]fetch.SyncLevel{fetch.SyncLine, fetch.SyncWord}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := resultStub("only", tc.result)
			got, warnings, err := newService(t, src).Fetch(
				context.Background(), levelParams(tc.levels, "only"))
			if src.calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", src.calls, tc.wantCalls)
			}
			gotWants := make([]fetch.SyncLevel, len(warnings))
			for i, w := range warnings {
				if w.Kind != fetch.Downgraded || w.Source != "only" {
					t.Fatalf("warning = %+v, want a downgrade from only", w)
				}
				gotWants[i] = w.Want
			}
			want := slices.Clone(tc.wantWants)
			slices.Sort(gotWants)
			slices.Sort(want)
			if !slices.Equal(gotWants, want) {
				t.Fatalf("downgrade wants = %v, want %v", gotWants, want)
			}
			if !tc.wantResult {
				requireAs[fetch.NoResultError](t, err)
				return
			}
			if err != nil {
				t.Fatalf("Fetch error = %v", err)
			}
			if got.Level != tc.wantLevel || got.Lyrics != tc.result.Lyrics ||
				got.Source != "only" {
				t.Fatalf("result = %+v", got)
			}
		})
	}
}

func TestResultTrust(t *testing.T) {
	cases := []struct {
		name         string
		result       source.Result
		wantUsed     bool
		wantDeclared bool
		wantField    source.ResultField
	}{
		{"declared but empty metadata is used",
			source.Result{Filled: source.FieldLyrics | source.FieldTitle,
				Lyrics: "plain lyrics", Level: source.SyncNone},
			true, true, source.FieldTitle},
		{"filled but undeclared metadata is dropped",
			source.Result{Filled: source.FieldLyrics, Lyrics: "plain lyrics",
				Level: source.SyncNone, Title: "ghost"},
			true, false, source.FieldTitle},
		{"declared empty lyrics is no result",
			source.Result{Filled: source.FieldLyrics, Lyrics: "   ",
				Level: source.SyncNone},
			false, true, source.FieldLyrics},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService(t, resultStub("trust", tc.result))
			got, warnings, err := svc.Fetch(context.Background(),
				baseParams("trust"))
			if tc.wantUsed {
				if err != nil {
					t.Fatalf("Fetch error = %v", err)
				}
				if got.Lyrics != "plain lyrics" || got.Title != "" {
					t.Fatalf("result = %+v", got)
				}
			} else {
				requireAs[fetch.NoResultError](t, err)
			}
			w := requireOneWarning(t, warnings, fetch.ResultMismatch)
			if w.Source != "trust" || w.Declared != tc.wantDeclared ||
				w.Field != tc.wantField {
				t.Fatalf("result warning = %+v", w)
			}
		})
	}
}
