---
title: Load
order: 30
summary: Server-side Load functions, parent layout data, request-scoped fetch.
---

# Load

`Load` is the server-side data loader for a page or layout. It receives a `*kit.LoadCtx` and returns a typed `PageData` (or `LayoutData`) plus an error.

## Page load

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
  Title string `json:"title"`
  Posts []Post `json:"posts"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
  posts, err := db.RecentPosts(ctx.Request.Context())
  if err != nil {
    return PageData{}, err
  }
  return PageData{Title: "Recent posts", Posts: posts}, nil
}
```

`_page.server.go` does not need a `//go:build sveltego` tag — the `_` prefix already hides the file from Go's default toolchain. Inside the template, fields are accessed via the `data` prop with names driven by JSON tags: `{data.title}`, `{data.posts}`. `null` not `nil`. Codegen reads this `Load` return type and emits a sibling `_page.svelte.d.ts` so Svelte LSP autocompletes `data.*` end to end.

## Layout load

`_layout.server.go` works the same way and exports `LayoutData`. The pipeline runs each layer's `Load` outer-to-inner. Children read the immediate parent through `ctx.Parent()`:

```go
parent := ctx.Parent().(rootlayout.LayoutData)
```

The cast is explicit so type changes show up at build.

## Errors and short-circuits

`Load` returns `error` for typed control flow:

```go
return PageData{}, kit.Error(404, "post not found")
return PageData{}, kit.Redirect(303, "/login")
```

`kit.Redirect` and `kit.Error` carry HTTP semantics; the pipeline routes them to the appropriate response. See [Errors](/guide/errors) for the full set.

## Fetch through hooks

`ctx.Request` is the live request; outbound HTTP from `Load` should go through `ev.Fetch(req)` so `HandleFetch` can intercept it. Generated wrappers reach for that method when available — write your own outbound calls the same way:

```go
req, _ := http.NewRequestWithContext(ctx.Request.Context(), "GET", "https://api/...", nil)
res, err := ev.Fetch(req)
```

## Streaming

Defer slow work via `kit.Stream`:

```go
return PageData{
  Hero:    fast,
  Reviews: kit.Stream(func() ([]Review, error) { return slow() }),
}, nil
```

The render path emits a placeholder, flushes the shell, then patches the slot when the goroutine resolves. Default timeout is `kit.DefaultStreamTimeout` (30s).

## What `Load` cannot do

- It cannot run on the client. Loaders are server-only.
- It must not mutate `RequestEvent.Locals` after `Handle` finished — read only.
- It cannot write to `http.ResponseWriter`; responses come from the page template or a `_server.go` handler.
