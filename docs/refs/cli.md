# cli (cli/get-lyrics)

`get-lyrics` fetches song lyrics from a registered source.

```
Usage: get-lyrics [--source <names>] [--author <name>] [--album <name>]
                   [--isrc <code>] [--duration <secs>] [--output <file>]
                   [--user-agent <ua>] [--sync-level <levels>] [--json]
                   [--quiet] [--version] <song>
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | success (stderr may still carry warnings) |
| 2 | usage error (missing song, unknown/typo flag, invalid `--sync-level` value, invalid `--duration`, invalid/duplicate `--env`) |
| 3 | unknown source (strict precheck) |
| 4 | no valid result: every source skipped (lenient) or failed, or no result matched the requested sync levels |
| 5 | output failure (file open, write, or close) |
| 6 | source-required parameter missing in strict precheck |
| 7 | `--output` points to an existing file and `--overwrite` was not given |
| 8 | duplicate `--source` entry (strict precheck) |

## run.go

- `version` is stamped at release build time via `-ldflags "-X main.version=<tag>"`; `dev` is
  the local-build default.
- `defaultUserAgent()` returns the UA the CLI sends on every upstream request unless the
  caller overrides it with `--user-agent`: `get-lyrics/<version>
  (+https://github.com/PloyBox/get-lyrics)`. The version is injected from the stamped build
  version — it replaces the UA the built-in sources previously hardcoded; the sources now
  trust whatever they are handed.
- `registry` is populated at package-init time so `RegisterAll` runs before `main()`.
  `mustRegisterAll` panics on registration failure: an adapter init failure is a programmer
  error, so it bails out before any CLI handling runs.
- `Run(argv, stdout, stderr) (code int)` is the testable core — `argv` excludes the program
  name, and stdout/stderr are explicit writers. `main` calls it with `os.Args[1:]`.

Run flow:

1. Parse flags; when `--quiet` is set, stderr is redirected to `io.Discard` before anything
   is printed, silencing every later line (warnings and errors alike) while exit codes stay
   meaningful — honored even when parsing itself fails after the flag. On parse error →
   `error[usage]` + usage, exit 2.
2. `--help` → full declaration for rendering only, no env fallback (lenient mode and the
   sorted registry names make this query infallible); print usage, exit 0.
3. `--version` → print `get-lyrics <version>`, exit 0.
4. Empty song → `error[usage]: song title is required` + usage, exit 2.
5. `svc.CustomParamsFor(params)` for the requested sources — strict mode reports exit 3/8
   here (same codes as the fetch precheck, but reported first) — then `mergeEnv` fills every
   key not supplied by flag from the process environment.
6. Open the output sink before the fetch.
7. `svc.Fetch(...)`: on every failure path the in-flight warnings are printed before the
   `error[...]` line, then the error is mapped to its exit code.
8. On success: print the warnings, truncate + seek the output file, then write the plain
   lyrics or the complete `fetch.Result` as JSON.

Output-file cleanup (`defer`): only files this process created via `O_EXCL` are ever
removed; before removing, the path's current inode is compared with the open fd so a file
that replaced ours is never deleted. A failed run must not leave a freshly created empty file
behind; pre-existing files are never touched here.

Truncate/seek happen only on the real output file — stdout (the fallback) must never be
truncated or seeked. Because truncation happens only after a successful fetch, existing files
keep their content on every failure path (exit 3/4/6/7/8).

JSON output marshals `{formatVersion, fetch.Result}` with `formatVersion` 1; it includes every
result field, including empty strings. Increment `formatVersion` whenever the JSON structure
or parameter semantics change.

## flags.go

- `parsedFlags` holds the parsed inputs; `song` is kept separate because it is a positional
  argument, not a flag.
- `parseFlags(argv)` handles both `-x`/`--x` forms using Go `flag`'s default behavior:
  parsing stops at the first positional argument, so flags must precede the song. Unknown
  flags become a non-nil error which `Run` maps to `exitUsage`. Flag's own usage writer is
  silenced (`io.Discard`); `Run` writes its own on error. On error the flags parsed so far
  are still returned, so `Run` honors `--quiet` even when a later parse step fails.
- `parseSyncLevels`: comma-separated `--sync-level` → ordered `[]fetch.SyncLevel` (`line` →
  `SyncLine`, `word` → `SyncWord`, `none` → `SyncNone`); entries trimmed, empty entries
  dropped; anything else is a usage error (exit 2).
- `parseDuration`: plain positive integer (`225`) or `mm:ss` (`3:45`) → whole seconds;
  whitespace-only input is treated as not provided (`0`, mirroring the `--author` whitespace
  precedent); anything else is a usage error (exit 2).
- `splitTrimmed`: splits a comma-separated flag value, trimming whitespace and dropping empty
  entries.
- `parsedFlagsToParams`: converts the raw flags and positional song into `fetch.Params`
  without applying defaults (those live in `parseFlags`); source lists are trimmed and empty
  entries dropped here; sync levels are already parsed into `SyncLevels`.

## env.go

- `envList` collects repeated `--env key=value` flags. `flag.Value` calls `Set` once per
  occurrence, so both `--env LANG=en` and `--env=LANG=en` work.
- `validateEnv(envs)` validates the collected entries at parse time and returns them as a
  key→value map. Each entry is split on the first `=`; the key must match `ParamNamePattern`,
  the value must be non-empty after trimming (a whitespace-only value counts as empty,
  mirroring the typed params' `TrimSpace` semantics), and duplicate keys are rejected. Any
  violation is a usage error (exit 2).
- `mergeEnv(custom, decls)` fills every key any requested source declares from the process
  environment when the user did not supply it via `--env`. Precedence: `--env` > environment
  > missing. An environment variable that exists but is empty (e.g. `LANG=`) counts as
  missing and is not injected. Injected keys are treated exactly like user-provided ones — a
  source that does not declare the key still warns unsupported.

## output.go

- `outputExistsError` — `--output` points to an existing file while `--overwrite` was not
  given; `Run` maps it to exit code 7.
- `openOutput(path, overwrite, fallback)` returns the lyrics sink: stdout when path is empty,
  an `*os.File` otherwise. A new file is created exclusively (`O_CREATE|O_EXCL`) and reported
  via `created` so the caller can remove it on failure. An existing file is only opened when
  `overwrite` is set, and never with `O_TRUNC` — truncation happens only after a successful
  fetch, so a failed run leaves existing content intact. The caller must invoke the closer.
  - `WARNING: O_EXCL is not completely safe; file races cannot be fully eliminated.`
  - If the file vanishes between the two opens, the exclusive create is retried.

## render.go

The fetch layer supplies structured data only; the CLI owns every byte of display text,
including the `[kind]` tag.

- `flagForParam` maps a `Param` bit to its CLI flag spelling (`--author`, `--album`,
  `--isrc`, `--duration`).
- `flagFor` renders a parameter reference: a custom key as `--env <KEY>`, a typed bit via
  `flagForParam`.
- `renderWarning` renders one `fetch.Warning` into the exact stderr line:
  - `unsupported`: `warning[unsupported]: source "<s>" does not support <flag>`
  - `downgraded`: exhaustive over the three requestable levels — `SyncLine` → "returned no
    synced lyrics", `SyncWord` → "returned no word-synced lyrics", `SyncNone` → "returned
    only synced lyrics"
  - `precheck`: `requires <flag>` / `not found` / `duplicate`
  - `precheck-mismatch`: fetch-time `requires <flag> but precheck did not enforce it (source
    bug); trying next source`, or `declared invalid --env key "<K>" (source bug)`
  - `fetch`: `failed: <err>; trying next source`
  - `result`: `declares field "<F>" but left it empty (source issue)` / `filled field "<F>"
    without declaring it (source issue)`
  - fallback: `warning[unknown]` — a safety net so a future kind never renders as an empty
    line
- `renderRequiredError` renders the body of the `error[required]` line: `source "<s>"
  requires <flag>`.

## usage.go

`printUsage(w, reg, decls)` writes the help text. Examples use the long (`--`) form; the
underlying flag library also accepts short forms. When `decls` is non-nil it feeds the
"Source parameters:" section: a per-source list of the static `--env` keys and their
descriptions. When `reg` is non-nil it lists the available sources.

## loadmock.go

Build tag `test`. Registers the `mock-*` sources so binaries built with `-tags test` (and the
test suite itself) can exercise them. Production builds exclude this file.
