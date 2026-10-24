# mock (internal/provider/mock)

Table-driven source adapters used only by tests. Every file in the package carries
`//go:build test`, so production builds never include it.

`mock.Adapter` is one generic `source.Source`; the table in `specs.go` describes each
registered source, and `All()` turns every entry into an adapter.

## `spec`

```go
type spec struct {
	name   string
	static []source.ParamSpec
	caps   func(source.Request) source.Capabilities
	fetch  func(context.Context, source.Request) (source.Result, error)
}
```

- `name` — registered source name, always `mock-*`.
- `static` — what `CustomParams()` returns, i.e. the request-independent declarations
  used by `--help` and the environment fallback. A normal mock keeps it equal to
  `caps(req).Custom`; `mock-gate2` breaks that on purpose to exercise gate 2.
- `caps` — the request-aware `Capabilities` (filters, required typed params, custom
  declarations).
- `fetch` — the behavior: a result, an error, or a deliberate block.

## Build-tag chain

1. `internal/provider/mock/*.go` — `//go:build test`.
2. `bootstrap/bootstrap_mock.go` — same tag; implements `RegisterAllMock` over
   `mock.All()`.
3. `cli/get-lyrics/loadmock.go` — same tag; `init()` registers the mocks into the
   global registry used by the command package.
4. CLI test files — same tag; when the test binary builds the command package,
   `init()` runs before `Run`, so the mock names resolve.

`go build ./...` and untagged `go test ./...` exclude every link of this chain. The
production `--help` lists no `mock-*` source; a `-tags test` binary lists them.

## Mocks

| Name | Behavior | Purpose |
|------|----------|---------|
| `mock-plain` | Plain-text lyrics, `SyncNone`, supports all four typed filters | Success path, file output, JSON |
| `mock-lrc` | Timestamped LRC, `SyncLine` | Line match, plain-request downgrade |
| `mock-word` | TTML, `SyncWord`, `SubSource` `mock-ttml` with `FieldSubSource` | Word match, aggregate sub-source reporting |
| `mock-fail` | Returns a plain error | `warning[fetch]`, failover |
| `mock-notfound` | Wraps `source.ErrNotFound` | `warning[not-found]`, failover |
| `mock-bodylimit` | Wraps `source.ErrBodyTooLarge` | `warning[body-limit]`, failover |
| `mock-require-author` | `Filters` include author, `Required` is author; returns plain lyrics | Exit 6, lenient precheck |
| `mock-customparams` | Declares `MOCK_KEY` (required) and `MOCK_OPT` (optional); lyrics echo both values | `--env` injection, environment fallback, optional keys |
| `mock-gate2` | Empty static list; `Capabilities` adds `BAD_KEY` | `warning[precheck-mismatch]` from gate 2 |
| `mock-fetchmismatch` | Statically declares `MOCK_TOKEN`; `Fetch` returns `RequiredParamMismatchError` for it | Fetch-time declaration mismatch |
| `mock-result-mismatch` | Sets `FieldTitle` in `Filled` but leaves `Title` empty | `warning[result]` (declared-but-empty) |
| `mock-nosupport` | `Filters` is zero | `warning[unsupported]` |
| `mock-block` | Waits for `ctx.Done()`, then returns `ctx.Err()` | Per-source and global timeouts |
| `mock-capture` | Serializes the received `source.Request` as its lyrics payload | Request field and User-Agent pass-through |

Keep the behaviors consistent with these assertions when editing the table:

- `mock-require-author` must keep author in `Filters`; otherwise a request with
  `--author` also produces `warning[unsupported]`, contradicting `Required`.
- `mock-fetchmismatch` must fill `ParamName` (`MOCK_TOKEN`), or the rendered
  `requires` text is empty.
- `mock-block` returns `ctx.Err()` after the context ends, never nil.
- `mock-capture` returns `SyncNone` with at least `FieldLyrics`.

## `mock-capture` output

The fetch function runs `json.Marshal(req)` on the received `source.Request` and
returns the bytes unchanged as the lyrics payload. The CLI writes lyrics without
adding a newline, so stdout is exactly one line of compact JSON in struct field order:

```json
{"Song":"Song title","Author":"Artist","Album":"Album","ISRC":"ISRC123","Duration":225,"SyncLevel":0,"UserAgent":"get-lyrics/dev (+https://github.com/PloyBox/get-lyrics)","Custom":{}}
```

Fields, in output order:

| Key | Type | Meaning |
|-----|------|---------|
| `Song` | string | `--name` value |
| `Author` | string | `--author` value |
| `Album` | string | `--album` value |
| `ISRC` | string | `--isrc` value |
| `Duration` | number | Whole seconds; 0 means not provided |
| `SyncLevel` | number | Numeric `source.SyncLevel`: `SyncNone` 0, `SyncLine` 1, `SyncWord` 2 |
| `UserAgent` | string | Resolved `--user-agent` value |
| `Custom` | object | Custom key map; `{}` when no keys are present |

`encoding/json` sorts object keys inside `Custom`, and there is no indentation and no
trailing newline. The CLI always passes an initialized map, so `Custom` is `{}` rather
than `null` when no keys are given. The CLI test `TestRequestFields` decodes this payload
to assert field pass-through, duration normalization and User-Agent selection.

## Adding a mock

Never clone a real adapter into a mock: its capabilities and parsing are covered by
that adapter's own tests, and a copy drifts. Append one entry to `specs` in
`specs.go`:

1. Pick a unique `mock-*` name.
2. Set `static` and `caps` (equal for normal mocks; deliberately inconsistent only for
   a gate-2 scenario).
3. Implement `fetch` with the contract the scenario needs: use `plainResult` or
   `syncResult` for lyrics, wrap `source.ErrNotFound`/`source.ErrBodyTooLarge` when
   the fetch layer's mapping is the target, or return a filled error type.
4. Register nothing: `All()` picks the entry up, and `RegisterAllMock` runs it in test
   builds.
5. Use the mock from the CLI or fetch tests, and add its row to the table above in the
   same change.
