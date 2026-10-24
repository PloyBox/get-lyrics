# bootstrap

Wires the built-in adapters into a `source.Registry`. Public contract:
[library/bootstrap.md](../library/bootstrap.md).

- `bootstrap.go` imports every adapter under `internal/provider/real/` and registers
  each in `RegisterAll`; `cli/get-lyrics/run.go` calls it from `mustRegisterAll` at
  package-init time and panics on error.
- `bootstrap_mock.go` (build tag `test`) implements `RegisterAllMock`, registering every
  adapter from `internal/provider/mock` (see [internal/mock.md](internal/mock.md)).
  Production builds exclude the file, so mock sources never ship.
- `Registry.Register` validates a source's static `CustomParams()` (gate 1: legal,
  distinct keys) and rejects a duplicate name; both are returned as errors.
- The `test`-tag CLI wiring in `cli/get-lyrics/loadmock.go` calls `RegisterAllMock` at
  init and panics on failure.

Adding a new built-in source:

1. create `internal/provider/real/<name>`, implementing `source.Source`;
2. add an import and `r.Register(<name>.New())` in `RegisterAll`.

Full walkthrough: [Add a New Source](add-a-source.md).
