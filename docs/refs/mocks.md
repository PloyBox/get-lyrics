# mocks (internal/provider/mock)

Registered only under the `test` build tag via `bootstrap.RegisterAllMock` (never in
production). Names must start with `mock-`. Each covers one testing concern:

| Mock | Concern |
|---|---|
| `mock-success` | happy path; honors and requires `--author` |
| `mock-require` | exit 6; requires `--author` while listing no filters, so the required-param precheck is exercised in isolation |
| `mock-nosupport` | no-param path; declares nothing |
| `mock-fail` | exit 4; `Fetch` always errors |
| `mock-lrc` | synced path; returns LRC lyrics on a `SyncLine` request, plain otherwise |
| `mock-nosync` | downgrade path; honors `--sync-level` in principle but never returns synced lyrics |
| `mock-synconly` | synced-only path; never fills plain lyrics and always reports `Level = SyncLine`, so a plain request cannot match it (downgrade warning, no empty success) while a later `line` iteration reuses it from the cache |
| `mock-mismatch` | precheck-vs-requirement mismatch path; lists `--author` as a filter but declares nothing required, so the missing parameter only surfaces inside `Fetch` as a `RequiredParamMismatchError` |
| `mock-custom` | custom `--env` params: statically declares `LANG` (always recognized and required) and `COUNTRY` (recognized and required only when `LANG` is present — demonstrating conditional custom parameters) |
| `mock-word` | word-level path; returns pseudo TTML on a `SyncWord` request, plain lyrics otherwise |
| `mock-slow` | timeout path; blocks until the context ends, exercising `--timeout` (per-source) and `--timeout-global` (exit 9) |
| `mock-gate2` | gate-2 path; the request-aware declaration contradicts the empty static list (`RequiredCustom: LANG-X`), so precheck skips it with `warning[precheck-mismatch]` — never exit 6 — even though it also requires `--author` |
| `mock-useragent` | user-agent path; echoes the incoming `Request.UserAgent` into its lyrics, so a CLI run can assert the header the adapter actually received |

CLI-level tests reach these adapters only through `--source`; they never register fixtures into the shared registry themselves. A new test concern means a new `mock-*` adapter registered in `bootstrap/bootstrap_mock.go`.
