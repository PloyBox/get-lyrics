package fetch

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// TestFetch_AdapterErrorFailsOverToNextSource verifies that a failing
// adapter produces a FetchFailed warning and the loop moves on to the
// next source instead of aborting.
func TestFetch_AdapterErrorFailsOverToNextSource(t *testing.T) {
	bad := &fakeSrc{
		name: "bad",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, errors.New("boom")
		},
	}
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "L",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, bad, ok)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"bad", "ok"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "ok" || res.Lyrics != "L" {
		t.Fatalf("res = %+v; want result from ok", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != FetchFailed || warnings[0].Source != "bad" {
		t.Fatalf("warnings = %+v; want one FetchFailed warning for bad", warnings)
	}
}

func TestFetch_AllSourcesFailReturnsNoResult(t *testing.T) {
	bad := &fakeSrc{
		name: "bad",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, errors.New("boom")
		},
	}
	r := newRegistry(t, bad)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"bad"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != FetchFailed {
		t.Fatalf("warnings = %+v; want one FetchFailed warning", warnings)
	}
}
