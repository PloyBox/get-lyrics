package fetch

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// TestFetch_AllParamsSupportedEmitsNoWarnings drives the clean path: every
// supplied filter is honored, so no warning is produced, and the result
// carries the adapter's own Level plus the backfilled Source name.
func TestFetch_AllParamsSupportedEmitsNoWarnings(t *testing.T) {
	full := &fakeSrc{
		name: "full",
		caps: source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum | source.ParamISRC},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			text := "x"
			level := source.SyncNone
			if r.SyncLevel == source.SyncLine {
				text = "[00:00.00] x"
				level = source.SyncLine
			}
			return source.Result{
				Lyrics: text,
				Level:  level,
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, full)
	svc := New(r)

	params := Params{Song: "S", Author: "A", Album: "B", ISRC: "I", SyncLevels: []SyncLevel{SyncLine}, Source: []string{"full"}}
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "full" {
		t.Fatalf("Source = %q; want full (backfill)", res.Source)
	}
	if res.Level != SyncLine {
		t.Fatalf("Level = %v; want line (the level the adapter declared)", res.Level)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

// TestFetch_UndeclaredFieldsDroppedAndWarned proves the trust policy in both
// directions: fields the adapter filled without declaring them never reach
// the caller, and each one produces a ResultMismatch warning.
func TestFetch_UndeclaredFieldsDroppedAndWarned(t *testing.T) {
	sloppy := &fakeSrc{
		name: "sloppy",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics:    "L",
				Level:     source.SyncNone,
				Title:     r.Song,
				Artist:    "ghost artist",
				Album:     "ghost album",
				SubSource: "ghost sub",
				Filled:    source.FieldLyrics | source.FieldTitle,
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
	if len(warnings) != 3 {
		t.Fatalf("warnings = %+v; want 3 ResultMismatch warnings", warnings)
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
