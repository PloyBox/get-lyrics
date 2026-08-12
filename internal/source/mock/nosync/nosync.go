//go:build test

package nosync

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-nosync" }

func (a *Adapter) SupportedParams() source.Param { return source.ParamTimestamp }

// Fetch honors --timestamp in principle but never returns SyncedLyrics,
// exercising the CLI's plain-output fallback warning.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[mock-nosync] lyrics for: " + req.Song + "\n",
		Title:  req.Song,
	}, nil
}
