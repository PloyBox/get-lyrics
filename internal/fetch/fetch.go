// Package fetch is the thin orchestration layer between the CLI and
// the pluggable source adapters. It prechecks every requested source
// (existence + required params), then tries them in user-given order
// per timestamp format, failing over on adapter errors and matching
// results against the requested synced/plain flag via a per-call cache.
package fetch

import (
	"context"
	"fmt"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// WarningKind classifies a Warning by the stage that produced it.
type WarningKind int

const (
	// UnsupportedParam: a user-supplied optional parameter the source
	// does not honor; emitted alongside a successful result.
	UnsupportedParam WarningKind = iota
	// Downgraded: synced lyrics were requested but the source returned
	// plain lyrics only.
	Downgraded
	// PreCheck: --lenient mode skipped a source during precheck
	// (unknown name, missing required parameter, or duplicate).
	PreCheck
	// FetchFailed: the adapter returned an error; fetch moved on to the
	// next source.
	FetchFailed
	// ResultMismatch: the adapter's Filled mask disagrees with the
	// actual field contents (a declared field left empty, or a filled
	// field not declared). The result is still used as-is (trust
	// policy); the warning flags a source implementation problem.
	ResultMismatch
)

// Warning describes one issue observed while resolving lyrics. The CLI
// writes each Warning.Message to stderr verbatim — messages are
// pre-formatted here, including the [kind] tag.
type Warning struct {
	Kind    WarningKind  // which stage produced the warning
	Source  string       // name of the source the warning refers to
	Param   source.Param // parameter involved (UnsupportedParam / PreCheck)
	Message string       // pre-formatted, user-facing text (for stderr)
}

// Params bundles all CLI inputs the fetch layer needs. Source is the
// ordered list of source names to try (failover order). Timestamp is
// the ordered list of requested output formats ("line" → synced,
// "none" → plain); the first match wins. Lenient controls the precheck
// stage only: when false, the first precheck problem aborts; when true,
// problem sources are skipped with a PreCheck warning.
type Params struct {
	Song      string
	Source    []string
	Author    string
	Album     string
	ISWC      string
	Timestamp []string
	Lenient   bool
}

// Result is the fetch layer's output, consolidating the two lyrics
// tracks into a single field. Synced is true when Lyrics contains
// synced (LRC) content.
//
// Source always names the adapter that produced the result. SubSource
// is the sub-source identifier reported by aggregate sources (e.g. a
// multi-provider aggregator); standalone adapters leave it empty.
type Result struct {
	Lyrics    string
	Title     string
	Artist    string
	Album     string
	ISWC      string
	Source    string // adapter that produced the result
	SubSource string // sub-source for aggregate adapters, copied only when the adapter declared FieldSubSource; empty otherwise
	Synced    bool
}

// NoResultError is returned when every source was skipped or failed and
// no result matched any requested timestamp format. The CLI maps it to
// exit code 4 and prints the collected warnings first.
type NoResultError struct{}

func (NoResultError) Error() string { return "no source returned a valid result" }

// UnknownSourceError identifies the requested but unregistered source
// name. It unwraps to source.ErrNotFound so callers can match with
// errors.Is and still render the offending name.
type UnknownSourceError struct {
	Name string
}

func (e UnknownSourceError) Error() string {
	return fmt.Sprintf("source %q not found", e.Name)
}

func (e UnknownSourceError) Unwrap() error { return source.ErrNotFound }

// DuplicateSourceError identifies a source name listed more than once.
// The CLI maps it to a dedicated exit code (8); --lenient mode instead
// emits a PreCheck warning and drops the duplicate.
type DuplicateSourceError struct {
	Name string
}

func (e DuplicateSourceError) Error() string {
	return fmt.Sprintf("source %q is listed more than once", e.Name)
}

// Service fetches lyrics through a source registry.
type Service struct {
	reg *source.Registry
}

// New returns a Service bound to reg.
func New(reg *source.Registry) *Service {
	return &Service{reg: reg}
}

// Fetch prechecks every requested source, then tries them in order for
// each requested timestamp format. The first result whose Synced flag
// matches the current iteration is returned immediately; mismatched
// results are cached per call so a later iteration ("none" after
// "line") can reuse them without a second request.
//
// Error semantics:
//   - strict precheck (default) → the single first problem: a
//     DuplicateSourceError (exit 8), source.ErrNotFound (exit 3), or a
//     source.RequiredParamError (exit 6); no source is fetched and
//     warnings are empty.
//   - lenient precheck (--lenient) → problem sources are skipped with a
//     PreCheck warning; eligible sources proceed.
//   - adapter errors during fetch → FetchFailed warning + fail over to
//     the next source (never aborts, regardless of lenient).
//   - no result matched any timestamp format → NoResultError, with the
//     in-flight warnings so the caller can print why each source failed.
func (s *Service) Fetch(ctx context.Context, params Params) (Result, []Warning, error) {
	warnings := make([]Warning, 0, 4)
	eligible, err := s.precheck(params, &warnings)
	if err != nil {
		return Result{}, nil, err
	}

	// Per-call cache of results that did not match the requested flag.
	// Lookup scans for Source == name && Synced == current flag.
	cache := make([]Result, 0, len(eligible))
	warnedUnsupported := make(map[string]bool, len(eligible))

	for _, tsName := range params.Timestamp {
		wantSynced := tsName == "line"

		for _, name := range eligible {
			if hit := findCached(cache, name, wantSynced); hit != nil {
				return *hit, warnings, nil
			}

			// eligible sources passed precheck, so Get cannot fail here.
			src, _ := s.reg.Get(name)
			if !warnedUnsupported[name] {
				warnings = append(warnings, detectUnsupported(params, src)...)
				warnedUnsupported[name] = true
			}

			req := source.Request{
				Song:      params.Song,
				Author:    params.Author,
				Album:     params.Album,
				ISWC:      params.ISWC,
				Timestamp: wantSynced,
			}
			sr, ferr := src.Fetch(ctx, req)
			if ferr != nil {
				warnings = append(warnings, Warning{
					Kind:    FetchFailed,
					Source:  name,
					Message: fmt.Sprintf(`warning[fetch]: source "%s" failed: %v; trying next source`, name, ferr),
				})
				continue
			}

			warnings = append(warnings, detectResultMismatch(name, sr)...)

			res := toResult(src.Name(), sr, wantSynced)
			if wantSynced && !res.Synced {
				warnings = append(warnings, Warning{
					Kind:    Downgraded,
					Source:  name,
					Message: fmt.Sprintf(`warning[downgraded]: source "%s" returned no timestamped lyrics`, name),
				})
			}

			if res.Synced == wantSynced {
				return res, warnings, nil
			}
			cache = append(cache, res)
		}
	}

	return Result{}, warnings, NoResultError{}
}

// precheck walks params.Source in order, filtering out problem sources
// into *warnings under --lenient or aborting with the first single
// error in strict mode. The returned slice holds the eligible source
// names in the user-given order.
func (s *Service) precheck(params Params, warnings *[]Warning) ([]string, error) {
	eligible := make([]string, 0, len(params.Source))
	seen := make(map[string]bool, len(params.Source))
	for _, name := range params.Source {
		if seen[name] {
			if !params.Lenient {
				return nil, DuplicateSourceError{Name: name}
			}
			*warnings = append(*warnings, Warning{
				Kind:    PreCheck,
				Source:  name,
				Message: fmt.Sprintf(`warning[precheck]: source "%s" skipped: duplicate`, name),
			})
			continue
		}
		seen[name] = true

		src, err := s.reg.Get(name)
		missing, need := source.Param(0), false
		if err == nil {
			missing, need = checkRequired(src, params)
		}

		if err != nil || need {
			if !params.Lenient {
				if err != nil {
					return nil, UnknownSourceError{Name: name}
				}
				return nil, source.RequiredParamError{
					Source: src.Name(),
					Param:  missing,
					Flag:   flagForParam(missing),
				}
			}

			msg := fmt.Sprintf(`warning[precheck]: source "%s" skipped: not found`, name)
			if err == nil {
				msg = fmt.Sprintf(`warning[precheck]: source "%s" skipped: requires %s`, name, flagForParam(missing))
			}
			*warnings = append(*warnings, Warning{
				Kind:    PreCheck,
				Source:  name,
				Param:   missing,
				Message: msg,
			})
			continue
		}
		eligible = append(eligible, name)
	}
	return eligible, nil
}

// checkRequired compares the non-empty optional fields in params against
// the adapter's required capabilities and reports the first missing bit.
// The returned bool is true when a required parameter is absent.
func checkRequired(src source.Source, params Params) (source.Param, bool) {
	req := src.Capabilities(requestFromParams(params)).Required
	if req&source.ParamAuthor != 0 && strings.TrimSpace(params.Author) == "" {
		return source.ParamAuthor, true
	}
	if req&source.ParamAlbum != 0 && strings.TrimSpace(params.Album) == "" {
		return source.ParamAlbum, true
	}
	if req&source.ParamISWC != 0 && strings.TrimSpace(params.ISWC) == "" {
		return source.ParamISWC, true
	}
	return 0, false
}

// requestFromParams projects the CLI params onto a source.Request for
// capability queries. The timestamp flag is deliberately omitted:
// synced output is a runtime property, not part of capability checks.
func requestFromParams(params Params) source.Request {
	return source.Request{
		Song:   params.Song,
		Author: params.Author,
		Album:  params.Album,
		ISWC:   params.ISWC,
	}
}

// flagForParam maps a Param bit to the CLI flag spelling used in
// messages and RequiredParamError.
func flagForParam(p source.Param) string {
	switch p {
	case source.ParamAuthor:
		return "--author"
	case source.ParamAlbum:
		return "--album"
	case source.ParamISWC:
		return "--iswc"
	}
	return ""
}

// detectUnsupported compares the non-empty optional fields in params
// against the adapter's filters for this request and returns one
// UnsupportedParam warning per mismatch. The timestamp format is
// deliberately excluded: a synced request on a plain-only source is
// covered by the Downgraded warning.
func detectUnsupported(params Params, src source.Source) []Warning {
	filters := src.Capabilities(requestFromParams(params)).Filters
	out := make([]Warning, 0, 3)

	if strings.TrimSpace(params.Author) != "" && filters&source.ParamAuthor == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamAuthor,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --author`, src.Name()),
		})
	}
	if strings.TrimSpace(params.Album) != "" && filters&source.ParamAlbum == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamAlbum,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --album`, src.Name()),
		})
	}
	if strings.TrimSpace(params.ISWC) != "" && filters&source.ParamISWC == 0 {
		out = append(out, Warning{
			Kind:    UnsupportedParam,
			Source:  src.Name(),
			Param:   source.ParamISWC,
			Message: fmt.Sprintf(`warning[unsupported]: source "%s" does not support --iswc`, src.Name()),
		})
	}
	return out
}

