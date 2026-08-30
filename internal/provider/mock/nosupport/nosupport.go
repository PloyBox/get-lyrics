//go:build test

package nosupport

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-nosupport" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	lyrics := "[mock-nosupport] lyrics for: " + req.Song + "\n"
	return source.Result{
		Lyrics: lyrics,
		Filled: source.FieldLyrics,
	}, nil
}
