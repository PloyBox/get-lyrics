// Package fetch orchestrates precheck, failover and sync-level matching
// over the registered sources. See docs/refs/fetch.md.
package fetch

import (
	"context"
	"errors"
	"time"

	"github.com/PloyBox/get-lyrics/source"
)

// Service fetches lyrics through a source registry.
type Service struct {
	reg *source.Registry
}

func New(reg *source.Registry) *Service {
	return &Service{reg: reg}
}

// Fetch prechecks the requested sources, then tries them in order for
// each requested sync level, failing over on adapter errors.
func (s *Service) Fetch(ctx context.Context, params Params) (Result, []Warning, error) {
	warnings := make([]Warning, 0, 4)
	eligible, err := s.precheck(params, &warnings)
	if err != nil {
		return Result{}, warnings, err
	}

	// Per-call cache of tracks that did not match the requested level;
	// a later iteration ("none" after "line") can reuse them.
	cache := make([]Result, 0, len(eligible))
	warnedUnsupported := make(map[string]bool, len(eligible))

	for _, want := range params.SyncLevels {
		if ctx.Err() != nil {
			return Result{}, warnings, StoppedError{Cause: ctx.Err()}
		}
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
			sr, ferr := callSource(ctx, params.Timeout, src, req)
			// The caller's context wins over any adapter error: when it
			// ended, stop instead of failing over.
			if ctx.Err() != nil {
				return Result{}, warnings, StoppedError{Cause: ctx.Err()}
			}
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
				// Nothing matched: store the produced tracks and warn on
				// the downgrade.
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

// callSource invokes src.Fetch under a per-source deadline. A zero budget
// means no deadline; the child context never cancels the caller's context.
func callSource(ctx context.Context, timeout uint, src source.Source, req source.Request) (source.Result, error) {
	if timeout == 0 {
		return src.Fetch(ctx, req)
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	return src.Fetch(cctx, req)
}

// CustomParamsFor returns each requested source's static CustomParams()
// declaration, keyed by source name. Strict mode aborts on an unknown or
// duplicate name; lenient mode silently skips it. Read-only: no
// warnings, no required-param checks, no gate 2.
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
