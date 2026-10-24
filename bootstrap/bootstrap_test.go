// Package bootstrap_test verifies the built-in registration contract.
package bootstrap_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/PloyBox/get-lyrics/bootstrap"
	"github.com/PloyBox/get-lyrics/source"
)

func TestRegisterAll(t *testing.T) {
	want := []string{"betterlyrics", "lrccx", "lrclib", "lyricsovh", "musixmatch"}

	r := source.NewRegistry()
	if err := bootstrap.RegisterAll(r); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if got := r.Names(); !slices.Equal(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
	for _, name := range want {
		if _, err := r.Get(name); err != nil {
			t.Errorf("Get(%q): %v", name, err)
		}
	}

	t.Run("SecondCall", func(t *testing.T) {
		err := bootstrap.RegisterAll(r)
		if !errors.Is(err, source.ErrDuplicate) {
			t.Errorf("second RegisterAll error = %v, want ErrDuplicate", err)
		}
	})
}
