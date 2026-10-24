//go:build test

package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PloyBox/get-lyrics/source"
)

// allFilters is the complete typed-parameter set.
const allFilters = source.ParamAuthor | source.ParamAlbum |
	source.ParamISRC | source.ParamDuration

// plainLyrics is the shared plain-text payload.
const plainLyrics = "plain lyrics line one\nplain lyrics line two"

// mockKey and mockOpt are the custom keys declared by mock-customparams.
const (
	mockKey = "MOCK_KEY"
	mockOpt = "MOCK_OPT"
)

// customParams declares the two keys of mock-customparams.
var customParams = []source.ParamSpec{
	{Name: mockKey, Description: "Required mock key"},
	{Name: mockOpt, Description: "Optional mock key"},
}

// tokenParam is declared statically yet never enforced by precheck.
var tokenParam = []source.ParamSpec{
	{Name: "MOCK_TOKEN", Description: "Mock token (never enforced)"},
}

// plainFetch serves the shared plain lyrics without reading the request.
func plainFetch(context.Context, source.Request) (source.Result, error) {
	return plainResult(plainLyrics), nil
}

// specs is the mock source table; All turns each entry into an adapter.
var specs = []spec{
	{name: "mock-plain", caps: capsAll, fetch: plainFetch},
	{
		name: "mock-lrc",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return syncResult("[00:01.00]line one\n[00:02.00]line two",
				source.SyncLine), nil
		},
	},
	{
		name: "mock-word",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			res := syncResult(
				`<tt xmlns="http://www.w3.org/ns/ttml">`+
					`<body><p>word one</p></body></tt>`,
				source.SyncWord)
			res.SubSource = "mock-ttml"
			res.Filled |= source.FieldSubSource
			return res, nil
		},
	},
	{
		name: "mock-fail",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, errors.New("mock-fail: upstream failure")
		},
	},
	{
		name: "mock-notfound",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, fmt.Errorf("mock-notfound: %w", source.ErrNotFound)
		},
	},
	{
		name: "mock-bodylimit",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{},
				fmt.Errorf("mock-bodylimit: %w", source.ErrBodyTooLarge)
		},
	},
	{
		name: "mock-require-author",
		caps: func(_ source.Request) source.Capabilities {
			return source.Capabilities{Filters: allFilters,
				Required: source.ParamAuthor}
		},
		fetch: plainFetch,
	},
	{
		name:   "mock-customparams",
		static: customParams,
		caps: func(_ source.Request) source.Capabilities {
			return source.Capabilities{Filters: allFilters, Custom: customParams,
				RequiredCustom: []string{mockKey}}
		},
		fetch: func(_ context.Context, req source.Request) (source.Result, error) {
			return plainResult("MOCK_KEY=" + req.Custom[mockKey] +
				"\nMOCK_OPT=" + req.Custom[mockOpt]), nil
		},
	},
	{
		name: "mock-gate2",
		caps: func(_ source.Request) source.Capabilities {
			return source.Capabilities{Filters: allFilters,
				Custom: []source.ParamSpec{
					{Name: "BAD_KEY", Description: "Absent from static list"},
				}}
		},
		fetch: plainFetch,
	},
	{
		name:   "mock-fetchmismatch",
		static: tokenParam,
		caps: func(_ source.Request) source.Capabilities {
			return source.Capabilities{Filters: allFilters, Custom: tokenParam}
		},
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{}, source.RequiredParamMismatchError{
				Source: "mock-fetchmismatch", ParamName: "MOCK_TOKEN"}
		},
	},
	{
		name: "mock-result-mismatch",
		caps: capsAll,
		fetch: func(_ context.Context, _ source.Request) (source.Result, error) {
			return source.Result{Filled: source.FieldLyrics | source.FieldTitle,
				Lyrics: plainLyrics, Level: source.SyncNone}, nil
		},
	},
	{
		name:  "mock-nosupport",
		caps:  func(_ source.Request) source.Capabilities { return source.Capabilities{} },
		fetch: plainFetch,
	},
	{
		name: "mock-block",
		caps: capsAll,
		fetch: func(ctx context.Context, _ source.Request) (source.Result, error) {
			<-ctx.Done()
			return source.Result{}, ctx.Err()
		},
	},
	{
		name: "mock-capture",
		caps: capsAll,
		fetch: func(_ context.Context, req source.Request) (source.Result, error) {
			data, err := json.Marshal(req)
			if err != nil {
				return source.Result{}, fmt.Errorf("mock-capture: %w", err)
			}
			return plainResult(string(data)), nil
		},
	},
}

// capsAll reports support for every typed filter.
func capsAll(source.Request) source.Capabilities {
	return source.Capabilities{Filters: allFilters}
}

// plainResult builds a plain-text result.
func plainResult(lyrics string) source.Result {
	return syncResult(lyrics, source.SyncNone)
}

// syncResult builds a result at the given sync level.
func syncResult(lyrics string, level source.SyncLevel) source.Result {
	return source.Result{Filled: source.FieldLyrics, Lyrics: lyrics, Level: level}
}
