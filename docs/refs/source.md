# source

The `source` package defines the pluggable lyrics-source abstraction. A `Source` is a
named adapter that knows which optional metadata parameters it can use to refine a lyrics
lookup, and can fetch lyrics for a given `Request`. Built-in adapters are registered
explicitly through `bootstrap.RegisterAll`.

The package lives at the module root (importable as a dependency library), as do `fetch`
and `bootstrap`; concrete adapters live under `internal/provider/` (`real/`, `mock/`) —
source is the abstraction, provider is the implementation.

## Param

`Param` identifies one optional request field an adapter may use as a filter when refining
a lookup. It is a bitmask so capability sets can be a single `uint`.

| Value | Meaning |
|---|---|
| `ParamAuthor` | author / artist filter |
| `ParamAlbum` | album filter |
| `ParamISRC` | ISRC identifier |
| `ParamDuration` | track duration filter |

Output format (plain vs synced) is **not** declared statically: it is a runtime property of
the fetched lyrics, reported per result by the adapter via `Result.Level`.

## Custom parameter keys

`ParamNamePattern` is the legal syntax for custom parameter keys: env-style upper snake
case, e.g. `LANG`.

```
^[A-Z][A-Z0-9_]*$
```

It is defined in one place and reused by the registration-time check (gate 1), the precheck
dynamic check (gate 2), and the CLI's `--env` input validation.

`ValidParamName` reports whether a name matches the pattern. Keys are matched exactly and
case-sensitively; the pattern forces uppercase, so lowercase/mixed-case keys are rejected by
the CLI before they reach a source.

## ParamSpec

`ParamSpec` describes one custom input parameter a source declares.

| Field | Meaning |
|---|---|
| `Name` | e.g. `LANG`; must match `ParamNamePattern` |
| `Description` | rendered into the `--help` "Source parameters:" section |

Whether a parameter is required is not part of the static declaration:
`Capabilities(req).RequiredCustom` decides that per request, so conditional requirements are
expressible.

## Capabilities

`Capabilities` describes how an adapter handles a specific request. The query takes the
actual `Request` so conditional support is expressible: an adapter may honor a filter only
when another field is present (e.g. lrclib uses `--album` only when `--author` is given).

| Field | Meaning |
|---|---|
| `Filters` | `Request` fields the adapter uses to refine the lookup for this request; supplied-but-unlisted fields produce unsupported-parameter warnings |
| `Required` | `Request` fields that must be non-empty; enforced during precheck (exit 6) |
| `Custom` | custom parameters the adapter recognizes for this request; conditionally supported ones are listed only when their precondition holds (`mock-custom` lists `COUNTRY` only with `LANG`) |
| `RequiredCustom` | names of custom parameters that must be present; must be a duplicate-free subset of this request's `Custom` names |

Every `Custom`/`RequiredCustom` name must be legal and present in `CustomParams()` — gate 2;
a violation is a source bug. If a required field is missing anyway when `Fetch` runs — a
capability-declaration bug — the adapter raises `RequiredParamMismatchError`.

## SyncLevel

`SyncLevel` identifies the lyrics format a `Request` asks for.

| Value | Meaning |
|---|---|
| `SyncNone` | plain (non-timestamped) lyrics. Zero value |
| `SyncLine` | synced (LRC timestamped) lyrics |
| `SyncWord` | word-level (TTML timeline) lyrics |

It mirrors `fetch.SyncLevel` minus `SyncUnknown`: the fetch layer needs `SyncUnknown` to
classify results, a request does not. The numeric values deliberately do **not** match
(`fetch.SyncNone` is 1) — never cast between the two types; map explicitly.

## Request

`Request` is the input to a `Source.Fetch` call.

| Field | Meaning |
|---|---|
| `Song` | required |
| `Author`, `Album`, `ISRC` | optional refinements, may be empty |
| `Duration` | track duration in whole seconds; `0` means not provided (optional matching hint, e.g. lrclib `/api/get`) |
| `SyncLevel` | lyrics format requested; zero value is `SyncNone` |
| `UserAgent` | HTTP `User-Agent` header the source should send, from `--user-agent`; empty → the source falls back to its own default UA |
| `Custom` | user-passed `--env` key/value pairs plus process-environment fallbacks; unsupplied keys are absent |

## ResultField

`ResultField` identifies one result field a `Source` may populate. A source declares which
fields it actually filled by setting the corresponding bits on `Result.Filled`; the fetch
layer treats unset fields as empty and never infers population from string contents. It is a
bitmask so a set of filled fields is a single `uint`.

