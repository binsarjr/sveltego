---
title: Migration from SvelteKit
order: 90
summary: What carries over, what changes, what does not exist in sveltego.
---

# Migration from SvelteKit

sveltego mirrors SvelteKit's *shape*, not its *implementation*. The file conventions and mental model are the same; `.svelte` files are 100% pure Svelte/JS/TS as in any SvelteKit project. The break is on the server side: `_page.server.go` replaces `+page.server.ts`, and a few SvelteKit features are explicit non-goals.

## What carries over

| SvelteKit | sveltego |
|---|---|
| `+page.svelte` | `_page.svelte` (identical content — pure Svelte/JS/TS) |
| `+page.server.ts` | `_page.server.go` (`_` prefix auto-skips Go toolchain) |
| `+layout.svelte` | `_layout.svelte` (identical content) |
| `+layout.server.ts` | `_layout.server.go` (`_` prefix auto-skips Go toolchain) |
| `+server.ts` | `_server.go` (`_` prefix auto-skips Go toolchain) |
| `+error.svelte` | `_error.svelte` |
| `hooks.server.ts` | `hooks.server.go` (still needs `//go:build sveltego` — no `_` prefix) |
| `[param]`, `[[opt]]`, `[...rest]`, `(group)` | identical |
| `$lib` alias | `$lib` alias (resolves to `src/lib`) |
| `$env/static/private`, `$env/static/public`, `$env/dynamic/*` | `kit/env` package |
| Svelte 5 runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) | identical |
| Form actions, `use:enhance`, `<form>` POST | identical on the client; Go on the server |

## What changes

- **Server language.** `_page.server.go` is Go, not TypeScript. `Load`, `Actions`, and REST handlers (`GET`, `POST`, ...) are Go functions.
- **Templates stay pure.** `.svelte` content is unchanged from SvelteKit — `let { data } = $props()`, `{data.user.name}`, `{#if data.posts}`. No Go syntax in templates.
- **Field naming.** Field names follow JSON tags from the Go side — typically camelCase: `{data.userName}` (with `UserName string \`json:"userName"\`` on the Go struct).
- **Nullability.** `null` and `undefined` in Svelte; `nil` and zero values on the Go side. JSON tags determine serialization (use `,omitempty` to drop zero values).
- **Errors.** Idiomatic Go errors on the server. Return `kit.Redirect(303, "/login")` or `kit.Error(404, "not found")` from `Load` instead of throwing.
- **Form actions.** `kit.ActionResult` is a sealed sum: `ActionData`, `ActionFailData`, `ActionRedirectResult`. Construct via `kit.ActionDataResult`, `kit.ActionFail`, `kit.ActionRedirect`.
- **Build constraint.** Files under `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`); Go's default toolchain skips files whose names start with `_`, so no `//go:build sveltego` constraint is required there. Files under `src/params/**` and `hooks.server.go` (project root) MUST still start with `//go:build sveltego` because their filenames have no `_` prefix. Codegen reads every user `.go` file through `go/parser`.
- **Type sharing.** Codegen reads your Go `Load` return type and emits a sibling `_page.svelte.d.ts` so Svelte LSP autocompletes `data.*` end to end. No manual type duplication.

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

```svelte
<!-- +page.svelte -->
<script lang="ts">
  let { data } = $props();
</script>

<h1>{data.post.title}</h1>
<article>{@html data.post.body}</article>
```

### sveltego

`src/routes/posts/[slug]/_page.server.go`:

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
  Post Post `json:"post"`
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

`src/routes/posts/[slug]/_page.svelte` (pure Svelte — copy from SvelteKit unchanged):

```svelte
<script lang="ts">
  let { data } = $props();
</script>

<h1>{data.post.title}</h1>
<article>{@html data.post.body}</article>
```

## Migrating from the legacy Mustache-Go template dialect

If you have an in-progress sveltego project that used the pre-2026-05-01 Mustache-Go template dialect (Go expressions inside `{...}`, PascalCase `{Data.User.Name}`, `nil` instead of `null`), [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md) (RFC #379) replaces it with pure Svelte. Migration recipe:

| Before (Mustache-Go) | After (pure Svelte) |
|---|---|
| `{Data.User.Name}` | `{data.user.name}` (camelCase via JSON tags) |
| `{len(Data.Posts)}` | `{data.posts.length}` |
| `{#if Data.User != nil}` | `{#if data.user}` |
| `{#each Data.Posts as post}` | `{#each data.posts as post}` (identical) |
| `<script lang="go">` block in `.svelte` | Move logic to `_page.server.go` (`Load`) or use `<script lang="ts">` for client-only state |
| `Templates: "go-mustache"` in page options | Removed — pure Svelte is the only mode |

The Go side (`_page.server.go`) does not change. JSON tags on your `PageData` struct drive the field names visible from the template; add `json:"camelCase"` tags to keep client-side names idiomatic.

## Incremental migration

You cannot incrementally swap a SvelteKit project to sveltego — the Go server-side language and codegen pipeline rule that out. Treat sveltego as a port: rewrite `+page.server.ts` and friends in Go, keep the `.svelte` files unchanged.
