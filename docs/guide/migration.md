---
title: Migration from SvelteKit
order: 90
summary: What carries over, what changes, what does not exist in sveltego.
---

# Migration from SvelteKit

sveltego mirrors SvelteKit's *shape*, not its *implementation*. The file conventions and mental model are the same; the language is Go, and a few features are explicit non-goals.

## What carries over

| SvelteKit | sveltego |
|---|---|
| `+page.svelte` | `+page.svelte` |
| `+page.server.ts` | `page.server.go` (no `+` prefix; use `//go:build sveltego`) |
| `+layout.svelte` | `+layout.svelte` |
| `+layout.server.ts` | `layout.server.go` (no `+` prefix; use `//go:build sveltego`) |
| `+server.ts` | `server.go` (no `+` prefix; use `//go:build sveltego`) |
| `+error.svelte` | `+error.svelte` |
| `hooks.server.ts` | `hooks.server.go` |
| `[param]`, `[[opt]]`, `[...rest]`, `(group)` | identical |
| `$lib` alias | `$lib` alias (resolves to `src/lib`) |
| `$env/static/private`, `$env/static/public`, `$env/dynamic/*` | `kit/env` package |

## What changes

- **Language.** `<script>` is Go, not TypeScript. Mustaches are Go expressions.
- **Field naming.** PascalCase fields, not camelCase: `{Data.UserName}`, not `{data.userName}`.
- **Nullability.** `nil`, not `null`. `data == nil`, not `data === undefined`.
- **Length.** `len(xs)`, not `xs.length`.
- **Errors.** Idiomatic Go errors. `kit.Redirect(303, ...)` and `kit.Error(404, ...)` are returned, not thrown.
- **Form actions.** `kit.ActionResult` is a sealed sum: `ActionData`, `ActionFailData`, `ActionRedirectResult`. Construct via `kit.ActionDataResult`, `kit.ActionFail`, `kit.ActionRedirect`.
- **Build constraint.** Every `.go` file under `src/routes/**` and `src/params/**` MUST start with `//go:build sveltego` so Go's default toolchain skips them; codegen reads them through `go/parser`.

## What does not exist

These are explicit non-goals (see [FAQ](/guide/faq#non-goals)):

- **Universal Load** (`+page.ts`, `+layout.ts`). Loaders are server-only.
- **`<script context="module">`.** Deprecated upstream.
- **vitePreprocess** and arbitrary preprocessor pipelines.
- **Svelte 4 legacy reactivity** (`$:`, store auto-subscribe, `export let`). Use runes.
- **WebSocket primitives in core.** Bring `gorilla/websocket`.
- **i18n primitives.** Bring `go-i18n` (or similar).
- **Form-validation library.** Bring `go-playground/validator`.
- **View Transitions API** in core.
- **JS code splitting beyond per-route.**

## Side-by-side example

### SvelteKit

```ts
// +page.server.ts
import { redirect, error } from '@sveltejs/kit';

export async function load({ locals, params }) {
  if (!locals.user) throw redirect(303, '/login');
  const post = await db.post(params.slug);
  if (!post) throw error(404, 'not found');
  return { post };
}
```

### sveltego

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
  Post Post
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
  user, _ := ctx.Locals["user"].(*User)
  if user == nil {
    return PageData{}, kit.Redirect(303, "/login")
  }
  post, err := db.Post(ctx.Request.Context(), ctx.Params["slug"])
  if err != nil {
    return PageData{}, err
  }
  if post == nil {
    return PageData{}, kit.Error(404, "not found")
  }
  return PageData{Post: *post}, nil
}
```

## Incremental migration

You cannot incrementally swap a SvelteKit project to sveltego — the Go expression model and codegen pipeline rule that out. Treat sveltego as a port: rewrite the server side in Go, keep the `.svelte` markup mostly intact, replace mustaches with Go expressions.
