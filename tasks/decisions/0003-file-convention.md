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
| `src/routes/api/users/server.go` | `.gen/routes/api/users/server.gen.go` | `users` |

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

### Amendments

**2026-04-30 (Phase 0g):** Group encoding canonical form is `_g_marketing` (no trailing underscore). The original locked sub-decision Q4 mapped `(marketing) → _g_marketing/`; phrase preserved here for clarity. Scanner enforces; no trailing underscore.

**2026-04-30 (Phase 0g):** Manifest filename locked at `.gen/manifest.gen.go`. Generator name: `internal/codegen.GenerateManifest`. Manifest emits a `Routes() []router.Route` factory (not a `var Routes`) so each call returns a fresh slice that the dispatcher can wrap in `router.NewTree`. This matches the `Page{}.Render` method-on-struct convention from ADR 0004.

**2026-04-30 (Phase 0g):** Built-in param matchers `int`, `uuid`, `slug` ship in `exports/kit/params/`. `DefaultMatchers()` returns a fresh map composable with user-discovered matchers. Matcher dispatch in `runtime/router/match.go` invokes `matchers[name].Match(value)`; missing matcher names fail at `router.NewTree` build time.

**2026-04-30 (Phase 0i-fix, supersedes #108):** User `.go` filename convention amended.

- Drop `+` prefix on user `.go` files: `+page.server.go` → `page.server.go`,
  `+layout.server.go` → `layout.server.go`, `+server.go` → `server.go`.
- All sveltego user `.go` files MUST start with `//go:build sveltego` build
  constraint so Go's default toolchain (`go list / vet / build / test /
  golangci-lint`) skips them. Codegen reads them via `go/parser` directly
  and surfaces a warning diagnostic when the constraint is missing.
- `.svelte` filenames keep `+` prefix unchanged (`+page.svelte`,
  `+layout.svelte`, `+error.svelte`, `+page@.svelte`).
- `[slug]/`, `(group)/`, `[[opt]]/`, `[...rest]/` directory naming unchanged
  for user source. With the `//go:build sveltego` tag in place, Go silently
  skips these directories instead of erroring on the bracketed path token.
- Codegen emits a user-source mirror tree at `.gen/usersrc/<encoded path>/`
  containing byte-equivalent copies of user `.go` files with the build
  constraint stripped and the package clause set to the encoded directory
  name. Generated wire glue (`.gen/routes/<encoded>/wire.gen.go`) imports
  from the mirror, never from the user's `src/` tree (whose dir names
  contain invalid Go import path characters like `[`).
- Manifest emits per-route `render__<alias>` adapters wrapping
  `Page{}.Render` in a `(w, ctx, data any) error` signature so the route
  table satisfies `router.PageHandler`. Type assertion happens inside the
  adapter; mismatches return a descriptive `fmt.Errorf` instead of panicking.
- Wire emits both `Load` and `Actions` wrappers unconditionally for routes
  with `HasPageServer`. `Actions()` becomes a `nil`-returning stub when the
  user file does not declare it; the manifest references the symbol
  unconditionally, so the stub keeps the build clean.

This unblocks #23 (hello-world) and closes #106, #107, #108.
