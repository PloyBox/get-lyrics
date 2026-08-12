# get-lyrics

Fetch song lyrics from the command line. Supports multiple backend sources with a pluggable adapter system.

## Install

### Option 1: Download a prebuilt binary from Releases (Recommended)

Prebuilt binaries are attached to each [Release](https://github.com/PloyBox/get-lyrics/releases) — no Go toolchain needed.

### Option 2: Build from source with `go install`

```sh
go install github.com/PloyBox/get-lyrics@latest
```

Requires Go 1.25.10+.

### Option 3: Build from source with a downgraded Go version

If your Go toolchain is older than the version pinned in `go.mod`, you can lower the `go` directive and compile from source:

```sh
go mod edit -go=1.22
go install .
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
get-lyrics --source lyricsovh --author "Queen" "Bohemian Rhapsody"
```

**Output to file:**

```sh
get-lyrics --author "Queen" --output lyrics.txt "Bohemian Rhapsody"
```

**Timestamped (LRC) lyrics:**

```sh
get-lyrics --author "Queen" --timestamp line "Bohemian Rhapsody"
```

**List available sources:**

```sh
get-lyrics --help
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--source` | `-s` | Comma-separated lyrics source names (default: `lrclib`) |
| `--author` | `-a` | Artist / author filter |
| `--album` | `-A` | Album filter |
| `--iswc` | `-i` | ISWC identifier |
| `--output` | `-o` | Write lyrics to file (default: stdout) |
| `--timestamp` | `-t` | Comma-separated timestamp formats (default: `line,none`; `line` enables LRC) |
| `--help` | `-h` | Show help and exit |
| `--version` | `-v` | Print version and exit |

Both `--flag` and `-flag` forms are accepted.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may be on stderr) |
| 2 | Usage error (missing song, unknown flag) |
| 3 | Unknown `--source` name |
| 4 | Fetch failure (network, no lyrics found) |
| 5 | Output failure (can't create/write file) |
| 6 | Source requires a parameter (e.g. `--author` missing for `lyricsovh`) |

## Built-in Sources

- **`lrclib`** — Searches [lrclib.net](https://lrclib.net). Supports `--author`, `--album` and `--timestamp`. Uses `/api/get` when artist is given, `/api/search` otherwise.
- **`lyricsovh`** — Uses [api.lyrics.ovh](https://api.lyrics.ovh). Requires `--author`. Plain text only.
- **`lrccx`** — Searches [lrc.cx](https://lrc.cx) via its legacy `/jsonapi` endpoint. Supports `--author`, `--album` and `--timestamp`; the response is always LRC-flavoured text, stripped of timestamps for plain output.

## Add New Source (Fork)

1. Create `internal/source/real/<name>/` implementing `source.Source`.
2. Add an import and `r.Register(<name>.New())` to `internal/bootstrap/bootstrap.go`.

No changes needed in the CLI layer — `--help` automatically lists the new source.

## License

Apache 2.0
