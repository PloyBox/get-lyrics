package fetch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type fakeSrc struct {
	name       string
	caps       source.Capabilities
	capsFn     func(source.Request) source.Capabilities
	fetch      func(context.Context, source.Request) (source.Result, error)
	fetchCalls int
}

func (f *fakeSrc) Name() string { return f.name }
func (f *fakeSrc) Capabilities(req source.Request) source.Capabilities {
	if f.capsFn != nil {
		return f.capsFn(req)
	}
	return f.caps
}
func (f *fakeSrc) Fetch(ctx context.Context, r source.Request) (source.Result, error) {
	f.fetchCalls++
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
	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"nope"}})
	if !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
	var unk UnknownSourceError
	if !errors.As(err, &unk) || unk.Name != "nope" {
		t.Fatalf("err = %v; want UnknownSourceError{Name: nope}", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v; want none", warnings)
	}
}

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

	params := Params{Song: "Song", Author: "A", Album: "B", ISWC: "I", Timestamp: []string{"none"}, Source: []string{"stub"}}
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
			if r.Timestamp {
				res.SyncedLyrics = "[00:00.00] x"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
	r := newRegistry(t, full)
	svc := New(r)

	params := Params{Song: "S", Author: "A", Album: "B", ISWC: "I", Timestamp: []string{"line"}, Source: []string{"full"}}
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
			return source.Result{Lyrics: "x", Source: "sub", Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, agg)
	svc := New(r)

	res, _, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"agg"}})
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"line"}, Source: []string{"stub"}})
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"line", "none"}, Source: []string{"stub"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Synced || res.Lyrics != "plain" {
		t.Fatalf("res = %+v; want plain lyrics", res)
	}
	if stub.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (cache hit on second iteration)", stub.fetchCalls)
	}
	if len(warnings) != 1 || warnings[0].Kind != Downgraded {
		t.Fatalf("warnings = %+v; want one Downgraded warning", warnings)
	}
}

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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"bad", "ok"}})
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

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"bad"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != FetchFailed {
		t.Fatalf("warnings = %+v; want one FetchFailed warning", warnings)
	}
}

// TestFetch_StrictPrecheckRequiredParamFailsFast proves the strict
// precheck aborts on the first missing required parameter without
// fetching any source — not even earlier eligible ones.
func TestFetch_StrictPrecheckRequiredParamFailsFast(t *testing.T) {
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L"}, nil
		},
	}
	req := &fakeSrc{
		name: "req",
		caps: source.Capabilities{Required: source.ParamAuthor},
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Lyrics: "R"}, nil
		},
	}
	r := newRegistry(t, ok, req)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"ok", "req"}})
	var reqErr source.RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want RequiredParamError", err)
	}
	if reqErr.Source != "req" || reqErr.Param != source.ParamAuthor || reqErr.Flag != "--author" {
		t.Fatalf("reqErr = %+v; want author-required for req", reqErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none on strict precheck failure", warnings)
	}
	if ok.fetchCalls != 0 {
		t.Fatalf("ok.fetchCalls = %d; want 0 (fail-fast before any fetch)", ok.fetchCalls)
	}
}

func TestFetch_StrictPrecheckUnknownSourceFailsFast(t *testing.T) {
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L"}, nil
		},
	}
	r := newRegistry(t, ok)
	svc := New(r)

	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"ok", "nope"}})
	if !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
	if ok.fetchCalls != 0 {
		t.Fatalf("ok.fetchCalls = %d; want 0 (fail-fast before any fetch)", ok.fetchCalls)
	}
}

// TestFetch_LenientSkipsProblemSources verifies --lenient semantics:
// sources failing precheck are skipped with a PreCheck warning, the
// eligible ones proceed.
func TestFetch_LenientSkipsProblemSources(t *testing.T) {
	req := &fakeSrc{
		name: "req",
		caps: source.Capabilities{Required: source.ParamAuthor},
	}
	nosup := &fakeSrc{
		name: "nosup",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "N",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, req, nosup)
	svc := New(r)

	params := Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"req", "nope", "nosup"}, Lenient: true}
	res, warnings, err := svc.Fetch(context.Background(), params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "nosup" {
		t.Fatalf("res.Source = %q; want nosup", res.Source)
	}
	wantKinds := []WarningKind{PreCheck, PreCheck}
	gotKinds := make([]WarningKind, len(warnings))
	for i, w := range warnings {
		gotKinds[i] = w.Kind
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("warning kinds = %v; want %v", gotKinds, wantKinds)
	}
	if warnings[0].Source != "req" || warnings[1].Source != "nope" {
		t.Fatalf("precheck warnings = %+v; want req then nope", warnings)
	}
}

