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
├── main_test.go                # End-to-end tests for Run(argv, stdout, stderr); registers
│                               # mock sources via init() + bootstrap.RegisterAllMock
├── go.mod                      # Module: github.com/PloyBox/get-lyrics (Go 1.25)
├── AGENTS.md                   # This file
└── internal/
    ├── bootstrap/
    │   ├── bootstrap.go        # RegisterAll(r *Registry) — registers real (production) sources
    │   └── bootstrap_mock.go   # RegisterAllMock(r *Registry) — registers mock/test-only sources
    ├── source/
    │   ├── source.go           # Source interface, Request, Result, Param bitmask,
    │   │                       # Registry, RequiredParamError, ErrNotFound/ErrDuplicate
    │   ├── source_test.go
    │   ├── mock/
    │   │   ├── success/        # mock-success: happy path; ParamAuthor, requires --author
    │   │   ├── require/        # mock-require: RequiredParamError path; Param(0), requires --author
    │   │   ├── nosupport/      # mock-nosupport: Param(0), requires nothing, always succeeds
    │   │   ├── fail/           # mock-fail: always errors; exercises exit code 4
    │   │   ├── lrc/            # mock-lrc: ParamTimestamp, returns SyncedLyrics when asked
    │   │   └── nosync/         # mock-nosync: ParamTimestamp but never returns SyncedLyrics
    │   └── real/
    │       ├── lrccx/
    │       │   └── lrccx.go   # Adapter for https://api.lrc.cx/jsonapi (legacy API)
    │       ├── lrclib/
    │       │   └── lrclib.go   # Adapter for https://lrclib.net (search + get endpoints)
    │       └── lyricsovh/
    │           └── lyricsovh.go # Adapter for https://api.lyrics.ovh (artist/title path)
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
- `go test -tags test ./...` — run the test suite
- `go vet -tags test ./...` — static checks (test tag required, same as `go test`: `main_test.go` references mock adapters compiled only under it)

**Prerequisites:**
- Go toolchain installed (recent stable release; module declares Go 1.25.10)
- Network access to configured lyrics sources at runtime (`lrclib`, `lyricsovh`, `lrccx`); the `mock-*` sources are offline-only and never registered in production builds

**Environment Variables:**
- None required.

## CLI Surface

- **Positional argument (required):** `<song>` — the song title to fetch lyrics for. Multiple positional arguments are joined with spaces into a single title (e.g. `get-lyrics --source mock-success Bohemian Rhapsody` and `get-lyrics --source mock-success "Bohemian Rhapsody"` are equivalent).
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
| 6 | Source-required parameter missing (e.g. `--author` not supplied to `mock-require` or `lyricsovh`) |

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

