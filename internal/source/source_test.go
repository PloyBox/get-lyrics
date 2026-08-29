package source

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeSource struct {
	name   string
	caps   Capabilities
	custom []ParamSpec
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Capabilities(req Request) Capabilities {
	return f.caps
}
func (f *fakeSource) CustomParams() []ParamSpec { return f.custom }
func (f *fakeSource) Fetch(ctx context.Context, req Request) (Result, error) {
	return Result{Lyrics: "hi"}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeSource{name: "a"}); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := r.Register(&fakeSource{name: "b"}); err != nil {
		t.Fatalf("register b: %v", err)
	}

	got, err := r.Get("a")
	if err != nil || got.Name() != "a" {
		t.Fatalf("Get(a) = %v, %v; want a, nil", got, err)
	}
	got, err = r.Get("b")
	if err != nil || got.Name() != "b" {
		t.Fatalf("Get(b) = %v, %v; want b, nil", got, err)
	}
	if _, err := r.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) err = %v; want ErrNotFound", err)
	}
}

func TestRegistry_DuplicateRegistrationRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeSource{name: "x"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(&fakeSource{name: "x"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second register err = %v; want ErrDuplicate", err)
	}
}

func TestRegistry_RegisterNilRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatalf("Register(nil) should fail")
	}
}

func TestRegistry_NamesSortedAndComplete(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"gamma", "alpha", "beta"} {
		if err := r.Register(&fakeSource{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	want := []string{"alpha", "beta", "gamma"}
	if got := r.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v; want %v", got, want)
	}
}

func TestParam_BitmaskComposition(t *testing.T) {
	combined := ParamAuthor | ParamAlbum
	if combined&ParamAuthor == 0 {
		t.Fatalf("combined missing Author bit")
	}
	if combined&ParamAlbum == 0 {
		t.Fatalf("combined missing Album bit")
	}
	if combined&ParamISWC != 0 {
		t.Fatalf("combined should not include ISWC")
	}
}

func TestValidParamName(t *testing.T) {
	valid := []string{"LANG", "COUNTRY", "A", "A_1", "X0_Y1_Z2", "HTTP_PROXY"}
	for _, name := range valid {
		if !ValidParamName(name) {
			t.Fatalf("ValidParamName(%q) = false; want true", name)
		}
	}
	invalid := []string{"", "lang", "1A", "A-B", "A B", "_A", "A.", "A-B_C"}
	for _, name := range invalid {
		if ValidParamName(name) {
			t.Fatalf("ValidParamName(%q) = true; want false", name)
		}
	}
}

func TestRegistry_RegisterAcceptsValidCustomParams(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&fakeSource{
		name:   "ok",
		custom: []ParamSpec{{Name: "LANG"}, {Name: "COUNTRY"}},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
}

// TestRegistry_RegisterRejectsInvalidParamName locks gate 1: a static
// CustomParams() entry that does not match ParamNamePattern fails
// registration with ErrInvalidParamName carrying source and key.
func TestRegistry_RegisterRejectsInvalidParamName(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&fakeSource{name: "bad", custom: []ParamSpec{{Name: "lang"}}})
	var iae ErrInvalidParamName
	if !errors.As(err, &iae) {
		t.Fatalf("err = %v; want ErrInvalidParamName", err)
	}
	if iae.Source != "bad" || iae.Name != "lang" || iae.Duplicate {
		t.Fatalf("err = %+v; want Source=bad, Name=lang, Duplicate=false", iae)
	}
	if _, err := r.Get("bad"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(bad) err = %v; want ErrNotFound (source must not be registered)", err)
	}
}

// TestRegistry_RegisterRejectsDuplicateParamName locks gate 1's second
// half: duplicate entries in the static list are also a source bug.
func TestRegistry_RegisterRejectsDuplicateParamName(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&fakeSource{
		name:   "dup",
		custom: []ParamSpec{{Name: "LANG"}, {Name: "LANG"}},
	})
	var iae ErrInvalidParamName
	if !errors.As(err, &iae) {
		t.Fatalf("err = %v; want ErrInvalidParamName", err)
	}
	if !iae.Duplicate || iae.Source != "dup" || iae.Name != "LANG" {
		t.Fatalf("err = %+v; want Duplicate=true, Source=dup, Name=LANG", iae)
	}
}

func TestErrInvalidParamName_Message(t *testing.T) {
	got := (ErrInvalidParamName{Source: "x", Name: "lang"}).Error()
	want := `source "x" declared invalid custom key "lang" (source bug: must match ^[A-Z][A-Z0-9_]*$)`
	if got != want {
		t.Fatalf("message = %q; want %q", got, want)
	}
	got = (ErrInvalidParamName{Source: "x", Name: "LANG", Duplicate: true}).Error()
	want = `source "x" declared duplicate custom key "LANG" (source bug)`
	if got != want {
		t.Fatalf("message = %q; want %q", got, want)
	}
}
