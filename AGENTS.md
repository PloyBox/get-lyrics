# AGENTS.md

This file provides guidance to AI coding agents working in this repository.
The contents of this document should be in English.

## Project Overview

A Go CLI that fetches song lyrics from pluggable sources. No TUI/GUI — pure command line. The song title is a mandatory positional argument; everything else comes via flags. No third-party runtime dependencies; stdlib `flag` for parsing.

## Repository Layout

```
get-lyrics/
├── cli/
│   └── get-lyrics/             # command package (package main): run.go entrypoint, flags/env/usage/output, loadmock.go (test tag), tests
├── internal/
│   ├── bootstrap/              # bootstrap.go: registers real sources; bootstrap_mock.go (test tag): mocks
│   ├── provider/               # concrete adapters implementing source.Source
│   │   ├── mock/               # mock-* test-only adapters (success/require/nosupport/fail/lrc/nosync/mismatch/custom)
│   │   └── real/               # lrclib, lyricsovh, lrccx, musixmatch adapters
│   ├── source/                 # Source interface, Request/Result, Param/ResultField bitmasks, ParamSpec, Registry
│   └── fetch/                  # Fetch(ctx, params): precheck (incl. gate 2), failover, synced-vs-plain resolution, CustomParamsFor
```

## Build & Test

- `go build ./...` — compile
- `go test -tags test ./...` — test suite (the `test` tag is required: the `cli/get-lyrics` `*_test.go` tests exercise the `mock-*` sources registered by `loadmock.go`, which compiles only under the tag)
- `go vet -tags test ./...` — static checks (same tag requirement)
- Prerequisites: Go toolchain (module declares Go 1.25.10); network access for real sources at runtime

## CLI Surface

- **Positional (required):** `<song>` — multiple positionals are joined with spaces.
- **Flags (long/short):**
  - `--source`/`-s` — comma-separated source names, tried in order (failover). Default `lrclib`. Entries are trimmed; empty ones dropped.
  - `--author`/`-a`, `--album`/`-A`, `--iswc`/`-i` — filters.
  - `--output`/`-o` — write lyrics to this file instead of stdout. **Refuses to overwrite** an existing file (exit 7) unless `--overwrite`/`-O` is given.
  - `--sync-level`/`-S` — comma-separated `line`/`none` levels; user-given order is the priority (first match wins). Default `line,none`. Any other value is a usage error (exit 2).
  - `--user-agent`/`-u` — HTTP `User-Agent` header sent to sources. Default `get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics)` (`<ver>` is the version stamped at build time). The built-in sources carry no default of their own — they trust whatever UA they are handed; a non-empty value replaces the CLI default on every upstream request.
  - `--env`/`-e` — repeatable custom source parameter `key=value` (open-ended; keys are source-declared). Key must match `^[A-Z][A-Z0-9_]*$`; value non-empty after trimming; duplicate keys rejected — any violation is a usage error (exit 2).
  - `--lenient`/`-l` — skip invalid sources with `warning[precheck]` instead of failing fast.
  - `--help`/`-h`, `--version`/`-v` — exit 0.
- Go's `flag` package accepts both `--flag` and `-flag`; both work.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may still go to stderr) |
| 2 | Usage error (missing song, unknown flag, invalid `--sync-level`, invalid/duplicate `--env` entry) |
| 3 | Unknown `--source` name (strict precheck) |
| 4 | No valid result: all sources skipped/failed, or no format match |
| 5 | Output failure (file open, truncate, write, or close) |
| 6 | Source-required parameter missing (e.g. `--author` for `lyricsovh`, or a required `--env` key) |
| 7 | `--output` exists and `--overwrite` not given |
| 8 | Duplicate `--source` entry (strict precheck) |

## Architecture

Thin CLI layer over a pluggable-source abstraction:

