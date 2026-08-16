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

// Param identifies one optional request field an adapter may use as a
// filter when refining a lookup. Implemented as a bitmask so capability
// sets can be a single uint. Output format (plain vs synced) is NOT
// declared statically: it is a runtime property of the fetched lyrics,
// detected by the fetch layer.
type Param uint

const (
	ParamAuthor Param = 1 << iota
	ParamAlbum
	ParamISWC
)

// Capabilities describes how an adapter handles a specific request. The
// query takes the actual Request so conditional support is expressible:
// an adapter may honor a filter only when another field is present
// (e.g. lrclib uses --album only when --author is given).
type Capabilities struct {
	// Filters lists the Request fields the adapter uses to refine the
	// lookup for this request. Fields the user supplied but the adapter
	// does not list produce unsupported-parameter warnings.
	Filters Param

	// Required lists the Request fields that must be non-empty for this
	// request. The fetch layer enforces them during precheck by building
	// a RequiredParamError for the first missing field; adapters must
	// NOT raise that error themselves in Fetch.
	Required Param
}

// Request is the input to a Source.Fetch call. Song is required;
// Author/Album/ISWC are optional refinements and may be empty strings.
type Request struct {
	Song      string // required
	Author    string
	Album     string
	ISWC      string
	Timestamp bool // whether to request timestamped lyrics (from --timestamp)
}

// ResultField identifies one result field a Source may populate. A
// Source declares which fields it actually filled by setting the
// corresponding bits on Result.Filled; the fetch layer treats unset
// fields as empty and never infers population from string contents.
// Implemented as a bitmask so a set of filled fields is a single uint.
type ResultField uint

const (
	// FieldLyrics marks Result.Lyrics as populated (plain text).
	FieldLyrics ResultField = 1 << iota
	// FieldSyncedLyrics marks Result.SyncedLyrics as populated (LRC text).
	FieldSyncedLyrics
	// FieldTitle marks Result.Title as populated.
	FieldTitle
	// FieldArtist marks Result.Artist as populated.
	FieldArtist
	// FieldAlbum marks Result.Album as populated.
	FieldAlbum
	// FieldISWC marks Result.ISWC as populated.
	FieldISWC
)

// Result carries the fetched lyrics together with the metadata that
// identifies the matched song. Filled declares which fields below are
// populated: fields without their bit set must be treated as empty,
// regardless of their string contents. A source may set both FieldLyrics
// and FieldSyncedLyrics at once (both tracks filled); the fetch layer
// picks the one matching the requested output format.
type Result struct {
	// Filled declares which of the fields below the source actually
	// populated. It is the single source of truth for the fetch layer:
	// unset fields are not read, and a set bit with an empty value is a
	// source implementation problem.
	Filled ResultField
	// Lyrics is the plain-text track, free of timestamps and safe to
	// cat directly. Populated (FieldLyrics) for plain output; a
	// synced-only hit may leave it unfilled, in which case the fetch
	// layer selects SyncedLyrics as the output.
	Lyrics string
	// SyncedLyrics is the LRC-style ([mm:ss.xx] lines) track.
	// Populated (FieldSyncedLyrics) only when Request.Timestamp is true
	// and the source had synced lyrics.
	SyncedLyrics string
	Title        string
	Artist       string
	Album        string
	ISWC         string
	// Source is reserved for aggregate sources: it identifies the
	// sub-source that produced this result. Standalone adapters leave
	// it empty — the fetch layer backfills it into Result.SubSource
	// and keeps Result.Source set to the adapter's Name(). It is not
	// part of the Filled mask: it is provenance metadata, not a
	// fetched result field.
	Source string
}

// Source is the contract every lyrics adapter must satisfy.
type Source interface {
	// Name returns the identifier used on the CLI: --source <name>.
	// Must be stable, lowercase, and unique across registered adapters.
	Name() string

	// Capabilities reports how this adapter handles req: which filters
	// it honors (Filters) and which it requires to be non-empty
	// (Required). The request is passed so conditional support is
	// expressible; most adapters return a constant. The fetch layer
	// compares Filters against the user-supplied request to produce
	// per-field warnings, and enforces Required during precheck.
	Capabilities(req Request) Capabilities

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

// ErrRequiredParam is the sentinel returned when a source requires a
// parameter the caller did not supply. The fetch layer constructs it
// during precheck from Capabilities(req).Required; adapters must not
// raise it themselves. Callers should test with errors.Is against this
// sentinel and use errors.As to recover the typed RequiredParamError.
var ErrRequiredParam = errors.New("source: required parameter missing")

// RequiredParamError enriches ErrRequiredParam with the offending
// parameter so the CLI can render a stable, greppable message.
//
// The fetch layer builds it during precheck; adapters never return it:
//
//	return source.Result{}, source.RequiredParamError{
//	    Source: src.Name(),
//	    Param:  missing,
//	    Flag:   flagForParam(missing),
//	}
type RequiredParamError struct {
	Source string // adapter Name() that requires the parameter
	Param  Param  // which Param bit is required
	Flag   string // CLI flag spelling for the missing field (e.g. "--author")
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
