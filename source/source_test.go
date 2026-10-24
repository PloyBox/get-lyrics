// Package source_test verifies the public registry contract as a library
// user sees it.
package source_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

// stub is a minimal Source used to exercise registration and lookup.
type stub struct {
	name   string
	params []source.ParamSpec
}

func (s *stub) Name() string { return s.name }

func (s *stub) Capabilities(source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (s *stub) CustomParams() []source.ParamSpec { return s.params }

// Fetch fails loudly so any registry path that calls it is caught.
func (s *stub) Fetch(context.Context, source.Request) (source.Result, error) {
	return source.Result{}, errors.New("stub Fetch must not be called")
}

func TestRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := source.NewRegistry()
		src := &stub{name: "demo"}
		if err := r.Register(src); err != nil {
			t.Fatalf("Register: %v", err)
		}
		got, err := r.Get("demo")
		if err != nil || got != src {
			t.Fatalf("Get(demo) = %v, %v, want the registered stub", got, err)
		}
		if _, err := r.Get("missing"); !errors.Is(err, source.ErrNotFound) {
			t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
		}
	})

	t.Run("names sorted", func(t *testing.T) {
		r := source.NewRegistry()
		for _, name := range []string{"zeta", "alpha", "mike"} {
			if err := r.Register(&stub{name: name}); err != nil {
				t.Fatalf("Register(%q): %v", name, err)
			}
		}
		want := []string{"alpha", "mike", "zeta"}
		if got := r.Names(); !slices.Equal(got, want) {
			t.Errorf("Names() = %v, want %v", got, want)
		}
	})

	t.Run("duplicate and nil", func(t *testing.T) {
		r := source.NewRegistry()
		if err := r.Register(&stub{name: "dup"}); err != nil {
			t.Fatalf("Register first: %v", err)
		}
		if err := r.Register(&stub{name: "dup"}); !errors.Is(err, source.ErrDuplicate) {
			t.Errorf("duplicate Register error = %v, want ErrDuplicate", err)
		}
		if err := r.Register(nil); err == nil {
			t.Error("Register(nil) = nil, want a non-nil error")
		}
	})

	t.Run("gate one", func(t *testing.T) {
		cases := []struct {
			name    string
			params  []source.ParamSpec
			wantBad *source.ErrInvalidParamName
		}{
			{"lowercase key", []source.ParamSpec{{Name: "lower"}},
				&source.ErrInvalidParamName{Source: "stub", Name: "lower"}},
			{"leading digit key", []source.ParamSpec{{Name: "1BAD"}},
				&source.ErrInvalidParamName{Source: "stub", Name: "1BAD"}},
			{"hyphen key", []source.ParamSpec{{Name: "BAD-KEY"}},
				&source.ErrInvalidParamName{Source: "stub", Name: "BAD-KEY"}},
			{"duplicate key",
				[]source.ParamSpec{{Name: "DUP"}, {Name: "DUP"}},
				&source.ErrInvalidParamName{Source: "stub", Name: "DUP",
					Duplicate: true}},
			{"legal distinct keys",
				[]source.ParamSpec{{Name: "A"}, {Name: "B_2"}}, nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := source.NewRegistry()
				err := r.Register(&stub{name: "stub", params: tc.params})
				if tc.wantBad == nil {
					if err != nil {
						t.Fatalf("Register: %v", err)
					}
					if _, err := r.Get("stub"); err != nil {
						t.Errorf("Get after Register: %v", err)
					}
					return
				}
				var bad source.ErrInvalidParamName
				if !errors.As(err, &bad) {
					t.Fatalf("Register error = %v, want ErrInvalidParamName", err)
				}
				if bad != *tc.wantBad {
					t.Errorf("ErrInvalidParamName = %+v, want %+v", bad, *tc.wantBad)
				}
				if _, err := r.Get("stub"); !errors.Is(err, source.ErrNotFound) {
					t.Errorf("invalid source was registered: Get = %v", err)
				}
			})
		}
	})
}
