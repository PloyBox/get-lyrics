# Exit Codes

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