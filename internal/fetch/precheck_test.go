package fetch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/PloyBox/get-lyrics/internal/source"
)

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
	var reqErr RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want RequiredParamError", err)
	}
	if reqErr.Source != "req" || reqErr.Param != source.ParamAuthor {
		t.Fatalf("reqErr = %+v; want author-required for req", reqErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none on strict precheck failure", warnings)
	}
	if ok.fetchCalls != 0 {
		t.Fatalf("ok.fetchCalls = %d; want 0 (fail-fast before any fetch)", ok.fetchCalls)
	}
}

func TestFetch_RequiredParamMismatchFailsOverWithWarning(t *testing.T) {
	buggy := &fakeSrc{
		name: "buggy",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, source.RequiredParamMismatchError{
				Source: "buggy",
				Param:  source.ParamAuthor,
			}
		},
	}
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, buggy, ok)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"buggy", "ok"}})
	if err != nil {
		t.Fatalf("err = %v; want success via failover", err)
	}
	if res.Lyrics != "L" {
		t.Fatalf("res.Lyrics = %q; want %q", res.Lyrics, "L")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v; want exactly one", warnings)
	}
	w := warnings[0]
	if w.Kind != PrecheckMismatch || w.Source != "buggy" || w.Param != source.ParamAuthor {
		t.Fatalf("warning = %+v; want PrecheckMismatch for buggy/ParamAuthor", w)
	}
	var mmErr source.RequiredParamMismatchError
	if !errors.As(w.Err, &mmErr) {
		t.Fatalf("warning Err = %v; want RequiredParamMismatchError", w.Err)
	}
	if ok.fetchCalls != 1 {
		t.Fatalf("ok.fetchCalls = %d; want 1 (failover after mismatch)", ok.fetchCalls)
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

// TestFetch_UnknownSyncLevelRejectedInPrecheck locks the request-level
// precheck: SyncUnknown in Params.SyncLevels is a caller bug (the CLI
// rejects such values at parse time), so it aborts with
// InvalidSyncLevelError before any fetch, in BOTH strict and lenient
// mode, and even before per-source validation (an unknown source name
// never masks it).
func TestFetch_UnknownSyncLevelRejectedInPrecheck(t *testing.T) {
	probe := &fakeSrc{
		name: "probe",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, errors.New("probe must not be fetched")
		},
	}
	r := newRegistry(t, probe)
	svc := New(r)

	// Strict mode: the error aborts, no warnings, the probe is never called.
	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncUnknown}, Source: []string{"probe"}})
	var inv InvalidSyncLevelError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v; want InvalidSyncLevelError", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", warnings)
	}
	if probe.fetchCalls != 0 {
		t.Fatalf("probe.fetchCalls = %d; want 0 (fail-fast before any fetch)", probe.fetchCalls)
	}

	// Lenient mode: still the error — a caller bug has no source to skip.
	_, warnings, err = svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncUnknown}, Source: []string{"probe"}, Lenient: true})
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v; want InvalidSyncLevelError in lenient mode too", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v; want none (no downgrade to a warning)", warnings)
	}

	// Order: the request-level check precedes per-source validation, so
	// an unknown source name in strict mode must not mask the invalid
	// sync level.
	_, _, err = svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncUnknown}, Source: []string{"nope"}})
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v; want InvalidSyncLevelError before unknown-source validation", err)
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

	params := Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"req", "nope", "nosup"}, Lenient: true}
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

func TestFetch_DuplicateSourceStrictReturnsUsageError(t *testing.T) {
	src := &fakeSrc{
		name: "a",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L"}, nil
		},
	}
	r := newRegistry(t, src)
	svc := New(r)

	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"a", "a"}})
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

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"a", "a"}, Lenient: true})
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

