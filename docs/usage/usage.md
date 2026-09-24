# Usage

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

Sources can declare custom input keys (see [Source Parameters](source-parameters.md)). Pass them with a repeatable `--env key=value` flag; a missing key falls back to the process environment:

```sh
# Pass a key directly (repeatable; -e is the short form)
get-lyrics --source mock-custom --env LANG=en --env COUNTRY=cn "TEST_SONG"

# The same keys can come from the environment when not passed via --env
LANG=en COUNTRY=cn get-lyrics --source mock-custom "TEST_SONG"

# musixmatch requires its API key as a custom parameter (a free Basic key
# works for plain lyrics; see "Built-in Sources")
get-lyrics --source musixmatch --env MUSIXMATCH_API_KEY=xxx --author "Queen" "Bohemian Rhapsody"
```

**List available sources:**

```sh
get-lyrics --help
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--source` | `-s` | Comma-separated lyrics source names, tried in order (default: `lrclib`) |
| `--author` | `-a` | Artist / author filter |
| `--album` | `-A` | Album filter |
| `--isrc` | `-i` | ISRC identifier |
| `--duration` | `-d` | Track duration filter (seconds or mm:ss) |
| `--output` | `-o` | Write lyrics to file (default: stdout; refuses to overwrite an existing file) |
| `--overwrite` | `-O` | Overwrite an existing `--output` file |
| `--json` | `-j` | Write the complete fetch result as JSON, including empty-string fields; `formatVersion` is fixed at `1` |
| `--sync-level` | `-S` | Comma-separated sync levels (default: `line,none`; `line` enables LRC, `word` syllable-level TTML). User-given order is the priority |
| `--user-agent` | `-u` | HTTP `User-Agent` header sent to sources (default: `get-lyrics/<ver> (+https://github.com/PloyBox/get-lyrics)`) |
| `--env` | `-e` | Custom source parameter `key=value` (repeatable; key must match `^[A-Z][A-Z0-9_]*$`) |
| `--lenient` | `-l` | Skip invalid sources instead of failing fast (precheck only) |
| `--quiet` | `-q` | Suppress all stderr output (warnings and errors; default: off) |
| `--help` | `-h` | Show help and exit |
| `--version` | `-v` | Print version and exit |

- Both `--flag` and `-flag` forms are accepted.
- Quotes are optional for values without spaces.
- Everything after the first positional (flags included) is parsed as the song title.

See [Exit Codes](exit-codes.md) for what each failure returns.