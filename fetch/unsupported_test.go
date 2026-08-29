package fetch

import (
	"context"
	"reflect"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

func TestFetch_AllParamsUnsupportedEmitsThreeWarnings(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "L",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	params := Params{Song: "Song", Author: "A", Album: "B", ISWC: "I", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"stub"}}
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L" {
		t.Fatalf("res.Lyrics = %q; want %q", res.Lyrics, "L")
	}
	wantParams := []source.Param{source.ParamAuthor, source.ParamAlbum, source.ParamISWC}
	gotParams := make([]source.Param, len(warnings))
	for i, w := range warnings {
		gotParams[i] = w.Param
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("warnings order = %v; want %v", gotParams, wantParams)
	}
	for _, w := range warnings {
		if w.Kind != UnsupportedParam {
			t.Fatalf("warning %+v; want Kind UnsupportedParam", w)
		}
	}
}

func TestDetectUnsupported_EmptyRequestYieldsNoWarnings(t *testing.T) {
	stub := &fakeSrc{name: "stub", caps: source.Capabilities{}, fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
		return source.Result{}, nil
	}}
	got := detectUnsupported(Params{Song: "S"}, stub)
	if len(got) != 0 {
		t.Fatalf("got %+v; want none", got)
	}
}

func TestDetectUnsupported_ParamOrder(t *testing.T) {
	// Lock in stable warning ordering: author, album, iswc. The
	// sync level is intentionally absent — it is covered by the
	// Downgraded warning instead.
	stub := &fakeSrc{name: "stub", caps: source.Capabilities{}, fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
		return source.Result{}, nil
	}}
	got := detectUnsupported(
		Params{Song: "S", Author: "A", Album: "B", ISWC: "I", SyncLevels: []SyncLevel{SyncLine}},
		stub,
	)
	want := []source.Param{source.ParamAuthor, source.ParamAlbum, source.ParamISWC}
	gotParams := make([]source.Param, len(got))
	for i, w := range got {
		gotParams[i] = w.Param
	}
	if !reflect.DeepEqual(gotParams, want) {
		t.Fatalf("got order %v; want %v", gotParams, want)
	}
}

func TestDetectUnsupported_WhitespaceOnlyParamsIgnored(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, nil
		},
	}
	got := detectUnsupported(Params{Song: "S", Author: " ", Album: "\t", ISWC: "  "}, stub)
	if len(got) != 0 {
		t.Fatalf("got %+v; want none for whitespace-only params", got)
	}
}

// TestDetectUnsupported_ConditionalAlbumWithoutAuthor drives the
// request-aware capability query (lrclib semantics): an adapter that
// honors --album only when --author is present must produce an
// unsupported warning for the album filter when author is absent, and
// none when author is present.
func TestDetectUnsupported_ConditionalAlbumWithoutAuthor(t *testing.T) {
	cond := &fakeSrc{
		name: "cond",
		capsFn: func(req source.Request) source.Capabilities {
			c := source.Capabilities{Filters: source.ParamAuthor | source.ParamAlbum}
			if req.Author == "" {
				c.Filters &^= source.ParamAlbum
			}
			return c
		},
	}
	got := detectUnsupported(Params{Song: "S", Album: "B"}, cond)
	if len(got) != 1 || got[0].Param != source.ParamAlbum {
		t.Fatalf("got %+v; want one Album unsupported warning without author", got)
	}
	got = detectUnsupported(Params{Song: "S", Author: "A", Album: "B"}, cond)
	if len(got) != 0 {
		t.Fatalf("got %+v; want no warnings when author is present", got)
	}
}

// TestFetch_ConditionalAlbumWarnsWithoutAuthor locks the defect fix
// end to end: the fetch pipeline surfaces the unsupported warning when
// --album is given without --author to a conditionally-supporting
// adapter, and stays silent when both are present.
func TestFetch_ConditionalAlbumWarnsWithoutAuthor(t *testing.T) {
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
			return source.Result{
				Lyrics: "L",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, cond)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Album: "B", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"cond"}})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want lyrics", res, err)
	}
	if len(warnings) != 1 || warnings[0].Kind != UnsupportedParam || warnings[0].Param != source.ParamAlbum {
		t.Fatalf("warnings = %+v; want one Album unsupported warning", warnings)
	}

	res, warnings, err = svc.Fetch(context.Background(), Params{Song: "S", Author: "A", Album: "B", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"cond"}})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want lyrics", res, err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none when author is present", warnings)
	}
}
