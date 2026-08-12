package source

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeSource struct {
	name string
	sup  Param
}

func (f *fakeSource) Name() string           { return f.name }
func (f *fakeSource) SupportedParams() Param { return f.sup }
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
	if combined&ParamTimestamp != 0 {
		t.Fatalf("combined should not include Timestamp")
	}
}
