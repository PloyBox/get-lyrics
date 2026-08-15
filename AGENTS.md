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
├── main_loadmock.go            # //go:build test — registers mock sources via bootstrap.RegisterAllMock
├── main_test.go                # End-to-end tests for Run(argv, stdout, stderr)
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
    │   │   ├── success/        # mock-success: happy path; ParamAuthor + RequiredParams(Author)
    │   │   ├── require/        # mock-require: RequiredParamError path; Param(0) + RequiredParams(Author)
    │   │   ├── nosupport/      # mock-nosupport: Param(0), requires nothing, always succeeds
    │   │   ├── fail/           # mock-fail: always errors; drives the no-result path
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
    │   ├── fetch.go            # Fetch(ctx, params) — precheck, per-call cache, failover,
    │   │                       # synced-vs-plain resolution, Warning/NoResultError
    │   └── fetch_test.go
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
  - `--source` (`-s`) — comma-separated lyrics source names, tried in the given order (sequential failover). Defaults to `lrclib`. Entries are trimmed and empty ones dropped (`--source "mock-lrc,"` is a normal single source).
  - `--author` (`-a`) — author/artist filter.
  - `--album` (`-A`) — album filter.
  - `--iswc` (`-i`) — ISWC identifier.
  - `--output` (`-o`) — write lyrics to this file; if omitted, print to stdout. **Refuses to overwrite an existing file** (exit 7) unless `--overwrite` is given.
  - `--overwrite` (`-O`) — allow `--output` to replace an existing file.
  - `--timestamp` (`-t`) — comma-separated timestamp formats. Defaults to `line,none` (`line` enables LRC lyrics, `none` disables). The user-given order is the priority: `line,none` prefers synced, `none,line` prefers plain; the first match wins. Each value must be `line` or `none` — anything else is a usage error (exit 2). Entries are trimmed and empty ones dropped (`--timestamp ","` yields an empty format list → no match → exit 4).
  - `--lenient` (`-l`) — precheck mode switch: instead of failing fast on the first invalid source (unknown name / missing required parameter), skip it with a `warning[precheck]` and keep trying the remaining sources.
  - `--help` (`-h`) — print help information and exit with code 0. The help text includes the sorted list of registered sources.
  - `--version` (`-v`) — print version and exit with code 0.

**Note on flag forms:** Go's `flag` package accepts both `--flag` and `-flag`. The CLI does **not** intercept this; both work.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (stderr may still carry warnings) |
| 2 | Usage error (missing song, unknown/typo flag, invalid `--timestamp` value, duplicate `--source` entry) |
| 3 | Unknown `--source` name (strict precheck) |
| 4 | No valid result: every source skipped (lenient) or failed, or no result matched the requested timestamp formats |
| 5 | Output failure (file open, truncate, write, or close) |
| 6 | Source-required parameter missing in strict precheck (e.g. `--author` not supplied to `mock-require` or `lyricsovh`) |
| 7 | `--output` file already exists and `--overwrite` was not given |

Warnings are emitted to **stderr** alongside the successful stdout output — they do not change the exit code. On the exit-4 path, the in-flight warnings are printed first so the user sees why each source failed.

## Architecture

Thin CLI layer over a pluggable lyrics-source abstraction, with explicit registry wiring and a unified fetch layer:

