package fetch

import (
	"strings"

	"github.com/PloyBox/get-lyrics/source"
)

// precheck validates params.Source in order, returning the eligible names
// in user-given order. Under --lenient, problem sources become warnings
// instead of aborting. Gate 2 runs before the missing-required check.
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
				Kind:   PreCheck,
				Source: name,
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
				Kind:   PreCheck,
				Source: name,
				Err:    err,
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
			})
			continue
		}

		missing, missingCustom, need := checkRequired(caps, params)
		if need {
			if !params.Lenient {
				return nil, RequiredParamError{
					Source:    src.Name(),
					Param:     missing,
					ParamName: missingCustom,
				}
			}
			*warnings = append(*warnings, Warning{
				Kind:      PreCheck,
				Source:    name,
				Param:     missing,
				ParamName: missingCustom,
			})
			continue
		}
		eligible = append(eligible, name)
	}
	return eligible, nil
}

// validateCustomDecl enforces gate 2, returning the first offending key
// name or "" when the request-aware declaration is consistent.
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

// checkRequired reports the first missing requirement: typed Required
// bits first, then RequiredCustom names in declaration order.
func checkRequired(caps source.Capabilities, params Params) (missingParam source.Param, missingCustom string, need bool) {
	if caps.Required&source.ParamAuthor != 0 && strings.TrimSpace(params.Author) == "" {
		return source.ParamAuthor, "", true
	}
	if caps.Required&source.ParamAlbum != 0 && strings.TrimSpace(params.Album) == "" {
		return source.ParamAlbum, "", true
	}
	if caps.Required&source.ParamISRC != 0 && strings.TrimSpace(params.ISRC) == "" {
		return source.ParamISRC, "", true
	}
	if caps.Required&source.ParamDuration != 0 && params.Duration <= 0 {
		return source.ParamDuration, "", true
	}
	for _, name := range caps.RequiredCustom {
		if v, ok := params.Custom[name]; !ok || strings.TrimSpace(v) == "" {
			return 0, name, true
		}
	}
	return 0, "", false
}
