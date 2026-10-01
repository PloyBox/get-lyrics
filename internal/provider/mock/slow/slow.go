//go:build test

package slow

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-slow" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

// Fetch blocks until ctx is done, simulating a hung upstream.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	<-ctx.Done()
	return source.Result{}, ctx.Err()
}
