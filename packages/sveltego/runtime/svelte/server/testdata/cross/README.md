# Cross-check fixtures

Each `*.json` here pairs an `(input, expected)` row derived from running the
named Svelte helper in Node against `svelte/server`. The Go side asserts
byte equality.

## Provenance

Captured against Svelte 5.x (the pin in `package.json` at the repo root —
the `svelte_version` field on each fixture file marks the exact pin used
to capture). When the pin bumps in a Phase-3-emitted minor, regenerate
this corpus and review the diff.

## Capture script (not committed)

A small Node script that imports `escape_html`, `attr`, `clsx`,
`stringify`, `merge_styles`, `spread_attributes` from `svelte/server`'s
internal modules and prints `[input, expected]` JSON rows. The reason it
is not committed: Svelte deliberately does not export internal helpers,
so the script reaches into `node_modules/svelte/src/internal/...` which
is non-public surface and brittle. The fixtures themselves are the
durable artifact.

## Schema

```json
{
  "helper": "EscapeHTML",
  "svelte_version": "5.x",
  "cases": [
    { "name": "amp", "in": "a & b", "want": "a &amp; b" }
  ]
}
```

`in` and `want` are JSON-typed — strings, numbers, bools, nulls, arrays,
objects all flow through. The Go test loader picks the helper by name.
