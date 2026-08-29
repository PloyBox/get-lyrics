package fetch

import (
	"context"
	"testing"

	"github.com/PloyBox/get-lyrics/source"
)

type fakeSrc struct {
	name       string
	caps       source.Capabilities
	capsFn     func(source.Request) source.Capabilities
	custom     []source.ParamSpec
	fetch      func(context.Context, source.Request) (source.Result, error)
	fetchCalls int
}

func (f *fakeSrc) Name() string { return f.name }
func (f *fakeSrc) Capabilities(req source.Request) source.Capabilities {
	if f.capsFn != nil {
		return f.capsFn(req)
	}
	return f.caps
}
func (f *fakeSrc) CustomParams() []source.ParamSpec { return f.custom }
func (f *fakeSrc) Fetch(ctx context.Context, r source.Request) (source.Result, error) {
	f.fetchCalls++
	return f.fetch(ctx, r)
}

func newRegistry(t *testing.T, ss ...source.Source) *source.Registry {
	t.Helper()
	r := source.NewRegistry()
	for _, s := range ss {
		if err := r.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name(), err)
		}
	}
	return r
}
