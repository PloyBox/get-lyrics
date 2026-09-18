# fetch

The `fetch` package is the thin orchestration layer between the CLI and the pluggable
source adapters. It prechecks every requested source (existence + required params), then
tries them in user-given order per sync level, failing over on adapter errors and matching
results against the requested `SyncLevel` via a per-call cache.

File layout:

| File | Contents |
|---|---|
| `params.go` | request-side types (`Params`, `SyncLevel`, request projection) |
| `result.go` | the output type and track matching |
| `errors.go` | typed errors |
| `warnings.go` | the `Warning` contract and warning detectors |
| `precheck.go` | the precheck stage (gate 2 + required-param checks) |
| `fetch.go` | the orchestration core only |

## Service

`Service` fetches lyrics through a `*source.Registry`; `New(reg)` binds one.

### Fetch

`Fetch(ctx, params) (Result, []Warning, error)` prechecks every requested source, then tries
them in order for each requested sync level. The first result whose `Level` matches the
current iteration is returned immediately; when nothing matches, every produced track is
cached per call so a later iteration (`none` after `line`) can reuse them without a second
request.

Loop shape: outer over `params.SyncLevels` (priority order), inner over eligible sources
(failover). Unsupported-parameter warnings are detected once per source (memoized).

Error semantics:

- `SyncUnknown` in `Params.SyncLevels` → `InvalidSyncLevelError` (a caller bug; rejected
  before any per-source validation, in BOTH strict and lenient mode).
- strict precheck (default) → the single first problem: a `DuplicateSourceError` (exit 8),
  `source.ErrNotFound` (exit 3), or a `RequiredParamError` (exit 6); no source is fetched, but
  warnings accumulated before the abort (e.g. a gate-2 source-bug warning) are returned with
  the error so the caller can print them first.
- lenient precheck (`--lenient`) → problem sources are skipped with a `PreCheck` warning;
  eligible sources proceed.
- adapter errors during fetch → `FetchFailed` warning + fail over to the next source (never
  aborts, regardless of lenient); a source raising `RequiredParamMismatchError` instead
  becomes a `PrecheckMismatch` warning + fail over.
- no result matched any sync level → `NoResultError`, with the in-flight warnings so the
  caller can print why each source failed.

Cache lookup scans for `Source == name && Level == want`.

### CustomParamsFor

`CustomParamsFor(params)` returns, in `params.Source` order, the static `CustomParams()`
declaration of every source that passes validation; the map is keyed by source name. Only
`params.Source` and `params.Lenient` participate — `Song`/`Author`/`Album`/`ISRC`/
`Duration`/`SyncLevels`/`Custom` are ignored.

- Strict mode: the first problem aborts with `UnknownSourceError` (unregistered name) or
  `DuplicateSourceError` (duplicate entry), the same codes as the `Fetch` precheck, surfaced
  before any fetch.
- Lenient mode: problem sources are silently skipped and never enter the map.

It produces no warnings, performs no required-param checks, and does not trigger gate 2 — it
is the read-only query `main` uses for the `--help` "Source parameters:" section and for the
pre-fetch environment-variable fallback.

## Params

`Params` bundles all CLI inputs the fetch layer needs.

- `Source` — ordered list of source names to try (failover order).
- `SyncLevels` — ordered list of requested sync levels (`SyncLine` → LRC-synced, `SyncWord` →
  word-synced, `SyncNone` → plain); the CLI parses `--sync-level` names into it; first match
  wins. `SyncUnknown` is not a legal request value; precheck rejects it with
  `InvalidSyncLevelError`.
- `Lenient` controls the precheck stage only: `false` → the first precheck problem aborts;
  `true` → problem sources are skipped with a `PreCheck` warning.
- `Duration` — whole seconds; `0` means not provided.
- `UserAgent` — the HTTP `User-Agent` header to send upstream (from `--user-agent`); passed
  to every requested source; empty means the source uses its own default UA.
- `Custom` — user-supplied `--env` keys plus process-environment fallbacks injected by the
  CLI; absent keys are simply absent; env-injected keys behave exactly like user-provided
  ones.

## SyncLevel

`SyncLevel` classifies the lyrics content a `fetch.Result` carries by its sync level.