// TestFetch_Gate2SkipsInconsistentDeclarations drives gate 2: every way
// a request-aware custom declaration can disagree with the static
// CustomParams() list (invalid name, static-list mismatch, non-subset
// RequiredCustom, invalid RequiredCustom name, duplicate RequiredCustom)
// makes the source skipped with a precheck-mismatch warning — in BOTH
// strict and lenient mode, never a RequiredParamError.
func TestFetch_Gate2SkipsInconsistentDeclarations(t *testing.T) {
	cases := []struct {
		name string
		caps source.Capabilities
		bad  string // expected offending key in the warning
	}{
		{
			"invalid name in Custom",
			source.Capabilities{Custom: []source.ParamSpec{{Name: "lang"}}},
			"lang",
		},
		{
			"dynamic name missing from static list",
			source.Capabilities{Custom: []source.ParamSpec{{Name: "GHOST"}}},
			"GHOST",
		},
		{
			"RequiredCustom not a subset of Custom",
			source.Capabilities{
				Custom:         []source.ParamSpec{{Name: "LANG"}},
				RequiredCustom: []string{"COUNTRY"},
			},
			"COUNTRY",
		},
		{
			"RequiredCustom invalid name",
			source.Capabilities{RequiredCustom: []string{"LANG-X"}},
			"LANG-X",
		},
		{
			"RequiredCustom duplicate",
			source.Capabilities{
				Custom:         []source.ParamSpec{{Name: "LANG"}, {Name: "COUNTRY"}},
				RequiredCustom: []string{"LANG", "LANG"},
			},
			"LANG",
		},
	}
	for _, tc := range cases {
		for _, lenient := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lenient=%v", tc.name, lenient), func(t *testing.T) {
				buggy := &fakeSrc{
					name:   "buggy",
					caps:   tc.caps,
					custom: []source.ParamSpec{{Name: "LANG"}, {Name: "COUNTRY"}},
				}
				r := newRegistry(t, buggy)
				svc := New(r)

				_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"buggy"}, Lenient: lenient})
				var reqErr RequiredParamError
				if errors.As(err, &reqErr) {
					t.Fatalf("err = %+v; gate-2 bug must never surface as RequiredParamError", reqErr)
				}
				var noRes NoResultError
				if !errors.As(err, &noRes) {
					t.Fatalf("err = %v; want NoResultError (only source skipped)", err)
				}
				if len(warnings) != 1 || warnings[0].Kind != PrecheckMismatch {
					t.Fatalf("warnings = %+v; want one PrecheckMismatch warning", warnings)
				}
				if warnings[0].ParamName != tc.bad || warnings[0].Param != 0 {
					t.Fatalf("warning = %+v; want ParamName=%q, Param=0", warnings[0], tc.bad)
				}
			})
		}
	}
}

// TestFetch_Gate2BugFailsOverToNextSource verifies a gate-2-skipped
// source does not abort the run: the next source is fetched with the
// precheck-mismatch warning still emitted.
func TestFetch_Gate2BugFailsOverToNextSource(t *testing.T) {
	buggy := &fakeSrc{
		name: "buggy",
		caps: source.Capabilities{RequiredCustom: []string{"LANG-X"}},
	}
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Title: r.Song, Filled: source.FieldLyrics | source.FieldTitle}, nil
		},
	}
	r := newRegistry(t, buggy, ok)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"buggy", "ok"}})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want failover result from ok", res, err)
	}
	if len(warnings) != 1 || warnings[0].Kind != PrecheckMismatch || warnings[0].Source != "buggy" {
		t.Fatalf("warnings = %+v; want one buggy precheck-mismatch warning", warnings)
	}
}

// TestFetch_Gate2PrecedesMissingRequired locks the check order: a source
// with both a gate-2 declaration bug and a missing typed required param
// is treated as buggy and skipped — never a RequiredParamError.
func TestFetch_Gate2PrecedesMissingRequired(t *testing.T) {
	buggy := &fakeSrc{
		name: "buggy",
		caps: source.Capabilities{
			Required:       source.ParamAuthor, // would abort exit 6 if the check ran
			RequiredCustom: []string{"LANG-X"}, // gate-2 violation
		},
	}
	r := newRegistry(t, buggy)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"buggy"}})
	var reqErr RequiredParamError
	if errors.As(err, &reqErr) {
		t.Fatalf("err = %+v; gate 2 must precede the missing-required check", reqErr)
	}
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != PrecheckMismatch {
		t.Fatalf("warnings = %+v; want one PrecheckMismatch warning", warnings)
	}
}

// TestFetch_StrictAbortCarriesGate2Warnings locks the review C.7
// decision: warnings accumulated before a strict precheck abort (here a
// gate-2 bug on source A, then source B missing --author) are returned
// with the error so the caller can print them before the error message.
func TestFetch_StrictAbortCarriesGate2Warnings(t *testing.T) {
	buggy := &fakeSrc{
		name: "buggy",
		caps: source.Capabilities{RequiredCustom: []string{"LANG-X"}},
	}
	req := &fakeSrc{name: "req", caps: source.Capabilities{Required: source.ParamAuthor}}
	r := newRegistry(t, buggy, req)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"buggy", "req"}})
	var reqErr RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want RequiredParamError from req", err)
	}
	if reqErr.Source != "req" || reqErr.Param != source.ParamAuthor {
		t.Fatalf("reqErr = %+v; want author-required for req", reqErr)
	}
	if len(warnings) != 1 || warnings[0].Kind != PrecheckMismatch || warnings[0].Source != "buggy" {
		t.Fatalf("warnings = %+v; want buggy's precheck-mismatch warning carried with the error", warnings)
	}
}
