# ADR 0003 — File Convention (`.gen/` Layout)

- **Status:** Accepted
- **Date:** 2026-04-29
- **Issue:** [binsarjr/sveltego#3](https://github.com/binsarjr/sveltego/issues/3)

## Decision

**Selected:** Option A — `.gen/` mirrors `src/routes/` directory structure.

## Rationale

- One-to-one source-to-output mapping makes debugging trivial. `src/routes/posts/[slug]/+page.svelte` → `.gen/routes/posts/_slug_/page.gen.go`.
- Each output directory becomes its own Go package — namespace isolation per route prevents identifier collisions.
- Selector reads naturally: `gen.Routes.Posts._slug_.Page` mirrors URL path.
- Tooling friendly: file watcher invalidates one output file per source change. No global manifest churn for a single edit.

## Locked sub-decisions

- **Route group `(marketing)` (Q4):** preserved as `_g_marketing/` package directory. Maintains 1:1 source-to-output mapping. The `_g_` prefix encodes "group" and avoids identifier clash with a regular route segment named `marketing`.
- **Optional and rest segment naming (Q5):** distinct visual via underscore count.
  - `[lang]` → `_lang_`
  - `[[lang]]` → `__lang__`
  - `[...path]` → `___path`
- **Layout reset `+page@.svelte` filename (Q6):** `page_reset.gen.go`. Semantically meaningful; reader sees "this page resets the layout chain".

## Naming rules (full)

| Source | `.gen/` path | Package name |
|---|---|---|
| `src/routes/+page.svelte` | `.gen/routes/page.gen.go` | `routes` |
| `src/routes/+layout.svelte` | `.gen/routes/layout.gen.go` | `routes` |
| `src/routes/+page@.svelte` | `.gen/routes/page_reset.gen.go` | `routes` |
| `src/routes/+error.svelte` | `.gen/routes/error.gen.go` | `routes` |
| `src/routes/about/+page.svelte` | `.gen/routes/about/page.gen.go` | `about` |
| `src/routes/posts/[slug]/+page.svelte` | `.gen/routes/posts/_slug_/page.gen.go` | `_slug_` |
| `src/routes/posts/[[lang]]/+page.svelte` | `.gen/routes/posts/__lang__/page.gen.go` | `__lang__` |
| `src/routes/files/[...path]/+page.svelte` | `.gen/routes/files/___path/page.gen.go` | `___path` |
| `src/routes/(marketing)/+page.svelte` | `.gen/routes/_g_marketing/page.gen.go` | `_g_marketing` |
| `src/routes/api/users/+server.go` | `.gen/routes/api/users/server.gen.go` | `users` |

`hooks.server.go` lives at `src/hooks.server.go` (user-written, not under `.gen/`). Codegen produces `.gen/hooks.gen.go` containing the dispatch wiring that calls user hooks.

## Cross-route manifest

Single top-level `.gen/manifest.gen.go`:

```go
package gen

import (
    page_root       "myapp/.gen/routes"
    page_about      "myapp/.gen/routes/about"
    page_posts_slug "myapp/.gen/routes/posts/_slug_"
)

var Routes = []Route{
    {Path: "/",            Page: page_root.Page{}},
    {Path: "/about",       Page: page_about.Page{}},
    {Path: "/posts/:slug", Page: page_posts_slug.Page{}, Params: []string{"slug"}},
}
```

Router (`packages/sveltego/runtime/router`) consumes this slice at startup, builds radix tree.

## Implementation outline

1. `core/manifest` walks `src/routes/`, produces a `RouteSet` data structure.
2. `core/codegen.Emit(routeSet, outDir)` writes per-route `.gen/` files plus root manifest.
3. `core/codegen.PackageName(srcPath)` is the deterministic encoder. Round-trip test: `Decode(Encode(x)) == x` for all known route shapes.

## References

- SvelteKit routing types: https://svelte.dev/docs/kit/advanced-routing
- Go identifier rules: https://go.dev/ref/spec#Identifiers
