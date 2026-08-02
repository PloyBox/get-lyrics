# AGENTS.md

This file provides guidance to AI coding agents working in this repository.
The contents of this document should be in English.

## Project Overview

A command-line tool (CLI) written in Go that fetches song lyrics from a registered source. It has no TUI or GUI — interaction is strictly through the command line. The song title is a mandatory positional argument; everything else is supplied via named flags (long or short form).

**Tech Stack:**
- Go (the only implementation language, per project requirement)
- Standard library for CLI argument parsing (`flag` package)
- One or more external lyrics source integrations (pluggable by source name)
- No third-party runtime dependencies (see `go.mod`)

## Repository Layout

```
get-lyrics/
├── LICENSE                     # Apache 2.0
├── README.md                   # Usage examples and build instructions
├── main.go                     # CLI wiring: argv parsing, exit-code mapping, output routing
├── main_test.go                # End-to-end tests for Run(argv, stdout, stderr)
├── go.mod                      # Module: github.com/PloyBox/get-lyrics (Go 1.25)
├── AGENTS.md                   # This file
└── internal/
    ├── bootstrap/
    │   └── bootstrap.go        # RegisterAll(r *Registry) — single wiring point
    ├── source/
    │   ├── source.go           # Source interface, Request, Result, Param bitmask,
    │   │                       # Registry, RequiredParamError, ErrNotFound/ErrDuplicate
    │   ├── source_test.go
    │   ├── stub/
    │   │   ├── stub.go         # Offline deterministic adapter; exercises required-param path
    │   │   └── stub_test.go
    │   ├── lrclib/
    │   │   ├── lrclib.go       # Adapter for https://lrclib.net (search + get endpoints)
    │   │   └── lrclib_test.go
    │   └── lyricsovh/
    │       ├── lyricsovh.go    # Adapter for https://api.lyrics.ovh (artist/title path)
    │       └── lyricsovh_test.go
    ├── fetch/
    │   ├── fetch.go            # Fetch(ctx, req, sourceName) — registry lookup + warnings
    │   └── fetch_test.go
    └── output/
        ├── output.go           # Write(w, res, mode) — plain or LRC (synced) output
        └── output_test.go
```

## Setup & Build

Build and run with the Go toolchain:
- `go build ./...` — compile the project
- `go run . --source <name> [--author <name>] <song>` — run against a song title
- `go test ./...` — run the test suite
- `go vet ./...` — static checks

**Prerequisites:**
- Go toolchain installed (recent stable release; module declares Go 1.25.10)
- Network access to configured lyrics sources at runtime (`lrclib`, `lyricsovh`); the `stub` source is offline-only

**Environment Variables:**
- None required.

## CLI Surface

- **Positional argument (required):** `<song>` — the song title to fetch lyrics for.
- **Named flags (long form shown; short form in parentheses is also accepted):**
  - `--source` (`-s`) — which lyrics source to use. **Required in practice** (without it the help text lists available sources, but no lyrics are fetched).
  - `--author` (`-a`) — author/artist filter.
  - `--album` (`-A`) — album filter.
  - `--iswc` (`-i`) — ISWC identifier.
  - `--output` (`-o`) — write lyrics to this file; if omitted, print to stdout.
  - `--timestamp` (`-t`) — request timestamped (LRC) lyrics when the source supports it; falls back to plain lyrics with a stderr warning otherwise.
  - `--help` (`-h`) — print help information and exit with code 0. The help text includes the sorted list of registered sources.
  - `--version` (`-v`) — print version and exit with code 0.

**Note on flag forms:** Go's `flag` package accepts both `--flag` and `-flag`. The CLI does **not** intercept this; both work.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (stderr may still carry warnings) |
| 2 | Usage error (missing song, unknown/typo flag, etc.) |
| 3 | Unknown `--source` name |
| 4 | Fetch failure (network, parse, no usable lyrics) |
| 5 | Output failure (file create or write) |
| 6 | Source-required parameter missing (e.g. `--author` not supplied to `stub` or `lyricsovh`) |

