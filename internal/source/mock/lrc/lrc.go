//go:build test

package lrc

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-lrc" }

// Capabilities lists no filters: this mock's only concern is the synced
// output path, which is a runtime property of the returned result.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	res := source.Result{
		Lyrics: "[mock-lrc] lyrics for: " + req.Song + "\n",
		Title:  req.Song,
		Filled: source.FieldLyrics | source.FieldTitle,
	}
	if req.Timestamp {
		res.SyncedLyrics = "[00:00.00] [mock-lrc] first line for: " + req.Song + "\n" +
			"[00:05.00] [mock-lrc] second line for: " + req.Song + "\n"
		res.Filled |= source.FieldSyncedLyrics
	}
	return res, nil
}
