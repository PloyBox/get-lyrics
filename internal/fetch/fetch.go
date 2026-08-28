// Package fetch is the thin orchestration layer between the CLI and
// the pluggable source adapters. It prechecks every requested source
// (existence + required params), then tries them in user-given order
// per timestamp format, failing over on adapter errors and matching
// results against the requested SyncLevel via a per-call cache.
package fetch

import (
	"context"
	"errors"
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
	// Downgraded: the requested timestamp format got no match — synced
	// was requested but only plain lyrics returned, or plain was
	// requested but only synced lyrics returned. The unmatched result
	// stays cached and can satisfy a later iteration.
	Downgraded
	// PreCheck: --lenient mode skipped a source during precheck
	// (unknown name, missing required parameter, or duplicate).
	PreCheck
	// PrecheckMismatch: a source raised RequiredParamMismatchError from
	// Fetch — its capability declaration disagrees with what Fetch
	// actually needs. Flags a source implementation bug.
	PrecheckMismatch
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
	Kind      WarningKind  // which stage produced the warning
	Source    string       // name of the source the warning refers to
	Param     source.Param // typed parameter involved (UnsupportedParam / PreCheck); 0 for custom
	ParamName string       // custom --env key involved; empty for typed parameters
	Message   string       // pre-formatted, user-facing text (for stderr)
}

// Params bundles all CLI inputs the fetch layer needs. Source is the
// ordered list of source names to try (failover order). Timestamp is
// the ordered list of requested SyncLevels (SyncLine → synced,
// SyncNone → plain) — the CLI parses the --timestamp format names into
// them; the first match wins. Lenient controls the precheck
// stage only: when false, the first precheck problem aborts; when true,
// problem sources are skipped with a PreCheck warning.
type Params struct {
	Song      string
	Source    []string
	Author    string
	Album     string
	ISWC      string
	Timestamp []SyncLevel
	Lenient   bool
	// UserAgent is the HTTP User-Agent header to send on upstream
	// requests (from --user-agent). It is passed to every requested
	// source; empty means the source uses its own default UA.
	UserAgent string
	// Custom carries the user-supplied --env keys (plus process-
	// environment fallbacks injected by the CLI). Keys the caller did
	// not provide are absent; env-injected keys are treated exactly
	// like user-provided ones.
	Custom map[string]string
}

// SyncLevel classifies the lyrics content a fetch.Result carries by its
// timestamp format.
type SyncLevel uint8

const (
	// SyncUnknown: unknown / no valid lyrics content (neither lyrics
	// track was populated).
	SyncUnknown SyncLevel = iota
	// SyncNone: plain (non-timestamped) lyrics.
	SyncNone
	// SyncLine: synced (LRC timestamped) lyrics.
	SyncLine
)

// Result is the fetch layer's output, consolidating the two lyrics
// tracks into a single field. Level records which track Lyrics
// carries: SyncNone for plain text, SyncLine for synced (LRC) content,
// SyncUnknown when neither track was populated.
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
	Level     SyncLevel
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

// RequiredParamError reports a source whose Capabilities.Required list
// (or RequiredCustom list) includes a parameter the caller did not
// supply. The precheck stage builds it for the first missing field;
// adapters never return it themselves. The CLI maps it to exit code 6.
type RequiredParamError struct {
	Source    string       // adapter Name() that requires the parameter
	Param     source.Param // typed Param bit; 0 for a custom key
	ParamName string       // custom key name; empty for a typed parameter
	Flag      string       // CLI flag spelling: "--author" etc., or "--env <KEY>"
}

