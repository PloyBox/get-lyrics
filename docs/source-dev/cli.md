# cli (cli/get-lyrics)

`get-lyrics` fetches song lyrics from a registered source.

```
Usage: get-lyrics --name <title> [--source <names>] [--author <name>]
                   [--album <name>] [--isrc <code>] [--duration <secs>]
                   [--timeout <secs>] [--timeout-global <secs>]
                   [--output <file>] [--overwrite] [--json]
                   [--sync-level <levels>] [--user-agent <ua>]
                   [--env <key=value>] [--lenient] [--quiet] [--version]
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | success (stderr may still carry warnings) |
| 2 | usage error (missing `--name`, any positional argument, unknown/typo flag, a non-`--env` flag given more than once, invalid or empty `--sync-level`, invalid `--duration`, empty `--source`, invalid/duplicate `--env`) |
| 3 | unknown source (strict precheck) |
| 4 | no valid result: every source skipped (lenient) or failed, or no result matched the requested sync levels |
| 5 | output failure (temp file create, write, sync, close, or rename) |
| 6 | source-required parameter missing in strict precheck |
| 7 | `--output` points to an existing file and `--overwrite` was not given |
| 8 | duplicate `--source` entry (strict precheck) |
| 9 | overall fetch timeout (`--timeout-global`) exceeded, or the context was cancelled (e.g. Ctrl-C) |

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
- `Run(argv, stdout, stderr) int` is the testable core — `argv` excludes the program name,
  and stdout/stderr are explicit writers. It delegates to `RunContext` with a background
  context. `main` builds a `signal.NotifyContext` (SIGINT/SIGTERM) and calls `RunContext`
  with it, so Ctrl-C cancels the fetch and the deferred cleanup runs.

Run flow:

1. Parse flags; `parseFlags` returns early for `--help`/`--version`, skipping repeat and value
   validation. When `--quiet` is set, stderr is redirected to `io.Discard` before anything is
   printed, silencing every later line (warnings and errors alike) while exit codes stay
   meaningful — honored even when parsing itself fails after the flag.
2. `--help` — checked before the parse error, so invalid values or repeated flags cannot stop it →
   full declaration for rendering only, no env fallback (lenient mode and the sorted registry
   names make this query infallible); print usage, exit 0.
3. `--version` → print `get-lyrics <version>`, exit 0.
4. Parse errors (unknown flag, invalid value, repeated flag) → `error[usage]` + usage, exit 2.
5. Any positional argument → `error[usage]: unexpected positional argument "<arg>" (use --name for the song title)` + usage, exit 2. Missing/empty `--name` (empty after trimming) → `error[usage]: song title is required (use --name)` + usage, exit 2.
6. `parsedFlagsToParams` splits `--source` and rejects an empty source or sync-level list
   (exit 2). Then `svc.CustomParamsFor(params)` for the requested sources — strict mode reports
   exit 3/8 here (same codes as the fetch precheck, but reported first) — followed by `mergeEnv`
   filling every key not supplied by flag from the process environment.
7. Open the output sink before the fetch. File output is written to a temp file in the target
   directory; stdout is used unchanged when `--output` is empty.
8. Build the fetch context — a deadline from `--timeout-global`, when set, derived from the
   caller's context — then `svc.Fetch(ctx, params)`: on every failure path the in-flight
   warnings are printed before the `error[...]` line, then the error is mapped to its exit code.
   A `fetch.StoppedError` (the context's deadline or cancellation ended) maps to exit 9.
9. On success: print the warnings, write the plain lyrics or the complete `fetch.Result` as
   JSON, then `commit` the sink (`Sync`, `Close`, `Rename`).

Output-file handling: a failed run never touches the target. New output is created only on
success (rename into place); an existing `--output` file is replaced atomically on success and
kept byte-for-byte on every failure path. `--overwrite` is required when the target already
exists and is checked before the fetch (with a final re-check before the rename). Temp files
are removed by the sink's `abort` on every failure path, including Ctrl-C.

JSON output marshals the CLI-owned struct `{FormatVersion, Level, fetch.Result}` with
`FormatVersion` 3 (`jsonFormatVersion` in `run.go`); it includes every result field, including
empty strings, and every key is UpperCamelCase (`FormatVersion`, `Level`, `Lyrics`, `Title`,
`Artist`, `Album`, `ISRC`, `Source`, `SubSource`). The outer `Level` shadows the embedded
numeric `fetch.Result.Level`. Increment `FormatVersion` whenever the JSON structure or
parameter semantics change.

## flags.go

- `parsedFlags` holds the parsed inputs, including `song` from `--name`/`-n`. The positional
  arguments are kept separate because they are not flags. `parseFlags` returns them raw and
  `Run` rejects any of them — the song title is a flag, never a positional (exit 2).
- `parseFlags(argv)` handles both `-x`/`--x` forms using Go `flag`'s default behavior:
  parsing stops at the first positional argument, which is then reported as a usage error.
  Unknown flags become a non-nil error which `Run` maps to `exitUsage`. Flag's own usage
  writer is silenced (`io.Discard`); `Run` writes its own on error. On error the flags parsed
  so far are still returned, so `Run` honors `--quiet` even when a later parse step fails.
- When `--help`/`--version` was parsed, `parseFlags` returns before repeat detection and value
  validation, so those checks cannot turn a help/version request into a usage error; `Run`
  dispatches them before handling the parse error.
- Every flag except `--env`/`-e` registers a setter that records a second occurrence; after
  `Parse`, `parseFlags` reports the first repeated flag (in argv order) as a usage error
  (exit 2). String flags use `fs.Func`, bool flags `fs.BoolFunc`; aliases share one setter, so
  `-s a --source b` also counts as a repeat.
- `parseSyncLevels`: comma-separated `--sync-level` → ordered `[]fetch.SyncLevel` (`line` →
  `SyncLine`, `word` → `SyncWord`, `none` → `SyncNone`); entries trimmed, empty entries
  dropped; anything else is a usage error (exit 2). An empty result is rejected later by
  `parsedFlagsToParams`.
- `parseDuration`: plain positive integer (`225`) or `mm:ss` (`3:45`) → `int64` whole seconds;
  whitespace-only input is treated as not provided (`0`, mirroring the `--author` whitespace
  precedent); values outside `1`–`fetch.MaxSeconds` and anything else are usage errors (exit 2).
  The `mm:ss` minutes are checked against the `maxMinute`/`maxSecond` tuple first, so no
  multiplication wraps before the bound is enforced.
- `parseTimeout`: `--timeout`/`--timeout-global` value → `int64` whole seconds; empty means not
  provided (`0`); an integer in `0`–`fetch.MaxSeconds` is accepted (`0` = no timeout); negative,
  non-numeric or oversized values are usage errors (exit 2). The `--timeout` default (`10`) is
  the flag's raw default.
- `splitTrimmed`: splits a comma-separated flag value, trimming whitespace and dropping empty
  entries.
- `parseFlags` trims `--name` (`song`) and the scalar `--source` value; it does not split
  `--source`.
- `parsedFlagsToParams`: converts the raw flags (including `--name`) into `fetch.Params`
  without applying defaults (those live in `parseFlags`). It splits `--source` and rejects an
  empty source or sync-level list with a usage error (exit 2); sync levels are already parsed
  into `SyncLevels`.

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
- `outputSink` embeds `io.Writer` plus `commit()` and `abort()`. The stdout sink's hooks
  do nothing; a file sink writes to a temp file.
- `openOutput(path, overwrite, fallback)` returns the sink. Without `--overwrite`, an existing
  target is rejected before the fetch. The temp file is created in the target directory
  (`os.CreateTemp`) so the final `os.Rename` stays on one filesystem. The target's permission
  bits are preserved on overwrite; new files get 0644.
- `commit` runs `Sync` → `Close` → a final existence check (without `--overwrite`) →
  `Rename`. Any failure removes the temp file and leaves the target untouched; a concurrent
  create still loses the final check-to-rename race (as did the previous `O_EXCL` approach).
- `abort` closes and removes the temp file; it does nothing after a successful commit and
  runs from `Run`'s defer, so Ctrl-C and every failure path clean up.

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
  - `not-found`: `found no match; trying next source`
  - `body-limit`: `returned an oversized response; trying next source`
  - `result`: `declares field "<F>" but left it empty (source issue)` / `filled field "<F>"
    without declaring it (source issue)`
  - fallback: `warning[unknown]` — a safety net so a future kind never renders as an empty
    line
- `renderRequiredError` renders the body of the `error[required]` line: `source "<s>"
  requires <flag>`.

## usage.go

`printUsage(w, reg, decls)` writes the help text. The synopsis shows `--name <title>` as
required and every flag; examples use the long (`--`) form, and the underlying flag library
also accepts short forms. It states that positional arguments are not accepted. When `decls`
is non-nil it feeds the "Source parameters:" section: a per-source list of the static `--env`
keys and their descriptions. When `reg` is non-nil it lists the available sources.

## loadmock.go

Build tag `test`. Its `init()` calls `bootstrap.RegisterAllMock(registry)`, which
registers every adapter from `internal/provider/mock`; production builds exclude the
file, so they carry no mock sources.

The CLI black-box tests live in this same directory: `package main_test`, build tag
`test`, importing the command package as `app` and calling only `Run`/`RunContext`.
