// Package source defines the pluggable lyrics-source abstraction.
//
// A Source is a named adapter that knows which optional metadata parameters
// (author, album, ISWC, duration, sync level) it can use to refine a lyrics lookup, and
// can fetch lyrics for a given Request. Built-in adapters are registered
// explicitly via RegisterAll in package internal/bootstrap.
package source

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	ParamDuration
)

// ParamNamePattern is the legal syntax for custom parameter keys
// (env-style upper snake case, e.g. "LANG"). It is defined in one place
// and reused by the registration-time check (gate 1), the precheck
// dynamic check (gate 2), and the CLI's --env input validation.
const ParamNamePattern = "^[A-Z][A-Z0-9_]*$"

var paramNameRe = regexp.MustCompile(ParamNamePattern)

// ValidParamName reports whether name matches ParamNamePattern. Keys are
// matched exactly and case-sensitively; the pattern forces uppercase, so
// lowercase/mixed-case keys are rejected by the CLI before they ever
// reach a source.
func ValidParamName(name string) bool {
	return paramNameRe.MatchString(name)
}

// ParamSpec describes one custom input parameter a source declares.
// Whether a parameter is required is not part of the static
// declaration: Capabilities(req).RequiredCustom decides that per
// request, so conditional requirements are expressible.
type ParamSpec struct {
	Name        string // e.g. "LANG"; must match ParamNamePattern
	Description string // rendered into the --help "Source parameters:" section
}

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
	// request. The fetch layer enforces them during precheck and reports
	// a fetch.RequiredParamError for the first missing field. If a
	// required field is missing anyway when Fetch runs — a capability
	// declaration bug — the adapter raises RequiredParamMismatchError.
	Required Param

	// Custom lists the custom parameters this adapter recognizes for
	// this request. Conditionally supported parameters are listed only
	// when their precondition holds (mirrors lrclib's conditional
	// album semantics). Every name must be legal (ParamNamePattern) and
	// present in CustomParams() (gate 2; a violation is a source bug).
	Custom []ParamSpec

	// RequiredCustom lists the names of custom parameters that must be
	// present for this request. It must be a duplicate-free subset of
	// this request's Custom names, and every name must be legal and
	// present in CustomParams() (gate 2; a violation is a source bug).
	RequiredCustom []string
}

// SyncLevel identifies the lyrics format a Request asks for.
//
// It mirrors fetch.SyncLevel minus SyncUnknown: the fetch layer needs
// SyncUnknown to classify results, a request does not. The numeric
// values deliberately do NOT match (fetch.SyncNone is 1) — never cast
// between the two types; map explicitly.
type SyncLevel uint8

const (
	// SyncNone: plain (non-timestamped) lyrics. Zero value.
	SyncNone SyncLevel = iota
	// SyncLine: synced (LRC timestamped) lyrics.
	SyncLine
)

// Request is the input to a Source.Fetch call. Song is required;
// Author/Album/ISWC/Duration are optional refinements and may be empty.
type Request struct {
	Song   string // required
	Author string
	Album  string
	ISWC   string
	// Duration is the track duration in whole seconds; 0 means not
	// provided. It is an optional matching hint the source may use
	// (e.g. lrclib /api/get).
	Duration int
	// SyncLevel is the lyrics format the request asks for (from
	// --sync-level). Zero value is SyncNone (plain lyrics).
	SyncLevel SyncLevel
	// UserAgent is the HTTP User-Agent header the source should send on
	// upstream requests, taken from --user-agent. When empty, the
	// source falls back to its own default UA string.
	UserAgent string
	// Custom carries the key/value pairs the user passed via --env
	// (plus process-environment fallbacks); keys the user did not
	// supply are absent.
	Custom map[string]string
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
	// FieldSubSource marks Result.SubSource as populated. Only
	// aggregate adapters set it; standalone adapters leave both the
	// field and the bit unset.
	FieldSubSource
)

// String returns the name of the first set bit in f (e.g. "Lyrics",
// "SyncedLyrics") — the CLI renders it into result-mismatch warnings.
// f is a single-bit mask by contract: the result is undefined for a
// zero value or a multi-bit combination.
func (f ResultField) String() string {
	switch {
	case f&FieldLyrics != 0:
		return "Lyrics"
	case f&FieldSyncedLyrics != 0:
		return "SyncedLyrics"
	case f&FieldTitle != 0:
		return "Title"
	case f&FieldArtist != 0:
		return "Artist"
	case f&FieldAlbum != 0:
		return "Album"
	case f&FieldISWC != 0:
		return "ISWC"
	case f&FieldSubSource != 0:
		return "SubSource"
	}
	return fmt.Sprintf("ResultField(%d)", uint(f))
}

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
	// Populated (FieldSyncedLyrics) only when Request.SyncLevel is
	// SyncLine and the source had synced lyrics.
	SyncedLyrics string
	Title        string
	Artist       string
	Album        string
	ISWC         string
	// SubSource identifies the sub-source that produced this result in
	// aggregate adapters. Like every other result field it is only read
	// when declared: standalone adapters leave it empty with the
	// FieldSubSource bit unset; aggregate adapters set both.
	SubSource string
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

	// CustomParams returns the full static list of custom parameters
	// this source supports, independent of any request. It backs the
	// --help "Source parameters:" section and the pre-fetch
	// environment-variable fallback; sources without custom parameters
	// return nil.
	CustomParams() []ParamSpec

	// Fetch performs the lyrics lookup. It must respect ctx for
	// cancellation/deadlines. A non-nil error means the lookup failed
	// and no lyrics are available; warnings about unsupported parameters
	// are NOT returned here — they are computed by the fetch layer. A
	// required parameter missing at fetch time (precheck should have
	// caught it) is reported as RequiredParamMismatchError.
	Fetch(ctx context.Context, req Request) (Result, error)
}

// ErrNotFound is returned by Registry.Get when name is unknown.
var ErrNotFound = errors.New("source: not found")

// ErrDuplicate is returned by Registry.Register when a source with the
// same Name() is already registered.
var ErrDuplicate = errors.New("source: duplicate registration")

// ErrInvalidParamName is returned by Registry.Register (gate 1) when a
// source's static CustomParams() declaration violates the --env key
// contract: a name not matching ParamNamePattern, or a duplicate entry.
// It carries the source name and the offending key so the startup panic
// can point straight at the misbehaving adapter.
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

// RequiredParamMismatchError is raised by a source's Fetch when it
// needs a parameter the request does not carry. Precheck normally
// prevents this by enforcing Capabilities(req).Required, so a missing
// parameter at fetch time means the source's capability declaration
// disagrees with what its Fetch implementation actually needs — a
// source bug, not a caller error. The fetch layer converts it to a
// PrecheckMismatch warning and fails over to the next source.
type RequiredParamMismatchError struct {
	Source    string // adapter Name() that requires the parameter
	Param     Param  // typed Param bit; 0 for a custom key
	ParamName string // custom key name; empty for a typed parameter
}

// Error renders a neutral message; the CLI renders the user-facing
// text from the structured fields.
func (e RequiredParamMismatchError) Error() string {
	return fmt.Sprintf("source %q requires a parameter precheck did not enforce (source bug)", e.Source)
}

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
//
// Gate 1: the source's static CustomParams() declaration must contain
// only legal (ParamNamePattern), distinct key names. A violation is a
// source bug and fails fast at registration time — main's startup panic
// surfaces it to developers/CI before any CLI handling runs.
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
