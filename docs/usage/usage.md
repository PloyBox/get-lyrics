# Usage

The song title is mandatory and is passed with `--name`/`-n`. Positional
arguments are not accepted.

**Basic:**

```sh
# Search lrclib for "Bohemian Rhapsody" (default source)
get-lyrics --name "Bohemian Rhapsody"

# Narrow by artist
get-lyrics --name "Bohemian Rhapsody" --author "Queen"

# Use lyrics.ovh (requires --author)
get-lyrics --name "Bohemian Rhapsody" --source "lyricsovh" --author "Queen"

# Try several sources in order (failover)
get-lyrics --name "Bohemian Rhapsody" --source "lrclib,lyricsovh" --author "Queen"

# Skip sources that can't be used instead of failing (precheck only)
get-lyrics --name "Bohemian Rhapsody" --lenient --source "lyricsovh,lrclib"
```

**Output to file:**

```sh
# Write to a new file (refuses to overwrite an existing one)
get-lyrics --name "Bohemian Rhapsody" --author "Queen" --output "lyrics.txt"

# Explicitly overwrite an existing file
get-lyrics --name "Bohemian Rhapsody" --author "Queen" --output "lyrics.txt" --overwrite
```

File output is written to a temp file and renamed into place only after a complete write:
a failed run leaves an existing file untouched, and a new file appears only on success.

**Force sync level:**

```sh
# Force LRC output
get-lyrics --name "Bohemian Rhapsody" --author "Queen" --sync-level "line"

# Force plain lyrics instead
get-lyrics --name "Bohemian Rhapsody" --author "Queen" --sync-level "none"
```

**Set a custom User-Agent:**

```sh
# Override the HTTP User-Agent sent to sources (useful for attribution)
get-lyrics --name "Bohemian Rhapsody" --user-agent "my-app/1.0 (contact@example.com)" --author "Queen"
```

**Custom source parameters (`--env`):**

Sources can declare custom input keys (see [Source Parameters](source-parameters.md)). Pass them with a repeatable `--env key=value` flag; a missing key falls back to the process environment:

```sh
# musixmatch requires its API key as a custom parameter (a free Basic key
# works for plain lyrics; see "Built-in Sources")
get-lyrics --name "Bohemian Rhapsody" --source musixmatch --env MUSIXMATCH_API_KEY=xxx --author "Queen"

# The same keys can come from the environment when not passed via --env
MUSIXMATCH_API_KEY=xxx get-lyrics --name "Bohemian Rhapsody" --source musixmatch --author "Queen"
```

**List available sources:**

```sh
get-lyrics --help
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--name` | `-n` | Song title (required; surrounding whitespace is trimmed) |
| `--source` | `-s` | Comma-separated lyrics source names, tried in order (default: `lrclib`; at least one name required) |
| `--author` | `-a` | Artist / author filter |
| `--album` | `-A` | Album filter |
| `--isrc` | `-i` | ISRC identifier |
| `--duration` | `-d` | Track duration filter (seconds or mm:ss) |
| `--timeout` | `-t` | Per-source request timeout in seconds (default: `10`; `0` = no timeout) |
| `--timeout-global` | `-T` | Overall fetch timeout in seconds (default: off) |
| `--output` | `-o` | Write lyrics to file (default: stdout; refuses to overwrite an existing file) |
| `--overwrite` | `-O` | Overwrite an existing `--output` file |
| `--json` | `-j` | Write the complete fetch result as JSON, including empty-string fields; keys are UpperCamelCase and `FormatVersion` is `3`, incrementing when the JSON shape or parameter semantics change |
| `--sync-level` | `-S` | Comma-separated sync levels (default: `line,none`; `line` enables LRC, `word` syllable-level TTML; at least one level required). User-given order is the priority |
| `--user-agent` | `-u` | HTTP `User-Agent` header sent to sources (default: `get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics)`) |
| `--env` | `-e` | Custom source parameter `key=value` (repeatable; key must match `^[A-Z][A-Z0-9_]*$`) |
| `--lenient` | `-l` | Skip invalid sources instead of failing fast (precheck only) |
| `--quiet` | `-q` | Suppress all stderr output (warnings and errors; default: off) |
| `--help` | `-h` | Show help and exit |
| `--version` | `-v` | Print version and exit |

- The song title is mandatory: pass it with `--name`/`-n`; surrounding whitespace is trimmed, so a whitespace-only value counts as missing. Positional arguments are rejected (exit 2).
- Empty `--source`/`--sync-level` values (including comma-only lists) are usage errors (exit 2).
- Every flag except `--env`/`-e` may be given only once; a repeated flag is a usage error (exit 2).
- `--help`/`--version` are honored before value validation: they exit 0 even when another flag's value is invalid or a flag is repeated. If both are given, `--help` wins.
- `--json` keys are UpperCamelCase (`FormatVersion`, `Level`, `Lyrics`, `Title`, `Artist`, `Album`, `ISRC`, `Source`, `SubSource`).
- Ctrl-C cancels an in-flight fetch (exit 9) and removes any temp output file.
- Both `--flag` and `-flag` forms are accepted.
- Quotes are optional for values without spaces.

See [Exit Codes](exit-codes.md) for what each failure returns.
