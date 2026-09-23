# Source Parameters

Sources may declare custom input keys beyond the built-in filters, passed via the repeatable `--env key=value` flag (short: `-e`). `--help` lists each source's declared keys under "Source parameters:".

Rules:

- **Key syntax**: env-style upper snake case, `^[A-Z][A-Z0-9_]*$` (e.g. `LANG`, `HTTP_PROXY`). Keys are matched exactly and case-sensitively; lowercase/mixed-case or otherwise invalid keys are rejected at parse time (exit 2).
- **Value**: must be non-empty after trimming; a whitespace-only value is a usage error (exit 2). Duplicate keys are rejected (exit 2).
- **Environment fallback**: for every key a requested source declares, a key you did not pass via `--env` is filled from the process environment (`os.LookupEnv`) when it exists and is non-empty. Precedence: `--env` > environment > missing. An environment variable that exists but is empty (e.g. `LANG=`) counts as missing.
- **Injected keys behave like user-passed ones**: a source that does not declare a key emits `warning[unsupported]` for it either way — the fallback only fills values, it never changes what a source recognizes.
- **Required keys**: a source may require a key for a given request (conditionally, based on other inputs). A missing required key is exit 6 in strict mode; under `--lenient` the source is skipped with a `warning[precheck]`.
- **Unrecognized keys**: never hard-fail; each produces one `warning[unsupported]` per source (order between multiple keys is unspecified).
- **Multiple sources sharing a key** all consume the same value.

See [Add a New Source](add-a-source.md) for declaring keys from an adapter.