1. **Registration** — `cli/get-lyrics/run.go` runs `bootstrap.RegisterAll(r)` once at package-init time, before `main()`; test builds additionally register `mock-*` sources via `init()` in `cli/get-lyrics/loadmock.go`.
   - `Registry.Register` is **gate 1**: a source's static `CustomParams()` list must contain only legal (`^[A-Z][A-Z0-9_]*$`), distinct keys — a violation returns `ErrInvalidParamName` and panics at startup (adapter init failure is a programmer error).
2. **Parse** — required positional `<song>` plus flags via `flag.NewFlagSet`; `--sync-level` (`line`/`none` → `[]fetch.SyncLevel`) and `--env` values parsed and validated at parse time (violations are usage errors, exit 2).
3. **Env fallback** — before the fetch, `main` calls `svc.CustomParamsFor(params)` (strict: unknown source → exit 3, duplicate → exit 8, reported before the fetch; lenient: problem sources silently skipped) and fills every declared key the user did not pass via `-e` from the process environment (`-e` > env > missing; an empty env var counts as missing). Injected keys behave exactly like user-passed ones.
4. **Fetch** (`fetch.New(registry).Fetch(ctx, params)`) — two-level loop: outer over `params.SyncLevels` (priority order), inner over sources (failover); a per-call result cache dedupes by source+SyncLevel.
   - **Precheck** — first a request-level check: `SyncUnknown` in `params.SyncLevels` → `InvalidSyncLevelError` (a caller bug; rejected before any per-source validation including gate 2, in BOTH modes — never downgraded to a warning). Then, per source: duplicate source → exit 8, unknown name → exit 3, missing required param (typed first, then `RequiredCustom` in declaration order) → exit 6 (`--lenient` downgrades these to `warning[precheck]` + skip).
   - **Gate 2** — runs before the missing check: a request-aware custom declaration inconsistent with the static list (invalid name, static mismatch, `RequiredCustom` not a subset of `Custom`, or duplicate `RequiredCustom`) skips the source with `warning[precheck-mismatch]` in BOTH modes — never exit 6.
   - **Abort warnings** — strict precheck errors return the warnings accumulated before the abort, so `main` can print them before the error.
   - **Adapter errors** — `warning[fetch]`, next source (a fetch-time `RequiredParamMismatchError` → `warning[precheck-mismatch]` carrying the error as `Err`, next source).
   - **Result trust policy** — a result whose `Filled` mask disagrees with its contents (declared-but-empty or filled-but-undeclared) → `warning[result]`, result still used as-is (trust policy).
   - **Downgrade** — symmetric in both directions: a synced request yielding plain lyrics → `warning[downgraded]` ("returned no synced lyrics"); a plain request yielding only synced lyrics → the same warning ("returned only synced lyrics"). In both cases the unmatched result stays cached and can satisfy a later iteration (`none` after `line`, or `line` after `none`).
5. **Output** — opened before the fetch (`O_CREATE|O_EXCL` for new files, `O_WRONLY` without `O_TRUNC` otherwise); on any failure a freshly created file is removed (guarded by a same-inode check). Truncate+Seek happen only after a successful fetch, so existing files keep their content on every failure path (exit 3/4/6/7/8).
6. **Warnings** — structured data produced by the fetch layer; the CLI (`cli/get-lyrics/render.go`) renders every byte of display text, including the `[kind]` tag. Warnings never change the exit code. Every error path in `main` prints the rendered warnings before the `error[...]` line.

**Key types (`internal/source/source.go`):**

