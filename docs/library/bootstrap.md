# bootstrap

`bootstrap` registers every built-in adapter into a caller-supplied `source.Registry`,
so a dependency user gets the full built-in set without importing `internal/`.

`RegisterAll(r *source.Registry) error` registers every built-in adapter once. It
returns an error for a duplicate name or an invalid static declaration instead of
panicking.

- Built-in adapters live in `internal/provider/real/` and are not importable;
  `RegisterAll` is the only public entry to them.
- Call it once during startup, then fetch through `fetch.New(r)`.
- A library may register its own `source.Source` implementations alongside the
  built-ins, or skip `bootstrap` and use its own registry (see [source](source.md)).
- `RegisterAllMock` exists only under the `test` build tag; production builds exclude
  the mock adapters.
- Adding a built-in is source-level work: [Add a New Source](../source-dev/add-a-source.md).
