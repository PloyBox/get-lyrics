//go:build test

package fail

import (
	"context"
	"errors"

	"github.com/PloyBox/get-lyrics/internal/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-fail" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{Filters: source.ParamAuthor}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{}, errors.New("mock-fail: intentional fetch failure")
}
