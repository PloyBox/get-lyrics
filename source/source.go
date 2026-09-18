// Package source defines the pluggable lyrics-source abstraction.
// See docs/refs/source.md.
package source

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// Param is a bitmask of the optional request fields an adapter may use
// as a lookup filter.
type Param uint

const (
	ParamAuthor Param = 1 << iota
	ParamAlbum
	ParamISRC
	ParamDuration
)

// ParamNamePattern is the legal syntax for custom parameter keys.
const ParamNamePattern = "^[A-Z][A-Z0-9_]*$"

var paramNameRe = regexp.MustCompile(ParamNamePattern)

func ValidParamName(name string) bool {
	return paramNameRe.MatchString(name)
}

type ParamSpec struct {
	Name        string // e.g. "LANG"; must match ParamNamePattern
	Description string // rendered into the --help "Source parameters:" section
}

// Capabilities reports how an adapter handles one specific request.
type Capabilities struct {
	Filters        Param
	Required       Param
	Custom         []ParamSpec
	RequiredCustom []string
}

// SyncLevel identifies the lyrics format a Request asks for. It mirrors
// fetch.SyncLevel minus SyncUnknown, and its numeric values deliberately
// do not match — map explicitly, never cast.
type SyncLevel uint8

const (
	SyncNone SyncLevel = iota
	SyncLine
	SyncWord
)

type Request struct {
	Song   string // required
	Author string
	Album  string
	ISRC   string
	// Duration is whole seconds; 0 means not provided.
	Duration  int
	SyncLevel SyncLevel
	UserAgent string
	Custom    map[string]string
}

// ResultField is a bitmask of the result fields a source populated.
type ResultField uint

const (
	FieldLyrics ResultField = 1 << iota
	FieldTitle
	FieldArtist
	FieldAlbum
	FieldISRC
	FieldSubSource // aggregate adapters only
)

// String names the first set bit of f; f is a single-bit mask by
// contract.
func (f ResultField) String() string {
	switch {
	case f&FieldLyrics != 0:
		return "Lyrics"
	case f&FieldTitle != 0:
		return "Title"
	case f&FieldArtist != 0:
		return "Artist"
	case f&FieldAlbum != 0:
		return "Album"
	case f&FieldISRC != 0:
		return "ISRC"
	case f&FieldSubSource != 0:
		return "SubSource"
	}
	return fmt.Sprintf("ResultField(%d)", uint(f))
}

// Result carries one fetched lyrics track plus the metadata the upstream
// actually returned. Filled is the single source of truth for which
// fields are populated.
type Result struct {
	Filled    ResultField
	Lyrics    string
	Level     SyncLevel
	Title     string
	Artist    string
	Album     string
	ISRC      string
	SubSource string // aggregate adapters only
}

type Source interface {
	Name() string
	Capabilities(req Request) Capabilities
	CustomParams() []ParamSpec
	Fetch(ctx context.Context, req Request) (Result, error)
}

var ErrNotFound = errors.New("source: not found")

var ErrDuplicate = errors.New("source: duplicate registration")

// ErrInvalidParamName is raised by gate 1: a static CustomParams()
// declaration with an illegal or duplicate key.
type ErrInvalidParamName struct {
	Source    string // adapter Name() that declared the invalid key
	Name      string // the offending key
	Duplicate bool   // true when the key is a duplicate entry, not a syntax violation
}

func (e ErrInvalidParamName) Error() string {
	if e.Duplicate {
		return fmt.Sprintf("source %q declared duplicate custom key %q (source bug)", e.Source, e.Name)
	}
	return fmt.Sprintf("source %q declared invalid custom key %q (source bug: must match %s)", e.Source, e.Name, ParamNamePattern)
}

// RequiredParamMismatchError is raised by a source's Fetch when it needs
// a parameter the request does not carry — a capability-declaration bug.
type RequiredParamMismatchError struct {
	Source    string // adapter Name() that requires the parameter
	Param     Param  // typed Param bit; 0 for a custom key
	ParamName string // custom key name; empty for a typed parameter
}

func (e RequiredParamMismatchError) Error() string {
	return fmt.Sprintf("source %q requires a parameter precheck did not enforce (source bug)", e.Source)
}

// Registry is a concurrency-safe, name→Source lookup table.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]Source
}

func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]Source)}
}

// Register adds src under src.Name(). Gate 1 rejects a static custom
// key that is illegal or duplicated.
func (r *Registry) Register(src Source) error {
	if src == nil {
		return errors.New("source: nil Source")
	}
	seen := make(map[string]bool)
	for _, spec := range src.CustomParams() {
		if !ValidParamName(spec.Name) {
			return ErrInvalidParamName{Source: src.Name(), Name: spec.Name}
		}
		if seen[spec.Name] {
			return ErrInvalidParamName{Source: src.Name(), Name: spec.Name, Duplicate: true}
		}
		seen[spec.Name] = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[src.Name()]; exists {
		return ErrDuplicate
	}
	r.sources[src.Name()] = src
	return nil
}

// Get returns the Source registered under name, or ErrNotFound.
func (r *Registry) Get(name string) (Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sources[name]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// Names returns the registered source names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.sources))
	for n := range r.sources {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Unregister removes name; used by tests to stage and tear down fixtures
// against a shared *Registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[name]; !ok {
		return ErrNotFound
	}
	delete(r.sources, name)
	return nil
}