// Error renders a stable message; main prints it verbatim after the
// error[required] tag.
func (e RequiredParamError) Error() string {
	return "source \"" + e.Source + "\" requires " + e.Flag
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
// each requested timestamp format. The first result whose Level matches
// the current iteration is returned immediately; when nothing matches,
// every produced track is cached per call so a later iteration ("none"
// after "line") can reuse them without a second request.
//
// Error semantics:
//   - strict precheck (default) → the single first problem: a
//     DuplicateSourceError (exit 8), source.ErrNotFound (exit 3), or a
//     RequiredParamError (exit 6); no source is fetched, but warnings
//     accumulated before the abort (e.g. a gate-2 source-bug warning)
//     are returned with the error so the caller can print them first.
//   - lenient precheck (--lenient) → problem sources are skipped with a
//     PreCheck warning; eligible sources proceed.
//   - adapter errors during fetch → FetchFailed warning + fail over to
//     the next source (never aborts, regardless of lenient); a source
//     raising RequiredParamMismatchError instead becomes a
//     PrecheckMismatch warning + fail over.
//   - no result matched any timestamp format → NoResultError, with the
//     in-flight warnings so the caller can print why each source failed.
func (s *Service) Fetch(ctx context.Context, params Params) (Result, []Warning, error) {
	warnings := make([]Warning, 0, 4)
	eligible, err := s.precheck(params, &warnings)
	if err != nil {
		return Result{}, warnings, err
	}

	// Per-call cache of tracks that did not match the requested level.
	// Lookup scans for Source == name && Level == want.
	cache := make([]Result, 0, len(eligible))
	warnedUnsupported := make(map[string]bool, len(eligible))

	for _, want := range params.Timestamp {
		for _, name := range eligible {
			if hit := findCached(cache, name, want); hit != nil {
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
				Timestamp: want == SyncLine,
				UserAgent: params.UserAgent,
				Custom:    params.Custom,
			}
			sr, ferr := src.Fetch(ctx, req)
			if ferr != nil {
				var mm source.RequiredParamMismatchError
				if errors.As(ferr, &mm) {
					// Reuse mm.Flag directly: for a custom key Param is
					// 0 and flagForParam would render an empty spelling.
					warnings = append(warnings, Warning{
						Kind:      PrecheckMismatch,
						Source:    name,
						Param:     mm.Param,
						ParamName: mm.ParamName,
						Message:   fmt.Sprintf(`warning[precheck-mismatch]: source "%s" requires %s but precheck did not enforce it (source bug); trying next source`, name, mm.Flag),
					})
					continue
				}
				warnings = append(warnings, Warning{
					Kind:    FetchFailed,
					Source:  name,
					Message: fmt.Sprintf(`warning[fetch]: source "%s" failed: %v; trying next source`, name, ferr),
				})
				continue
			}

			warnings = append(warnings, detectResultMismatch(name, sr)...)

			match, storable := filtResult(src.Name(), sr, want)
			if match != nil {
				return *match, warnings, nil
			}
			if len(storable) > 0 {
				// Nothing matched the requested level: store every
				// produced track for later iterations and warn on the
				// downgrade. storable only holds tracks whose Level
				// differs from want, so want picks the message.
				cache = append(cache, storable...)
				msg := "returned no timestamped lyrics"
				if want == SyncNone {
					msg = "returned only timestamped lyrics"
				}
				warnings = append(warnings, Warning{
					Kind:    Downgraded,
					Source:  name,
					Message: fmt.Sprintf(`warning[downgraded]: source "%s" %s`, name, msg),
				})
			}
		}
	}

	return Result{}, warnings, NoResultError{}
}

// CustomParamsFor returns, in params.Source order, the static
// CustomParams() declaration of every source that passes validation; the
// map is keyed by source name. Only params.Source and params.Lenient
// participate — Song/Author/Album/ISWC/Timestamp/Custom are ignored.
//
// Strict mode: the first problem aborts with UnknownSourceError
// (unregistered name) or DuplicateSourceError (duplicate entry), the
// same codes as the Fetch precheck, surfaced before any fetch. Lenient
// mode: problem sources are silently skipped and never enter the map.
//
// It produces no warnings, performs no required-param checks, and does
// not trigger gate 2 — it is the read-only query main uses for the
// --help "Source parameters:" section and for the pre-fetch
// environment-variable fallback.
func (s *Service) CustomParamsFor(params Params) (map[string][]source.ParamSpec, error) {
	out := make(map[string][]source.ParamSpec)
	seen := make(map[string]bool, len(params.Source))
	for _, name := range params.Source {
		if seen[name] {
			if !params.Lenient {
				return nil, DuplicateSourceError{Name: name}
			}
			continue
		}
		seen[name] = true
		src, err := s.reg.Get(name)
		if err != nil {
			if !params.Lenient {
				return nil, UnknownSourceError{Name: name}
			}
			continue
		}
		out[name] = src.CustomParams()
	}
	return out, nil
}

// precheck walks params.Source in order, filtering out problem sources
// into *warnings under --lenient or aborting with the first single
// error in strict mode. The returned slice holds the eligible source
// names in the user-given order.
//
// Gate 2 runs before the missing-required check: a source whose
// request-aware custom declaration is inconsistent (a source bug) is
// skipped with a precheck-mismatch warning in BOTH strict and lenient
// mode — never a RequiredParamError, since the offending key cannot be
// legitimately supplied by the caller.
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
		if err != nil {
			if !params.Lenient {
				return nil, UnknownSourceError{Name: name}
			}
			*warnings = append(*warnings, Warning{
				Kind:    PreCheck,
				Source:  name,
				Message: fmt.Sprintf(`warning[precheck]: source "%s" skipped: not found`, name),
			})
			continue
		}

		req := requestFromParams(params)
		caps := src.Capabilities(req)

		if bad := validateCustomDecl(src, caps); bad != "" {
			*warnings = append(*warnings, Warning{
				Kind:      PrecheckMismatch,
				Source:    name,
				ParamName: bad,
				Message:   fmt.Sprintf(`warning[precheck-mismatch]: source "%s" declared invalid --env key %q (source bug)`, name, bad),
			})
			continue
		}

		missing, missingCustom, need := checkRequired(caps, params)
		if need {
			flag := flagForParam(missing)
			if missingCustom != "" {
				flag = "--env " + missingCustom
			}
			if !params.Lenient {
				return nil, RequiredParamError{
					Source:    src.Name(),
					Param:     missing,
					ParamName: missingCustom,
					Flag:      flag,
				}
			}
			*warnings = append(*warnings, Warning{
				Kind:      PreCheck,
				Source:    name,
				Param:     missing,
				ParamName: missingCustom,
				Message:   fmt.Sprintf(`warning[precheck]: source "%s" skipped: requires %s`, name, flag),
			})
			continue
		}
		eligible = append(eligible, name)
	}
	return eligible, nil
}

