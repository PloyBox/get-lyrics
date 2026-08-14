//go:build test

package lrc

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-lrc" }

func (a *Adapter) SupportedParams() source.Param { return source.ParamTimestamp }

func (a *Adapter) RequiredParams() source.Param { return 0 }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	res := source.Result{
		Lyrics: "[mock-lrc] lyrics for: " + req.Song + "\n",
		Title:  req.Song,
	}
	if req.Timestamp {
		res.SyncedLyrics = "[00:00.00] [mock-lrc] first line for: " + req.Song + "\n" +
			"[00:05.00] [mock-lrc] second line for: " + req.Song + "\n"
	}
	return res, nil
}
