// Package bootstrap registers every built-in source adapter.
// See docs/refs/bootstrap.md.
package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/provider/real/betterlyrics"
	"github.com/PloyBox/get-lyrics/internal/provider/real/lrccx"
	"github.com/PloyBox/get-lyrics/internal/provider/real/lrclib"
	"github.com/PloyBox/get-lyrics/internal/provider/real/lyricsovh"
	"github.com/PloyBox/get-lyrics/internal/provider/real/musixmatch"
	"github.com/PloyBox/get-lyrics/source"
)

// RegisterAll registers every built-in adapter into r; main calls it once
// during startup.
func RegisterAll(r *source.Registry) error {
	if err := r.Register(lrclib.New()); err != nil {
		return err
	}
	if err := r.Register(lyricsovh.New()); err != nil {
		return err
	}
	if err := r.Register(lrccx.New()); err != nil {
		return err
	}
	if err := r.Register(musixmatch.New()); err != nil {
		return err
	}
	if err := r.Register(betterlyrics.New()); err != nil {
		return err
	}
	return nil
}
