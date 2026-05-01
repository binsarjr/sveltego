---
title: Build
order: 80
summary: sveltego build pipeline — codegen, Vite client bundle, SSG prerender, single-binary deploy.
---

# Build

`sveltego build` runs codegen, builds the client bundle, prerenders SSG routes, then hands off to `go build`. The output is a single Go binary plus a `static/` directory of prerendered HTML for any routes that opted in to SSG.

## Pipeline

1. Scan `src/routes/**` for `_page.svelte`, `_page.server.go`, `_layout.svelte`, `_layout.server.go`, `_server.go`, `_error.svelte`.
2. Parse each `_page.server.go` / `_layout.server.go` via `go/parser`. Extract the `Load` return type, `Actions`, and any page-options constants (`Prerender`, `SSR`, `CSR`, `TrailingSlash`).
3. Emit `.gen/`:
   - A sibling `.svelte.d.ts` per `_page.svelte` / `_layout.svelte`, declaring `data`'s shape (Go AST → TypeScript). Svelte LSP picks these up for autocomplete.
   - A route manifest registering routes, layouts, params, and resolved page options.
4. Run Vite to produce the client bundle (`dist/`).
5. For routes with `Prerender: true`: invoke a Node sidecar to render their HTML via `svelte/server`, write to `static/<route>/index.html`. **Node runs only at build time**; it is not on the deployed request path.
6. Hand off to `go build` to produce the server binary, which `go:embed`s `dist/` for client asset serving.

## Generated layout

```
.gen/
  manifest.gen.go             # routes, layouts, params, hooks
  links.gen.go                # typed kit.Link helpers per route
src/routes/
  _page.svelte
  _page.svelte.d.ts           # generated; declares the `data` prop
  _page.server.go
static/                       # SSG output for prerendered routes (optional)
  blog/
    hello-world/
      index.html
dist/                         # Vite client bundle
```

The `.gen/` directory and `*.svelte.d.ts` files are gitignored; both regenerate on every build.

## Page options

Declare per-page options as exported constants in `_page.server.go` or `_layout.server.go`:

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const (
  Prerender     = true
  SSR           = true
  CSR           = false
  TrailingSlash = kit.TrailingSlashNever
)
```

`Prerender = true` opts the route into SSG: `svelte/server` renders the HTML at build time and the Go binary serves the cached file. Routes without `Prerender` ship as a SPA — the Go server returns a small shell + JSON payload from `Load`, and the Svelte client mounts and renders.

Layout values cascade to descendants; page values override the cascade. The manifest stores the resolved value per route, so the pipeline does not re-walk the layout chain at request time.

## Tooling commands

| Command | Purpose | Status |
|---|---|---|
| `sveltego build` | Full codegen + Vite + SSG prerender + `go build`. | Shipped. |
| `sveltego compile` | Codegen only — manifest + `.svelte.d.ts` (no `go build`). | Shipped. |
| `sveltego routes` | Print the resolved route table. | Shipped. |
| `sveltego version` | Print version. | Shipped. |
| `sveltego dev` | Watch + regenerate + HMR proxy. | Stub — deferred to v0.3 ([#42](https://github.com/binsarjr/sveltego/issues/42)). |
| `sveltego check` | Validate without writing output. | Stub — milestone TBD. |
| `sveltego-init` | Scaffold a new project (separate binary under `packages/init`). | Shipped (with [#356](https://github.com/binsarjr/sveltego/issues/356) gap). |

## Determinism

Codegen is deterministic byte-for-byte. Two builds of the same source produce identical `.gen/manifest.gen.go` and `.svelte.d.ts` output. Golden tests in the codegen package enforce this; see issue #104 for the `-update` flag flow.