func TestFetch_LenientAllSkippedReturnsNoResult(t *testing.T) {
	req := &fakeSrc{
		name: "req",
		caps: source.Capabilities{Required: source.ParamAuthor},
	}
	r := newRegistry(t, req)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"req", "nope"}, Lenient: true})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v; want two PreCheck warnings", warnings)
	}
	for _, w := range warnings {
		if w.Kind != PreCheck {
			t.Fatalf("warning %+v; want Kind PreCheck", w)
		}
	}
}

// TestFetch_TimestampOrderDeterminesPriority proves the user-given
// timestamp order is the priority: with "none,line" a plain result from
// the first iteration matches and is returned before any synced request.
func TestFetch_TimestampOrderDeterminesPriority(t *testing.T) {
	lrc := &fakeSrc{
		name: "lrc",
		caps: source.Capabilities{},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			res := source.Result{
				Lyrics: "plain",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}
			if r.Timestamp {
				res.SyncedLyrics = "[00:00.00] synced"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
	r := newRegistry(t, lrc)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none", "line"}, Source: []string{"lrc"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Synced || res.Lyrics != "plain" {
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
			if r.Timestamp {
				res.SyncedLyrics = "[00:00.00] synced"
				res.Filled |= source.FieldSyncedLyrics
			}
			return res, nil
		},
	}
	r := newRegistry(t, lrc)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"line"}, Source: []string{"lrc"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Synced || res.Lyrics != "[00:00.00] synced" {
		t.Fatalf("res = %+v; want synced lyrics", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
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
	// timestamp format is intentionally absent — it is covered by the
	// Downgraded warning instead.
	stub := &fakeSrc{name: "stub", caps: source.Capabilities{}, fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
		return source.Result{}, nil
	}}
	got := detectUnsupported(
		Params{Song: "S", Author: "A", Album: "B", ISWC: "I", Timestamp: []string{"line"}},
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

func TestFetch_DuplicateSourceStrictReturnsUsageError(t *testing.T) {
	src := &fakeSrc{
		name: "a",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L"}, nil
		},
	}
	r := newRegistry(t, src)
	svc := New(r)

	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"a", "a"}})
	var dup DuplicateSourceError
	if !errors.As(err, &dup) || dup.Name != "a" {
		t.Fatalf("err = %v; want DuplicateSourceError{Name: a}", err)
	}
	if src.fetchCalls != 0 {
		t.Fatalf("fetchCalls = %d; want 0", src.fetchCalls)
	}
}

func TestFetch_DuplicateSourceLenientDedupes(t *testing.T) {
	src := &fakeSrc{
		name: "a",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{
				Lyrics: "L",
				Title:  r.Song,
				Filled: source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, src)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"a", "a"}, Lenient: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Source != "a" || res.Lyrics != "L" {
		t.Fatalf("res = %+v; want result from a", res)
	}
	if src.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d; want 1 (duplicate dropped)", src.fetchCalls)
	}
	if len(warnings) != 1 || warnings[0].Kind != PreCheck || warnings[0].Source != "a" {
		t.Fatalf("warnings = %+v; want one duplicate PreCheck warning for a", warnings)
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Album: "B", Timestamp: []string{"none"}, Source: []string{"cond"}})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want lyrics", res, err)
	}
	if len(warnings) != 1 || warnings[0].Kind != UnsupportedParam || warnings[0].Param != source.ParamAlbum {
		t.Fatalf("warnings = %+v; want one Album unsupported warning", warnings)
	}

	res, warnings, err = svc.Fetch(context.Background(), Params{Song: "S", Author: "A", Album: "B", Timestamp: []string{"none"}, Source: []string{"cond"}})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want lyrics", res, err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none when author is present", warnings)
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"line"}, Source: []string{"synced-only"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Synced || res.Lyrics != "[00:00.00] synced line" {
		t.Fatalf("res = %+v; want synced lyrics from the declared synced track", res)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none for a consistent result", warnings)
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
				Filled:       source.FieldLyrics | source.FieldTitle,
			}, nil
		},
	}
	r := newRegistry(t, sloppy)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"sloppy"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L" || res.Title != "S" || res.Synced {
		t.Fatalf("res = %+v; want only declared lyrics/title", res)
	}
	if res.Artist != "" || res.Album != "" {
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
// empty value is flagged as a source implementation problem while the
// rest of the result is still used as-is (trust policy).
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Timestamp: []string{"none"}, Source: []string{"empty"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Title != "S" {
		t.Fatalf("res = %+v; want title preserved despite the mismatch", res)
	}
	if len(warnings) != 1 || warnings[0].Kind != ResultMismatch {
		t.Fatalf("warnings = %+v; want one ResultMismatch warning", warnings)
	}
	if !strings.Contains(warnings[0].Message, `declares field "Lyrics" but left it empty`) {
		t.Fatalf("warning message = %q; want declared-but-empty note", warnings[0].Message)
	}
}
