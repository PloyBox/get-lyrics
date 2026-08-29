package fetch

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// TestFetch_SyncedRequestOnPlainOnlySourceDowngrades drives the
// Downgraded path in isolation: a synced-only request on a plain-only
// source produces the warning, and with no "none" iteration left to
// match the cached plain result, Fetch ends in NoResultError.
func TestFetch_SyncedRequestOnPlainOnlySourceDowngrades(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "x",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
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
	stub := &fakeSrc{
		name: "stub",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine, SyncNone}, Source: []string{"stub"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncNone || res.Lyrics != "plain" {
		t.Fatalf("res = %+v; want plain lyrics", res)
	}
	if stub.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (cache hit on second iteration)", stub.fetchCalls)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded {
		t.Fatalf("warnings = %+v; want one Downgraded warning", warnings)
	}
}

// TestFetch_SyncLevelOrderDeterminesPriority proves the user-given
// sync level order is the priority: with "none,line" a plain result from
// the first iteration matches and is returned before any synced request.
func TestFetch_SyncLevelOrderDeterminesPriority(t *testing.T) {
	lrc := &fakeSrc{
		name: "lrc",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			res := source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}
			if r.SyncLevel == source.SyncLine {
				res.SyncedLyrics = "[00:00.00] synced"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
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
	lrc := &fakeSrc{
		name: "lrc",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			res := source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}
			if r.SyncLevel == source.SyncLine {
				res.SyncedLyrics = "[00:00.00] synced"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
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
// hit: the adapter declares FieldSyncedLyrics (and no FieldLyrics), and
// the synced track becomes the fetch result's Lyrics.
func TestFetch_SyncedOnlyResultFillsLyrics(t *testing.T) {
	syncedOnly := &fakeSrc{
		name: "synced-only",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				SyncedLyrics: "[00:00.00] synced line",
				Title:        r.Song,
				Filled:       source.FieldSyncedLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, syncedOnly)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"synced-only"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Level != SyncLine || res.Lyrics != "[00:00.00] synced line" {
		t.Fatalf("res = %+v; want synced lyrics from the declared synced track", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none for a consistent result", warnings)
	}
}

// TestFetch_PlainRequestOnSyncedOnlySourceNoResult is the empty-success
// regression: a synced-only adapter (FieldSyncedLyrics declared, no
// FieldLyrics) with a lone "none" iteration must NOT silently succeed
// with empty lyrics — the synced track cannot match the plain request,
// so the fetch ends in NoResultError with one symmetric Downgraded
// warning.
func TestFetch_PlainRequestOnSyncedOnlySourceNoResult(t *testing.T) {
	stub := &fakeSrc{
		name: "synconly",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				SyncedLyrics: "[00:00.00] synced",
				Title:        r.Song,
				Filled:       source.FieldSyncedLyrics | source.FieldTitle,
			}, nil
		},
	}
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
	stub := &fakeSrc{
		name: "synconly",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				SyncedLyrics: "[00:00.00] synced",
				Title:        r.Song,
				Filled:       source.FieldSyncedLyrics | source.FieldTitle,
			}, nil
		},
	}
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

// TestFetch_DualTrackLineRoundShortCircuitsWithoutCaching locks the
// short-circuit semantics: a dual-track source (plain + synced both
// declared) with a "line" request matches the synced track and returns
// immediately, storing nothing — so a follow-up "none" call misses the
// (per-call) cache and must fetch again, returning plain.
func TestFetch_DualTrackLineRoundShortCircuitsWithoutCaching(t *testing.T) {
	dual := &fakeSrc{
		name: "dual",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			res := source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}
			if r.SyncLevel == source.SyncLine {
				res.SyncedLyrics = "[00:00.00] synced"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
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
