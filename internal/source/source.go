// Package source defines the pluggable lyrics-source abstraction.
//
// A Source is a named adapter that knows which optional metadata parameters
// (author, album, ISWC, timestamp) it can use to refine a lyrics lookup, and
// can fetch lyrics for a given Request. Built-in adapters are registered
// explicitly via RegisterAll in package internal/bootstrap.
package source

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// Param identifies one optional metadata field a Source may or may not honor.
// Implemented as a bitmask so SupportedParams can be a single uint.
type Param uint

const (
	ParamAuthor Param = 1 << iota
	ParamAlbum
	ParamISWC
	ParamTimestamp // whether the source can return timestamped lyrics
)

// Request is the input to a Source.Fetch call. Song is required;
// Author/Album/ISWC are optional refinements and may be empty strings.
type Request struct {
	Song      string // required
	Author    string
	Album     string
	ISWC      string
	Timestamp bool // whether to request timestamped lyrics (from --timestamp)
}

// Result carries the fetched lyrics together with the metadata that
// identifies the matched song. Lyrics is always populated; SyncedLyrics is
// only populated when the source supports ParamTimestamp and the request
// asked for it.
type Result struct {
	// The two lyrics tracks are mutually exclusive: a source returns one
	// based on its capability. Lyrics is the primary track — plain text,
	// free of timestamps and safe to cat directly. SyncedLyrics is the
	// secondary track — LRC-style ([mm:ss.xx] lines) — populated only when
	// the source supports ParamTimestamp and Request.Timestamp is true.
	Lyrics       string // normalized plain text, always populated
	SyncedLyrics string // LRC-style timestamped lyrics; empty when unsupported
	Title        string
	Artist       string
	Album        string
	ISWC         string
	Source       string // backfilled by the fetch layer so it is never empty
}

// Source is the contract every lyrics adapter must satisfy.
type Source interface {
	// Name returns the identifier used on the CLI: --source <name>.
	// Must be stable, lowercase, and unique across registered adapters.
	Name() string

	// SupportedParams declares which optional Request fields this adapter
	// actually uses when refining a lookup. The CLI layer compares this
	// against the user-supplied Request to produce per-field warnings.
	SupportedParams() Param

	// Fetch performs the lyrics lookup. It must respect ctx for
	// cancellation/deadlines. A non-nil error means the lookup failed
	// and no lyrics are available; warnings about unsupported parameters
	// are NOT returned here — they are computed by the fetch layer.
	Fetch(ctx context.Context, req Request) (Result, error)
}

// ErrNotFound is returned by Registry.Get when name is unknown.
var ErrNotFound = errors.New("source: not found")

// ErrDuplicate is returned by Registry.Register when a source with the
// same Name() is already registered.
var ErrDuplicate = errors.New("source: duplicate registration")

// ErrRequiredParam is the sentinel adapters return (or wrap) when the
// caller failed to supply a parameter the source *requires* — as
// opposed to merely *supports*. Today no built-in adapter triggers it;
// it is reserved for future providers that refuse lookups without a
// specific field. Callers should test with errors.Is against this
// sentinel and use errors.As to recover the typed RequiredParamError.
var ErrRequiredParam = errors.New("source: required parameter missing")

// RequiredParamError enriches ErrRequiredParam with the offending
// parameter so the CLI can render a stable, greppable message.
//
// Adapters that need to raise this error should either return a
// literal:
//
//	return source.Result{}, source.RequiredParamError{
//	    Source: a.Name(),
//	    Param:  source.ParamAuthor,
//	    Flag:   "--author",
//	}
//
// or wrap it so errors.Is(err, source.ErrRequiredParam) still matches:
//
//	return source.Result{}, fmt.Errorf("musixmatch: %w",
//	    source.RequiredParamError{Source: "musixmatch", Param: source.ParamISWC, Flag: "--iswc"})
type RequiredParamError struct {
	Source string // adapter Name() that raised the error
	Param  Param  // which Param bit is required
	Flag   string // CLI flag spelling used by the user (e.g. "--author")
}

// Error renders a stable message; main formats the same shape.
func (e RequiredParamError) Error() string {
	return "source \"" + e.Source + "\" requires " + e.Flag
}

// Unwrap lets errors.Is(err, ErrRequiredParam) succeed for plain and
// wrapped values alike.
func (e RequiredParamError) Unwrap() error { return ErrRequiredParam }

// Registry is a concurrency-safe, name→Source lookup table populated
// explicitly by RegisterAll (or by tests).
type Registry struct {
	mu      sync.RWMutex
	sources map[string]Source
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]Source)}
}

// Register adds src to the registry under src.Name(). Returns
// ErrDuplicate if a source with the same name is already registered.
func (r *Registry) Register(src Source) error {
	if src == nil {
		return errors.New("source: nil Source")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[src.Name()]; exists {
		return ErrDuplicate
	}
	r.sources[src.Name()] = src
	return nil
}

// Get returns the Source registered under name, or (nil, ErrNotFound)
// if no such source exists.
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
// Used by --help/-h to list available sources.
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

// Unregister removes the source registered under name. It returns
// ErrNotFound if no source is registered under that name.
//
// This method exists primarily so tests can stage and tear down
// fixtures against a shared *Registry without exposing its internal
// map. Production code should not need to call it: built-in adapters
// are registered once at startup via RegisterAll and live for the
// lifetime of the process.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[name]; !ok {
		return ErrNotFound
	}
	delete(r.sources, name)
	return nil
}
