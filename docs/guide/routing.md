---
title: Routing
order: 20
summary: File-based routing — +page.svelte, page.server.go, server.go, params, groups.
---

# Routing

sveltego uses file-based routing under `src/routes/`. The conventions match SvelteKit, with one constraint: every Go file in `src/routes/**` and `src/params/**` MUST start with `//go:build sveltego` so Go's default toolchain skips them. Codegen reads them through `go/parser` directly.

## Files

| File | Purpose |
|---|---|
| `+page.svelte` | SSR template. Mustache expressions are Go. |
| `page.server.go` | Page server module: `Load`, `Actions`. No `+` prefix; identified by `//go:build sveltego`. |
| `+layout.svelte` | Layout chain. Wraps descendant `+page.svelte`. |
| `layout.server.go` | Layout-level `Load` cascading to children. No `+` prefix. |
| `server.go` | REST endpoints (`GET`, `POST`, ...). No template, no `+` prefix. |
| `+error.svelte` | Error boundary for the subtree. |
| `+page@.svelte` | Layout reset — opt out of the parent chain. |

## Path patterns

| Pattern | Example | Description |
|---|---|---|
| `[param]` | `blog/[slug]/+page.svelte` | Required parameter. |
| `[[optional]]` | `[[lang]]/+page.svelte` | Optional segment. |
| `[...rest]` | `docs/[...path]/+page.svelte` | Catch-all rest. |
| `(group)` | `(marketing)/+layout.svelte` | Group with no URL segment. |
| `[name=matcher]` | `users/[id=int]/+page.svelte` | Param matcher. |

## Param matchers

Built-in matchers: `int`, `uuid`, `slug`. Add your own under `src/params/`:

```go
//go:build sveltego

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

`server.go` exposes named handlers per HTTP method:

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

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

A `+page@.svelte` (note the trailing `@`) opts out of the parent layout chain — useful for full-bleed pages inside a section that otherwise applies a marketing shell.

## Out of scope

`+page.ts` / `+layout.ts` (universal Load) is **not** supported. Loaders are server-only by design. See [non-goals](/guide/faq#non-goals).