// resultFieldSpecs lists every field tracked by the Filled mask, with
// the accessor used both by toResult and by the mismatch detector.
type resultFieldSpec struct {
	bit   source.ResultField
	name  string
	value func(source.Result) string
}

var resultFieldSpecs = []resultFieldSpec{
	{source.FieldLyrics, "Lyrics", func(r source.Result) string { return r.Lyrics }},
	{source.FieldSyncedLyrics, "SyncedLyrics", func(r source.Result) string { return r.SyncedLyrics }},
	{source.FieldTitle, "Title", func(r source.Result) string { return r.Title }},
	{source.FieldArtist, "Artist", func(r source.Result) string { return r.Artist }},
	{source.FieldAlbum, "Album", func(r source.Result) string { return r.Album }},
	{source.FieldISWC, "ISWC", func(r source.Result) string { return r.ISWC }},
	{source.FieldSubSource, "SubSource", func(r source.Result) string { return r.SubSource }},
}

// detectResultMismatch compares sr.Filled against the actual field
// contents and reports one warning per inconsistency: a declared bit
// with an empty value, or a non-empty value without a declared bit.
// Either way the result is still used as-is (trust policy) — the
// warning only flags a source implementation problem.
func detectResultMismatch(srcName string, sr source.Result) []Warning {
	out := make([]Warning, 0, 2)
	for _, spec := range resultFieldSpecs {
		declared := sr.Filled&spec.bit != 0
		empty := strings.TrimSpace(spec.value(sr)) == ""
		if declared && empty {
			out = append(out, Warning{
				Kind:    ResultMismatch,
				Source:  srcName,
				Message: fmt.Sprintf(`warning[result]: source "%s" declares field %q but left it empty (source issue)`, srcName, spec.name),
			})
		} else if !declared && !empty {
			out = append(out, Warning{
				Kind:    ResultMismatch,
				Source:  srcName,
				Message: fmt.Sprintf(`warning[result]: source "%s" filled field %q without declaring it (source issue)`, srcName, spec.name),
			})
		}
	}
	return out
}