- `Param` bitmask — `ParamAuthor | ParamAlbum | ParamISWC`.
- `ResultField` bitmask — `FieldLyrics | FieldSyncedLyrics | FieldTitle | FieldArtist | FieldAlbum | FieldISWC | FieldSubSource`.
- `ParamNamePattern` (`^[A-Z][A-Z0-9_]*$`) + `ValidParamName`.
- `ParamSpec` — `Name` + `Description`; required-ness is decided per request, never statically.
- `Capabilities` — `Filters` + `Required` typed bitmasks + `Custom []ParamSpec` + `RequiredCustom []string` for this request.
- `Request` — now carries `Custom map[string]string`.
- `Result` — its `Filled` mask declares which fields the adapter actually populated (unset fields are treated as empty by the fetch layer); its `SubSource` field + `FieldSubSource` bit are for aggregate sub-source identification — standalone adapters leave both empty.
- `Source` interface — `Name`/`Capabilities`/`CustomParams`/`Fetch`; `CustomParams()` returns the static, request-independent list for `--help` rendering and env fallback.
- `Registry` — concurrency-safe name→source map; gate 1 validation in `Register`.
- `ErrInvalidParamName` — gate 1, carries source + key, `Duplicate` flag.
- `RequiredParamMismatchError` — raised by `Fetch` when a required parameter is missing (a capability declaration bug); carries `Param`/`ParamName` for the CLI renderer.

