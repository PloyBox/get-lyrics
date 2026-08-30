//go:build test

package nosync

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-nosync" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch honors --sync-level in principle but never returns synced
// lyrics, exercising the CLI's plain-output fallback warning.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[mock-nosync] lyrics for: " + req.Song + "\n",
		Level:  source.SyncNone,
		Filled: source.FieldLyrics,
	}, nil
}
