//go:build test

package synconly

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-synconly" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch honors --sync-level in principle but only ever fills synced
// (LRC) lyrics, never plain — exercising the synced-only result path:
// a plain request cannot match it (downgrade warning, no empty
// success), while a later "line" iteration reuses it from the cache.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[00:00.00] [mock-synconly] synced lyrics for: " + req.Song,
		Level:  source.SyncLine,
		Filled: source.FieldLyrics,
	}, nil
}
