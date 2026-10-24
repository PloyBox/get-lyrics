package fetch_test

import (
	"slices"
	"testing"

	"github.com/PloyBox/get-lyrics/fetch"
	"github.com/PloyBox/get-lyrics/source"
)

func TestCustomParamsFor(t *testing.T) {
	declA := []source.ParamSpec{{Name: "A_KEY", Description: "a"}}
	declB := []source.ParamSpec{
		{Name: "B_KEY", Description: "b"},
		{Name: "B_OPT", Description: "opt"},
	}
	newAB := func() (*stub, *stub) {
		return declaredStub("a", declA, source.Capabilities{Custom: declA}),
			declaredStub("b", declB, source.Capabilities{Custom: declB})
	}

	t.Run("static declarations and lenient skips", func(t *testing.T) {
		a, b := newAB()
		svc := newService(t, a, b)
		got, err := svc.CustomParamsFor(fetch.Params{
			Source: []string{"nosuch", "a", "a", "b"}, Lenient: true})
		if err != nil {
			t.Fatalf("CustomParamsFor error = %v", err)
		}
		if len(got) != 2 || !slices.Equal(got["a"], declA) ||
			!slices.Equal(got["b"], declB) {
			t.Fatalf("declarations = %v", got)
		}
		if a.calls != 0 || b.calls != 0 {
			t.Fatalf("calls = %d/%d", a.calls, b.calls)
		}
	})

	t.Run("strict rejects unknown and duplicate", func(t *testing.T) {
		a, _ := newAB()
		svc := newService(t, a)
		_, err := svc.CustomParamsFor(fetch.Params{Source: []string{"nosuch"}})
		unknown := requireAs[fetch.UnknownSourceError](t, err)
		if unknown.Name != "nosuch" {
			t.Fatalf("unknown name = %q", unknown.Name)
		}
		_, err = svc.CustomParamsFor(fetch.Params{Source: []string{"a", "a"}})
		dup := requireAs[fetch.DuplicateSourceError](t, err)
		if dup.Name != "a" {
			t.Fatalf("duplicate name = %q", dup.Name)
		}
	})

	t.Run("no required or gate 2 checks", func(t *testing.T) {
		bad := declaredStub("bad", nil, source.Capabilities{
			Required: source.ParamAuthor,
			Custom:   []source.ParamSpec{{Name: "BAD_KEY"}},
		})
		svc := newService(t, bad)
		got, err := svc.CustomParamsFor(fetch.Params{Source: []string{"bad"},
			SyncLevels: []fetch.SyncLevel{fetch.SyncUnknown}})
		if err != nil {
			t.Fatalf("CustomParamsFor error = %v", err)
		}
		if specs, ok := got["bad"]; !ok || len(specs) != 0 {
			t.Fatalf("bad declarations = %v", got)
		}
		if bad.calls != 0 {
			t.Fatalf("calls = %d", bad.calls)
		}
	})
}
