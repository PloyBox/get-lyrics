package fetch

import (
	"fmt"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// precheck walks params.Source in order, filtering out problem sources
// into *warnings under --lenient or aborting with the first single
// error in strict mode. The returned slice holds the eligible source
// names in the user-given order.
//
// Before any per-source validation (including gate 2 and the lenient
// skip logic), a request-level check rejects SyncUnknown in
// Params.SyncLevels with InvalidSyncLevelError — a caller bug that no
// source could satisfy, so neither mode downgrades it to a warning.
//
// Gate 2 runs before the missing-required check: a source whose
// request-aware custom declaration is inconsistent (a source bug) is
// skipped with a precheck-mismatch warning in BOTH strict and lenient
// mode — never a RequiredParamError, since the offending key cannot be
// legitimately supplied by the caller.
func (s *Service) precheck(params Params, warnings *[]Warning) ([]string, error) {
	for _, want := range params.SyncLevels {
		if want == SyncUnknown {
			return nil, InvalidSyncLevelError{}
		}
	}
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
