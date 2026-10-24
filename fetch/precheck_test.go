package fetch_test

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

var (
	needAuthor = source.Capabilities{Required: source.ParamAuthor}
	needSpecs  = []source.ParamSpec{{Name: "NEED"}}
	needCustom = source.Capabilities{
		Custom: needSpecs, RequiredCustom: []string{"NEED"},
	}
	badCaps = source.Capabilities{
		Custom: []source.ParamSpec{{Name: "BAD_KEY"}},
	}
)

type errKind int

const (
	errUnknown errKind = iota
	errDuplicate
	errRequired
	errInvalid
)

func TestPrecheckStrict(t *testing.T) {
	cases := []struct {
		name, wantSrc, wantName, wantWarn string
		stubs                             []*stub
		params                            fetch.Params
		kind                              errKind
		wantParam                         source.Param
	}{
		{"unknown source", "nosuch", "", "", nil, baseParams("nosuch"),
			errUnknown, 0},
		{"duplicate source", "dup", "", "",
			[]*stub{newStub("dup", source.Capabilities{}, nil)},
			baseParams("dup", "dup"), errDuplicate, 0},
		{"missing typed parameter", "req", "", "",
			[]*stub{newStub("req", needAuthor, nil)}, baseParams("req"),
			errRequired, source.ParamAuthor},
		{"whitespace author counts as missing", "req", "", "",
			[]*stub{newStub("req", needAuthor, nil)},
			fetch.Params{Song: "Song", Author: "  \t ", Source: []string{"req"},
				SyncLevels: []fetch.SyncLevel{fetch.SyncNone}},
			errRequired, source.ParamAuthor},
		{"missing custom parameter", "reqc", "NEED", "",
			[]*stub{declaredStub("reqc", needSpecs, needCustom)}, baseParams("reqc"),
			errRequired, 0},
		{"sync unknown level", "", "", "",
			[]*stub{newStub("plain", source.Capabilities{}, nil)},
			levelParams([]fetch.SyncLevel{fetch.SyncNone, fetch.SyncUnknown},
				"plain"), errInvalid, 0},
		{"gate 2 warning then missing parameter", "req", "", "BAD_KEY",
			[]*stub{newStub("bad", badCaps, nil), newStub("req", needAuthor, nil)},
			baseParams("bad", "req"), errRequired, source.ParamAuthor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService(t, tc.stubs...)
			_, warnings, err := svc.Fetch(context.Background(), tc.params)
			requireNoCalls(t, tc.stubs)
			switch tc.kind {
			case errUnknown:
				got := requireAs[fetch.UnknownSourceError](t, err)
				if got.Name != tc.wantSrc || !errors.Is(err, source.ErrNotFound) {
					t.Fatalf("unknown error = %+v (wraps ErrNotFound: %v)",
						got, errors.Is(err, source.ErrNotFound))
				}
			case errDuplicate:
				if got := requireAs[fetch.DuplicateSourceError](t, err); got.Name != tc.wantSrc {
					t.Fatalf("duplicate name = %q", got.Name)
				}
			case errRequired:
				got := requireAs[fetch.RequiredParamError](t, err)
				if got.Source != tc.wantSrc || got.Param != tc.wantParam ||
					got.ParamName != tc.wantName {
					t.Fatalf("RequiredParamError = %+v", got)
				}
			case errInvalid:
				requireAs[fetch.InvalidSyncLevelError](t, err)
			}
			if tc.wantWarn == "" {
				requireNoWarnings(t, warnings)
				return
			}
			w := requireOneWarning(t, warnings, fetch.PrecheckMismatch)
			if w.Source != "bad" || w.ParamName != tc.wantWarn {
				t.Fatalf("gate 2 warning = %+v", w)
			}
		})
	}
}