1. **Package init (in `main.go`)** — `var registry = mustRegisterAll()` runs `bootstrap.RegisterAll(r)` exactly once before `main()`. Adapter init failure panics (programmer error). Test-tagged builds additionally register the `mock-*` sources via `init()` in `main_loadmock.go`.
2. **Parse arguments** — required positional `<song>`, plus named flags via `flag.NewFlagSet`. Both `--x` and `-x` forms are accepted. Defaults: `--source`=`lrclib`, `--timestamp`=`line,none`. `--timestamp` values are validated at parse time: each comma-separated entry must be `line` or `none`, else exit 2.
3. **Convert flags** — `parsedFlagsToParams` splits comma-separated `--source` and `--timestamp` into `[]string`, trimming whitespace and dropping empty entries (matching the parse-time validation), passes through `--lenient`, and builds a `fetch.Params`.
4. **Precheck** — `fetch.New(registry).Fetch(ctx, params)` walks `params.Source` in order: a repeated name is a `DuplicateSourceError` (exit 2) in strict mode, or a `warning[precheck]` + drop in `--lenient` mode; then `Registry.Get` (unknown name → `UnknownSourceError`, unwraps to `source.ErrNotFound`) and `RequiredParams()` enforcement (missing field → `source.RequiredParamError` built by the fetch layer). Strict mode (default) fails fast on the first problem without fetching anything; `--lenient` skips problem sources with a `warning[precheck]` and keeps the eligible ones.
5. **Two-level fetch loop** — outer loop over `params.Timestamp` (user-given order = priority), inner loop over the eligible sources. For each `(source, format)` pair, a per-call `[]fetch.Result` cache is scanned for `Source == name && Synced == current flag`; a hit returns immediately, otherwise the adapter is called. Adapter errors become `warning[fetch]` and the next source is tried. Results that do not match the current flag are appended to the cache (downgraded plain results are reusable by a later `none` iteration).
6. **Synced vs plain resolution** — a result matches the `line` iteration when the adapter returned `SyncedLyrics` (so `fetch.Result.Synced == true`), else it is a plain result (`Synced == false`). A synced request that yields plain lyrics produces `warning[downgraded]` — the message (`source "..." returned no timestamped lyrics`) reports the source's capability gap, not what the final output uses — and the downgraded result still enters the cache and can satisfy a later `none` iteration.
7. **Emit warnings to stderr** — every `Warning.Message` is pre-formatted by the fetch layer (including the `[kind]` tag); `main.go` prints them verbatim before the result.
8. **Open output (refuse-or-create)** — `openOutput(path, overwrite, stdout)` runs before the fetch (fast fail on unwritable paths and on existing files). A missing target is created exclusively (`O_CREATE|O_EXCL`, no `O_TRUNC`) and reported via a `created` bool; an existing target is only opened when `--overwrite` is set (plain `O_WRONLY`, still no `O_TRUNC`), otherwise an `outputExistsError` maps to exit 7. `Run` has a named return value and a single deferred closer that, on any non-zero exit, removes the file **only if** `created` is true and the path still names the same inode as the open fd (`f.Stat` + `os.Lstat` + `os.SameFile`) — so a failed run never leaves a freshly created empty file behind and never deletes a pre-existing or replaced one.
9. **Write lyrics (two-phase)** — only after a successful fetch is the `--output` file `Truncate(0)` + `Seek(0, 0)` (stdout — the fallback writer — is never truncated or seeked; guard on `parsed.output != ""`), then `io.WriteString` writes `fetch.Result.Lyrics`, and a non-nil `Close` error surfaces as exit 5 (via the defer). Because truncation is deferred, an existing file keeps its content intact on every failure path (exit 3/4/6/7), even with `--overwrite`.

**Key types (in `internal/source/source.go`):**
- `Param uint` — bitmask of `ParamAuthor | ParamAlbum | ParamISWC | ParamTimestamp`.
- `Request{Song, Author, Album, ISWC, Timestamp}` — input to `Source.Fetch`. `Song` is required.
- `Result{Lyrics, SyncedLyrics, Title, Artist, Album, ISWC, Source}` — fetched output. `Lyrics` is populated when plain lyrics are available; a synced-only hit may leave it empty, and the fetch layer then uses `SyncedLyrics` as the final output. `SyncedLyrics` is populated only when timestamped output was requested and the source supports it. `Source` is reserved for aggregate sources to identify their sub-source; standalone adapters leave it empty and the fetch layer fills it in.
- `Source` interface — `Name() string`, `SupportedParams() Param`, `RequiredParams() Param`, `Fetch(ctx, req) (Result, error)`. `RequiredParams` declares which optional fields the adapter **requires**; the fetch layer enforces it during precheck and adapters must NOT raise `RequiredParamError` themselves.
- `Registry` — concurrency-safe name→`Source` map; populated by `bootstrap.RegisterAll`.
- `RequiredParamError{Source, Param, Flag}` — typed error for the missing-required-parameter case; `Unwrap()` returns `ErrRequiredParam`. Constructed by the fetch layer, never by adapters.

