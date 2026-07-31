// Package fetch is the thin orchestration layer between the CLI and
// the pluggable source adapters. It resolves the requested source,
// detects parameters the source does not support, and emits a Warning
// per such parameter without aborting the fetch.
package fetch

import (
	"context"
	"fmt"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// Warning describes one user-supplied parameter that the chosen source
// does not support. The CLI writes each Warning.Message to stderr verbatim.
type Warning struct {
	Source  string       // name of the source the warning refers to
	Param   source.Param // which parameter is unsupported
	Message string       // pre-formatted, user-facing text (for stderr)
}

// Service fetches lyrics through a source registry.
type Service struct {
	reg *source.Registry
}

// New returns a Service bound to reg.
func New(reg *source.Registry) *Service {
	return &Service{reg: reg}
}

// Fetch resolves srcName, computes per-parameter warnings, then calls
// src.Fetch. The returned Warnings slice is never nil — it is empty
// when every supplied parameter is supported (or none were supplied).
//
// Error semantics:
//   - source.ErrNotFound → returned as-is; main prints "unknown source".
//   - adapter error      → returned as-is; main prints it and exits non-zero.
//     Warnings are discarded on hard failure so callers do not see
//     "unsupported param" noise when the lookup itself failed.
//   - successful fetch   → result is non-zero; warnings may still be present.
func (s *Service) Fetch(ctx context.Context, req source.Request, srcName string) (source.Result, []Warning, error) {
	src, err := s.reg.Get(srcName)
	if err != nil {
		return source.Result{}, nil, err
	}

	warnings := detectUnsupported(req, src)

	result, err := src.Fetch(ctx, req)
	if err != nil {
		return source.Result{}, nil, err
	}

	if result.Source == "" {
		result.Source = src.Name()
	}
	return result, warnings, nil
}

// detectUnsupported compares the non-empty optional fields in req
// against src.SupportedParams() and returns one Warning per mismatch.
//
// Behavior:
//   - Author/Album/ISWC: one warning per non-empty field the source
//     does not honor.
//   - Timestamp: one warning if Request.Timestamp==true and the source
//     does not honor ParamTimestamp. fetch still calls the adapter;
//     the output layer falls back to plain text.
func detectUnsupported(req source.Request, src source.Source) []Warning {
	supported := src.SupportedParams()
	out := make([]Warning, 0, 4)

	if req.Author != "" && supported&source.ParamAuthor == 0 {
		out = append(out, Warning{
			Source:  src.Name(),
			Param:   source.ParamAuthor,
			Message: FormatWarning(src.Name(), "--author"),
		})
	}
	if req.Album != "" && supported&source.ParamAlbum == 0 {
		out = append(out, Warning{
			Source:  src.Name(),
			Param:   source.ParamAlbum,
			Message: FormatWarning(src.Name(), "--album"),
		})
	}
	if req.ISWC != "" && supported&source.ParamISWC == 0 {
		out = append(out, Warning{
			Source:  src.Name(),
			Param:   source.ParamISWC,
			Message: FormatWarning(src.Name(), "--iswc"),
		})
	}
	if req.Timestamp && supported&source.ParamTimestamp == 0 {
		out = append(out, Warning{
			Source:  src.Name(),
			Param:   source.ParamTimestamp,
			Message: FormatWarning(src.Name(), "--timestamp"),
		})
	}
	return out
}

// FormatWarning produces the canonical stderr message for one
// unsupported-parameter case.
func FormatWarning(sourceName, flag string) string {
	return fmt.Sprintf("warning: source %q does not support %s", sourceName, flag)
}
