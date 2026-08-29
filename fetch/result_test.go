package fetch

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

func TestFetch_AllParamsSupportedEmitsNoWarnings(t *testing.T) {
	full := &fakeSrc{
		name: "full",
		caps: source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum | source.ParamISWC},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			res := source.Result{
				Lyrics: "x",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}
			if r.SyncLevel == source.SyncLine {
				res.SyncedLyrics = "[00:00.00] x"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
	r := newRegistry(t, full)
	svc := New(r)

	params := Params{Song: "S", Author: "A", Album: "B", ISWC: "I", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"full"}}
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "full" {
		t.Fatalf("Source = %q; want full (backfill)", res.Source)
	}
	if res.SubSource != "" {
		t.Fatalf("SubSource = %q; want empty for standalone adapter", res.SubSource)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

func TestFetch_AggregateSourceSubSource(t *testing.T) {
	agg := &fakeSrc{
		name: "agg",
		caps: source.Capabilities{Filters: source.ParamAuthor},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "x", SubSource: "sub", Filled: source.FieldLyrics | source.FieldSubSource}, nil
		},
	}
	r := newRegistry(t, agg)
	svc := New(r)

	res, _, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"agg"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "agg" {
		t.Fatalf("Source = %q; want agg", res.Source)
	}
	if res.SubSource != "sub" {
		t.Fatalf("SubSource = %q; want sub (no concatenation)", res.SubSource)
	}
}

// TestFetch_ResultUsesOnlyDeclaredFields locks the mask semantics: the
// fetch layer must not read fields the adapter did not declare, and
// every undeclared-but-filled field produces a ResultMismatch warning
// (the result is still used as-is — trust policy).
func TestFetch_ResultUsesOnlyDeclaredFields(t *testing.T) {
	sloppy := &fakeSrc{
		name: "sloppy",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics:       "L",
				SyncedLyrics: "[00:00.00] ghost synced",
				Title:        r.Song,
				Artist:       "ghost artist",
				Album:        "ghost album",
				SubSource:    "ghost sub",
				Filled:       source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, sloppy)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"sloppy"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L" || res.Title != "S" || res.Level != SyncNone {
		t.Fatalf("res = %+v; want only declared lyrics/title", res)
	}
	if res.Artist != "" || res.Album != "" || res.SubSource != "" {
		t.Fatalf("res = %+v; undeclared fields must not be read", res)
	}
	if len(warnings) != 4 {
		t.Fatalf("warnings = %+v; want 4 ResultMismatch warnings", warnings)
	}
	for _, w := range warnings {
		if w.Kind != ResultMismatch {
			t.Fatalf("warning %+v; want Kind ResultMismatch", w)
		}
	}
}

// TestFetch_DeclaredButEmptyFieldWarns proves a declared bit with an
// empty value is flagged as a source implementation problem, and that
// it produces no result track: with no usable lyrics the fetch cannot
// match any format and ends in NoResultError (the mismatch warning is
// still emitted).
func TestFetch_DeclaredButEmptyFieldWarns(t *testing.T) {
	empty := &fakeSrc{
		name: "empty",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, empty)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"empty"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError (no usable lyrics produced)", err)
	}
	if res.Lyrics != "" || res.Title != "" || res.Level != SyncUnknown {
		t.Fatalf("res = %+v; want empty result on NoResultError", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != ResultMismatch {
		t.Fatalf("warnings = %+v; want one ResultMismatch warning", warnings)
	}
	if warnings[0].Field != source.FieldLyrics || !warnings[0].Declared {
		t.Fatalf("warning = %+v; want Field=Lyrics, Declared=true", warnings[0])
	}
}

// TestFetch_DeclaredButEmptySubSourceWarns locks the mask semantics for
// the provenance field: an aggregate adapter that declares
// FieldSubSource but leaves the value empty is flagged the same way as
// any other declared-but-empty field.
func TestFetch_DeclaredButEmptySubSourceWarns(t *testing.T) {
	empty := &fakeSrc{
		name: "empty",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics:    "L",
				SubSource: " ",
				Filled:    source.FieldLyrics | source.FieldSubSource,
			}, nil
		},
	}
	r := newRegistry(t, empty)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"empty"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L" {
		t.Fatalf("res = %+v; want lyrics preserved despite the mismatch", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != ResultMismatch {
		t.Fatalf("warnings = %+v; want one ResultMismatch warning", warnings)
	}
	if warnings[0].Field != source.FieldSubSource || !warnings[0].Declared {
		t.Fatalf("warning = %+v; want Field=SubSource, Declared=true", warnings[0])
	}
}
