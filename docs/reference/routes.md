---
title: Route conventions
order: 210
summary: File names, build tag, generated output, and the route-matching contract.
---

# Route conventions

## Tree

```
src/routes/
  _page.svelte               # pure Svelte/JS/TS template
  _page.server.go            # Load(), Actions          (`_` prefix; Go toolchain skips)
  _layout.svelte             # layout chain
  _layout.server.go          # parent data flow         (`_` prefix; Go toolchain skips)
  _server.go                 # REST endpoints           (`_` prefix; Go toolchain skips)
  _error.svelte              # error boundary
  (group)/                   # route group, no URL segment
  _page@.svelte              # layout reset
  [param]/                   # required param
  [[optional]]/              # optional segment
  [...rest]/                 # catch-all
src/params/<name>/<name>.go  # param matcher            (auto-registered via gen.Matchers())
src/lib/                     # $lib alias target
hooks.server.go              # hooks pipeline           (`//go:build sveltego`)
```

## Build tag and the `_` prefix

Files under `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`). Go's default toolchain automatically ignores any source file whose name starts with `_`, so no build tag is required there (RFC #379 phase 1b).

The project-level `hooks.server.go` MUST still start with:

```go
//go:build sveltego
```

because its filename has no `_` prefix. Without the tag, Go's default toolchain (build, vet, lint) would try to compile it. Codegen reads every user `.go` file through `go/parser` regardless.

Param matchers live under `src/params/<name>/<name>.go` (one matcher per subdirectory; package name equals `<name>`). They do **not** need the `//go:build sveltego` constraint — codegen mirrors each matcher into `.gen/paramssrc/<name>/` and emits `.gen/matchers.gen.go` exposing `func Matchers() router.Matchers` so the runtime gets the full registry without manual `cmd/app/main.go` wiring (#511). See ADR 0003 amendment and RFC #379.

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

- `manifest.gen.go` — registers routes, layouts, hooks, params, page options.
- `links.gen.go` — typed `kit.Link` helpers per route.

Sibling files emitted next to each `.svelte`:

- `_page.svelte.d.ts` / `_layout.svelte.d.ts` — TypeScript declarations of the `data` (and `form`, where relevant) prop, derived from the `Load` return type and `Actions` shape on the Go side. Picked up by Svelte LSP for end-to-end autocomplete. Gitignored.

Two builds of the same source produce byte-identical output. Golden tests enforce determinism (#104).

## File naming gotchas

- `_page.svelte` — `_` prefix is required.
- `_page.server.go` — `_` prefix hides it from Go's default toolchain. No `//go:build sveltego` tag needed.
- `+page.server.go` (with `+` instead of `_`) is **not** recognized by routescan. Use `_` (RFC #379 phase 1b).
- `_server.go` — REST endpoint, no template.
- `_page@.svelte` — trailing `@` (before the extension) signals a layout reset.

When in doubt, run `sveltego routes` and verify the entry appears.
