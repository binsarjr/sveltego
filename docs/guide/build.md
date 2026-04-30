---
title: Build
order: 80
summary: sveltego build, .gen output, Vite client bundle, single-binary deploy.
---

# Build

`sveltego build` runs codegen then hands off to `go build`. The output is a single binary plus the Vite client bundle.

## Pipeline

1. Scan `src/routes/**` for `+page.svelte`, `page.server.go`, `+layout.svelte`, `layout.server.go`, `server.go`, `+error.svelte`.
2. Parse `.svelte` files via the sveltego parser; validate Go expressions in mustaches via `go/parser.ParseExpr`.
3. Emit `.gen/*.go` — one Go file per template, plus a manifest registering routes, layouts, and page options.
4. Run Vite to produce the client hydration bundle (`dist/`).
5. Hand off to `go build` to produce the server binary.

## Generated layout

```
.gen/
  routes/
    page__root.gen.go         # +page.svelte at /
    page__blog__slug.gen.go   # +page.svelte at /blog/[slug]
    layout__root.gen.go       # +layout.svelte at root
    server__api__hello.gen.go # +server.go at /api/hello
  manifest.gen.go             # routes, layouts, params, hooks
  links.gen.go                # typed kit.Link helpers per route
```

The `.gen/` directory is gitignored; it is regenerated on every build.

## Page options

Declare per-page options as exported constants in `page.server.go` or `layout.server.go`:

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

const (
  Prerender     = true
  SSR           = true
  CSR           = false
  TrailingSlash = kit.TrailingSlashNever
)
```

Layout values cascade to descendants; page values override the cascade. The manifest stores the resolved value per route, so the pipeline does not re-walk the layout chain at request time.

## Tooling commands

| Command | Purpose |
|---|---|
| `sveltego build` | Full codegen + go build. |
| `sveltego compile` | Codegen only. |
| `sveltego dev` | Watch + regenerate. |
| `sveltego check` | Validate without writing output. |
| `sveltego routes` | Print the route table. |
| `sveltego version` | Print version. |

## Determinism

Codegen is deterministic byte-for-byte. Two builds of the same source produce identical `.gen/` output. Golden tests in the codegen package enforce this; see issue #104 for the `-update` flag flow.