**Key types (in `internal/fetch/fetch.go`):**
- `Params{Song, Source, Author, Album, ISWC, Timestamp, Lenient}` — unified input bundle. `Source` and `Timestamp` are `[]string`; the first timestamp match wins, sources fail over in order. `Lenient` switches the precheck stage between fail-fast (false) and skip-with-warning (true).
- `Result{Lyrics, Title, Artist, Album, ISWC, Source, SubSource, Synced}` — single-lyrics output. `Source` is always the adapter's `Name()`. `SubSource` carries the sub-source identifier when the adapter is an aggregate (`source.Result.Source`), else empty. `Synced` is true when `Lyrics` contains LRC content. There is no `Downgraded` field — downgrades are emitted as `warning[downgraded]`.
- `Warning{Kind, Source, Param, Message}` — `Kind` is one of `UnsupportedParam`, `Downgraded`, `PreCheck`, `FetchFailed`; `Message` is pre-formatted with the `[kind]` tag and printed verbatim by `main.go`.
- `NoResultError` — unified "no valid result" error (exit 4): all sources skipped (lenient) or failed, or no result matched any requested timestamp format. In-flight warnings are still returned alongside it.
- `UnknownSourceError{Name}` — unwraps to `source.ErrNotFound`; carries the offending name so `main.go` can print `error[unknown]: source "nope" not found`.
- `DuplicateSourceError{Name}` — a source name listed more than once; `main.go` maps it to exit 2 (`error[usage]: source "<name>" is listed more than once`). In `--lenient` mode the duplicate is instead dropped with a `warning[precheck]`.

### Built-in sources

