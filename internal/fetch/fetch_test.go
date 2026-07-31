package fetch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type fakeSrc struct {
	name  string
	sup   source.Param
	fetch func(context.Context, source.Request) (source.Result, error)
}

func (f *fakeSrc) Name() string                  { return f.name }
func (f *fakeSrc) SupportedParams() source.Param { return f.sup }
func (f *fakeSrc) Fetch(ctx context.Context, r source.Request) (source.Result, error) {
	return f.fetch(ctx, r)
}

func newRegistry(t *testing.T, ss ...source.Source) *source.Registry {
	t.Helper()
	r := source.NewRegistry()
	for _, s := range ss {
		if err := r.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name(), err)
		}
	}
	return r
}

func TestFetch_UnknownSourceReturnsErrNotFound(t *testing.T) {
	r := newRegistry(t, &fakeSrc{name: "known"})
	svc := New(r)
	_, warnings, err := svc.Fetch(context.Background(), source.Request{Song: "S"}, "nope")
	if !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v; want none", warnings)
	}
}

func TestFetch_AllParamsUnsupportedEmitsThreeWarnings(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		sup:  0,
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Title: r.Song, Source: "stub"}, nil
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	req := source.Request{Song: "Song", Author: "A", Album: "B", ISWC: "I"}
	res, warnings, err := svc.Fetch(context.Background(), req, "stub")
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
}

func TestFetch_AllParamsSupportedEmitsNoWarnings(t *testing.T) {
	full := &fakeSrc{
		name: "full",
		sup:  source.ParamAuthor | source.ParamAlbum | source.ParamISWC | source.ParamTimestamp,
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "x", Title: r.Song}, nil
		},
	}
	r := newRegistry(t, full)
	svc := New(r)

	req := source.Request{Song: "S", Author: "A", Album: "B", ISWC: "I", Timestamp: true}
	res, warnings, err := svc.Fetch(context.Background(), req, "full")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "full" {
		t.Fatalf("Source = %q; want full (backfill)", res.Source)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
}

func TestFetch_TimestampUnsupportedAddsWarning(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		sup:  0,
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "x", Title: r.Song, Source: "stub"}, nil
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), source.Request{Song: "S", Timestamp: true}, "stub")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.SyncedLyrics != "" {
		t.Fatalf("SyncedLyrics should be empty")
	}
	if len(warnings) != 1 || warnings[0].Param != source.ParamTimestamp {
		t.Fatalf("warnings = %+v; want one timestamp warning", warnings)
	}
}

func TestFetch_AdapterErrorDiscardsWarnings(t *testing.T) {
	stub := &fakeSrc{
		name: "stub",
		sup:  0,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, errors.New("boom")
		},
	}
	r := newRegistry(t, stub)
	svc := New(r)

	req := source.Request{Song: "S", Author: "A"}
	_, warnings, err := svc.Fetch(context.Background(), req, "stub")
	if err == nil {
		t.Fatalf("want non-nil err")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none on hard failure", warnings)
	}
}

func TestDetectUnsupported_EmptyRequestYieldsNoWarnings(t *testing.T) {
	stub := &fakeSrc{name: "stub", sup: 0, fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
		return source.Result{}, nil
	}}
	got := detectUnsupported(source.Request{Song: "S"}, stub)
	if len(got) != 0 {
		t.Fatalf("got %+v; want none", got)
	}
}

func TestDetectUnsupported_ParamOrder(t *testing.T) {
	// Lock in stable warning ordering: author, album, iswc, timestamp.
	stub := &fakeSrc{name: "stub", sup: 0, fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
		return source.Result{}, nil
	}}
	got := detectUnsupported(
		source.Request{Song: "S", Author: "A", Album: "B", ISWC: "I", Timestamp: true},
		stub,
	)
	want := []source.Param{source.ParamAuthor, source.ParamAlbum, source.ParamISWC, source.ParamTimestamp}
	gotParams := make([]source.Param, len(got))
	for i, w := range got {
		gotParams[i] = w.Param
	}
	if !reflect.DeepEqual(gotParams, want) {
		t.Fatalf("got order %v; want %v", gotParams, want)
	}
}
