# AGENTS.md

This file provides guidance to AI coding agents working in this repository.
The contents of this document should be in English.

## Project Overview

A Go CLI that fetches song lyrics from pluggable sources. No TUI/GUI — pure command line. The song title is a mandatory positional argument; everything else comes via flags. No third-party runtime dependencies; stdlib `flag` for parsing.

## Repository Layout

```
get-lyrics/
├── main.go                     # CLI wiring: argv parsing, exit-code mapping, output routing
├── main_loadmock.go            # //go:build test — registers mock sources via bootstrap.RegisterAllMock
├── main_test.go                # End-to-end tests for Run(argv, stdout, stderr)
├── internal/
│   ├── bootstrap/              # bootstrap.go: registers real sources; bootstrap_mock.go (test tag): mocks
│   ├── source/                 # Source interface, Request/Result, Param bitmask, Registry, RequiredParamError
│   │   ├── mock/               # mock-* test-only adapters (success/require/nosupport/fail/lrc/nosync)
│   │   └── real/               # lrclib, lyricsovh, lrccx adapters
│   └── fetch/                  # Fetch(ctx, params): precheck, failover, synced-vs-plain resolution
```

## Build & Test

- `go build ./...` — compile
- `go test -tags test ./...` — test suite (the `test` tag is required: `main_test.go` and the mock sources compile only under it)
- `go vet -tags test ./...` — static checks (same tag requirement)
- Prerequisites: Go toolchain (module declares Go 1.25.10); network access for real sources at runtime

## CLI Surface

- **Positional (required):** `<song>` — multiple positionals are joined with spaces.
- **Flags (long/short):**
  - `--source`/`-s` — comma-separated source names, tried in order (failover). Default `lrclib`. Entries are trimmed; empty ones dropped.
  - `--author`/`-a`, `--album`/`-A`, `--iswc`/`-i` — filters.
  - `--output`/`-o` — write lyrics to this file instead of stdout. **Refuses to overwrite** an existing file (exit 7) unless `--overwrite`/`-O` is given.
  - `--timestamp`/`-t` — comma-separated `line`/`none` formats; user-given order is the priority (first match wins). Default `line,none`. Any other value is a usage error (exit 2).
  - `--lenient`/`-l` — skip invalid sources with `warning[precheck]` instead of failing fast.
  - `--help`/`-h`, `--version`/`-v` — exit 0.
- Go's `flag` package accepts both `--flag` and `-flag`; both work.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may still go to stderr) |
| 2 | Usage error (missing song, unknown flag, invalid `--timestamp`) |
| 3 | Unknown `--source` name (strict precheck) |
| 4 | No valid result: all sources skipped/failed, or no format match |
| 5 | Output failure (file open, truncate, write, or close) |
| 6 | Source-required parameter missing (e.g. `--author` for `lyricsovh`) |
| 7 | `--output` exists and `--overwrite` not given |
| 8 | Duplicate `--source` entry (strict precheck) |

## Architecture

Thin CLI layer over a pluggable-source abstraction:

1. **Registration** — `main.go` runs `bootstrap.RegisterAll(r)` once before `main()`; test builds additionally register `mock-*` sources via `init()` in `main_loadmock.go`.
2. **Parse** — required positional `<song>` plus flags via `flag.NewFlagSet`; `--timestamp` values validated at parse time.
3. **Fetch** (`fetch.New(registry).Fetch(ctx, params)`) — precheck: duplicate source → exit 8, unknown name → exit 3, missing required param → exit 6 (`--lenient` downgrades all three to `warning[precheck]` + skip). Then a two-level loop: outer over `params.Timestamp` (priority order), inner over sources (failover). A per-call result cache dedupes by source+synced flag. Adapter errors → `warning[fetch]`, next source. A synced request yielding plain lyrics → `warning[downgraded]`; the result stays cached and can satisfy a later `none` iteration.
4. **Output** — opened before the fetch (`O_CREATE|O_EXCL` for new files, `O_WRONLY` without `O_TRUNC` otherwise); on any failure a freshly created file is removed (guarded by a same-inode check). Truncate+Seek happen only after a successful fetch, so existing files keep their content on every failure path (exit 3/4/6/7/8).
5. **Warnings** — pre-formatted by the fetch layer with their `[kind]` tag and printed verbatim to stderr; they never change the exit code.

