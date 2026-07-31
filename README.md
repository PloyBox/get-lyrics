# get-lyrics

Fetch song lyrics from the command line. Supports multiple backend sources with a pluggable adapter system.

## Install

```sh
go install github.com/PloyBox/get-lyrics@latest
```

Requires Go 1.25+.

## Usage

**Basic:**

```sh
# Search lrclib for "Bohemian Rhapsody" (no artist needed)
get-lyrics --source lrclib "Bohemian Rhapsody"

# Narrow by artist
get-lyrics --source lrclib --author "Queen" "Bohemian Rhapsody"

# Use lyrics.ovh (requires --author)
get-lyrics --source lyricsovh --author "Queen" "Bohemian Rhapsody"
```

**Output to file:**

```sh
get-lyrics --source lrclib --author "Queen" --output lyrics.txt "Bohemian Rhapsody"
```

**Timestamped (LRC) lyrics:**

```sh
get-lyrics --source lrclib --author "Queen" --timestamp "Bohemian Rhapsody"
```

**List available sources:**

```sh
get-lyrics --help
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--source` | `-s` | Lyrics source name (required) |
| `--author` | `-a` | Artist / author filter |
| `--album` | `-A` | Album filter |
| `--iswc` | `-i` | ISWC identifier |
| `--output` | `-o` | Write lyrics to file (default: stdout) |
| `--timestamp` | `-t` | Request LRC timestamped lyrics |
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

- **`lrclib`** — Searches [lrclib.net](https://lrclib.net). Supports `--author` and `--timestamp`. Uses `/api/get` when artist is given, `/api/search` otherwise.
- **`lyricsovh`** — Uses [api.lyrics.ovh](https://api.lyrics.ovh). Requires `--author`. Plain text only.
- **`stub`** — Offline deterministic adapter for testing. Returns placeholder lyrics. Requires `--author`.

## Adding a New Source

1. Create `internal/source/<name>/` implementing `source.Source`.
2. Add an import and `r.Register(<name>.New())` to `internal/bootstrap/bootstrap.go`.

No changes needed in the CLI layer — `--help` automatically lists the new source.

## License

Apache 2.0