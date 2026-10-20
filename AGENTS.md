# AGENTS.md

This file provides guidance to AI coding agents working in this repository.
The contents of this document should be in English.

## Project Overview

A Go CLI that fetches song lyrics from pluggable sources. No TUI/GUI — pure command
line. The song title is a mandatory positional argument; everything else comes via
flags. No third-party runtime dependencies; stdlib `flag` for parsing.

The `source`, `fetch`, and `bootstrap` packages live outside `internal/` and are
importable as a dependency library; `internal/provider` (concrete adapters) stays
private.

## Repository Layout

```
get-lyrics/
├── cli/
│   └── get-lyrics/             # command package (package main): run.go entrypoint,
│                               # flags/env/usage/output, loadmock.go (test tag), tests
├── source/                     # Source interface, Request/Result, Param/ResultField
│                               # bitmasks, ParamSpec, Registry
├── fetch/                      # Fetch(ctx, params): precheck (incl. gate 2), failover,
│                               # sync-level resolution, CustomParamsFor
├── bootstrap/                  # bootstrap.go: registers real sources;
│                               # bootstrap_mock.go (test tag): mocks
├── internal/
│   └── provider/               # concrete adapters implementing source.Source
│       ├── mock/               # mock-* test-only adapters
│       └── real/               # lrclib, lyricsovh, lrccx, musixmatch, betterlyrics
└── docs/
    ├── refs/                   # package-level reference extracted from code comments
    └── usage/                  # user-facing docs (usage, exit codes, source
                                # parameters, built-in sources, add-a-source)
```

## Build & Test

- `go build ./...` — compile
- `go test -tags test ./...` — test suite (the `test` tag is required: the
  `cli/get-lyrics` `*_test.go` tests exercise the `mock-*` sources registered by
  `loadmock.go`, which compiles only under the tag)
- `go vet -tags test ./...` — static checks (same tag requirement)
- `gofmt -l .` — formatting check (must be empty)
- Prerequisites: Go toolchain (module declares Go 1.26); network access for real
  sources at runtime

## CLI Surface

Canonical flag reference: [`docs/usage/usage.md`](docs/usage/usage.md). Exit-code
table: [`docs/usage/exit-codes.md`](docs/usage/exit-codes.md). Update both when the
CLI changes; avoid restating per-flag prose here.

Agent-relevant invariants:

- Exactly one positional `<song>`; zero or more than one is a usage error (exit 2).
- `--source`/`-s`: comma-separated names tried in order (failover), default
  `lrclib`; entries are trimmed, empty ones dropped.
- `--sync-level`/`-S`: comma-separated `line`/`word`/`none`, default
  `line,none`; user-given order is the priority (first match wins).
- `--duration`/`-d`: whole seconds (`225`) or `mm:ss` (`3:45`), normalized to int
  seconds at parse time, 0 = not provided.
- `--timeout`/`-t`: per-source request timeout, whole seconds, default `10`
  (0 = none). `--timeout-global`/`-T`: whole-fetch budget, default `0` = off.
  Negative or non-numeric values for either are usage errors (exit 2).
- `--output`/`-o` refuses to overwrite an existing file (exit 7) unless
  `--overwrite`/`-O` is given.
- `--json`/`-j` writes the complete `fetch.Result` (all fields, including empty
  strings) plus `formatVersion`; increment it on any JSON structure or parameter
  semantics change.
- `--user-agent`/`-u`: HTTP `User-Agent`, default
  `get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics)`. Built-in sources carry
  no UA default — they send whatever they are handed.
- `--env`/`-e`: repeatable `KEY=value` custom source parameter. Key must match
  `^[A-Z][A-Z0-9_]*$`, value non-empty after trimming, duplicate keys rejected —
  any violation is a usage error (exit 2).
- `--lenient`/`-l`: downgrade precheck errors to `warning[precheck]` + skip.
- `--quiet`/`-q`: suppress all stderr (warnings and errors); exit codes unchanged.
- `--help`/`-h`, `--version`/`-v`: exit 0.
- Go's `flag` package accepts both `--flag` and `-flag`.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may still go to stderr) |
| 2 | Usage error (positionals, unknown flag, bad `--sync-level`/`--duration`/`--timeout`/`--timeout-global`/`--env`) |
| 3 | Unknown `--source` name (strict precheck) |
| 4 | No valid result: all sources skipped/failed, or no format match |
| 5 | Output failure (file open, truncate, write, or close) |
| 6 | Source-required parameter missing (e.g. `--author` for `lyricsovh`, or a required `--env` key) |
| 7 | `--output` exists and `--overwrite` not given |
| 8 | Duplicate `--source` entry (strict precheck) |
| 9 | Whole-fetch budget (`--timeout-global`) ended, or the caller's context ended |