func TestPrecheckLenient(t *testing.T) {
	t.Run("problems downgraded and skipped", func(t *testing.T) {
		req := newStub("req", needAuthor, nil)
		reqc := declaredStub("reqc", needSpecs, needCustom)
		good := newStub("good", source.Capabilities{}, nil)
		svc := newService(t, req, reqc, good)
		params := baseParams("nosuch", "req", "reqc", "good", "good")
		params.Lenient = true
		res, warnings := mustFetch(t, svc, params)
		if res.Source != "good" || res.Level != fetch.SyncNone {
			t.Fatalf("result = %+v", res)
		}
		got := make(map[warnKey]struct{}, len(warnings))
		for _, w := range warningsOfKind(warnings, fetch.PreCheck) {
			got[warnKey{w.Source, w.Param, w.ParamName, w.Err != nil}] = struct{}{}
		}
		want := map[warnKey]struct{}{
			{"nosuch", 0, "", true}:                {},
			{"req", source.ParamAuthor, "", false}: {},
			{"reqc", 0, "NEED", false}:             {},
			{"good", 0, "", false}:                 {},
		}
		if len(warnings) != 4 || !maps.Equal(got, want) {
			t.Fatalf("warnings = %+v", warnings)
		}
		if req.calls != 0 || reqc.calls != 0 || good.calls != 1 {
			t.Fatalf("calls = %d/%d/%d", req.calls, reqc.calls, good.calls)
		}
	})

	t.Run("all sources skipped", func(t *testing.T) {
		req := newStub("req", needAuthor, nil)
		svc := newService(t, req)
		params := baseParams("nosuch", "req")
		params.Lenient = true
		_, warnings, err := svc.Fetch(context.Background(), params)
		requireAs[fetch.NoResultError](t, err)
		if len(warningsOfKind(warnings, fetch.PreCheck)) != 2 {
			t.Fatalf("warnings = %+v", warnings)
		}
		requireNoCalls(t, []*stub{req})
	})

	t.Run("sync unknown still rejected", func(t *testing.T) {
		plain := newStub("plain", source.Capabilities{}, nil)
		svc := newService(t, plain)
		params := levelParams([]fetch.SyncLevel{fetch.SyncUnknown}, "plain")
		params.Lenient = true
		_, warnings, err := svc.Fetch(context.Background(), params)
		requireAs[fetch.InvalidSyncLevelError](t, err)
		requireNoWarnings(t, warnings)
		requireNoCalls(t, []*stub{plain})
	})
}

func TestGateTwo(t *testing.T) {
	ok := []source.ParamSpec{{Name: "OK"}}
	cases := []struct {
		name, want string
		static     []source.ParamSpec
		caps       source.Capabilities
	}{
		{"custom missing from static list", "BAD_KEY", nil, badCaps},
		{"required custom outside custom list", "MISSING", ok,
			source.Capabilities{Custom: ok, RequiredCustom: []string{"MISSING"}}},
		{"duplicate required custom", "OK", ok,
			source.Capabilities{Custom: ok, RequiredCustom: []string{"OK", "OK"}}},
		{"gate 2 beats missing typed param", "BAD_KEY", nil,
			source.Capabilities{Required: source.ParamAuthor,
				Custom: badCaps.Custom}},
	}
	for _, tc := range cases {
		for _, lenient := range []bool{false, true} {
			mode := "strict"
			if lenient {
				mode = "lenient"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				src := declaredStub("bad", tc.static, tc.caps)
				svc := newService(t, src)
				params := baseParams("bad")
				params.Lenient = lenient
				_, warnings, err := svc.Fetch(context.Background(), params)
				if src.calls != 0 {
					t.Fatalf("source called %d times", src.calls)
				}
				w := requireOneWarning(t, warnings, fetch.PrecheckMismatch)
				if w.Source != "bad" || w.Param != 0 ||
					w.ParamName != tc.want {
					t.Fatalf("gate 2 warning = %+v", w)
				}
				requireAs[fetch.NoResultError](t, err)
				var reqErr fetch.RequiredParamError
				if errors.As(err, &reqErr) {
					t.Fatalf("got RequiredParamError %+v", reqErr)
				}
			})
		}
	}
}
