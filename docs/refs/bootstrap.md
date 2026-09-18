# bootstrap

Concrete adapters implementing `source.Source` live under `internal/provider/`: `real/` for
live upstreams (see [providers/](./providers/)), `mock/` for test-only stubs (see
[../mocks.md](../mocks.md)).

`package bootstrap` aggregates every built-in source adapter so that `main` only needs one
call to wire them all up.

Adding a new built-in source:

1. create `internal/provider/real/<name>`, implementing `source.Source`;
2. add an import and `r.Register(<name>.New())` in `RegisterAll`.

`RegisterAll(r)` registers every built-in adapter; `main` calls it exactly once during
startup (via a package-level var initializer). `RegisterAllMock(r)` (build tag `test`)
registers every mock/test-only adapter.
