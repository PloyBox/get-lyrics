//go:build test

// Package useragent provides a test-only adapter that echoes the incoming
// Request.UserAgent, so a CLI run can prove which header reached the
// adapter.
package useragent

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-useragent" }

func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	return source.Capabilities{}
}

func (a *Adapter) CustomParams() []source.ParamSpec { return nil }

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	return source.Result{
		Lyrics: "[mock-useragent] " + req.UserAgent + "\n",
		Title:  req.Song,
		Level:  source.SyncNone,
		Filled: source.FieldLyrics | source.FieldTitle,
	}, nil
}
