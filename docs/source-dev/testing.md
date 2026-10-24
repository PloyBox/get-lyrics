# Testing

The suite is black-box: every test observes a public contract through exported APIs,
never unexported identifiers. If a test would need `filtResult`, `openOutput`, or another
unexported name to be written, it belongs to a different layer.

## Running

From the repository root:

```sh
go build ./...
go test ./...                            # library, bootstrap and adapter tests
go test -tags test -count=1 ./...        # full suite, including the CLI
go test -tags test -count=1 -race ./...  # full suite under the race detector
go vet -tags test ./...
gofmt -l .                               # must print nothing
```

`-count=1` bypasses the test cache, so a pass reflects a real run. There is no
coverage threshold: coverage is a manual debugging aid, not a gate, and CI does not
upload it.

## Black-box boundaries

| Layer | Test package | Observes |
|-------|--------------|----------|
| `source` | `source_test` | `Registry`, `Source`, `ParamSpec`, public error types |
| `fetch` | `fetch_test` | `New`, `Fetch`, `CustomParamsFor`, `Params`/`Result`/`Warning`, error types, and a test-local stub `Source` |
| adapters | `<name>_test` (e.g. `lrclib_test`) | `New`, `Adapter`, `HTTPClient`, `Capabilities`, `Fetch`, `internal/testutil.Transport` |
| `bootstrap` | `bootstrap_test` | `RegisterAll` and `Registry` |
| CLI | `package main_test` in `cli/get-lyrics/` | `app.Run`/`app.RunContext` with argv, env, two output buffers and temp files |

The CLI test files sit next to `run.go` but declare an external test package and import
the command package as `app`. The compiler enforces this isolation at compile time: no
subprocess is needed, and no test switches enter production builds. The cost is accepted:
`main()` and its `signal.NotifyContext` wiring are not covered and stay a manual smoke
check; the business logic lives in `Run`/`RunContext`.

## Build-tag chain

Mock sources exist only in `test` builds:

1. `internal/provider/mock/*.go` carry `//go:build test`.
2. `bootstrap/bootstrap_mock.go` carries the same tag, imports the mock package and
   implements `RegisterAllMock`.
3. `cli/get-lyrics/loadmock.go` carries the tag and registers every mock into the
   global registry from `init()`.
4. The CLI test files carry the tag too. When the test binary compiles the command
   package, the tagged files join it, `init()` runs first, and `Run` can resolve the
   `mock-*` names.

`go build ./...` and `go test ./...` exclude all of the above. Without the tag the CLI
test files are excluded as well, so a run cannot reach the half-state where tests
execute but the mocks are absent. Not importing the mock package from the CLI tests
means no direct symbol reference; the mocks are still linked through `loadmock.go`.

Spot-check the build boundary:

```sh
go build -o /tmp/get-lyrics ./cli/get-lyrics
/tmp/get-lyrics --help          # Available sources contain no mock-* entries
go build -tags test -o /tmp/get-lyrics-test ./cli/get-lyrics
/tmp/get-lyrics-test --help     # lists mock-plain and the other test sources
```

## Test infrastructure

> **Hard rule:** `internal/testutil` is test-only. Non-test files — production
> code, adapters, CLI sources, and `main` packages — must never import it. It
> deliberately has no `test` build tag (untagged adapter tests import it), so an
> accidental production import would compile and pull `testing` into the binary
> instead of failing the build. Guard the boundary with:
>
> ```sh
> go list -deps ./cli/get-lyrics | grep internal/testutil   # must print nothing
> ```

`internal/testutil` provides `Transport`, an `http.RoundTripper` that records each
request and delegates the response to a test-supplied function. Adapter tests set
`HTTPClient: &http.Client{Transport: tp}`, so they need no `httptest.Server` and can
still assert the exact URL, query and headers the adapter actually sent. It has no
`test` build tag because the adapter tests themselves are ordinary tests compiled
without the tag; tagging the helper would make it unavailable there. It lives under
`internal/`, is imported only by tests, and never enters a production binary. It also
exports `Respond`, which builds fixed-status JSON responses, and `Capabilities`, which
returns request-aware capabilities after asserting that every `Custom` key is in
`CustomParams()` and every `RequiredCustom` key is in `Custom`; the adapter test suites
share both helpers instead of duplicating response and capability boilerplate.

Mock sources are table-driven. `internal/provider/mock` contains one generic `Adapter`
plus a `spec` table; `CustomParams`, `Capabilities` and `Fetch` are fields of an
entry. Add a scenario by appending one entry, never by cloning a real adapter. There
is deliberately no `mock-lrclib`: copying lrclib's conditional capabilities would let
the mock drift from the adapter and give the tests false confidence — lrclib is
covered by its own package tests instead. `mock-customparams` is the shared fixture
for custom parameters: it declares `MOCK_KEY` (required) and `MOCK_OPT` (optional),
and echoes both values in its lyrics.

The full mock inventory, `spec` fields, build-tag chain and `mock-capture` output
format live in [`docs/source-dev/internal/mock.md`](internal/mock.md). That file is the
single source of truth; do not duplicate the table here.

## Environment and determinism

- CLI tests fetch only through `mock-*`. `TestMain` replaces `http.DefaultTransport`
  with a `RoundTripper` that always errors, so a test that accidentally reaches a real
  adapter fails instead of using the network. Adapter tests go through
  `testutil.Transport` only.
- `t.Setenv` and the package-level registry make parallel tests unsafe; the CLI
  package runs serial and never calls `t.Parallel`. The `run` helper clears
  `MOCK_KEY` and `MOCK_OPT` before applying the case's environment.
- Files live under `t.TempDir()`. Permission and rename-onto-a-directory assertions
  are skipped on Windows; CI is Linux.
- Only the timeout scenarios wait (~1s each: the global budget and the per-source
  budget against `mock-block`); every other test runs without sleeping.
- Warning order for custom keys comes from map iteration and is unspecified. Assert
  warning sets at the fetch layer, and exact text only where the CLI is the contract.

## CI

CI is release-only: `.github/workflows/simple_ci_cd.yml` runs on `v*` tag pushes,
executes the tests and `go vet`, cross-compiles the release binaries, writes
`checksums.txt` and publishes the release. There is no push/PR test workflow, and no
coverage upload or gate. Run the commands above locally before releasing.

## Adding tests

- A new adapter needs two black-box tests in an external `<name>_test` package:
  `TestRequest` for endpoint, query, headers and `Capabilities`, and `TestFetch` for
  response parsing, metadata and error mapping. Use `testutil.Transport` so the
  production URL is exercised.
- A new exit code or warning text goes into the corresponding CLI table
  (`TestUsageErrors`, `TestWarnings`, `TestRequiredParams`, ...) with the exact
  stderr line and exit code.
- A new mock scenario is one `spec` entry plus a CLI or fetch assertion that uses it;
  update [`docs/source-dev/internal/mock.md`](internal/mock.md) in the same change.
- Changing the JSON structure or parameter semantics requires bumping
  `FormatVersion` in `run.go` and syncing the nine-key assertion in the CLI
  `TestJSON` (`FormatVersion`, `Level`, `Lyrics`, `Title`, `Artist`, `Album`, `ISRC`,
  `Source`, `SubSource`).