## Architecture

Thin CLI layer over a pluggable-source abstraction:

1. **Registration** — `cli/get-lyrics/run.go` runs `bootstrap.RegisterAll(r)` once
   at package-init time, before `main()`; test builds additionally register
   `mock-*` sources via `init()` in `cli/get-lyrics/loadmock.go`.
   - `Registry.Register` is **gate 1**: a source's static `CustomParams()` list must
     contain only legal (`^[A-Z][A-Z0-9_]*$`), distinct keys — a violation returns
     `ErrInvalidParamName` and panics at startup (adapter init failure is a
     programmer error).
2. **Parse** — required positional `<song>` plus flags via `flag.NewFlagSet`;
   `--sync-level` (`line`/`word`/`none` → `[]fetch.SyncLevel`), `--env` values,
   `--duration` (seconds or `mm:ss` → int seconds), and
   `--timeout`/`--timeout-global` (non-negative whole seconds) are parsed and
   validated at parse time (violations are usage errors, exit 2).
3. **Env fallback** — before the fetch, `main` calls `svc.CustomParamsFor(params)`
   (strict: unknown source → exit 3, duplicate → exit 8, reported before the fetch;
   lenient: problem sources silently skipped) and fills every declared key the user
   did not pass via `-e` from the process environment (`-e` > env > missing; an
   empty env var counts as missing). Injected keys behave exactly like user-passed
   ones.
