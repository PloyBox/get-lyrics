package fetch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// conditionalCustom is a source that recognizes (and requires) LANG always
// and COUNTRY only once LANG is supplied — the conditional-declaration case
// the fetch layer must honor per request.
func conditionalCustom() *fakeSrc {
	return &fakeSrc{
		name:   "cond",
		custom: []source.ParamSpec{{Name: "LANG"}, {Name: "COUNTRY"}},
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
		fetch: func(_ context.Context, r source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L:" + r.Custom["LANG"], Level: source.SyncNone, Filled: source.FieldLyrics}, nil
		},
	}
}

// TestFetch_RequiredCustomPrecheck drives the custom required-param
// precheck in both modes, including the conditional declaration: a missing
// RequiredCustom key is a RequiredParamError (strict) or a skipped source
// with a PreCheck warning (lenient), and the key that joins the list
// depends on the request.
func TestFetch_RequiredCustomPrecheck(t *testing.T) {
	t.Run("strict missing LANG", func(t *testing.T) {
		cond := conditionalCustom()
		svc := New(newRegistry(t, cond))

		_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Source: []string{"cond"}})
		var reqErr RequiredParamError
		if !errors.As(err, &reqErr) {
			t.Fatalf("err = %v; want RequiredParamError", err)
		}
		if reqErr.Source != "cond" || reqErr.Param != 0 || reqErr.ParamName != "LANG" {
			t.Fatalf("reqErr = %+v; want custom LANG required", reqErr)
		}
		if cond.fetchCalls != 0 {
			t.Fatalf("fetchCalls = %d; want 0 (fail-fast before any fetch)", cond.fetchCalls)
		}
	})

	t.Run("lenient skips missing LANG and fails over", func(t *testing.T) {
		cond := conditionalCustom()
		ok := &fakeSrc{
			name: "ok",
			fetch: func(_ context.Context, r source.Request) (source.Result, error) {
				return source.Result{Lyrics: "L", Title: r.Song, Level: source.SyncNone, Filled: source.FieldLyrics | source.FieldTitle}, nil
			},
		}
		svc := New(newRegistry(t, cond, ok))

		res, warnings, err := svc.Fetch(context.Background(), Params{Song: "S", SyncLevels: []SyncLevel{SyncNone}, Source: []string{"cond", "ok"}, Lenient: true})
		if err != nil || res.Lyrics != "L" {
			t.Fatalf("res = %+v, err = %v; want failover result from ok", res, err)
		}
		if len(warnings) != 1 || warnings[0].Kind != PreCheck {
			t.Fatalf("warnings = %+v; want one PreCheck warning", warnings)
		}
		if warnings[0].ParamName != "LANG" || warnings[0].Param != 0 {
			t.Fatalf("warning = %+v; want ParamName=LANG, Param=0", warnings[0])
		}
	})

	t.Run("conditional COUNTRY after LANG", func(t *testing.T) {
		svc := New(newRegistry(t, conditionalCustom()))

		_, _, err := svc.Fetch(context.Background(), Params{Song: "S", Custom: map[string]string{"LANG": "en"}, Source: []string{"cond"}})
		var reqErr RequiredParamError
		if !errors.As(err, &reqErr) || reqErr.ParamName != "COUNTRY" {
			t.Fatalf("err = %v; want COUNTRY required once LANG is present", err)
		}
	})

	t.Run("custom keys reach the adapter", func(t *testing.T) {
		svc := New(newRegistry(t, conditionalCustom()))

		res, _, err := svc.Fetch(context.Background(), Params{
			Song:       "S",
			Custom:     map[string]string{"LANG": "en", "COUNTRY": "cn"},
			SyncLevels: []SyncLevel{SyncNone},
			Source:     []string{"cond"},
		})
		if err != nil || res.Lyrics != "L:en" {
			t.Fatalf("res = %+v, err = %v; want lyrics carrying LANG", res, err)
		}
	})
}

// TestFetch_UnsupportedCustomKeys drives the custom parallel path of the
// unsupported-parameter detection: recognized keys stay silent,
// unrecognized keys warn with ParamName set and Param 0, whitespace-only
// values are ignored, and multiple unrecognized keys produce a warning set
// (order unspecified).
func TestFetch_UnsupportedCustomKeys(t *testing.T) {
	stub := &fakeSrc{
		name:   "stub",
		caps:   source.Capabilities{Custom: []source.ParamSpec{{Name: "LANG"}}},
		custom: []source.ParamSpec{{Name: "LANG"}},
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Lyrics: "L", Level: source.SyncNone, Filled: source.FieldLyrics}, nil
		},
	}
	svc := New(newRegistry(t, stub))
	fetch := func(custom map[string]string) []Warning {
		t.Helper()
		_, warnings, err := svc.Fetch(context.Background(), Params{
			Song:       "S",
			Custom:     custom,
			SyncLevels: []SyncLevel{SyncNone},
			Source:     []string{"stub"},
		})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		return warnings
	}

	if got := fetch(map[string]string{"LANG": "en"}); len(got) != 0 {
		t.Fatalf("recognized key produced warnings: %+v", got)
	}

	if got := fetch(map[string]string{"FOO": " "}); len(got) != 0 {
		t.Fatalf("whitespace-only value produced warnings: %+v", got)
	}

	got := fetch(map[string]string{"FOO": "x"})
	if len(got) != 1 {
		t.Fatalf("got %+v; want one warning", got)
	}
	if w := got[0]; w.Kind != UnsupportedParam || w.ParamName != "FOO" || w.Param != 0 {
		t.Fatalf("warning = %+v; want UnsupportedParam FOO with Param 0", w)
	}

	got = fetch(map[string]string{"FOO": "x", "BAR": "y"})
	if len(got) != 2 {
		t.Fatalf("got %d warnings; want 2", len(got))
	}
	seen := make(map[string]bool, 2)
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

// TestFetch_RequiredParamMismatchCustomCarriesErr is the custom-key twin of
// the typed mismatch test: the fetch-time error surfaces as a PrecheckMismatch
// warning with ParamName set, Param 0 and the cause preserved in Err.
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

// TestCustomParamsFor locks the read-only declaration query: strict mode
// reports unknown and duplicate sources (other fields never participate),
// lenient mode skips exactly those, and the static declarations are
// returned unchanged.
func TestCustomParamsFor(t *testing.T) {
	a := &fakeSrc{name: "a", custom: []source.ParamSpec{{Name: "LANG", Description: "language hint"}}}
	b := &fakeSrc{name: "b"}
	svc := New(newRegistry(t, a, b))

	t.Run("strict unknown and duplicate", func(t *testing.T) {
		// Extra fields are set on purpose: only Source and Lenient may
		// participate in the query.
		_, err := svc.CustomParamsFor(Params{Song: "S", Author: "X", Album: "Y", Custom: map[string]string{"K": "v"}, Source: []string{"nope"}})
		var unk UnknownSourceError
		if !errors.As(err, &unk) || unk.Name != "nope" {
			t.Fatalf("err = %v; want UnknownSourceError{Name: nope}", err)
		}

		_, err = svc.CustomParamsFor(Params{Source: []string{"a", "a"}})
		var dup DuplicateSourceError
		if !errors.As(err, &dup) || dup.Name != "a" {
			t.Fatalf("err = %v; want DuplicateSourceError{Name: a}", err)
		}
	})

	t.Run("lenient skips problem sources", func(t *testing.T) {
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
	})

	t.Run("declarations", func(t *testing.T) {
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
	})
}
