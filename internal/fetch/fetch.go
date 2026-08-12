// Package fetch is the thin orchestration layer between the CLI and
// the pluggable source adapters. It resolves the requested source,
// detects parameters the source does not support, resolves synced vs
// plain lyrics, and emits a Warning per mismatch.
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

// Params bundles all CLI inputs the fetch layer needs. Source is a
// slice to support future multi-source dispatch; today only the first
// element is used. Timestamp is a slice so the CLI can split a future
// string flag value; today "line" means enabled, "none" means disabled.
type Params struct {
	Song      string
	Source    []string
	Author    string
	Album     string
	ISWC      string
	Timestamp []string
}

// Result is the fetch layer's output, consolidating the two lyrics
// tracks into a single field. Synced is true when Lyrics contains
// synced (LRC) content. Downgraded is true when synced was requested
// and the source supports it, but returned no synced lyrics — the
// caller should emit a FormatNoSyncedWarning.
type Result struct {
	Lyrics     string
	Title      string
	Artist     string
	Album      string
	ISWC       string
	Source     string
	Synced     bool
	Downgraded bool
}

// Service fetches lyrics through a source registry.
type Service struct {
	reg *source.Registry
}

// New returns a Service bound to reg.
func New(reg *source.Registry) *Service {
	return &Service{reg: reg}
}

// Fetch resolves the source from params.Source[0], builds a
// source.Request, computes per-parameter warnings, calls src.Fetch,
// and resolves synced vs plain lyrics into a single Lyrics field.
// The returned Warnings slice is never nil — it is empty when every
// supplied parameter is supported (or none were supplied).
//
// Error semantics:
//   - source.ErrNotFound → returned as-is; main prints "unknown source".
//   - adapter error      → returned as-is; main prints it and exits non-zero.
//     Warnings are discarded on hard failure so callers do not see
//     "unsupported param" noise when the lookup itself failed.
//   - successful fetch   → result is non-zero; warnings may still be present.
func (s *Service) Fetch(ctx context.Context, params Params) (Result, []Warning, error) {
	srcName := ""
	if len(params.Source) > 0 {
		srcName = params.Source[0]
	}
	src, err := s.reg.Get(srcName)
	if err != nil {
		return Result{}, nil, err
	}

	wantSynced := isTimestampLine(params.Timestamp)
	req := source.Request{
		Song:      params.Song,
		Author:    params.Author,
		Album:     params.Album,
		ISWC:      params.ISWC,
		Timestamp: wantSynced,
	}

	warnings := detectUnsupported(req, src)

	sr, err := src.Fetch(ctx, req)
	if err != nil {
		return Result{}, nil, err
	}

	resultSource := sr.Source
	if resultSource == "" {
		resultSource = src.Name()
	} else {
		resultSource = src.Name() + "#" + resultSource
	}

	srcSupportsTS := src.SupportedParams()&source.ParamTimestamp != 0
	synced := wantSynced && srcSupportsTS && sr.SyncedLyrics != ""
	downgraded := wantSynced && srcSupportsTS && sr.SyncedLyrics == ""

	lyrics := sr.Lyrics
	if synced {
		lyrics = sr.SyncedLyrics
	}

	return Result{
		Lyrics:     lyrics,
		Title:      sr.Title,
		Artist:     sr.Artist,
		Album:      sr.Album,
		ISWC:       sr.ISWC,
		Source:     resultSource,
		Synced:     synced,
		Downgraded: downgraded,
	}, warnings, nil
}

// isTimestampLine returns true when the first element of ts indicates
// LRC lyrics were requested. Today "line" means enabled.
func isTimestampLine(ts []string) bool {
	return len(ts) > 0 && ts[0] == "line"
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

// FormatNoSyncedWarning produces the canonical stderr message when a
// source supports timestamped lyrics but returned none for this track.
func FormatNoSyncedWarning(sourceName string) string {
	return fmt.Sprintf(`warning: source "%s" returned no timestamped lyrics; using plain lyrics`, sourceName)
}
