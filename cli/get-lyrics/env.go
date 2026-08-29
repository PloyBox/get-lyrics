package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/PloyBox/get-lyrics/internal/source"
)

// envList collects repeated --env key=value flags. flag.Value calls Set
// once per occurrence, so both --env LANG=en and --env=LANG=en work.
type envList []string

func (e *envList) String() string { return strings.Join(*e, ",") }

func (e *envList) Set(value string) error {
	*e = append(*e, value)
	return nil
}

// validateEnv validates the collected --env entries at parse time and
// returns them as a key→value map. Each entry is split on the first '=';
// the key must match ParamNamePattern, the value must be non-empty after
// trimming (a whitespace-only value counts as empty, mirroring the typed
// params' TrimSpace semantics), and duplicate keys are rejected. Any
// violation is a usage error (exit 2).
func validateEnv(envs envList) (map[string]string, error) {
	out := make(map[string]string, len(envs))
	for _, entry := range envs {
		key, value, _ := strings.Cut(entry, "=")
		if !source.ValidParamName(key) {
			return nil, fmt.Errorf("invalid --env key %q (must match %s)", key, source.ParamNamePattern)
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("invalid --env entry %q: value must be non-empty", entry)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate --env key %q", key)
		}
		out[key] = value
	}
	return out, nil
}

// mergeEnv fills every key any requested source declares from the
// process environment when the user did not supply it via --env.
// Precedence: --env > environment > missing. An environment variable
// that exists but is empty (e.g. LANG=) counts as missing and is not
// injected. Injected keys are treated exactly like user-provided ones —
// a source that does not declare the key still warns unsupported.
func mergeEnv(custom map[string]string, decls map[string][]source.ParamSpec) map[string]string {
	for _, specs := range decls {
		for _, spec := range specs {
			if _, provided := custom[spec.Name]; provided {
				continue
			}
			if v, ok := os.LookupEnv(spec.Name); ok && strings.TrimSpace(v) != "" {
				if custom == nil {
					custom = make(map[string]string)
				}
				custom[spec.Name] = v
			}
		}
	}
	return custom
}
