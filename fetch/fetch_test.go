package fetch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

const allFilters = source.ParamAuthor | source.ParamAlbum |
	source.ParamISRC | source.ParamDuration

type stub struct {
	name   string
	static []source.ParamSpec
	caps   func(source.Request) source.Capabilities
	fetch  func(context.Context, source.Request) (source.Result, error)
	calls  int
}

func (s *stub) Name() string                     { return s.name }
func (s *stub) CustomParams() []source.ParamSpec { return s.static }

func (s *stub) Capabilities(req source.Request) source.Capabilities {
	return s.caps(req)
}

func (s *stub) Fetch(ctx context.Context,
	req source.Request,
) (source.Result, error) {
	s.calls++
	return s.fetch(ctx, req)
}

func newStub(name string, caps source.Capabilities,
	run func(context.Context, source.Request) (source.Result, error),
) *stub {
	if run == nil {
		run = func(context.Context, source.Request) (source.Result, error) {
			return plainResult("plain lyrics"), nil
		}
	}
	return &stub{name: name, caps: func(source.Request) source.Capabilities {
		return caps
	}, fetch: run}
}

func declaredStub(name string, static []source.ParamSpec,
	caps source.Capabilities,
) *stub {
	s := newStub(name, caps, nil)
	s.static = static
	return s
}

func resultStub(name string, res source.Result) *stub {
	return newStub(name, source.Capabilities{},
		func(context.Context, source.Request) (source.Result, error) {
			return res, nil
		})
}

func newService(t *testing.T, stubs ...*stub) *fetch.Service {
	t.Helper()
	reg := source.NewRegistry()
	for _, s := range stubs {
		if err := reg.Register(s); err != nil {
			t.Fatalf("register %q: %v", s.name, err)
		}
	}
	return fetch.New(reg)
}

func baseParams(sources ...string) fetch.Params {
	return fetch.Params{Song: "Song", Source: sources,
		SyncLevels: []fetch.SyncLevel{fetch.SyncNone}}
}

func levelParams(levels []fetch.SyncLevel, sources ...string) fetch.Params {
	p := baseParams(sources...)
	p.SyncLevels = levels
	return p
}

func lyricsResult(level source.SyncLevel, lyrics string) source.Result {
	return source.Result{Filled: source.FieldLyrics, Lyrics: lyrics, Level: level}
}

func plainResult(lyrics string) source.Result {
	return lyricsResult(source.SyncNone, lyrics)
}

// warnKey identifies a warning for order-insensitive set comparison.
type warnKey struct {
	source    string
	param     source.Param
	paramName string
	hasErr    bool
}

func mustFetch(t *testing.T, svc *fetch.Service,
	params fetch.Params,
) (fetch.Result, []fetch.Warning) {
	t.Helper()
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("Fetch error = %v", err)
	}
	return res, warnings
}

func requireAs[T error](t *testing.T, err error) T {
	t.Helper()
	var found T
	if !errors.As(err, &found) {
		t.Fatalf("error = %v, want %T", err, found)
	}
	return found
}

func requireNoCalls(t *testing.T, stubs []*stub) {
	t.Helper()
	for _, s := range stubs {
		if s.calls != 0 {
			t.Fatalf("%q calls = %d, want 0", s.name, s.calls)
		}
	}
}

func requireNoWarnings(t *testing.T, warnings []fetch.Warning) {
	t.Helper()
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
}

func requireOneWarning(t *testing.T, warnings []fetch.Warning,
	kind fetch.WarningKind,
) fetch.Warning {
	t.Helper()
	if len(warnings) != 1 || warnings[0].Kind != kind {
		t.Fatalf("warnings = %+v, want exactly one of kind %d", warnings, kind)
	}
	return warnings[0]
}

func warningsOfKind(warnings []fetch.Warning,
	kind fetch.WarningKind,
) []fetch.Warning {
	out := make([]fetch.Warning, 0, len(warnings))
	for _, w := range warnings {
		if w.Kind == kind {
			out = append(out, w)
		}
	}
	return out
}