Adapters declare required typed params via `Capabilities(req).Required` and required custom keys via `Capabilities(req).RequiredCustom` (a subset of that request's `Custom` names); the fetch layer enforces them via `fetch.RequiredParamError`, and a fetch-time miss surfaces as `RequiredParamMismatchError`. `Capabilities(req)` is request-aware so conditional support is expressible (lrclib drops `--album` when `--author` is absent; mock-custom recognizes `COUNTRY` only when `LANG` is present).

**Key types (`internal/fetch/fetch.go`):**

- `Params` — now carries `Custom map[string]string`.
- `Result` — `Source` = adapter name, `SubSource` = aggregate sub-source, `Level` = SyncLevel of the lyrics (`SyncUnknown`/`SyncNone`/`SyncLine`).
- `Warning` — structured data only (the CLI renders all display text): kinds `UnsupportedParam`/`Downgraded`/`PreCheck`/`PrecheckMismatch`/`FetchFailed`/`ResultMismatch`; `ParamName` for custom keys — `Param` stays 0 for them; plus `Want` (Downgraded direction), `Field`+`Declared` (ResultMismatch), `Err` (underlying cause).
- `NoResultError` — exit 4.
- `InvalidSyncLevelError` — precheck rejects `SyncUnknown` in `Params.SyncLevels` (a caller bug; request-level check before per-source validation, not downgraded by `--lenient`).
- `UnknownSourceError` — exit 3.
- `DuplicateSourceError` — exit 8.
- `RequiredParamError` — exit 6; carries `Param` + `ParamName` (the CLI renderer spells custom keys as `--env <KEY>`).
- `CustomParamsFor(params)` — read-only query: static declarations per requested source, only `Source`/`Lenient` participate, no warnings/required checks/gate 2 — used by `main` for help rendering and env fallback.

Unsupported custom keys produce per-source `warning[unsupported]` in map iteration order (unspecified; assert warning sets, never order).

### Built-in sources

- **`lrclib`** — `https://lrclib.net`; `/api/get` when `--author` is given, else `/api/search`. Filters: Author, Album — the album filter only takes effect with `--author` (otherwise it is dropped with an unsupported warning). Synced LRC output when requested; a synced-only hit leaves `Lyrics` unfilled so the fetch layer outputs the synced track. 10s per-request timeout.
- **`lyricsovh`** — `https://api.lyrics.ovh/v1/{artist}/{title}`. Filter: Author, and **requires** it. Surfaces the API's 404 as not-found. 10s timeout.
- **`lrccx`** — `https://api.lrc.cx/jsonapi`. Filters: Author, Album (independent of each other). Always LRC-flavoured text: plain lyrics strip `[mm:ss]`/marker tags; synced lyrics only when the text has timestamped lines. 10s timeout.
- **`musixmatch`** — `https://api.musixmatch.com/ws/1.1`. Requires the custom `--env` key `MUSIXMATCH_API_KEY` (RequiredCustom). Filter: Author. With `--author`: `matcher.lyrics.get` / `matcher.subtitle.get`; title-only: `track.search` → `track.lyrics.get` / `track.subtitle.get` by commontrack id. Subtitle endpoints need the paid Scale plan — on Basic they 402/403, which the adapter treats as "no synced" and falls back to plain (fetch layer warns `downgraded`). Album/ISWC unsupported (no album param; `track_isrc` is ISRC, not ISWC). Instrumental `"...."` and the `*******` usage trailer are stripped. 10s timeout.

Adding a built-in source: create `internal/provider/real/<name>/`, then add an import and `r.Register(<name>.New())` in `internal/bootstrap/bootstrap.go`. No CLI-layer changes required.

### Mock sources

Registered only under the `test` build tag via `bootstrap.RegisterAllMock` (never in production). Names must start with `mock-`. Each covers one testing concern:

- `mock-success` — happy path.
- `mock-require` — exit 6.
- `mock-nosupport` — no-param path.
- `mock-fail` — exit 4.
- `mock-lrc` — synced path.
- `mock-nosync` — downgrade path.
- `mock-synconly` — synced-only path (never fills plain `Lyrics`).
- `mock-mismatch` — precheck-vs-requirement mismatch path.
- `mock-custom` — custom `--env` params: `LANG` always recognized+required, `COUNTRY` conditional on `LANG`.

## Testing

- Tests live alongside code as `*_test.go`; the `cli/get-lyrics` `*_test.go` files drive `Run(argv, stdout, stderr)` with buffers and assert exit code, stdout, and stderr independently.
- **Real sources are NOT covered by automated tests** — `lrclib`/`lyricsovh`/`lrccx`/`musixmatch` are exercised manually against their live endpoints. Only the mocks and the CLI/fetch layers are under test.
- CI (`.github/workflows/simple_ci_cd.yml`) is **release-only** — on `v*` tag pushes it:
  - runs `go test -tags test` + `go vet -tags test`;
  - cross-compiles for `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`, `windows/arm64` (`CGO_ENABLED=0`, `-trimpath`, version ldflag);
  - writes `checksums.txt`;
  - publishes via `softprops/action-gh-release@v3`.

## Code Style & Conventions

- Standard Go naming (`CamelCase` exported / `camelCase` unexported); module path `github.com/PloyBox/get-lyrics`.
- Operational warnings and hard errors go to **stderr**, never stdout.
- Branch on errors with `errors.Is(err, source.ErrNotFound)` / `errors.As(err, &fetch.RequiredParamError{})` / `errors.As(err, &fetch.NoResultError{})`.
- Comments only where behavior is non-obvious.

## Agent-Specific Guidance

- **Verify with:** `go build ./...`, `go test -tags test ./...`, `go vet -tags test ./...`, `gofmt -l .` (all must be clean).
- **Do NOT:** add a TUI/GUI; make the song title optional; write warnings to stdout; bypass the `source.Source` interface for new providers.
- **Adapters must:**
  - self-declare `Capabilities(req)` — filters honored + required params; request-aware so conditional support is expressible (most adapters return a constant);
  - declare required typed params via `Capabilities(req).Required` and required custom keys via `Capabilities(req).RequiredCustom` (a subset of that request's `Custom` names; both are precheck-enforced — if `Fetch` finds a required parameter missing anyway, a declaration bug — raise `RequiredParamMismatchError` with `Param`/`ParamName` filled so the CLI renders a correct message);
  - return the static custom-parameter list from `CustomParams()` — legal `^[A-Z][A-Z0-9_]*$`, distinct keys; sources without custom params return nil;
  - respect `ctx`;
  - set `Result.Filled` to declare exactly which result fields were populated (the fetch layer reads only declared fields and warns on mismatches);
  - leave `source.Result.SubSource` empty with the `FieldSubSource` bit unset (aggregate sub-source only);
  - never panic on missing `Song`.
- **Commits:** conventional prefixes (`feat:`, `chore:`); base branch is `main`.

## Pointers

- `README.md` — full usage guide: install, usage examples, exit-code table, built-in sources, how to add a source.
- `CONTRIBUTING.md` — contribution guidelines (not yet written).
- `docs/` — additional design or source-integration documentation (not yet written).
