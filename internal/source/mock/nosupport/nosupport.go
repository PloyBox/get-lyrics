//go:build test

package nosupport

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-nosupport" }

func (a *Adapter) SupportedParams() source.Param { return 0 }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	lyrics := "[mock-nosupport] lyrics for: " + req.Song + "\n"
	return source.Result{Lyrics: lyrics, Title: req.Song, Source: a.Name()}, nil
}