| Value | Meaning |
|---|---|
| `SyncUnknown` | unknown / no valid lyrics content (the result carries no populated lyrics track) |
| `SyncNone` | plain (non-timestamped) lyrics |
| `SyncLine` | synced (LRC timestamped) lyrics |
| `SyncWord` | word-level (TTML timeline) lyrics |

## requestFromParams

Projects the CLI params onto a `source.Request` for capability queries.

- `SyncLevel` is deliberately omitted — zero value is `SyncNone`; synced output is a runtime
  property.
- `Custom` is projected so capability queries see the user-supplied keys — conditional
  recognition/requirements (e.g. `mock-custom`'s `COUNTRY` depending on `LANG`) would
  otherwise never hold in precheck and `detectUnsupported`.

## sourceSyncLevel

Maps a requested `fetch.SyncLevel` onto its `source.Request` counterpart one-to-one via an
exhaustive switch. `SyncUnknown` never reaches this point — precheck rejects it before the
fetch loop runs — so the fallthrough panics with
`fetch: invalid sync level %d (precheck must reject SyncUnknown)`.

## precheck

`precheck(params, *warnings)` walks `params.Source` in order, filtering problem sources into
`*warnings` under `--lenient` or aborting with the first single error in strict mode. The
returned slice holds the eligible source names in user-given order.

- **Request-level check first**: before any per-source validation (including gate 2 and the
  lenient skip logic), `SyncUnknown` in `Params.SyncLevels` → `InvalidSyncLevelError`. No
  source could satisfy it, so neither mode downgrades it to a warning.
- **Gate 2 before the missing-required check**: a source whose request-aware custom
  declaration is inconsistent (a source bug) is skipped with a `PrecheckMismatch` warning in
  BOTH modes — never a `RequiredParamError`, since the offending key cannot be legitimately
  supplied by the caller.
- Duplicate source: strict → `DuplicateSourceError`; lenient → `PreCheck` warning + skip.
- Unknown source: strict → `UnknownSourceError`; lenient → `PreCheck` warning + skip
  (carrying the registry error).
- Missing required param: strict → `RequiredParamError`; lenient → `PreCheck` warning
  (carrying the missing param) + skip.

### validateCustomDecl (gate 2)

Enforces gate 2 on a source's request-aware custom declaration:

- every name in `caps.Custom` must be a legal key (`ParamNamePattern`) present in the static
  `CustomParams()` list;
- `RequiredCustom` must be a duplicate-free subset of `caps.Custom`'s names, with every name
  legal and present in the static list.

Returns the first offending key name, or `""` when the declaration is consistent.

### checkRequired

Compares the non-empty optional fields in `params` against `caps` and reports the first
missing requirement: typed `Required` bits first (author, album, isrc, duration), then
`RequiredCustom` names in declaration order.

Return values: `missingParam` is the first missing typed bit (`0` when a custom key is
missing); `missingCustom` is the first missing custom key name (empty when a typed bit is
missing); the bool is true when anything is missing.

## Result

`Result` is the fetch layer's output.

- `Level` records the sync level of `Lyrics`: `SyncNone` for plain text, `SyncLine` for
  synced (LRC) content, `SyncWord` for word-level (TTML) content, `SyncUnknown` when no lyrics
  track was populated.
- `Source` always names the adapter that produced the result.
- `SubSource` is the sub-source identifier reported by aggregate sources (e.g. a
  multi-provider aggregator); standalone adapters leave it empty.

### resultLevel

Maps the adapter-declared `source.SyncLevel` onto its `fetch.Result` counterpart one-to-one.
The two types deliberately do not share numeric values, so mapping is explicit. An unknown
value (a source bug) maps to `SyncUnknown` — the result can then never match a request, which
the cache/downgrade machinery handles safely.

### filtResult

Converts an adapter result into the `fetch.Result` track it legitimately contains.

- Field population follows the adapter's `Filled` mask — never string contents: a field
  whose bit is unset is treated as empty.
- An adapter produces exactly one lyrics track per `Fetch`; `Level` is copied from the
  adapter's declaration via `resultLevel`.
- Matching is level-based: when the track's `Level` equals `want`, it is returned as `match`
  and nothing is stored — the caller returns immediately. Only when it does not match does
  the caller receive it as `storable` for the per-call cache: `want` decides matching, never
  storage.
- When the adapter declared `FieldLyrics` but left it empty (a `detectResultMismatch` case),
  nothing is produced and nothing is stored.

### findCached

Returns the first cached track produced by `name` whose `Level` matches `want`, or nil.

## Errors

- `NoResultError` — every source was skipped or failed and no result matched any requested
  sync level. The CLI maps it to exit code 4 and prints the collected warnings first.
- `InvalidSyncLevelError` — precheck rejects `Params.SyncLevels` containing `SyncUnknown`,
  which classifies results but is not a requestable level (callers must request only
  `SyncNone`/`SyncLine`/`SyncWord`). A caller bug (the CLI rejects such values at parse time),
  so it aborts in BOTH strict and lenient mode.
- `UnknownSourceError` — identifies the requested but unregistered source name. Unwraps to
  `source.ErrNotFound` so callers can match with `errors.Is` and still render the offending
  name.
- `DuplicateSourceError` — a source name listed more than once. The CLI maps it to exit code
  8; `--lenient` instead emits a `PreCheck` warning and drops the duplicate.
- `RequiredParamError` — a source whose `Capabilities.Required` (or `RequiredCustom`)
  includes a parameter the caller did not supply. The precheck stage builds it for the first
  missing field; adapters never return it themselves. The CLI maps it to exit code 6 and
  renders user-facing text from the structured fields. `Error()` is neutral; the CLI renders
  the flag spelling.

## Warnings

`WarningKind` classifies a `Warning` by the stage that produced it.

| Value | Meaning |
|---|---|
| `UnsupportedParam` | a user-supplied optional parameter the source does not honor; emitted alongside a successful result |
| `Downgraded` | the requested sync level got no match — synced requested but only plain returned, or plain requested but only synced returned. The unmatched result stays cached and can satisfy a later iteration |
| `PreCheck` | `--lenient` skipped a source during precheck (unknown name, missing required parameter, or duplicate) |
| `PrecheckMismatch` | a source raised `RequiredParamMismatchError` from `Fetch` — its capability declaration disagrees with what `Fetch` actually needs (source implementation bug) |
| `FetchFailed` | the adapter returned an error; fetch moved on to the next source |
| `ResultMismatch` | the adapter's `Filled` mask disagrees with the actual field contents (declared field left empty, or filled field not declared). The result is still used as-is (trust policy) |

`Warning` carries structured data only — the CLI renders all display text, including the
`[kind]` tag, from these fields.

| Field | Meaning |
|---|---|
| `Kind` | which stage produced the warning |
| `Source` | name of the source the warning refers to |
| `Param` | typed parameter involved; `0` for a custom key or no parameter |
| `ParamName` | custom parameter key involved; empty for typed parameters |
| `Want` | the sync level requested by the iteration that produced a `Downgraded` warning: `SyncLine` → the source returned no LRC-synced lyrics, `SyncWord` → no word-synced lyrics, `SyncNone` → it returned only synced lyrics. Zero for every other kind |
| `Field` | the result field a `ResultMismatch` refers to |
| `Declared` | for a `ResultMismatch`: `true` when the source declared `Field` via `Filled` but left it empty, `false` when it filled it without declaring it |
| `Err` | underlying cause when one exists: the adapter error for `FetchFailed`, the registry error for a not-found `PreCheck`, the `RequiredParamMismatchError` for a fetch-time `PrecheckMismatch`; nil otherwise |

### detectUnsupported

Compares the non-empty optional fields in `params` against the adapter's filters for this
request and returns one `UnsupportedParam` warning per mismatch. The sync level is
deliberately excluded: a synced request on a plain-only source is covered by the
`Downgraded` warning.

Custom keys run a parallel path: every user-supplied key the adapter does not recognize for
this request gets one warning. Map iteration order is unspecified on purpose — multiple
unrecognized keys produce warnings in nondeterministic order; tests assert the warning set,
never its order.

### resultFieldSpecs

Lists every field tracked by the `Filled` mask, with the accessor used by the mismatch
detector: `Lyrics`, `Title`, `Artist`, `Album`, `ISRC`, `SubSource`.

### detectResultMismatch

Compares `sr.Filled` against the actual field contents and reports one warning per
inconsistency: a declared bit with an empty value, or a non-empty value without a declared
bit. Either way the result is still used as-is (trust policy) — the warning only flags a
source implementation problem.
