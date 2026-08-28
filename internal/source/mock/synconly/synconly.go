//go:build test

package synconly

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-synconly" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch honors --timestamp in principle but only ever fills
// SyncedLyrics, never plain Lyrics — exercising the synced-only result
// path: a plain request cannot match it (downgrade warning, no empty
// success), while a later "line" iteration reuses it from the cache.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		SyncedLyrics: "[00:00.00] [mock-synconly] synced lyrics for: " + req.Song,
		Title:        req.Song,
		Filled:       source.FieldSyncedLyrics | source.FieldTitle,
	}, nil
}
