package fetch

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// plainOnly returns a source that only ever fills plain (SyncNone) lyrics.
func plainOnly(name string, fetchCalls *int) *fakeSrc {
	return &fakeSrc{
		name: name,
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			if fetchCalls != nil {
				*fetchCalls++
			}
			return source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Level:  source.SyncNone,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
}

// lineOrPlain returns a source that returns LRC (SyncLine) lyrics for a
// line request and plain (SyncNone) lyrics otherwise.
func lineOrPlain(name string) *fakeSrc {
	return &fakeSrc{
		name: name,
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			text := "plain"
			level := source.SyncNone
			if r.SyncLevel == source.SyncLine {
				text = "[00:00.00] synced"
				level = source.SyncLine
			}
			return source.Result{
				Lyrics: text,
				Title:  r.Song,
				Level:  level,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
}

// lineOnly returns a source that only ever fills LRC (SyncLine) lyrics.
func lineOnly(name string) *fakeSrc {
	return &fakeSrc{
		name: name,
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "[00:00.00] synced",
				Title:  r.Song,
				Level:  source.SyncLine,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
}

// wordOrPlain returns a source that returns TTML (SyncWord) lyrics for
// a word request and plain (SyncNone) lyrics otherwise.
func wordOrPlain(name string) *fakeSrc {
	return &fakeSrc{
		name: name,
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			text := "plain"
			level := source.SyncNone
			if r.SyncLevel == source.SyncWord {
				text = "<tt><span begin=\"00:00:01.000\" end=\"00:00:02.000\">word</span></tt>"
				level = source.SyncWord
			}
			return source.Result{
				Lyrics: text,
				Title:  r.Song,
				Level:  level,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
}

// TestFetch_SyncedRequestOnPlainOnlySourceDowngrades drives the
// Downgraded path in isolation: a synced-only request on a plain-only
// source produces the warning, and with no "none" iteration left to
// match the cached plain result, Fetch ends in NoResultError.
func TestFetch_SyncedRequestOnPlainOnlySourceDowngrades(t *testing.T) {
	stub := plainOnly("stub", nil)
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"stub"}})
	if res.Lyrics != "" {
		t.Fatalf("res = %+v; want empty result on NoResultError", res)
	}
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded {
		t.Fatalf("warnings = %+v; want one Downgraded warning", warnings)
	}
}

// TestFetch_DefaultLineNoneReusesDowngradedResult proves the per-call
// cache: after the "line" iteration stores the plain result (Downgraded
// warning), the "none" iteration matches it and returns without a
// second adapter call.
func TestFetch_DefaultLineNoneReusesDowngradedResult(t *testing.T) {
	calls := 0
	stub := plainOnly("stub", &calls)
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine, SyncNone}, Source: []string{"stub"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncNone || res.Lyrics != "plain" {
		t.Fatalf("res = %+v; want plain lyrics", res)
	}
	if calls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (cache hit on second iteration)", calls)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded {
		t.Fatalf("warnings = %+v; want one Downgraded warning", warnings)
	}
}

// TestFetch_SyncLevelOrderDeterminesPriority proves the user-given
// sync level order is the priority: with "none,line" a plain result from
// the first iteration matches and is returned before any synced request.
func TestFetch_SyncLevelOrderDeterminesPriority(t *testing.T) {
	lrc := lineOrPlain("lrc")
	r := newRegistry(t, lrc)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone, SyncLine}, Source: []string{"lrc"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncNone || res.Lyrics != "plain" {
		t.Fatalf("res = %+v; want plain lyrics from the first iteration", res)
	}
	if lrc.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1", lrc.fetchCalls)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

func TestFetch_SyncedSourceMatchesLineIteration(t *testing.T) {
	lrc := lineOrPlain("lrc")
	r := newRegistry(t, lrc)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"lrc"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want synced lyrics", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

// TestFetch_SyncedOnlyResultFillsLyrics drives the lrclib synced-only
// hit: the adapter fills the single Lyrics field with LRC content and
// declares Level SyncLine; the fetch layer matches it against a line
// request.
func TestFetch_SyncedOnlyResultFillsLyrics(t *testing.T) {
	syncedOnly := lineOnly("synced-only")
	r := newRegistry(t, syncedOnly)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"synced-only"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want synced lyrics from the declared level", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none for a consistent result", warnings)
	}
}

// TestFetch_PlainRequestOnSyncedOnlySourceNoResult is the empty-success
// regression: a synced-only adapter (Level SyncLine) with a lone "none"
// iteration must NOT silently succeed with empty lyrics — the synced
// track cannot match the plain request, so the fetch ends in
// NoResultError with one symmetric Downgraded warning.
func TestFetch_PlainRequestOnSyncedOnlySourceNoResult(t *testing.T) {
	stub := lineOnly("synconly")
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"synconly"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError instead of an empty success", err)
	}
	if res.Lyrics != "" || res.Level != SyncUnknown {
		t.Fatalf("res = %+v; want empty result on NoResultError", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded || warnings[0].Source != "synconly" {
		t.Fatalf("warnings = %+v; want one Downgraded warning for synconly", warnings)
	}
	if warnings[0].Want != SyncNone {
		t.Fatalf("warning = %+v; want Want=SyncNone (only synced lyrics returned)", warnings[0])
	}
}

// TestFetch_PlainThenSyncedReusesCachedSyncedTrack: with "none,line" on
// a synced-only source, the "none" iteration stores the synced track
// (Downgraded warning) and the "line" iteration matches it from the
// cache — one adapter call total.
func TestFetch_PlainThenSyncedReusesCachedSyncedTrack(t *testing.T) {
	stub := lineOnly("synconly")
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone, SyncLine}, Source: []string{"synconly"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want the cached synced track", res)
	}
	if stub.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (cache hit on second iteration)", stub.fetchCalls)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded {
		t.Fatalf("warnings = %+v; want one Downgraded warning", warnings)
	}
}

// TestFetch_LineRoundShortCircuitsWithoutCaching locks the
// short-circuit semantics: a line-capable source with a "line" request
// matches immediately and stores nothing — so a follow-up "none" call
// misses the (per-call) cache and must fetch again, returning plain.
func TestFetch_LineRoundShortCircuitsWithoutCaching(t *testing.T) {
	dual := lineOrPlain("dual")
	r := newRegistry(t, dual)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine, SyncNone}, Source: []string{"dual"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want the synced track from the line round", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}

	// Nothing was stored by the short-circuit, so a fresh call with
	// "none" misses the cache and fetches again.
	res, warnings, err = svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"dual"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncNone || res.Lyrics != "plain" {
		t.Fatalf("res = %+v; want plain lyrics from the none round", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
	if dual.fetchCalls != 2 {
		t.Fatalf("fetchCalls = %d; want 2 (short-circuit stores nothing)", dual.fetchCalls)
	}
}

// TestFetch_WordRequestMatchesWordSource drives the SyncWord match: a
// word-level request on a source that returns TTML with Level SyncWord
// matches immediately.
func TestFetch_WordRequestMatchesWordSource(t *testing.T) {
	word := wordOrPlain("word")
	r := newRegistry(t, word)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncWord}, Source: []string{"word"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncWord || res.Lyrics != "<tt><span begin=\"00:00:01.000\" end=\"00:00:02.000\">word</span></tt>" {
		t.Fatalf("res = %+v; want the word-level track", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

// TestFetch_WordRequestOnLineOnlySourceDowngrades drives the word-side
// Downgraded direction: a word request on a source that only produces
// LRC (SyncLine) lyrics yields a Downgraded warning with Want=SyncWord.
func TestFetch_WordRequestOnLineOnlySourceDowngrades(t *testing.T) {
	stub := lineOnly("lrc")
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncWord}, Source: []string{"lrc"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if res.Lyrics != "" {
		t.Fatalf("res = %+v; want empty result on NoResultError", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded || warnings[0].Want != SyncWord {
		t.Fatalf("warnings = %+v; want one Downgraded warning with Want=SyncWord", warnings)
	}
}

// TestFetch_WordThenLineReusesCachedLineTrack: with "word,line" on a
// line-only source, the "word" iteration stores the line track
// (Downgraded warning) and the "line" iteration matches it from the
// cache — one adapter call total.
func TestFetch_WordThenLineReusesCachedLineTrack(t *testing.T) {
	stub := lineOnly("lrc")
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncWord, SyncLine}, Source: []string{"lrc"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want the cached line track", res)
	}
	if stub.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (cache hit on second iteration)", stub.fetchCalls)
	}
	if len(warnings) != 1 || warnings[0].Want != SyncWord {
		t.Fatalf("warnings = %+v; want one Downgraded warning with Want=SyncWord", warnings)
	}
}
