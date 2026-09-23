# get-lyrics

Designed to be **Lightweight** x **Usable** x **Composable**.

A binary under 10MB, an easy-to-use CLI, and clear, meaningful exit codes -- all at once.

Fetch song lyrics from the command line. Supports multiple backend sources with a pluggable adapter system.

It is now also available as a dependency library — the `source`, `fetch`, and `bootstrap` packages can be imported directly in your own Go projects.

## Install

### Option 1: Download a prebuilt binary from Releases (Recommended)

Prebuilt binaries are attached to each [Release](https://github.com/PloyBox/get-lyrics/releases) — no Go toolchain needed.

### Option 2: Build from source with `go install`

```sh
go install github.com/PloyBox/get-lyrics/cli/get-lyrics@latest
```

Requires Go 1.25.10+.

### Option 3: Build from source with a downgraded Go version

If your Go toolchain is older than the version pinned in `go.mod`, you can lower the `go` directive and compile from source:

```sh
go mod edit -go=1.22
go install ./cli/get-lyrics
```

No compatibility is guaranteed with older Go toolchains — the code is developed against the version pinned in `go.mod`, and downgrading may fail to build or misbehave at runtime.

## Documentation

- [Usage](docs/usage/usage.md) — examples and the flag reference
- [Exit Codes](docs/usage/exit-codes.md)
- [Source Parameters](docs/usage/source-parameters.md) — the `--env` keys sources declare
- [Built-in Sources](docs/usage/built-in-sources.md) — supported filters and sync levels per source
- [Add a New Source](docs/usage/add-a-source.md) — writing a pluggable adapter

## License

Apache 2.0
