// Package fetch is the thin orchestration layer between the CLI and
// the pluggable source adapters. It prechecks every requested source
// (existence + required params), then tries them in user-given order
// per sync level, failing over on adapter errors and matching
// results against the requested SyncLevel via a per-call cache.
//
// File layout: params.go holds the request-side types (Params,
// SyncLevel, request projection), result.go the output type and track
// matching, errors.go the typed errors, warnings.go the Warning
// contract and warning detectors, and precheck.go the precheck stage
// (gate 2 + required-param checks). fetch.go keeps the orchestration
// core only.
package fetch

import (
	"context"
	"errors"

	"github.com/PloyBox/get-lyrics/source"
)

// Service fetches lyrics through a source registry.
type Service struct {
	reg *source.Registry
}

// New returns a Service bound to reg.
func New(reg *source.Registry) *Service {
	return &Service{reg: reg}
}

// Fetch prechecks every requested source, then tries them in order for
// each requested sync level. The first result whose Level matches
// the current iteration is returned immediately; when nothing matches,
// every produced track is cached per call so a later iteration ("none"
// after "line") can reuse them without a second request.
//
// Error semantics:
//   - SyncUnknown in Params.SyncLevels → InvalidSyncLevelError (a
//     caller bug; rejected before any per-source validation, in BOTH
//     strict and lenient mode).
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
//   - no result matched any sync level → NoResultError, with the
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

	for _, want := range params.SyncLevels {
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
				ISRC:      params.ISRC,
				Duration:  params.Duration,
				SyncLevel: sourceSyncLevel(want),
				UserAgent: params.UserAgent,
				Custom:    params.Custom,
			}
			sr, ferr := src.Fetch(ctx, req)
			if ferr != nil {
				var mm source.RequiredParamMismatchError
				if errors.As(ferr, &mm) {
					warnings = append(warnings, Warning{
						Kind:      PrecheckMismatch,
						Source:    name,
						Param:     mm.Param,
						ParamName: mm.ParamName,
						Err:       ferr,
					})
					continue
				}
				warnings = append(warnings, Warning{
					Kind:   FetchFailed,
					Source: name,
					Err:    ferr,
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
				// differs from want, so Want records the requested
				// direction.
				cache = append(cache, storable...)
				warnings = append(warnings, Warning{
					Kind:   Downgraded,
					Source: name,
					Want:   want,
				})
			}
		}
	}

	return Result{}, warnings, NoResultError{}
}

// CustomParamsFor returns, in params.Source order, the static
// CustomParams() declaration of every source that passes validation; the
// map is keyed by source name. Only params.Source and params.Lenient
// participate — Song/Author/Album/ISRC/Duration/SyncLevels/Custom are
// ignored.
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