**Key types (`internal/source/source.go`):** `Param` bitmask (`ParamAuthor | ParamAlbum | ParamISWC`), `Capabilities` (`Filters` + `Required`), `Request`, `Result` (its `Source` field is reserved for aggregate sub-source identification — standalone adapters leave it empty), `Source` interface (`Name`/`Capabilities`/`Fetch`), `Registry` (concurrency-safe name→source map), `RequiredParamError`. Adapters declare required params via `Capabilities(req).Required`; the fetch layer enforces them — adapters must NOT raise the error themselves. `Capabilities(req)` is request-aware so conditional support is expressible (lrclib drops `--album` when `--author` is absent).

**Key types (`internal/fetch/fetch.go`):** `Params`, `Result` (`Source` = adapter name, `SubSource` = aggregate sub-source, `Synced` = lyrics contain LRC), `Warning` (kinds: `UnsupportedParam`/`Downgraded`/`PreCheck`/`FetchFailed`), `NoResultError` (exit 4), `UnknownSourceError` (exit 3), `DuplicateSourceError` (exit 8).

### Built-in sources

- **`lrclib`** — `https://lrclib.net`; `/api/get` when `--author` is given, else `/api/search`. Filters: Author, Album — the album filter only takes effect with `--author` (otherwise it is dropped with an unsupported warning). Synced LRC output when requested. 10s per-request timeout.
- **`lyricsovh`** — `https://api.lyrics.ovh/v1/{artist}/{title}`. Filter: Author, and **requires** it. Surfaces the API's 404 as not-found. 10s timeout.
- **`lrccx`** — `https://api.lrc.cx/jsonapi`. Filters: Author, Album (independent of each other). Always LRC-flavoured text: plain lyrics strip `[mm:ss]`/marker tags; synced lyrics only when the text has timestamped lines. 10s timeout.

Adding a built-in source: create `internal/source/real/<name>/`, then add an import and `r.Register(<name>.New())` in `internal/bootstrap/bootstrap.go`. No CLI-layer changes required.

### Mock sources

Registered only under the `test` build tag via `bootstrap.RegisterAllMock` (never in production). Names must start with `mock-`. Each covers one testing concern: `mock-success` (happy path), `mock-require` (exit 6), `mock-nosupport` (no-param path), `mock-fail` (exit 4), `mock-lrc` (synced path), `mock-nosync` (downgrade path).

## Testing

- Tests live alongside code as `*_test.go`; `main_test.go` drives `Run(argv, stdout, stderr)` with buffers and asserts exit code, stdout, and stderr independently.
- **Real sources are NOT covered by automated tests** — `lrclib`/`lyricsovh`/`lrccx` are exercised manually against their live endpoints. Only the mocks and the CLI/fetch layers are under test.
- CI (`.github/workflows/simple_ci_cd.yml`) is **release-only**: on `v*` tag pushes it runs `go test -tags test` + `go vet -tags test`, cross-compiles for `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`, `windows/arm64` (`CGO_ENABLED=0`, `-trimpath`, version ldflag), writes `checksums.txt`, and publishes via `softprops/action-gh-release@v3`.

## Code Style & Conventions

- Standard Go naming (`CamelCase` exported / `camelCase` unexported); module path `github.com/PloyBox/get-lyrics`.
- Operational warnings and hard errors go to **stderr**, never stdout.
- Branch on errors with `errors.Is(err, source.ErrNotFound)` / `errors.As(err, &source.RequiredParamError{})` / `errors.As(err, &fetch.NoResultError{})`.
- Comments only where behavior is non-obvious.

## Agent-Specific Guidance

- **Verify with:** `go build ./...`, `go test -tags test ./...`, `go vet -tags test ./...`, `gofmt -l .` (all must be clean).
- **Do NOT:** add a TUI/GUI; make the song title optional; write warnings to stdout; bypass the `source.Source` interface for new providers.
- **Adapters must:** self-declare `Capabilities(req)` (filters honored + required params; request-aware so conditional support is expressible — most adapters return a constant); declare required params via `Capabilities(req).Required` (not by raising errors inside `Fetch`); respect `ctx`; leave `source.Result.Source` empty (aggregate sub-source only); never panic on missing `Song`.
- **Commits:** conventional prefixes (`feat:`, `chore:`); base branch is `main`.

## Pointers

- `README.md` — full usage guide: install, usage examples, exit-code table, built-in sources, how to add a source.
- `CONTRIBUTING.md` — contribution guidelines (not yet written).
- `docs/` — additional design or source-integration documentation (not yet written).
