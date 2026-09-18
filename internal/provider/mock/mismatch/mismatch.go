//go:build test

package mismatch

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-mismatch" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if req.Author == "" {
		return source.Result{}, source.RequiredParamMismatchError{
			Source: a.Name(),
			Param:  source.ParamAuthor,
		}
	}
	lyrics := "[mock-mismatch] lyrics for: " + req.Song + "\n"
	return source.Result{
		Lyrics: lyrics,
		Level:  source.SyncNone,
		Filled: source.FieldLyrics,
	}, nil
}
