//go:build test

package require

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-require" }

// Capabilities demands --author while listing no filters: the source
// requires the field but does not treat it as an optional refinement,
// exercising the required-param precheck in isolation.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Required: source.ParamAuthor}
}

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	lyrics := "[mock-require] lyrics for: " + req.Song + "\n"
	return source.Result{
		Lyrics: lyrics,
		Title:  req.Song,
		Artist: req.Author,
		Filled: source.FieldLyrics | source.FieldTitle | source.FieldArtist,
	}, nil
}
