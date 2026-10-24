//go:build test

// Package mock provides table-driven source adapters for tests. It is built
// only under the test tag.
package mock

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

// spec configures one mock adapter; the table lives in specs.go.
type spec struct {
	name   string
	static []source.ParamSpec
	caps   func(source.Request) source.Capabilities
	fetch  func(context.Context, source.Request) (source.Result, error)
}

// Adapter is a source.Source backed by one spec.
type Adapter struct{ spec spec }

func (a *Adapter) Name() string { return a.spec.name }

func (a *Adapter) CustomParams() []source.ParamSpec { return a.spec.static }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return a.spec.caps(req)
}

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return a.spec.fetch(ctx, req)
}

// All returns one adapter per spec, in table order.
func All() []source.Source {
	out := make([]source.Source, 0, len(specs))
	for _, s := range specs {
		out = append(out, &Adapter{spec: s})
	}
	return out
}