- **`lrclib`** — Real adapter against `https://lrclib.net`. Picks `/api/get` when `--author` is supplied (structured `track_name`/`artist_name`/`album_name` query), otherwise `/api/search` with freeform `q=`. Supports `ParamAuthor | ParamAlbum | ParamTimestamp` (`album` only acts on the `/api/get` path); honors `SyncedLyrics` from the response when `--timestamp` was requested. 10-second per-request timeout.
- **`lyricsovh`** — Real adapter against `https://api.lyrics.ovh/v1/{artist}/{title}`. Supports `ParamAuthor` and **requires** it (the path cannot be built without an artist). Surfaces the API's 404 as a not-found error rather than a generic HTTP-status failure. 10-second per-request timeout.
- **`lrccx`** — Real adapter against the legacy `https://api.lrc.cx/jsonapi` endpoint. GETs `title`/`artist`/`album` query params (`artist` optional; `album` value `[Unknown Album]` treated as empty; the API's `path` param is never sent) and picks the first hit whose `lrc` field is non-empty from the score-descending JSON array. Supports `ParamAuthor | ParamAlbum | ParamTimestamp`. The response is always LRC-flavoured text: plain `Lyrics` are produced by stripping `[mm:ss]` timestamps and section/marker tags (`[Verse]`, `[!text]`, ...), while `SyncedLyrics` carries the raw text only when `--timestamp` was requested AND the text actually contains timestamped lines — unsynced `[!text]` entries fall back to plain lyrics. The `lrc` field may be `null` (instrumental entries). 10-second per-request timeout.

Adding a new built-in source means: create `internal/source/real/<name>/` implementing `source.Source`, then add an import and `r.Register(<name>.New())` line to `internal/bootstrap/bootstrap.go`. No CLI-layer changes are required.

### Mock/Test-Only Sources

Registered only during tests via `bootstrap.RegisterAllMock` (never in production). Each mock covers one dedicated testing concern:

- **`mock-success`** — Happy-path adapter. Advertises `ParamAuthor` and **requires** it: a fetch without `--author` returns `RequiredParamError`; with it, returns deterministic lyrics. Drives stdout/file output, single-dash long-form flags, and unsupported `--album`/`--iswc`/`--timestamp` warnings.
- **`mock-require`** — Exercises the `RequiredParamError` path in isolation. Advertises `Param(0)` (no optional params) but **requires** `--author`: a fetch without it returns `RequiredParamError`, with it succeeds. Drives exit code 6.
- **`mock-nosupport`** — Advertises `Param(0)` and requires nothing; fetch always succeeds. Proves the no-`--author` path and that every optional flag trips an unsupported-parameter warning.
- **`mock-fail`** — Advertises `ParamAuthor` but always returns a fetch error. Drives the exit code 4 (fetch failure) path at the CLI level.
- **`mock-lrc`** — Advertises `ParamTimestamp` and requires nothing; with `--timestamp` it returns LRC-style `SyncedLyrics` (plain `Lyrics` otherwise). Drives the `ModeSynced` output path at the CLI level.
- **`mock-nosync`** — Advertises `ParamTimestamp` but never returns `SyncedLyrics`: a `--timestamp` fetch succeeds with plain `Lyrics` only. Drives the "source returned no timestamped lyrics; using plain lyrics" stderr fallback warning.

All mock/test-only source names must start with the `mock-` prefix (e.g. `mock-success`, `mock-fail`). This distinguishes them from real sources in the registry at a glance.

## Testing

**Test conventions:**
- Tests live alongside the code as `*_test.go` files.
- `main_test.go` drives the public `Run(argv, stdout, stderr) int` entry point with `bytes.Buffer` writers so exit codes, stdout, and stderr are all asserted independently. It registers mock/test-only sources via `init() + bootstrap.RegisterAllMock`.
- Mock/test-only source files (under `internal/source/mock/`) carry `//go:build test` and are only compiled during `go test`.
- `bootstrap/bootstrap_mock.go` also carries `//go:build test`, so mock sources are never linked into production binaries.
- **Real sources are NOT covered by automated tests.** `lrclib` and `lyricsovh` have no `*_test.go` files (0% statement coverage) and are exercised only manually against their live endpoints. Only the mock adapters and the CLI/fetch/output layers are under automated test.
- `internal/source/source_test.go` covers the `Registry` itself (register/lookup/duplicate/unregister).

**Coverage in `main_test.go` includes:**
- Missing song → exit 2 with the required-song stderr message.
- `--help` and `-h` → exit 0; stdout lists `mock-success`, `lrclib`, `lyricsovh`.
- `--version` and `-v` → exit 0; stdout contains the version string.
- Default stdout sink and `--output` file sink paths.
- Unsupported `--album` / `--iswc` warnings on `mock-success` (which advertises only `ParamAuthor`).
- Unknown source name → exit 3 with the unknown-source message.
- Single-dash long-form flags (`-source`, `-author`) are accepted.
- `--timestamp` on a source without `ParamTimestamp` falls back to plain lyrics with a stderr warning.
- `mock-require` without `--author` → exit 6 with the canonical `source "mock-require" requires --author` message.
- Fetch failure via `mock-fail` → exit 4 with the adapter's error on stderr.
- `mock-require` with `--author` → exit 0 (the required-param mock succeeds when the flag is supplied).
- `mock-nosupport` without `--author` → exit 0 (a source can succeed without any optional params).
- Unknown/typo flag (e.g. `--bogus`) → exit 2 with the flag package's parse error on stderr.
- `--timestamp` on `mock-lrc` → exit 0 with LRC timestamped lyrics on stdout.
- `--timestamp` on `mock-nosync` → exit 0, plain lyrics with the "returned no timestamped lyrics" stderr warning.

**CI Pipeline:**
- The only workflow is `.github/workflows/simple_ci_cd.yml` ("Simple CI/CD"). It is **release-only**: it triggers on `v*` tag pushes; there is no CI on ordinary pushes to `main` or on pull requests.
- On a release tag it: checks out the repo, sets up Go 1.25 (`actions/setup-go@v5` with cache), runs `go test -tags test ./...` and `go vet -tags test ./...`, cross-compiles release binaries for `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`, `windows/arm64` (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=<tag>"`), writes `checksums.txt` via `sha256sum`, then publishes assets with `softprops/action-gh-release@v3` (auto-generated release notes). The workflow declares `permissions: contents: write`.

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
- `go build`, `go test -tags test`, `go vet -tags test`, `gofmt` for verification.

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
- `go test -tags test ./...` passes
- `go vet -tags test ./...` is clean
- `gofmt -l .` produces no output (or `gofmt -d .` shows no diffs)
- Missing song title produces exit code 2 with the required-song stderr message
- `--help` / `-h` prints usage and the sorted list of registered sources, exit 0
- `--version` prints version string and exits 0
- Unknown `--source` produces exit code 3
- Source requiring a missing parameter produces exit code 6 with `source "<name>" requires --<flag>`
- Unsupported `--album`/`--iswc` produces a stderr warning for the relevant source
- Omitted `--output` prints lyrics to stdout; provided `--output` writes the file
- Unwritable `--output` path produces exit code 5

## Pointers

- `README.md` — full usage guide: install (prebuilt binaries from Releases + `go install`), usage examples with flags, exit-code table, built-in sources, and how to add a new source
- `CONTRIBUTING.md` — contribution guidelines (not yet written)
- `docs/` — additional design or source-integration documentation (not yet written)
