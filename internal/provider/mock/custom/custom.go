//go:build test

package custom

import (
	"context"

	"github.com/PloyBox/get-lyrics/source"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "mock-custom" }

// CustomParams statically declares LANG and COUNTRY, independent of any
// request. LANG is always recognized and required; COUNTRY is only
// recognized (and required) when LANG is present — demonstrating
// conditional custom parameters.
func (a *Adapter) CustomParams() []source.ParamSpec {
	return []source.ParamSpec{
		{Name: "LANG", Description: "language hint"},
		{Name: "COUNTRY", Description: "country hint"},
	}
}

// Capabilities recognizes LANG unconditionally (and requires it);
// COUNTRY joins both lists only when LANG was supplied.
func (a *Adapter) Capabilities(req source.Request) source.Capabilities {
	caps := source.Capabilities{
		Custom:         []source.ParamSpec{{Name: "LANG", Description: "language hint"}},
		RequiredCustom: []string{"LANG"},
	}
	if _, ok := req.Custom["LANG"]; ok {
		caps.Custom = append(caps.Custom, source.ParamSpec{Name: "COUNTRY", Description: "country hint"})
		caps.RequiredCustom = append(caps.RequiredCustom, "COUNTRY")
	}
	return caps
}

func (a *Adapter) Fetch(ctx context.Context, req source.Request) (source.Result, error) {
	lyrics := "[mock-custom] lyrics for: " + req.Song + " (lang=" + req.Custom["LANG"] + ")\n"
	return source.Result{
		Lyrics: lyrics,
		Title:  req.Song,
		Filled: source.FieldLyrics | source.FieldTitle,
	}, nil
}
