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
hooks.server.go              # hooks pipeline           (tag-free; standalone `hooks` package)
```

## Build tags are not required

No user `.go` file needs `//go:build sveltego` (#527). The convention works because:

- Files under `src/routes/**` use the `_` prefix. Go's default toolchain automatically ignores any source file whose name starts with `_` (RFC #379 phase 1b).
- `src/hooks.server.go` and `sveltego.config.go` compile as standalone packages (`hooks` and `config`), but `cmd/app/main.go` only imports the codegen mirrors at `.gen/hookssrc/` and `.gen/configsrc/`. The user files are never linked into the binary. `go vet` and `golangci-lint` see them — a feature, not a bug.
- Param matchers under `src/params/<name>/<name>.go` (one matcher per subdirectory; package name equals `<name>`) are mirrored into `.gen/paramssrc/<name>/` and registered via `gen.Matchers()` so the runtime sees them without manual `cmd/app/main.go` wiring (#511).

Codegen reads every user `.go` file through `go/parser`, which ignores `//go:build` constraints — so existing projects that still carry `//go:build sveltego` keep working. The tag is a harmless no-op now. See ADR 0003 amendment and RFC #379.

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