// validateCustomDecl enforces gate 2 on a source's request-aware custom
// declaration: every name in caps.Custom must be a legal key
// (ParamNamePattern) present in the static CustomParams() list, and
// RequiredCustom must be a duplicate-free subset of caps.Custom's
// names. It returns the first offending key name, or "" when the
// declaration is consistent.
func validateCustomDecl(src source.Source, caps source.Capabilities) string {
	static := make(map[string]bool, len(src.CustomParams()))
	for _, spec := range src.CustomParams() {
		static[spec.Name] = true
	}
	recognized := make(map[string]bool, len(caps.Custom))
	for _, spec := range caps.Custom {
		recognized[spec.Name] = true
		if !source.ValidParamName(spec.Name) || !static[spec.Name] {
			return spec.Name
		}
	}
	seen := make(map[string]bool, len(caps.RequiredCustom))
	for _, name := range caps.RequiredCustom {
		if !source.ValidParamName(name) || !static[name] || !recognized[name] {
			return name
		}
		if seen[name] {
			return name
		}
		seen[name] = true
	}
	return ""
}

// checkRequired compares the non-empty optional fields in params against
// caps and reports the first missing requirement: typed Required bits
// first (author, album, iswc), then RequiredCustom names in declaration
// order. missingParam is the first missing typed bit (0 when a custom
// key is missing); missingCustom is the first missing custom key name
// (empty when a typed bit is missing). The bool is true when anything is
// missing.
func checkRequired(caps source.Capabilities, params Params) (missingParam source.Param, missingCustom string, need bool) {
	if caps.Required&source.ParamAuthor != 0 && strings.TrimSpace(params.Author) == "" {
		return source.ParamAuthor, "", true
	}
	if caps.Required&source.ParamAlbum != 0 && strings.TrimSpace(params.Album) == "" {
		return source.ParamAlbum, "", true
	}
	if caps.Required&source.ParamISWC != 0 && strings.TrimSpace(params.ISWC) == "" {
		return source.ParamISWC, "", true
	}
	for _, name := range caps.RequiredCustom {
		if v, ok := params.Custom[name]; !ok || strings.TrimSpace(v) == "" {
			return 0, name, true
		}
	}
	return 0, "", false
}

