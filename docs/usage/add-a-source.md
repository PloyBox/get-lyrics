# Add a New Source

Backends are pluggable via the `source.Source` interface. Use an existing adapter as a template, such as `internal/provider/real/lrclib/` — it covers the full surface (filters, required params, single-track output at every sync level):

1. Create `internal/provider/real/<name>/` implementing `source.Source` (`Name` / `Capabilities` / `Fetch` / `CustomParams`), modeled on the template.
2. Modify it to fit your needs — endpoint, filters, required params, output behavior.
3. Add an import and `r.Register(<name>.New())` in `bootstrap/bootstrap.go`.

To declare custom input keys:

- Return them from `CustomParams()` (the static, request-independent list, e.g. `[]source.ParamSpec{{Name: "LANG", Description: "language hint"}}`). This backs the `--help` "Source parameters:" section and the environment-variable fallback.
- List the request's recognized keys in `Capabilities(req).Custom` and the request's required keys in `Capabilities(req).RequiredCustom`. Both may be conditional on `req` (a key required only when another key/field is present). `RequiredCustom` must be a subset of the request's `Custom` names.
- Read the values from `req.Custom` inside `Fetch`.
- Keys are env-style `^[A-Z][A-Z0-9_]*$`; an invalid or duplicate static declaration fails registration at startup (a source bug), and an inconsistent dynamic declaration is flagged at precheck with `warning[precheck-mismatch]` and the source is skipped.

`--help` automatically lists the new source and its parameters.