//go:build test

package require

import (
	"context"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-require" }

func (a *Adapter) SupportedParams() source.Param { return 0 }

// RequiredParams advertises ParamAuthor while SupportedParams stays 0:
// the source demands --author but does not treat it as an optional
// refinement, exercising the required-param precheck in isolation.
func (a *Adapter) RequiredParams() source.Param { return source.ParamAuthor }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	lyrics := "[mock-require] lyrics for: " + req.Song + "\n"
	return source.Result{Lyrics: lyrics, Title: req.Song, Artist: req.Author}, nil
}
