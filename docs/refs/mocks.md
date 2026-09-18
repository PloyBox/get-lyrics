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
