//go:build test

package success

import (
	"context"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-success" }

func (a *Adapter) SupportedParams() source.Param { return source.ParamAuthor }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Author) == "" {
		return source.Result{}, source.RequiredParamError{
			Source: a.Name(), Param: source.ParamAuthor, Flag: "--author",
		}
	}
	lyrics := "[mock-success] lyrics for: " + req.Song + "\n"
	return source.Result{Lyrics: lyrics, Title: req.Song, Artist: req.Author, Source: a.Name()}, nil
}