- **`lrclib`** — Real adapter against `https://lrclib.net`. Picks `/api/get` when `--author` is supplied (structured `track_name`/`artist_name`/`album_name` query), otherwise `/api/search` with freeform `q=`. Supports `ParamAuthor | ParamAlbum | ParamTimestamp` (`album` only acts on the `/api/get` path); honors `SyncedLyrics` from the response when `--timestamp` was requested. 10-second per-request timeout.
- **`lyricsovh`** — Real adapter against `https://api.lyrics.ovh/v1/{artist}/{title}`. Supports `ParamAuthor` and **requires** it (the path cannot be built without an artist). Surfaces the API's 404 as a not-found error rather than a generic HTTP-status failure. 10-second per-request timeout.
- **`lrccx`** — Real adapter against the legacy `https://api.lrc.cx/jsonapi` endpoint. GETs `title`/`artist`/`album` query params (`artist` optional; `album` value `[Unknown Album]` treated as empty; the API's `path` param is never sent) and picks the first hit whose `lrc` field is non-empty from the score-descending JSON array. Supports `ParamAuthor | ParamAlbum | ParamTimestamp`. The response is always LRC-flavoured text: plain `Lyrics` are produced by stripping `[mm:ss]` timestamps and section/marker tags (`[Verse]`, `[!text]`, ...), while `SyncedLyrics` carries the raw text only when `--timestamp` was requested AND the text actually contains timestamped lines — unsynced `[!text]` entries fall back to plain lyrics. The `lrc` field may be `null` (instrumental entries). 10-second per-request timeout.

Adding a new built-in source means: create `internal/source/real/<name>/` implementing `source.Source`, then add an import and `r.Register(<name>.New())` line to `internal/bootstrap/bootstrap.go`. No CLI-layer changes are required.

### Mock/Test-Only Sources

Registered only during tests via `bootstrap.RegisterAllMock` (never in production). Each mock covers one dedicated testing concern:

- **`mock-success`** — Happy-path adapter. Advertises `ParamAuthor` and **requires** it (`RequiredParams()`): a fetch without `--author` is rejected by the precheck; with it, returns deterministic lyrics. Drives stdout/file output, single-dash long-form flags, and unsupported `--album`/`--iswc` warnings.
- **`mock-require`** — Exercises the `RequiredParamError` path in isolation. Advertises `Param(0)` (no optional params) but **requires** `--author` via `RequiredParams()`: a fetch without it trips the precheck, with it succeeds. Drives exit code 6.
- **`mock-nosupport`** — Advertises `Param(0)` and requires nothing; fetch always succeeds. Proves the no-`--author` path and that every optional flag trips an unsupported-parameter warning.
- **`mock-fail`** — Advertises `ParamAuthor` but always returns a fetch error. Drives the exit code 4 (no-result) path at the CLI level via `warning[fetch]`.
- **`mock-lrc`** — Advertises `ParamTimestamp` and requires nothing; with `--timestamp` it returns LRC-style `SyncedLyrics` (plain `Lyrics` otherwise). Drives the `Synced` output path at the CLI level.
- **`mock-nosync`** — Advertises `ParamTimestamp` but never returns `SyncedLyrics`: a `--timestamp line` request succeeds with plain `Lyrics` only. Drives the `warning[downgraded]` fallback and the "no match for the requested flag" no-result path.

All mock/test-only source names must start with the `mock-` prefix (e.g. `mock-success`, `mock-fail`). This distinguishes them from real sources in the registry at a glance.

## Testing

**Test conventions:**
- Tests live alongside the code as `*_test.go` files.
- `main_test.go` drives the public `Run(argv, stdout, stderr) int` entry point with `bytes.Buffer` writers so exit codes, stdout, and stderr are all asserted independently. The one exception is `TestRun_RealFileStdout`, which passes a real `os.Pipe` write end as stdout to prove the truncation/seek path is guarded — see the regression note below. Mock/test-only sources are registered via `init()` in `main_loadmock.go` (build tag `test`).
- Mock/test-only source files (under `internal/source/mock/`) carry `//go:build test` and are only compiled during `go test`.
- `bootstrap/bootstrap_mock.go` also carries `//go:build test`, so mock sources are never linked into production binaries.
- **Real sources are NOT covered by automated tests.** `lrclib` and `lyricsovh` have no `*_test.go` files (0% statement coverage) and are exercised only manually against their live endpoints. Only the mock adapters and the CLI/fetch layers are under automated test.
- `internal/source/source_test.go` covers the `Registry` itself (register/lookup/duplicate/unregister).

**Coverage in `main_test.go` includes:**
- Missing song → exit 2 with the required-song stderr message.
- `--help` and `-h` → exit 0; stdout lists `mock-success`, `lrclib`, `lyricsovh`, `lrccx`, and the `--lenient`/`--overwrite` flags.
- `--version` and `-v` → exit 0; stdout contains the version string.
- Default stdout sink and `--output` file sink paths.
- `TestRun_RealFileStdout` regression: stdout as a real `*os.File` (an `os.Pipe` write end) must not be truncated or seeked — a prior fix truncated `os.Stdout` unconditionally and broke every stdout-mode run with `error[output]: truncate ...: invalid argument` (exit 5); `bytes.Buffer` writers masked it because the `*os.File` type assertion never matched.
- Default `line,none` on a plain-only source (`mock-success`) → exit 0 with plain lyrics and `warning[downgraded]`.
- Unsupported `--album` / `--iswc` warnings on `mock-success` (which advertises only `ParamAuthor`).
- Unknown source name → exit 3 with `error[unknown]: source "nope" not found`.
- Single-dash long-form flags (`-source`, `-author`) are accepted.
- Explicit `--timestamp line` on `mock-success` / `mock-nosync` → exit 4 with `warning[downgraded]` + `error[no-result]` (no `none` iteration to fall back on).
- `mock-require` without `--author` → exit 6 with the canonical `error[required]: source "mock-require" requires --author` message.
- Strict fail-fast: `--source mock-nosupport,mock-require` without `--author` → exit 6, nothing fetched.
- `--lenient` skips `mock-require` (missing `--author`) and succeeds via `mock-nosupport` with `warning[precheck]`.
- `--lenient` with every source skipped → exit 4 with `warning[precheck]` + `error[no-result]`.
- Fetch failure via `mock-fail` → exit 4; stderr shows `warning[fetch]` before `error[no-result]`.
- `mock-require` with `--author` → exit 0 (the required-param mock succeeds when the flag is supplied).
- `mock-nosupport` without `--author` → exit 0 (a source can succeed without any optional params).
- Unknown/typo flag (e.g. `--bogus`) → exit 2 with `error[usage]` and the flag package's parse error on stderr.
- Invalid `--timestamp` value (e.g. `karaoke`) → exit 2 with `error[usage]` and the invalid-value message.
- `--timestamp line` on `mock-lrc` → exit 0 with LRC timestamped lyrics on stdout.
- `--timestamp none,line` on `mock-lrc` → exit 0 with plain lyrics (user-given order is the priority).
- `--source` / `--timestamp` entries are trimmed and empty ones dropped: `--source "mock-lrc,"` → exit 0; `--timestamp " line"` on `mock-lrc` → exit 0 with synced lyrics; `--timestamp ","` → exit 4 with `error[no-result]`.
- Duplicate `--source` entry (e.g. `mock-lrc,mock-lrc`) → exit 2 with `error[usage]: source "mock-lrc" is listed more than once`; with `--lenient` the duplicate is skipped via `warning[precheck]` and the run succeeds.
- Whitespace-only `--author` (e.g. `--author " "`) is treated as not provided: no unsupported-parameter warning on `mock-nosupport`.
- Existing `--output` file without `--overwrite` → exit 7 with `error[output]: file ... already exists (use --overwrite to replace it)`, file untouched, stderr hints at `--overwrite`.
- `--overwrite` (`-O`) with an existing file → exit 0, file truncated and replaced (no stale tail) on success; on a fetch failure (exit 3/4/6) the original content is preserved — truncation still happens only after a successful fetch.
- **Failed runs never create the `--output` file** (exit 3 / exit 4 via `mock-fail` / exit 6 via `mock-require`): `TestRun_FailedFetchDoesNotCreateOutputFile` asserts the path does not exist afterwards — regression for the "empty file left behind" defect.

**CI Pipeline:**
- The only workflow is `.github/workflows/simple_ci_cd.yml` ("Simple CI/CD"). It is **release-only**: it triggers on `v*` tag pushes; there is no CI on ordinary pushes to `main` or on pull requests.
- On a release tag it: checks out the repo, sets up Go 1.25 (`actions/setup-go@v5` with cache), runs `go test -tags test ./...` and `go vet -tags test ./...`, cross-compiles release binaries for `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`, `windows/arm64` (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=<tag>"`), writes `checksums.txt` via `sha256sum`, then publishes assets with `softprops/action-gh-release@v3` (auto-generated release notes). The workflow declares `permissions: contents: write`.

## Code Style & Conventions

**Naming:**
- Standard Go: `CamelCase` for exported, `camelCase` for unexported.
- CLI flag names match the spec: `source`, `author`, `album`, `iswc`, `output`, `overwrite`, `timestamp`, `help`, `version`.
- Source adapter `Name()` returns a stable lowercase identifier (e.g. `"lrclib"`, `"lyricsovh"`).

**Imports / Modules:**
- Module path: `github.com/PloyBox/get-lyrics`.
- New source adapters live under `internal/source/<name>/` and are wired in `internal/bootstrap/bootstrap.go` only.

**Error handling:**
- Operational warnings (e.g. a source not honoring a requested parameter) go to **stderr**, never stdout.
- Hard errors exit non-zero via the documented exit codes; messages are written to **stderr**.
- `errors.Is(err, source.ErrNotFound)`, `errors.As(err, &source.RequiredParamError{})`, and `errors.As(err, &fetch.NoResultError{})` are the supported ways to branch on registry/required-param/no-result errors in the CLI layer. `UnknownSourceError` unwraps to `ErrNotFound` and carries the name.

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
- Source adapters that **require** a parameter must declare it via `RequiredParams()` (not by raising errors inside `Fetch`); the fetch layer emits exit code 6 with a stable, greppable message.
- Adapters must respect `ctx` for cancellation/deadlines and should not panic on missing `Song`.
- Standalone adapters must leave `source.Result.Source` empty; only aggregate sources set it to their sub-source identifier, and the fetch layer copies it to `Result.SubSource` while keeping `Result.Source` set to the adapter's `Name()`.

**Verification Checklist:**
- `go build ./...` succeeds
- `go test -tags test ./...` passes
- `go vet -tags test ./...` is clean
- `gofmt -l .` produces no output (or `gofmt -d .` shows no diffs)
- Missing song title produces exit code 2 with the required-song stderr message
- `--help` / `-h` prints usage and the sorted list of registered sources, exit 0
- `--version` prints version string and exits 0
- Unknown `--source` produces exit code 3 with `error[unknown]: source "<name>" not found`
- Source requiring a missing parameter produces exit code 6 with `error[required]: source "<name>" requires --<flag>`
- Duplicate `--source` entry produces exit code 2 with `error[usage]: source "<name>" is listed more than once`; with `--lenient` it is skipped via `warning[precheck]` and the run succeeds
- All-sources-failed / all-skipped / no-flag-match produces exit code 4 with `error[no-result]` preceded by the in-flight warnings
- Invalid `--timestamp` value produces exit code 2
- Unsupported `--album`/`--iswc` produces a stderr warning for the relevant source
- Omitted `--output` prints lyrics to stdout; provided `--output` writes the file
- Existing `--output` file without `--overwrite` produces exit code 7 with `error[output]: file ... already exists`, leaving the file untouched
- With `--overwrite`, an existing `--output` file keeps its content on fetch failures (exit 3/4/6) and is truncated and replaced only on success
- A failed run (exit 3/4/6) leaves no freshly created `--output` file behind
- Unwritable `--output` path produces exit code 5

## Pointers

- `README.md` — full usage guide: install (prebuilt binaries from Releases + `go install`), usage examples with flags, exit-code table, built-in sources, and how to add a new source
- `CONTRIBUTING.md` — contribution guidelines (not yet written)
- `docs/` — additional design or source-integration documentation (not yet written)
