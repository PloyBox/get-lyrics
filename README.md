# get-lyrics

Designed to be **Lightweight** x **Usable** x **Composable**.

A binary under 10MB, an easy-to-use CLI, and clear, meaningful exit codes -- all at once.

Fetch song lyrics from the command line. Supports multiple backend sources with a pluggable adapter system.

## Install

### Option 1: Download a prebuilt binary from Releases (Recommended)

Prebuilt binaries are attached to each [Release](https://github.com/PloyBox/get-lyrics/releases) — no Go toolchain needed.

### Option 2: Build from source with `go install`

```sh
go install github.com/PloyBox/get-lyrics/cli/get-lyrics@latest
```

Requires Go 1.25.10+.

### Option 3: Build from source with a downgraded Go version

If your Go toolchain is older than the version pinned in `go.mod`, you can lower the `go` directive and compile from source:

```sh
go mod edit -go=1.22
go install ./cli/get-lyrics
```

No compatibility is guaranteed with older Go toolchains — the code is developed against the version pinned in `go.mod`, and downgrading may fail to build or misbehave at runtime.

## Usage

**Basic:**

```sh
# Search lrclib for "Bohemian Rhapsody" (default source)
get-lyrics "Bohemian Rhapsody"

# Narrow by artist
get-lyrics --author "Queen" "Bohemian Rhapsody"

# Use lyrics.ovh (requires --author)
get-lyrics --source "lyricsovh" --author "Queen" "Bohemian Rhapsody"

# Try several sources in order (failover)
get-lyrics --source "lrclib,lyricsovh" --author "Queen" "Bohemian Rhapsody"

# Skip sources that can't be used instead of failing (precheck only)
get-lyrics --lenient --source "lyricsovh,lrclib" "Bohemian Rhapsody"
```

**Output to file:**

```sh
# Write to a new file (refuses to overwrite an existing one)
get-lyrics --author "Queen" --output "lyrics.txt" "Bohemian Rhapsody"

# Explicitly overwrite an existing file
get-lyrics --author "Queen" --output "lyrics.txt" --overwrite "Bohemian Rhapsody"
```

**Force sync level:**

```sh
# Force LRC output
get-lyrics --author "Queen" --sync-level "line" "Bohemian Rhapsody"

# Force plain lyrics instead
get-lyrics --author "Queen" --sync-level "none" "Bohemian Rhapsody"
```

**Set a custom User-Agent:**

```sh
# Override the HTTP User-Agent sent to sources (useful for attribution)
get-lyrics --user-agent "my-app/1.0 (contact@example.com)" --author "Queen" "Bohemian Rhapsody"
```

**Custom source parameters (`--env`):**

Sources can declare custom input keys (see "Source Parameters" below). Pass them with a repeatable `--env key=value` flag; a missing key falls back to the process environment:

```sh
# Pass a key directly (repeatable; -e is the short form)
get-lyrics --source mock-custom --env LANG=en --env COUNTRY=cn "TEST_SONG"

# The same keys can come from the environment when not passed via --env
LANG=en COUNTRY=cn get-lyrics --source mock-custom "TEST_SONG"

# musixmatch requires its API key as a custom parameter (a free Basic key
# works for plain lyrics; see "Built-in Sources" below)
get-lyrics --source musixmatch --env MUSIXMATCH_API_KEY=xxx --author "Queen" "Bohemian Rhapsody"
```

**List available sources:**

```sh
get-lyrics --help
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--source` | `-s` | Comma-separated lyrics source names, tried in order (default: `lrclib`) |
| `--author` | `-a` | Artist / author filter |
| `--album` | `-A` | Album filter |
| `--iswc` | `-i` | ISWC identifier |
| `--output` | `-o` | Write lyrics to file (default: stdout; refuses to overwrite an existing file) |
| `--overwrite` | `-O` | Overwrite an existing `--output` file |
| `--sync-level` | `-S` | Comma-separated sync levels (default: `line,none`; `line` enables LRC). User-given order is the priority |
| `--user-agent` | `-u` | HTTP `User-Agent` header sent to sources (default: `get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics)`) |
| `--env` | `-e` | Custom source parameter `key=value` (repeatable; key must match `^[A-Z][A-Z0-9_]*$`) |
| `--lenient` | `-l` | Skip invalid sources instead of failing fast (precheck only) |
| `--help` | `-h` | Show help and exit |
| `--version` | `-v` | Print version and exit |

- Both `--flag` and `-flag` forms are accepted.
- Quotes are optional for values without spaces.
- Everything after the first positional (flags included) is parsed as the song title.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may be on stderr) |
| 2 | Usage error (missing song, unknown flag, invalid `--sync-level` value, invalid/duplicate `--env` entry) |
| 3 | Unknown `--source` name |
| 4 | No valid result (all sources failed/skipped, or nothing matched the requested sync level) |
| 5 | Output failure (can't create/write file) |
| 6 | Source requires a parameter (e.g. `--author` missing for `lyricsovh`, or a required `--env` key missing) |
| 7 | `--output` file already exists and `--overwrite` was not given |
| 8 | Duplicate `--source` entry |

## Source Parameters

Sources may declare custom input keys beyond the built-in filters, passed via the repeatable `--env key=value` flag (short: `-e`). `--help` lists each source's declared keys under "Source parameters:".

Rules:

- **Key syntax**: env-style upper snake case, `^[A-Z][A-Z0-9_]*$` (e.g. `LANG`, `HTTP_PROXY`). Keys are matched exactly and case-sensitively; lowercase/mixed-case or otherwise invalid keys are rejected at parse time (exit 2).
- **Value**: must be non-empty after trimming; a whitespace-only value is a usage error (exit 2). Duplicate keys are rejected (exit 2).
- **Environment fallback**: for every key a requested source declares, a key you did not pass via `--env` is filled from the process environment (`os.LookupEnv`) when it exists and is non-empty. Precedence: `--env` > environment > missing. An environment variable that exists but is empty (e.g. `LANG=`) counts as missing.
- **Injected keys behave like user-passed ones**: a source that does not declare a key emits `warning[unsupported]` for it either way — the fallback only fills values, it never changes what a source recognizes.
- **Required keys**: a source may require a key for a given request (conditionally, based on other inputs). A missing required key is exit 6 in strict mode; under `--lenient` the source is skipped with a `warning[precheck]`.
- **Unrecognized keys**: never hard-fail; each produces one `warning[unsupported]` per source (order between multiple keys is unspecified).
- **Multiple sources sharing a key** all consume the same value.

## Built-in Sources

Legend: `Y` = supported, `N` = not supported, `F` = required (must be given).

| Source | Author | Album | ISWC | Sync level | Notes |
|--------|--------|-------|------|-----------|-------|
| `lrclib` | Y | Y | N | Y | Searches [lrclib.net](https://lrclib.net); uses `/api/get` when artist is given, `/api/search` otherwise |
| `lyricsovh` | F | N | N | N | Uses [api.lyrics.ovh](https://api.lyrics.ovh); plain text only |
| `lrccx` | Y | Y | N | Y | Searches [lrc.cx](https://lrc.cx) via its legacy `/jsonapi` endpoint; the response is always LRC-flavoured text, stripped of timestamps for plain output |
| `musixmatch` | Y | N | N | Y* | [Musixmatch](https://developer.musixmatch.com) via `matcher.lyrics.get`/`matcher.subtitle.get` (with `--author`) or `track.search` → `track.lyrics.get`/`track.subtitle.get` (title only). Requires the `MUSIXMATCH_API_KEY` custom parameter. Synced LRC needs the paid Scale plan; on cheaper plans a `--sync-level` request falls back to plain lyrics with a `warning[downgraded]` |

`musixmatch` is the only built-in source that declares a custom `--env` parameter: `MUSIXMATCH_API_KEY` (required, fallback to the `MUSIXMATCH_API_KEY` environment variable). Get a free Basic key at <https://developer.musixmatch.com>.

## Add a New Source

Backends are pluggable via the `source.Source` interface. Use an existing adapter as a template, such as `internal/provider/real/lrclib/` — it covers the full surface (filters, required params, plain + synced output):

1. Create `internal/provider/real/<name>/` implementing `source.Source` (`Name` / `Capabilities` / `Fetch` / `CustomParams`), modeled on the template.
2. Modify it to fit your needs — endpoint, filters, required params, output behavior.
3. Add an import and `r.Register(<name>.New())` in `internal/bootstrap/bootstrap.go`.

To declare custom input keys:

- Return them from `CustomParams()` (the static, request-independent list, e.g. `[]source.ParamSpec{{Name: "LANG", Description: "language hint"}}`). This backs the `--help` "Source parameters:" section and the environment-variable fallback.
- List the request's recognized keys in `Capabilities(req).Custom` and the request's required keys in `Capabilities(req).RequiredCustom`. Both may be conditional on `req` (a key required only when another key/field is present). `RequiredCustom` must be a subset of the request's `Custom` names.
- Read the values from `req.Custom` inside `Fetch`.
- Keys are env-style `^[A-Z][A-Z0-9_]*$`; an invalid or duplicate static declaration fails registration at startup (a source bug), and an inconsistent dynamic declaration is flagged at precheck with `warning[precheck-mismatch]` and the source is skipped.

`--help` automatically lists the new source and its parameters.

## License

Apache 2.0
