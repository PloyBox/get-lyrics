package fetch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// TestFetch_MissingRequiredCustomParamName drives the custom
// required-param precheck: a missing RequiredCustom key produces a
// RequiredParamError carrying ParamName=<KEY> with the Param bit
// unset (0).
func TestFetch_MissingRequiredCustomParamName(t *testing.T) {
	custom := &fakeSrc{
		name:   "custom",
		caps:   source.Capabilities{Custom: []source.ParamSpec{{Name: "LANG"}}, RequiredCustom: []string{"LANG"}},
		custom: []source.ParamSpec{{Name: "LANG"}},
	}
	r := newRegistry(t, custom)
	svc := New(r)

	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"custom"}})
	var reqErr RequiredParamError
	if !errors.As(err, &reqErr) {
		t.Fatalf("err = %v; want RequiredParamError", err)
	}
	if reqErr.Source != "custom" || reqErr.Param != 0 || reqErr.ParamName != "LANG" {
		t.Fatalf("reqErr = %+v; want custom LANG required", reqErr)
	}
}

// TestFetch_MissingRequiredCustomLenientSkips mirrors the typed lenient
// path: a missing RequiredCustom key skips the source with a PreCheck
// warning under --lenient.
func TestFetch_MissingRequiredCustomLenientSkips(t *testing.T) {
	custom := &fakeSrc{
		name:   "custom",
		caps:   source.Capabilities{Custom: []source.ParamSpec{{Name: "LANG"}}, RequiredCustom: []string{"LANG"}},
		custom: []source.ParamSpec{{Name: "LANG"}},
	}
	ok := &fakeSrc{
		name: "ok",
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Title: r.Song, Filled: source.FieldLyrics | source.FieldTitle}, nil
		},
	}
	r := newRegistry(t, custom, ok)
	svc := New(r)

	res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"custom", "ok"}, Lenient: true})
	if err != nil || res.Lyrics != "L" {
		t.Fatalf("res = %+v, err = %v; want failover result from ok", res, err)
	}
	if len(warnings) != 1 || warnings[0].Kind != PreCheck {
		t.Fatalf("warnings = %+v; want one PreCheck warning", warnings)
	}
	if warnings[0].ParamName != "LANG" || warnings[0].Param != 0 {
		t.Fatalf("warning = %+v; want ParamName=LANG, Param=0", warnings[0])
	}
}

// TestFetch_ConditionalRequiredCustom drives mock-custom semantics:
// COUNTRY is recognized and required only when LANG is present, so the
// same source reports a different missing key depending on the request.
func TestFetch_ConditionalRequiredCustom(t *testing.T) {
	cond := &fakeSrc{
		name: "cond",
		capsFn: func(req source.Request) source.Capabilities {
			caps := source.Capabilities{
				Custom:         []source.ParamSpec{{Name: "LANG"}},
				RequiredCustom: []string{"LANG"},
			}
			if _, ok := req.Custom["LANG"]; ok {
				caps.Custom = append(caps.Custom, source.ParamSpec{Name: "COUNTRY"})
				caps.RequiredCustom = append(caps.RequiredCustom, "COUNTRY")
			}
			return caps
		},
		custom: []source.ParamSpec{{Name: "LANG"}, {Name: "COUNTRY"}},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L:" + r.Custom["LANG"], Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, cond)
	svc := New(r)

	// No custom keys: only LANG is required.
	_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"cond"}})
	var reqErr RequiredParamError
	if !errors.As(err, &reqErr) || reqErr.ParamName != "LANG" {
		t.Fatalf("err = %v; want LANG required", err)
	}

	// LANG present: COUNTRY joins the required list and is now missing.
	_, _, err = svc.Fetch(context.Background(), Params{Song: "S", Custom: map[string]string{"LANG": "en"}, Source: []string{"cond"}})
	if !errors.As(err, &reqErr) || reqErr.ParamName != "COUNTRY" {
		t.Fatalf("err = %v; want COUNTRY required once LANG is present", err)
	}

	// Both present: success, and the adapter saw req.Custom.
	res, _, err := svc.Fetch(context.Background(), Params{Song: "S", Custom: map[string]string{"LANG": "en", "COUNTRY": "cn"}, SyncLevels: []SyncLevel{SyncNone}, Source: []string{"cond"}})
	if err != nil || res.Lyrics != "L:en" {
		t.Fatalf("res = %+v, err = %v; want lyrics carrying LANG", res, err)
	}
}

// TestDetectUnsupported_CustomKeys drives the custom parallel path:
// recognized keys stay silent, unrecognized keys warn with ParamName
// set and Param 0, whitespace-only values are ignored, and multiple
// unrecognized keys produce a warning set (order unspecified).
func TestDetectUnsupported_CustomKeys(t *testing.T) {
	stub := &fakeSrc{
		name:   "stub",
		caps:   source.Capabilities{Custom: []source.ParamSpec{{Name: "LANG"}}},
		custom: []source.ParamSpec{{Name: "LANG"}},
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, nil
		},
	}

	if got := detectUnsupported(Params{Song: "S", Custom: map[string]string{"LANG": "en"}}, stub); len(got) != 0 {
		t.Fatalf("recognized key produced warnings: %+v", got)
	}

	got := detectUnsupported(Params{Song: "S", Custom: map[string]string{"FOO": "x"}}, stub)
	if len(got) != 1 {
		t.Fatalf("got %+v; want one warning", got)
	}
	w := got[0]
	if w.Kind != UnsupportedParam || w.ParamName != "FOO" || w.Param != 0 {
		t.Fatalf("warning = %+v; want UnsupportedParam FOO with Param 0", w)
	}

	if got := detectUnsupported(Params{Song: "S", Custom: map[string]string{"FOO": " "}}, stub); len(got) != 0 {
		t.Fatalf("whitespace-only value produced warnings: %+v", got)
	}

	got = detectUnsupported(Params{Song: "S", Custom: map[string]string{"FOO": "x", "BAR": "y"}}, stub)
	if len(got) != 2 {
		t.Fatalf("got %d warnings; want 2", len(got))
	}
	seen := make(map[string]bool)
	for _, w := range got {
		if w.Kind != UnsupportedParam {
			t.Fatalf("warning %+v; want Kind UnsupportedParam", w)
		}
		seen[w.ParamName] = true
	}
	if !seen["FOO"] || !seen["BAR"] {
		t.Fatalf("warning set = %v; want FOO and BAR (order unspecified)", seen)
	}
}

