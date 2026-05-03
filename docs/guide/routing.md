---
title: Routing
order: 20
summary: File-based routing — _page.svelte, _page.server.go, _server.go, params, groups.
---

# Routing

sveltego uses file-based routing under `src/routes/`. The conventions match SvelteKit. Server-side Go files in `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`); Go's default toolchain skips files whose names start with `_`, so no build tag is required there (RFC #379 phase 1b). Param matchers under `src/params/<name>/<name>.go` are also tag-free — codegen mirrors them into `.gen/paramssrc/<name>/` and `gen.Matchers()` registers them on the runtime automatically (#511). No user `.go` file needs `//go:build sveltego` (#527); existing projects that still carry it keep working — codegen reads via `go/parser`, which ignores build tags.

## Files

| File | Purpose |
|---|---|
| `_page.svelte` | Pure Svelte/JS/TS template. `data` props arrive from `_page.server.go`'s `Load`. |
| `_page.server.go` | Page server module: `Load`, `Actions`. The `_` prefix hides it from Go's default toolchain. |
| `_layout.svelte` | Layout chain. Wraps descendant `_page.svelte`. |
| `_layout.server.go` | Layout-level `Load` cascading to children. |
| `_server.go` | REST endpoints (`GET`, `POST`, ...). No template. |
| `_error.svelte` | Error boundary for the subtree. |
| `_page@.svelte` | Layout reset — opt out of the parent chain. |

## Path patterns

| Pattern | Example | Description |
|---|---|---|
| `[param]` | `blog/[slug]/_page.svelte` | Required parameter. |
| `[[optional]]` | `[[lang]]/_page.svelte` | Optional segment. |
| `[...rest]` | `docs/[...path]/_page.svelte` | Catch-all rest. |
| `(group)` | `(marketing)/_layout.svelte` | Group with no URL segment. |
| `[name=matcher]` | `users/[id=int]/_page.svelte` | Param matcher. |

## Param matchers

Built-in matchers: `int`, `uuid`, `slug`. Add your own under `src/params/`:

```go
package params

func Hex(value string) bool {
  for _, r := range value {
    switch {
    case r >= '0' && r <= '9':
    case r >= 'a' && r <= 'f':
    default:
      return false
    }
  }
  return true
}
```

Reference it as `[id=hex]`.

## Endpoints

`_server.go` exposes named handlers per HTTP method (the `_` prefix hides it from Go's default toolchain — no build tag needed):

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func GET(ev *kit.RequestEvent) *kit.Response {
  return kit.JSON(200, kit.M{"ok": true})
}

func POST(ev *kit.RequestEvent) *kit.Response {
  // read ev.Request.Body, return *kit.Response
  return kit.NoContent()
}
```

If a method is not exported, the router answers `405 Method Not Allowed` with a sorted `Allow` header.

## Layouts and reset

A `_page@.svelte` (note the trailing `@`) opts out of the parent layout chain — useful for full-bleed pages inside a section that otherwise applies a marketing shell.

## Out of scope

`+page.ts` / `+layout.ts` (universal Load) is **not** supported. Loaders are server-only by design. See [non-goals](/guide/faq#non-goals).