// requestFromParams projects the CLI params onto a source.Request for
// capability queries. The timestamp flag is deliberately omitted:
// synced output is a runtime property, not part of capability checks.
// Custom is projected so capability queries see the user-supplied keys
// — conditional recognition/requirements (e.g. mock-custom's COUNTRY
// depending on LANG) would otherwise never hold in precheck and
// detectUnsupported.
func requestFromParams(params Params) source.Request {
	return source.Request{
		Song:   params.Song,
		Author: params.Author,
		Album:  params.Album,
		ISWC:   params.ISWC,
		Custom: params.Custom,
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
//
// Custom keys run a parallel path: every user-supplied key the adapter
// does not recognize for this request gets one warning. The map
// iteration order is unspecified on purpose — multiple unrecognized
// keys produce warnings in nondeterministic order; tests assert the
// warning set, never its order.
func detectUnsupported(params Params, src source.Source) []Warning {
	caps := src.Capabilities(requestFromParams(params))
	filters := caps.Filters
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

	recognized := make(map[string]bool, len(caps.Custom))
	for _, spec := range caps.Custom {
		recognized[spec.Name] = true
	}
	for key, value := range params.Custom {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if !recognized[key] {
			out = append(out, Warning{
				Kind:      UnsupportedParam,
				Source:    src.Name(),
				ParamName: key,
				Message:   fmt.Sprintf(`warning[unsupported]: source "%s" does not support --env %s`, src.Name(), key),
			})
		}
	}
	return out
}

// resultFieldSpecs lists every field tracked by the Filled mask, with
// the accessor used by the mismatch detector.
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

// filtResult converts an adapter result into the fetch.Result tracks it
// legitimately contains. Field population follows the adapter's Filled
// mask — never string contents: a field whose bit is unset is treated
// as empty. A declared-and-populated Lyrics yields one SyncNone track,
// a declared-and-populated SyncedLyrics one SyncLine track; both
// together yield two tracks sharing the adapter's metadata.
//
// Matching is level-based: when any produced track's Level equals want,
// it is returned as match and nothing is stored — the caller returns
// immediately. Only when nothing matches does the caller receive every
// produced track as storable for the per-call cache: want decides
// matching, never storage. When the adapter populated neither lyrics
// track (despite declaring one — a detectResultMismatch case), nothing
// is produced and nothing is stored.
func filtResult(srcName string, sr source.Result, want SyncLevel) (match *Result, storable []Result) {
	var tracks []Result
	addTrack := func(bit source.ResultField, text string, level SyncLevel) {
		if sr.Filled&bit == 0 || strings.TrimSpace(text) == "" {
			return
		}
		r := Result{Source: srcName, Lyrics: text, Level: level}
		if sr.Filled&source.FieldTitle != 0 {
			r.Title = sr.Title
		}
		if sr.Filled&source.FieldArtist != 0 {
			r.Artist = sr.Artist
		}
		if sr.Filled&source.FieldAlbum != 0 {
			r.Album = sr.Album
		}
		if sr.Filled&source.FieldISWC != 0 {
			r.ISWC = sr.ISWC
		}
		if sr.Filled&source.FieldSubSource != 0 {
			r.SubSource = sr.SubSource
		}
		tracks = append(tracks, r)
	}
	addTrack(source.FieldLyrics, sr.Lyrics, SyncNone)
	addTrack(source.FieldSyncedLyrics, sr.SyncedLyrics, SyncLine)
	if len(tracks) == 0 {
		return nil, nil
	}
	for i := range tracks {
		if tracks[i].Level == want {
			return &tracks[i], nil
		}
	}
	return nil, tracks
}

// findCached returns the first cached track produced by name whose
// Level matches want, or nil.
func findCached(cache []Result, name string, want SyncLevel) *Result {
	for i := range cache {
		if cache[i].Source == name && cache[i].Level == want {
			return &cache[i]
		}
	}
	return nil
}
