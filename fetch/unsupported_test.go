package fetch

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// TestFetch_AllParamsUnsupportedEmitsFourWarnings drives the typed
// unsupported-parameter path: every supplied filter a source does not honor
// produces one warning. The warning set is asserted, not its order.
func TestFetch_AllParamsUnsupportedEmitsFourWarnings(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "L",
				Title:  r.Song,
				Level:  source.SyncNone,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	params := Params{Song: "Song", Author: "A", Album: "B", ISRC: "I", Duration: 5, SyncLevels: []SyncLevel{SyncNone}, Source: []string{"stub"}}
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L" {
		t.Fatalf("res.Lyrics = %q; want %q", res.Lyrics, "L")
	}
	wantParams := []source.Param{source.ParamAuthor, source.ParamAlbum, source.ParamISRC, source.ParamDuration}
	gotParams := make([]source.Param, len(warnings))
	for i, w := range warnings {
		if w.Kind != UnsupportedParam {
			t.Fatalf("warning %+v; want Kind UnsupportedParam", w)
		}
		gotParams[i] = w.Param
	}
	sort.Slice(gotParams, func(i, j int) bool { return gotParams[i] < gotParams[j] })
	if !slices.Equal(gotParams, wantParams) {
		t.Fatalf("warning set = %v; want %v (order unspecified)", gotParams, wantParams)
	}
}

// TestFetch_ConditionalAlbumSupport drives request-aware capabilities: a
// source that honors --album only together with --author must warn about the
// dropped album when --author is absent, and stay silent when it is present.
func TestFetch_ConditionalAlbumSupport(t *testing.T) {
	cond := &fakeSrc{
		name: "cond",
		capsFn: func(req source.Request) source.Capabilities {
			c := source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum}
			if req.Author == "" {
				c.Filters &^= source.ParamAlbum
			}
			return c
		},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Title: r.Song, Level: source.SyncNone, Filled: source.FieldLyrics | source.FieldTitle}, nil
		},
	}
	svc := New(newRegistry(t, cond))

	for _, tc := range []struct {
		name       string
		params     Params
		wantParams []source.Param
	}{
		{
			"album without author",
			Params{Song: "S", Album: "B"},
			[]source.Param{source.ParamAlbum},
		},
		{
			"album with author",
			Params{Song: "S", Author: "A", Album: "B"},
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.params.SyncLevels = []SyncLevel{SyncNone}
			tc.params.Source = []string{"cond"}
			_, warnings, err := svc.Fetch(context.Background(), tc.params)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(warnings) != len(tc.wantParams) {
				t.Fatalf("warnings = %+v; want %d", warnings, len(tc.wantParams))
			}
			for i, w := range warnings {
				if w.Kind != UnsupportedParam || w.Param != tc.wantParams[i] {
					t.Fatalf("warning %d = %+v; want UnsupportedParam %v", i, w, tc.wantParams[i])
				}
			}
		})
	}
}
