//go:build test

// Package gate2 provides a test-only adapter whose request-aware
// declaration is deliberately inconsistent: the static CustomParams()
// list is empty, so gate 1 (registration) passes, while RequiredCustom
// names an illegal key that gate 2 (precheck) flags.
package gate2

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-gate2" }

// Capabilities is self-contradictory on purpose. Required is set as well,
// so a gate-2 regression surfaces as exit 6 instead of a silent pass.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{
		Required:       source.ParamAuthor, // never honored: gate 2 skips us first
		RequiredCustom: []string{"LANG-X"}, // gate-2 violation
	}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{}, nil
}
