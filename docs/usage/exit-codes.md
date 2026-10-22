# Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (warnings may be on stderr) |
| 2 | Usage error (missing `--name`, any positional argument, unknown flag, a non-`--env` flag given more than once, invalid or empty `--sync-level` list, empty `--source` list, invalid `--duration`/`--timeout`/`--timeout-global` value, invalid/duplicate `--env` entry) |
| 3 | Unknown `--source` name |
| 4 | No valid result (all sources failed/skipped, or nothing matched the requested sync level) |
| 5 | Output failure (can't create, write, sync, close, or rename the output file) |
| 6 | Source requires a parameter (e.g. `--author` missing for `lyricsovh`, or a required `--env` key missing) |
| 7 | `--output` file already exists and `--overwrite` was not given |
| 8 | Duplicate `--source` entry |
| 9 | Overall fetch timeout (`--timeout-global`) exceeded, or the fetch was cancelled (e.g. Ctrl-C) |

`--help`/`--version` are dispatched before validation and exit 0 even when another flag's value is invalid or a flag is repeated (with both, `--help` wins). A flag-syntax error met before them (e.g. an unknown flag) still exits 2.