Warnings about **unsupported** parameters (a flag was supplied but the source does not honor it) are emitted to **stderr** alongside the successful stdout output — they do not change the exit code.

## Architecture

Thin CLI layer over a pluggable lyrics-source abstraction, with explicit registry wiring and a small fetch/output split:

1. **Package init (in `main.go`)** — `var registry = mustRegisterAll()` runs `bootstrap.RegisterAll(r)` exactly once before `main()`. Adapter init failure panics (programmer error).
2. **Parse arguments** — required positional `<song>`, plus named flags via `flag.NewFlagSet`. Both `--x` and `-x` forms are accepted.
3. **Open output sink** — stdout by default; if `--output` is set, `os.Create(path)`; the closer is deferred.
4. **Resolve source by name** — `fetch.New(registry).Fetch(ctx, req, parsed.source)` looks up via `*source.Registry`, returning `source.ErrNotFound` for unknown names.
5. **Compute warnings** — the fetch layer compares the requested `Request` fields against `Source.SupportedParams()` and returns one warning per unsupported field.
6. **Emit warnings to stderr** before writing the result.
7. **Route lyrics** — `output.Write(out, res, mode)` writes either plain (`ModePlain`) or LRC (`ModeSynced`) text. `ModeSynced` is selected only when `--timestamp` was set AND the source supports `ParamTimestamp` AND `result.SyncedLyrics` is non-empty; otherwise plain mode is used and a stderr warning is emitted.

**Key types (in `internal/source/source.go`):**
- `Param uint` — bitmask of `ParamAuthor | ParamAlbum | ParamISWC | ParamTimestamp`.
- `Request{Song, Author, Album, ISWC, Timestamp}` — input to `Source.Fetch`. `Song` is required.
- `Result{Lyrics, SyncedLyrics, Title, Artist, Album, ISWC, Source}` — fetched output. `Lyrics` is always populated when `Fetch` returns nil; `SyncedLyrics` is populated only when timestamped output was requested and the source supports it.
- `Source` interface — `Name() string`, `SupportedParams() Param`, `Fetch(ctx, req) (Result, error)`.
- `Registry` — concurrency-safe name→`Source` map; populated by `bootstrap.RegisterAll`.
- `RequiredParamError{Source, Param, Flag}` — typed error for the missing-required-parameter case; `Unwrap()` returns `ErrRequiredParam`.

### Built-in sources

- **`stub`** — Offline, deterministic. Advertises `ParamAuthor` and **requires** it: a fetch without `--author` returns `RequiredParamError` and the CLI exits with code 6. Used by the test suite to exercise both the unsupported-warning path and the required-parameter path without network access.
- **`lrclib`** — Real adapter against `https://lrclib.net`. Picks `/api/get` when `--author` is supplied (structured `track_name`/`artist_name`/`album_name` query), otherwise `/api/search` with freeform `q=`. Supports `ParamAuthor | ParamAlbum | ParamTimestamp` (`album` only acts on the `/api/get` path); honors `SyncedLyrics` from the response when `--timestamp` was requested. 10-second per-request timeout.
- **`lyricsovh`** — Real adapter against `https://api.lyrics.ovh/v1/{artist}/{title}`. Supports `ParamAuthor` and **requires** it (the path cannot be built without an artist). Surfaces the API's 404 as a not-found error rather than a generic HTTP-status failure. 10-second per-request timeout.

Adding a new built-in source means: create `internal/source/<name>/` implementing `source.Source`, then add an import and `r.Register(<name>.New())` line to `internal/bootstrap/bootstrap.go`. No CLI-layer changes are required.

## Testing

**Test conventions:**
- Tests live alongside the code as `*_test.go` files.
- `main_test.go` drives the public `Run(argv, stdout, stderr) int` entry point with `bytes.Buffer` writers so exit codes, stdout, and stderr are all asserted independently.
- Adapter tests use `httptest` servers pointed at via the `Endpoint` field on each adapter struct.
- `internal/source/source_test.go` covers the `Registry` itself (register/lookup/duplicate/unregister).