| Value | Meaning |
|---|---|
| `FieldLyrics` | `Result.Lyrics` populated — the single lyrics field; `Result.Level` declares the payload format |
| `FieldTitle` | `Result.Title` populated |
| `FieldArtist` | `Result.Artist` populated |
| `FieldAlbum` | `Result.Album` populated |
| `FieldISRC` | `Result.ISRC` populated |
| `FieldSubSource` | `Result.SubSource` populated; only aggregate adapters set it |

`ResultField.String()` returns the name of the first set bit (`"Lyrics"`, `"Title"`, …),
which the CLI renders into result-mismatch warnings. `f` is a single-bit mask by contract:
the result is undefined for a zero value or a multi-bit combination.

## Result

`Result` carries the fetched lyrics together with the metadata that identifies the matched
song.

- `Filled` declares which fields are populated: fields without their bit set must be treated
  as empty, regardless of string contents. It is the single source of truth for the fetch
  layer; a set bit with an empty value is a source implementation problem.
- A source produces exactly one lyrics track per `Fetch` — plain, LRC, or TTML — and
  declares which one it is via `Level`.
- `Lyrics` is that single payload, whichever format `Level` declares.
- `Level` must describe the actual content: the fetch layer matches results against the
  requested level via this field.
- `Title`, `Artist`, `Album`, `ISRC` carry only metadata the upstream response itself
  returned — never values echoed back from the `Request`. A field the server did not return
  stays unset with its `Filled` bit clear.
- `SubSource` identifies the sub-source that produced the result in aggregate adapters.
  Standalone adapters leave it empty with the `FieldSubSource` bit unset; aggregate adapters
  set both.

## Source interface

| Method | Contract |
|---|---|
| `Name() string` | identifier used on the CLI (`--source <name>`); stable, lowercase, unique across registered adapters |
| `Capabilities(req Request) Capabilities` | how this adapter handles `req` — which filters it honors and which it requires. The request is passed so conditional support is expressible; most adapters return a constant. The fetch layer compares `Filters` against the user-supplied request to produce per-field warnings, and enforces `Required` during precheck |
| `CustomParams() []ParamSpec` | the full static list of custom parameters this source supports, independent of any request. Backs the `--help` "Source parameters:" section and the pre-fetch environment-variable fallback; sources without custom parameters return nil |
| `Fetch(ctx, req) (Result, error)` | performs the lookup; must respect `ctx`. A non-nil error means the lookup failed and no lyrics are available. Warnings about unsupported parameters are **not** returned here — the fetch layer computes them. A required parameter missing at fetch time (precheck should have caught it) is reported as `RequiredParamMismatchError` |

## Errors

- `ErrNotFound` — `Registry.Get` on an unknown name.
- `ErrDuplicate` — `Registry.Register` on a name already registered.
- `ErrInvalidParamName` — raised by `Registry.Register` (gate 1) when a static
  `CustomParams()` declaration violates the `--env` key contract: a name not matching
  `ParamNamePattern`, or a duplicate entry. Carries the source name and offending key so the
  startup panic points straight at the misbehaving adapter.
- `RequiredParamMismatchError` — raised by a source's `Fetch` when it needs a parameter the
  request does not carry. Precheck normally prevents this by enforcing
  `Capabilities(req).Required`, so a miss means the capability declaration disagrees with
  what `Fetch` actually needs — a source bug, not a caller error. The fetch layer converts it
  to a `PrecheckMismatch` warning and fails over to the next source.

## Registry

`Registry` is a concurrency-safe, name→`Source` lookup table populated explicitly by
`RegisterAll` (or by tests).

- `NewRegistry()` returns an empty registry.
- `Register(src)` adds `src` under `src.Name()`; returns `ErrDuplicate` on a name collision.
  - **Gate 1**: the source's static `CustomParams()` declaration must contain only legal
    (`ParamNamePattern`), distinct key names. A violation is a source bug and fails fast at
    registration time — `main`'s startup panic surfaces it to developers/CI before any CLI
    handling runs.
- `Get(name)` returns the `Source` or `(nil, ErrNotFound)`.
- `Names()` returns the registered names in sorted order; used by `--help`/`-h` to list
  available sources.
- `Unregister(name)` removes a source; `ErrNotFound` when absent. It exists primarily so
  tests can stage and tear down fixtures against a shared `*Registry` without exposing its
  internal map. Production code should not need it: built-in adapters are registered once at
  startup and live for the process lifetime.
