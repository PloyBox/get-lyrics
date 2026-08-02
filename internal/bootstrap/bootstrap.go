// Package bootstrap aggregates every built-in source adapter so that
// main only needs one call to wire them all up.
//
// Adding a new built-in source means:
//  1. create internal/source/<real|mock>/<name>, implementing source.Source
//  2. add an import here and call r.Register(<name>.New()) in RegisterAll
package bootstrap

import (
	"github.com/PloyBox/get-lyrics/internal/source"
	"github.com/PloyBox/get-lyrics/internal/source/mock/stub"
	"github.com/PloyBox/get-lyrics/internal/source/real/lrclib"
	"github.com/PloyBox/get-lyrics/internal/source/real/lyricsovh"
)

// RegisterAll registers every built-in adapter into r. main calls this
// exactly once during startup (via a package-level var initializer).
func RegisterAll(r *source.Registry) error {
	if err := r.Register(stub.New()); err != nil {
		return err
	}
	if err := r.Register(lrclib.New()); err != nil {
		return err
	}
	if err := r.Register(lyricsovh.New()); err != nil {
		return err
	}
	// future: r.Register(musixmatch.New()), ...
	return nil
}
