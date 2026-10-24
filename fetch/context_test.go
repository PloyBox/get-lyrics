package fetch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

func TestContextStop(t *testing.T) {
	cases := []struct {
		name                      string
		cancelBefore, failInFetch bool
		wantCalls                 int
	}{
		{"already cancelled before loop", true, false, 0},
		{"cancelled while a call fails", false, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelBefore {
				cancel()
			}
			src := newStub("plain", source.Capabilities{},
				func(context.Context, source.Request) (source.Result, error) {
					if tc.failInFetch {
						cancel()
						return source.Result{}, errors.New("adapter error")
					}
					return plainResult("plain lyrics"), nil
				})
			_, warnings, err := newService(t, src).Fetch(ctx, baseParams("plain"))
			if src.calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", src.calls, tc.wantCalls)
			}
			stopped := requireAs[fetch.StoppedError](t, err)
			if !errors.Is(stopped.Cause, context.Canceled) ||
				!errors.Is(err, context.Canceled) {
				t.Fatalf("error %v does not unwrap to Canceled", err)
			}
			requireNoWarnings(t, warnings)
		})
	}
}

func TestUnsupported(t *testing.T) {
	t.Run("typed and custom inputs", func(t *testing.T) {
		params := baseParams("bare")
		params.Author, params.Album = "A", "B"
		params.ISRC, params.Duration = "C", 10
		params.Custom = map[string]string{"UNKNOWN": "value"}
		_, warnings := mustFetch(t,
			newService(t, newStub("bare", source.Capabilities{}, nil)), params)
		if len(warnings) != 5 {
			t.Fatalf("warnings = %+v, want 5 unsupported", warnings)
		}
		want := map[source.Param]bool{
			source.ParamAuthor: true, source.ParamAlbum: true,
			source.ParamISRC: true, source.ParamDuration: true,
		}
		custom := 0
		for _, w := range warningsOfKind(warnings, fetch.UnsupportedParam) {
			if w.Source != "bare" {
				t.Fatalf("warning = %+v", w)
			}
			if w.ParamName == "UNKNOWN" {
				custom++
				if w.Param != 0 {
					t.Fatalf("custom warning = %+v", w)
				}
				continue
			}
			if w.ParamName != "" {
				t.Fatalf("typed warning = %+v", w)
			}
			delete(want, w.Param)
		}
		if len(want) != 0 || custom != 1 {
			t.Fatalf("missing typed warnings: %v, custom = %d", want, custom)
		}
	})

	t.Run("supported inputs warn nothing", func(t *testing.T) {
		spec := []source.ParamSpec{{Name: "KNOWN", Description: "test"}}
		src := declaredStub("full", spec,
			source.Capabilities{Filters: allFilters, Custom: spec})
		params := baseParams("full")
		params.Author, params.Album = "A", "B"
		params.ISRC, params.Duration = "C", 10
		params.Custom = map[string]string{"KNOWN": "value"}
		_, warnings := mustFetch(t, newService(t, src), params)
		requireNoWarnings(t, warnings)
	})

	t.Run("no duplicates across sync levels", func(t *testing.T) {
		params := levelParams(
			[]fetch.SyncLevel{fetch.SyncLine, fetch.SyncNone}, "bare")
		params.Author = "A"
		res, warnings := mustFetch(t,
			newService(t, newStub("bare", source.Capabilities{}, nil)), params)
		if res.Level != fetch.SyncNone {
			t.Fatalf("result = %+v", res)
		}
		got := warningsOfKind(warnings, fetch.UnsupportedParam)
		if len(warnings) != 2 || len(got) != 1 ||
			got[0].Param != source.ParamAuthor {
			t.Fatalf("warnings = %+v, want 2 with one author warning", warnings)
		}
	})
}