// toResult converts an adapter result into a fetch.Result. Field
// population follows the adapter's Filled mask — never string contents:
// a field whose bit is unset is treated as empty. When synced output
// was requested and the adapter declared SyncedLyrics, Lyrics carries
// the timestamped track and Synced is true; otherwise Lyrics carries
// the declared plain track and Synced is false.
func toResult(srcName string, sr source.Result, wantSynced bool) Result {
	res := Result{Source: srcName}
	for _, spec := range resultFieldSpecs {
		if sr.Filled&spec.bit != 0 {
			switch spec.bit {
			case source.FieldLyrics:
				res.Lyrics = sr.Lyrics
			case source.FieldSyncedLyrics:
				if wantSynced {
					res.Lyrics = sr.SyncedLyrics
					res.Synced = true
				}
			case source.FieldTitle:
				res.Title = sr.Title
			case source.FieldArtist:
				res.Artist = sr.Artist
			case source.FieldAlbum:
				res.Album = sr.Album
			case source.FieldISWC:
				res.ISWC = sr.ISWC
			case source.FieldSubSource:
				res.SubSource = sr.SubSource
			}
		}
	}
	return res
}

// findCached returns the first cached result produced by name whose
// Synced flag matches wantSynced, or nil.
func findCached(cache []Result, name string, wantSynced bool) *Result {
	for i := range cache {
		if cache[i].Source == name && cache[i].Synced == wantSynced {
			return &cache[i]
		}
	}
	return nil
}