**Coverage in `main_test.go` includes:**
- Missing song → exit 2 with the required-song stderr message.
- `--help` and `-h` → exit 0; stdout lists `stub`, `lrclib`, `lyricsovh`.
- `--version` and `-v` → exit 0; stdout contains the version string.
- Default stdout sink and `--output` file sink paths.
- Unsupported `--album` / `--iswc` warnings on `stub` (which advertises only `ParamAuthor`).
- Unknown source name → exit 3 with the unknown-source message.
- Unwritable `--output` path → exit 5 before any fetch is attempted.
- Single-dash long-form flags (`-source`, `-author`) are accepted.
- `--timestamp` on a source without `ParamTimestamp` falls back to plain lyrics with a stderr warning.
- `stub` without `--author` → exit 6 with the canonical `source "stub" requires --author` message.

**CI Pipeline:**
- Not yet defined.

## Code Style & Conventions

**Naming:**
- Standard Go: `CamelCase` for exported, `camelCase` for unexported.
- CLI flag names match the spec: `source`, `author`, `album`, `iswc`, `output`, `timestamp`, `help`, `version`.
- Source adapter `Name()` returns a stable lowercase identifier (e.g. `"lrclib"`, `"lyricsovh"`).

**Imports / Modules:**
- Module path: `github.com/PloyBox/get-lyrics`.
- New source adapters live under `internal/source/<name>/` and are wired in `internal/bootstrap/bootstrap.go` only.

**Error handling:**
- Operational warnings (e.g. a source not honoring a requested parameter) go to **stderr**, never stdout.
- Hard errors exit non-zero via the documented exit codes; messages are written to **stderr**.
- `errors.Is(err, source.ErrNotFound)` and `errors.As(err, &source.RequiredParamError{})` are the supported ways to branch on registry/required-param errors in the CLI layer.

**Comments & Documentation:**
- Comments only where behavior is non-obvious; the public CLI surface is self-documenting via `--help`/`-h`.

## Development Workflow

**Branching:**
- Base branch is `main`; use feature branches for changes.

**Commits:**
- Follow conventional, descriptive commit messages (recent history uses `feat:`, `chore:` prefixes).

**Pull Requests / Releases:**
- Not yet defined.

## Agent-Specific Guidance

**Preferred Tools:**
- `go build`, `go test`, `go vet`, `gofmt` for verification.

**Things to Avoid:**
- Do not add a TUI or GUI — CLI only.
- Do not make the song title optional — it is a mandatory positional argument.
- Do not write operational warnings to stdout; they belong on stderr.
- Do not bypass the `source.Source` interface for new providers — keep adapters behind it so `--help` lists them and the registry/required-param semantics stay uniform.

**Tasting Notes:**
- Source adapters must self-declare `SupportedParams()` so the CLI can warn accurately about **unsupported** (not just unused) parameters.
- Source adapters that **require** a parameter must return `source.RequiredParamError` (not a generic error) so the CLI can emit exit code 6 with a stable, greppable message.
- Adapters must respect `ctx` for cancellation/deadlines and should not panic on missing `Song`.

**Verification Checklist:**
- `go build ./...` succeeds
- `go test ./...` passes
- `go vet ./...` is clean
- Missing song title produces exit code 2 with the required-song stderr message
- `--help` / `-h` prints usage and the sorted list of registered sources, exit 0
- `--version` prints version string and exits 0
- Unknown `--source` produces exit code 3
- Source requiring a missing parameter produces exit code 6 with `source "<name>" requires --<flag>`
- Unsupported `--album`/`--iswc` produces a stderr warning for the relevant source
- Omitted `--output` prints lyrics to stdout; provided `--output` writes the file
- Unwritable `--output` path produces exit code 5

## Pointers

- `README.md` — minimal placeholder; usage examples and build instructions not yet written
- `CONTRIBUTING.md` — contribution guidelines (not yet written)
- `docs/` — additional design or source-integration documentation (not yet written)
