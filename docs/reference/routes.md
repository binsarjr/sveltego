---
title: Route conventions
order: 210
summary: File names, build tag, generated output, and the route-matching contract.
---

# Route conventions

## Tree

```
src/routes/
  _page.svelte               # SSR template
  page.server.go             # Load(), Actions       (//go:build sveltego)
  _layout.svelte             # layout chain
  layout.server.go           # parent data flow      (//go:build sveltego)
  server.go                  # REST endpoints        (//go:build sveltego)
  _error.svelte              # error boundary
  (group)/                   # route group, no URL segment
  _page@.svelte              # layout reset
  [param]/                   # required param
  [[optional]]/              # optional segment
  [...rest]/                 # catch-all
src/params/<name>.go         # param matcher        (//go:build sveltego)
src/lib/                     # $lib alias target
hooks.server.go              # hooks pipeline       (//go:build sveltego)
```

## Build tag

Every `.go` file under `src/routes/**` and `src/params/**` and at `src/hooks.server.go` MUST start with:

```go
//go:build sveltego
```

The tag prevents Go's default toolchain (build, vet, lint) from compiling these files. Codegen reads them through `go/parser` directly. See ADR 0003 amendment.

## Match precedence

The router prefers more specific patterns. Roughly:

1. Static segments beat dynamic.
2. Required `[name]` beats optional `[[name]]`.
3. Optional beats catch-all `[...rest]`.
4. Param matchers (`[id=int]`) tie-break toward the matcher that accepts.
5. Layout reset (`_page@.svelte`) participates without affecting precedence.

`sveltego routes` prints the resolved table for inspection.

## Generated output

`.gen/` (gitignored) holds:

- `routes/` — one file per template, plus per-route render functions.
- `manifest.gen.go` — registers routes, layouts, hooks, params, page options.
- `links.gen.go` — typed `kit.Link` helpers per route.

Two builds of the same source produce byte-identical `.gen/` output. Golden tests enforce determinism (#104).

## File naming gotchas

- `_page.svelte` — leading `+` is required on Svelte template files.
- `page.server.go` — note the `.server.` infix; no leading `+`. The `//go:build sveltego` tag identifies it to codegen.
- `_page.server.go` (with `+`) is **not** recognized by routescan. Drop the `+`.
- `server.go` — REST endpoint, no template. No `+` prefix.
- `_page@.svelte` — trailing `@` (before the extension) signals a layout reset.

When in doubt, run `sveltego routes` and verify the entry appears.
