//go:build test

// Package stub is a stub implementation of the Source contract.
// It self-registers nothing; registration happens explicitly in
// internal/bootstrap.RegisterAllMock.
//
// The stub now exercises the "source-required parameter" path: it
// advertises support for --author and refuses any fetch where Author
// is empty, returning source.RequiredParamError so the CLI can surface
// exit code 6. This lets the rest of the toolchain be tested against
// a deterministic offline adapter that covers both the "params
// unsupported" warning path and the "params required" error path
// without any network calls.
package stub

import (
	"context"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// Adapter implements source.Source as a deterministic stub.
type Adapter struct{}

// New returns a fresh Adapter instance.
func New() *Adapter { return &Adapter{} }

// Name returns the stable CLI identifier.
func (a *Adapter) Name() string { return "stub" }

// SupportedParams advertises that the stub honors the author filter
// (and *requires* it — see Fetch).
func (a *Adapter) SupportedParams() source.Param {
	return source.ParamAuthor
}

// Fetch returns a RequiredParamError when Author is empty and
// otherwise emits deterministic placeholder lyrics. The stub is the
// offline test adapter; it intentionally models the future-provider
// "must supply --author" semantics so the CLI's exit-6 branch has a
// faithful representative.
func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	if strings.TrimSpace(req.Author) == "" {
		return source.Result{}, source.RequiredParamError{
			Source: a.Name(),
			Param:  source.ParamAuthor,
			Flag:   "--author",
		}
	}
	lyrics := "[stub] lyrics for: " + req.Song + "\n"
	return source.Result{
		Lyrics: lyrics,
		Title:  req.Song,
		Artist: req.Author,
		Source: a.Name(),
	}, nil
}