4. **Fetch** (`fetch.New(registry).Fetch(ctx, params)`) — two-level loop: outer over
   `params.SyncLevels` (priority order), inner over sources (failover); a per-call
   result cache dedupes by source+SyncLevel.
   - **Timeout/stop** — each `src.Fetch` runs under a per-source deadline
     (`params.Timeout`, 0 = none) derived from the caller's context; when the
     caller's context ends (deadline or cancellation), fetch stops immediately and
     returns `StoppedError` (exit 9) instead of failing over.
   - **Precheck** — first a request-level check: `SyncUnknown` in
     `params.SyncLevels` → `InvalidSyncLevelError` (a caller bug; rejected before
     any per-source validation including gate 2, in BOTH modes — never downgraded
     to a warning). Then, per source: duplicate source → exit 8, unknown name →
     exit 3, missing required param (typed first, then `RequiredCustom` in
     declaration order) → exit 6 (`--lenient` downgrades these to
     `warning[precheck]` + skip).
   - **Gate 2** — runs before the missing check: a request-aware custom declaration
     inconsistent with the static list (invalid name, static mismatch,
     `RequiredCustom` not a subset of `Custom`, or duplicate `RequiredCustom`)
     skips the source with `warning[precheck-mismatch]` in BOTH modes — never
     exit 6.
   - **Abort warnings** — strict precheck errors return the warnings accumulated
     before the abort, so `main` can print them before the error.
   - **Adapter errors** — `warning[fetch]`, next source (a fetch-time
     `RequiredParamMismatchError` → `warning[precheck-mismatch]` carrying the error
     as `Err`, next source).
   - **Result trust policy** — a result whose `Filled` mask disagrees with its
     contents (declared-but-empty or filled-but-undeclared) → `warning[result]`,
     result still used as-is.
   - **Downgrade** — symmetric across the three levels: a line/word request yielding
     plain lyrics → `warning[downgraded]` ("returned no synced lyrics" / "returned
     no word-synced lyrics"); a plain request yielding only synced lyrics → the same
     warning ("returned only synced lyrics"). The unmatched result stays cached and
     can satisfy a later iteration (`none` after `line`/`word`, or `line` after
     `none`).
5. **Output** — opened before the fetch (`O_CREATE|O_EXCL` for new files,
   `O_WRONLY` without `O_TRUNC` otherwise); on any failure a freshly created file
   is removed (guarded by a same-inode check). Truncate+Seek happen only after a
   successful fetch, so existing files keep their content on every failure path
   (exit 3/4/6/7/8). `--json` writes the complete `fetch.Result` with all fields,
   including empty strings, and `formatVersion`.
6. **Warnings** — structured data produced by the fetch layer; the CLI
   (`cli/get-lyrics/render.go`) renders every byte of display text, including the
   `[kind]` tag. Warnings never change the exit code. Every error path in `main`
   prints the rendered warnings before the `error[...]` line; `--quiet` silences
   all of it.

### Key types

#### `source` (`source/source.go`)

- `Param` bitmask — `ParamAuthor | ParamAlbum | ParamISRC | ParamDuration`.
- `ResultField` bitmask — `FieldLyrics | FieldTitle | FieldArtist | FieldAlbum |
  FieldISRC | FieldSubSource`.
- `ParamNamePattern` (`^[A-Z][A-Z0-9_]*$`) + `ValidParamName`.
- `ParamSpec` — `Name` + `Description`; required-ness is decided per request, never
  statically.
- `Capabilities` — `Filters` + `Required` typed bitmasks + `Custom []ParamSpec` +
  `RequiredCustom []string` for this request.
- `Request` — `Song` + `Author`/`Album`/`ISRC`/`Duration`/`SyncLevel`,
  `UserAgent`, and `Custom map[string]string`.
- `Result` — its `Filled` mask declares which fields the adapter actually populated
  (unset fields are treated as empty by the fetch layer); `Lyrics` is the single
  lyrics payload — plain, LRC or TTML — with `Level`
  (`SyncNone`/`SyncLine`/`SyncWord`) declaring which; `SubSource` +
  `FieldSubSource` identify an aggregate sub-source — standalone adapters leave
  both empty.
- `Source` interface — `Name`/`Capabilities`/`CustomParams`/`Fetch`;
  `CustomParams()` returns the static, request-independent list for `--help`
  rendering and env fallback.
- `Registry` — concurrency-safe name→source map; gate 1 validation in `Register`.
- `ErrInvalidParamName` — gate 1, carries source + key, `Duplicate` flag.
- `RequiredParamMismatchError` — raised by `Fetch` when a required parameter is
  missing (a capability declaration bug); carries `Param`/`ParamName` for the CLI
  renderer.

Adapters declare required typed params via `Capabilities(req).Required` and required
custom keys via `Capabilities(req).RequiredCustom` (a subset of that request's
`Custom` names); the fetch layer enforces them via `fetch.RequiredParamError`, and a
fetch-time miss surfaces as `RequiredParamMismatchError`. `Capabilities(req)` is
request-aware so conditional support is expressible (lrclib drops `--album` when
`--author` is absent; mock-custom recognizes `COUNTRY` only when `LANG` is present).

#### `fetch` (`fetch/fetch.go`)

- `Params` — `Song`, `Source []string`, `Author`/`Album`/`ISRC`/`Duration`,
  `SyncLevels`, `Lenient`, `UserAgent`, `Custom map[string]string`, and `Timeout uint`
  (per-source call timeout in seconds; 0 = no timeout).
- `Result` — `Source` = adapter name, `SubSource` = aggregate sub-source, `Level` =
  SyncLevel of the lyrics (`SyncUnknown`/`SyncNone`/`SyncLine`/`SyncWord`).
- `Warning` — structured data only (the CLI renders all display text): kinds
  `UnsupportedParam`/`Downgraded`/`PreCheck`/`PrecheckMismatch`/`FetchFailed`/
  `ResultMismatch`; `ParamName` for custom keys — `Param` stays 0 for them; plus
  `Want` (Downgraded direction), `Field`+`Declared` (ResultMismatch), `Err`
  (underlying cause).
- `NoResultError` — exit 4.
- `InvalidSyncLevelError` — precheck rejects `SyncUnknown` in `Params.SyncLevels`
  (a caller bug; request-level check before per-source validation, not downgraded
  by `--lenient`).
- `UnknownSourceError` — exit 3.
- `DuplicateSourceError` — exit 8.
- `RequiredParamError` — exit 6; carries `Param` + `ParamName` (the CLI renderer
  spells custom keys as `--env <KEY>`).
- `StoppedError` — exit 9; the caller's context ended (deadline or cancellation)
  mid-fetch, unwraps to `ctx.Err()`.
- `CustomParamsFor(params)` — read-only query: static declarations per requested
  source, only `Source`/`Lenient` participate, no warnings/required checks/gate 2 —
  used by `main` for help rendering and env fallback.

Unsupported custom keys produce per-source `warning[unsupported]` in map iteration
order (unspecified; assert warning sets, never order).

### Built-in sources

Per-source filters, endpoints, and quirks are documented once, not here:

- [`docs/usage/built-in-sources.md`](docs/usage/built-in-sources.md) — user-facing
  support matrix.
- [`docs/refs/internal/providers/<name>.md`](docs/refs/internal/providers/) —
  package reference per adapter.

Invariants shared by all built-ins:

- No adapter sets its own timeout — the fetch layer bounds every call via the
  context.
- Standalone adapters leave `Result.SubSource` empty with `FieldSubSource` unset;
  only aggregate adapters (e.g. `betterlyrics`) report a sub-source.
- `musixmatch` is the only built-in requiring a custom key
  (`MUSIXMATCH_API_KEY`).

Adding a built-in source: create `internal/provider/real/<name>/`, then add an
import and `r.Register(<name>.New())` in `bootstrap/bootstrap.go`. No CLI-layer
changes required.

### Mock sources

Registered only under the `test` build tag via `bootstrap.RegisterAllMock` (never
in production). Names must start with `mock-`. The adapter→concern mapping lives in
[`docs/refs/mocks.md`](docs/refs/mocks.md).

CLI-level tests reach these adapters only through `--source` and never register
fixtures into the shared registry themselves; a new test concern means a new
`mock-*` adapter in `bootstrap/bootstrap_mock.go`.

## Testing

- Tests live alongside code as `*_test.go`; the `cli/get-lyrics` `*_test.go` files
  drive `Run(argv, stdout, stderr)` with buffers and assert exit code, stdout, and
  stderr independently.
- **Real sources are NOT covered by automated tests** — `lrclib`/`lyricsovh`/
  `lrccx`/`musixmatch`/`betterlyrics` are exercised manually against their live
  endpoints. Only the mocks and the CLI/fetch layers are under test.
- CI (`.github/workflows/simple_ci_cd.yml`) is **release-only** — on `v*` tag
  pushes it:
  - runs `go test -tags test` + `go vet -tags test`;
  - cross-compiles for `linux/amd64`, `linux/arm64`, `darwin/arm64`,
    `windows/amd64`, `windows/arm64` (`CGO_ENABLED=0`, `-trimpath`, version
    ldflag);
  - writes `checksums.txt`;
  - publishes via `softprops/action-gh-release@v3`.

## Code Style & Conventions

- Standard Go naming (`CamelCase` exported / `camelCase` unexported); module path
  `github.com/PloyBox/get-lyrics`.
- Operational warnings and hard errors go to **stderr**, never stdout.
- Branch on errors with `errors.Is(err, source.ErrNotFound)` /
  `errors.As(err, &fetch.RequiredParamError{})` /
  `errors.As(err, &fetch.NoResultError{})`.
- Comments only where behavior is non-obvious.

## Agent-Specific Guidance

- **Verify with:** `go build ./...`, `go test -tags test ./...`,
  `go vet -tags test ./...`, `gofmt -l .` (all must be clean).
- **Do NOT:** add a TUI/GUI; make the song title optional; write warnings to stdout;
  bypass the `source.Source` interface for new providers.
- **Adapters must:**
  - self-declare `Capabilities(req)` — filters honored + required params;
    request-aware so conditional support is expressible (most adapters return a
    constant);
  - declare required typed params via `Capabilities(req).Required` and required
    custom keys via `Capabilities(req).RequiredCustom` (a subset of that request's
    `Custom` names; both are precheck-enforced — if `Fetch` finds a required
    parameter missing anyway, a declaration bug — raise
    `RequiredParamMismatchError` with `Param`/`ParamName` filled so the CLI
    renders a correct message);
  - return the static custom-parameter list from `CustomParams()` — legal
    `^[A-Z][A-Z0-9_]*$`, distinct keys; sources without custom params return nil;
  - respect `ctx`;
  - set `Result.Filled` to declare exactly which result fields were populated (the
    fetch layer reads only declared fields and warns on mismatches);
  - produce exactly one lyrics track per `Fetch` and declare `Result.Level` to
    match the payload the upstream actually returned
    (`SyncNone`/`SyncLine`/`SyncWord`) — the fetch layer matches results to the
    request by it;
  - fill `Result` metadata (Title/Artist/Album/ISRC) only from fields the upstream
    response actually returned — never echo `Request` values back; a field the
    server did not return stays unset with its `Filled` bit clear;
  - leave `source.Result.SubSource` empty with the `FieldSubSource` bit unset for
    standalone adapters; aggregate adapters (fronting multiple upstreams behind one
    CLI source name, like `betterlyrics`) set both — `SubSource` is the sub-source
    identifier (e.g. `"ttml"` / `"kugou"`) and `FieldSubSource` is the population
    bit;
  - never panic on missing `Song`.
- **Commits:** conventional prefixes (`feat:`, `chore:`); base branch is `main`.

## Pointers

- `README.md` — install plus a documentation index.
- `CONTRIBUTING.md` — contribution guidelines (not yet written).
- `docs/usage/` — user-facing docs: `usage.md`, `exit-codes.md`,
  `source-parameters.md`, `built-in-sources.md`, `add-a-source.md`.
- `docs/refs/` — package-level reference extracted from code comments:
  `source.md`, `fetch.md`, `cli.md`, `bootstrap.md`, `mocks.md`, and
  `internal/providers/<name>.md` per real adapter.