// TestFetch_PassesCustomToRequest locks the data flow: params.Custom
// reaches both the capability query and the adapter's Request.
func TestFetch_PassesCustomToRequest(t *testing.T) {
	var queryCustom map[string]string
	cond := &fakeSrc{
		name: "cond",
		capsFn: func(req source.Request) source.Capabilities {
			queryCustom = req.Custom
			return source.Capabilities{Custom: []source.ParamSpec{{Name: "LANG"}}}
		},
		custom: []source.ParamSpec{{Name: "LANG"}},
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L:" + r.Custom["LANG"], Filled: source.FieldLyrics}, nil
		},
	}
	r := newRegistry(t, cond)
	svc := New(r)

	res, _, err := svc.Fetch(context.Background(), Params{Song: "S", Custom: map[string]string{"LANG": "en"}, SyncLevels: []SyncLevel{SyncNone}, Source: []string{"cond"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Lyrics != "L:en" {
		t.Fatalf("res.Lyrics = %q; want adapter to see Custom in Request", res.Lyrics)
	}
	if queryCustom == nil || queryCustom["LANG"] != "en" {
		t.Fatalf("capability query Custom = %v; want LANG=en", queryCustom)
	}
}

// TestFetch_RequiredParamMismatchCustomCarriesErr verifies the
// fetch-time mismatch warning for a custom key carries ParamName with
// the Param bit unset and the adapter error as Err.
func TestFetch_RequiredParamMismatchCustomCarriesErr(t *testing.T) {
	buggy := &fakeSrc{
		name: "buggy",
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, source.RequiredParamMismatchError{
				Source:    "buggy",
				ParamName: "LANG",
			}
		},
	}
	r := newRegistry(t, buggy)
	svc := New(r)

	_, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"buggy"}})
	var noRes NoResultError
	if !errors.As(err, &noRes) {
		t.Fatalf("err = %v; want NoResultError", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != PrecheckMismatch {
		t.Fatalf("warnings = %+v; want one PrecheckMismatch warning", warnings)
	}
	if warnings[0].ParamName != "LANG" || warnings[0].Param != 0 {
		t.Fatalf("warning = %+v; want ParamName=LANG, Param=0", warnings[0])
	}
	var mmErr source.RequiredParamMismatchError
	if !errors.As(warnings[0].Err, &mmErr) {
		t.Fatalf("warning Err = %v; want RequiredParamMismatchError", warnings[0].Err)
	}
}

func TestCustomParamsFor_StrictUnknownAndDuplicate(t *testing.T) {
	a := &fakeSrc{name: "a", custom: []source.ParamSpec{{Name: "LANG"}}}
	r := newRegistry(t, a)
	svc := New(r)

	// Unknown source in strict mode, with irrelevant fields set — they
	// must not participate in the query.
	_, err := svc.CustomParamsFor(Params{Song: "S", Author: "X", Album: "Y", Custom: map[string]string{"K": "v"}, Source: []string{"nope"}})
	var unk UnknownSourceError
	if !errors.As(err, &unk) || unk.Name != "nope" {
		t.Fatalf("err = %v; want UnknownSourceError{Name: nope}", err)
	}
	_, err = svc.CustomParamsFor(Params{Source: []string{"nope"}})
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v; want same UnknownSourceError with only Source set", err)
	}

	_, err = svc.CustomParamsFor(Params{Source: []string{"a", "a"}})
	var dup DuplicateSourceError
	if !errors.As(err, &dup) || dup.Name != "a" {
		t.Fatalf("err = %v; want DuplicateSourceError{Name: a}", err)
	}
}

func TestCustomParamsFor_LenientSkipsProblemSources(t *testing.T) {
	a := &fakeSrc{name: "a", custom: []source.ParamSpec{{Name: "LANG", Description: "language hint"}}}
	r := newRegistry(t, a)
	svc := New(r)

	decls, err := svc.CustomParamsFor(Params{Source: []string{"a", "nope", "a"}, Lenient: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("decls = %v; want only source a", decls)
	}
	if !reflect.DeepEqual(decls["a"], a.custom) {
		t.Fatalf("decls[a] = %v; want CustomParams() unchanged", decls["a"])
	}
}

func TestCustomParamsFor_ReturnsDeclarations(t *testing.T) {
	a := &fakeSrc{name: "a", custom: []source.ParamSpec{{Name: "LANG", Description: "language hint"}}}
	b := &fakeSrc{name: "b"}
	r := newRegistry(t, a, b)
	svc := New(r)

	decls, err := svc.CustomParamsFor(Params{Song: "ignored", Author: "ignored", Source: []string{"b", "a"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(decls) != 2 {
		t.Fatalf("decls = %v; want entries for a and b", decls)
	}
	if len(decls["b"]) != 0 {
		t.Fatalf("decls[b] = %v; want nil for a source without custom params", decls["b"])
	}
	if !reflect.DeepEqual(decls["a"], a.custom) {
		t.Fatalf("decls[a] = %v; want the static declaration", decls["a"])
	}
}
