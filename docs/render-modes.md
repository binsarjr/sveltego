# Render modes

sveltego supports four render modes. Pick per route via [`kit.PageOptions`](../packages/sveltego/exports/kit/page_options.go) declared as exported constants in `_page.server.go` (or `_layout.server.go` — layouts cascade, page-level overrides win).

**SSR is the default.** `kit.DefaultPageOptions()` returns `SSR: true`, matching SvelteKit's convention. New users get fast first paint and SEO-friendly HTML out of the box; opt out per route only when the page genuinely benefits from a different mode.

| Mode    | When to use                          | Page-options recipe                                | Runtime path                                | Hydration |
|---------|--------------------------------------|----------------------------------------------------|---------------------------------------------|-----------|
| **SSR** (default)  | Dynamic, fresh data per request | Default — no opt-in needed (`SSR: true` is default)  | Go `Render()` emits HTML; client hydrates    | yes       |
| **SSG** | Marketing, docs, blog                | `kit.PageOptions{Prerender: true}`                 | Build-time HTML; static handler at runtime  | yes       |
| **SPA** | Authenticated dashboards, consoles   | `kit.PageOptions{SSR: false}`                      | App shell + JSON payload; client renders    | yes       |
| **Static** | No per-page data                  | No `_page.server.go`; pure `.svelte` only          | App shell + empty payload; client renders   | no        |

## Decision tree

```
Need per-request data from Go?
├── No → does the page have any server data at all?
│        ├── No → Static (just .svelte, no _page.server.go)
│        └── Yes, but it never changes → SSG (Prerender: true)
└── Yes → can the page be rendered ahead of time?
         ├── Yes (content site, blog, docs)        → SSG (Prerender: true)
         ├── No, needs SEO + LCP                   → SSR (default)
         └── No, behind login + zero SEO concern   → SPA (SSR: false)
```

If you cannot decide, leave the defaults alone. SSR is the default for a reason: it serves dynamic data at the lowest first-paint cost without forcing a client boot before render.

## Per-mode walkthrough

### SSR (default)

Use when each request needs fresh data and the page is publicly indexable (homepage, product page, search results). Go renders HTML at request time; the client hydrates after.

**File layout:**

```
src/routes/posts/[id]/
├── _page.svelte
└── _page.server.go
```

**`_page.server.go`:**

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
    Post Post `json:"post"`
}

func Load(ctx kit.LoadCtx) (PageData, error) {
    id := ctx.Params["id"]
    return PageData{Post: fetchPost(ctx, id)}, nil
}
```

**`_page.svelte`:**

```svelte
<script lang="ts">
  let { data } = $props();
</script>

<article>
  <h1>{data.post.title}</h1>
  <p>{data.post.body}</p>
</article>
```

**Runtime:** the pipeline runs `Load`, the build-time-generated `Render(payload, props)` (transpiled from `svelte/server` JS via [ADR 0009](../tasks/decisions/0009-ssr-option-b.md) Option B) emits the HTML, and the client hydrates from the inlined payload.

**Trade-off:** best LCP and SEO; pays Go CPU per request. Performance target is ≥10k rps p50 on mid-complexity pages (RFC #421 acceptance criterion).

### SSG (Prerender)

Use when the page rarely changes (marketing, docs, blog, changelog). The build emits static HTML once; the runtime serves it through a static handler at zero per-request CPU.

**`_page.server.go`:**

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Prerender = true

type PageData struct {
    Title string `json:"title"`
    Body  string `json:"body"`
}

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{Title: "About", Body: loadAboutBody()}, nil
}
```

**Runtime:** at `sveltego build`, a Node sidecar invokes `svelte/server` once per route with the data returned by `Load`, writes the HTML to `static/`, and the deployed binary serves the file directly. No Go template work at request time.

**Trade-off:** zero per-request cost; data is frozen at build time. Performance target: 20–40k rps for the static handler.

`PrerenderAuto: true` keeps `Prerender` opportunistic — the build prerenders only when the route has no dynamic params and no server `Load`, otherwise the route falls through to SSR. `PrerenderProtected: true` (#187) emits the static HTML but gates it behind the configured `PrerenderAuthGate` at request time.

### SPA (SSR: false)

Use when SEO is irrelevant (authenticated dashboards, internal consoles), the page benefits from rich client-side state, and the user is already past a login wall.

**`_page.server.go`:**

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const SSR = false

type PageData struct {
    User User `json:"user"`
}

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{User: currentUser(ctx)}, nil
}
```

**Runtime:** the server returns the app shell (`app.html`) plus the JSON hydration payload from `Load`. The client mounts and renders. No HTML is generated server-side.

**Trade-off:** server CPU per request is minimal — JSON encoding only — but first paint waits on the client bundle.

### Static (no `_page.server.go`)

Use when the page has no server data at all (a true static page: about, terms of service, simple landing). Drop the `_page.server.go` file entirely; only `_page.svelte` is needed.

**File layout:**

```
src/routes/about/
└── _page.svelte
```

**`_page.svelte`:**

```svelte
<h1>About sveltego</h1>
<p>SvelteKit-shape framework for Go.</p>
```

**Runtime:** the server returns the app shell with an empty payload; the client mounts and renders the template against `data = {}`. No Go-side `Load`, no Render, no hydration data.

**Trade-off:** simplest possible route; the page is still client-rendered, so first paint waits on the client bundle. For a true zero-JS page, combine with `Prerender: true` and a `_page.server.go` that returns an empty struct from `Load`.

## Common pitfalls

- **Layout cascade silently downgrades.** A `_layout.server.go` declaring `const SSR = false` makes every page underneath SPA-mode unless they explicitly set `const SSR = true`. The cascade is per-field. When in doubt, run `sveltego compile` and inspect the manifest.
- **`Prerender` skips Handle hooks.** Build-time prerender does not run `hooks.server.go`'s `Handle`, so `Locals` is empty during `Load`. Codegen warns when a prerendered route's `Load` reads `ctx.Locals`. Move the read to a non-prerendered route or seed the data into the build script.
- **`SSROnly: true` disables hydration.** The page renders HTML on the server but ships no client bundle. Use only for pages that need zero interactivity (printable receipts, RSS-rendered HTML mirrors).
- **`<!-- sveltego:ssr-fallback -->` requires Node at request time.** This opt-in escape hatch routes the page through the long-running Node sidecar instead of the Go transpiler. Use only when the page's compiled JS shape genuinely cannot be lowered to Go (rare). The deploy still ships the Go binary; the sidecar is supervised separately. See [`packages/sveltego/runtime/svelte/fallback/STABILITY.md`](../packages/sveltego/runtime/svelte/fallback/STABILITY.md).
- **Mixing modes in a layout chain.** A layout's data flows into every child page regardless of mode. SSG children that read SSR-only layout fields fail at build time with a clear error.
- **Authoring SPA + Static together.** Both result in the client doing the rendering work. Pick Static when the page has no server data (no `_page.server.go`); pick SPA (`SSR: false`) when the page has server data that should not be SSR'd.

## App-shell template (`app.html`)

Every sveltego app ships an `app.html` shell with two placeholders the runtime fills on every render:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>my app</title>
  %sveltego.head%
</head>
<body>
  %sveltego.body%
</body>
</html>
```

What expands where:

- **`%sveltego.head%`** — head-belonging tags. Per-route stylesheet links and `<link rel="modulepreload">` hints so the browser can discover and parallel-fetch the chunks during HTML parse, plus any `<svelte:head>` content from the page and its layout chain (deduped `<title>`, meta tags, etc.).
- **`%sveltego.body%`** — body content, expanded inline at the placeholder location, then the inline JSON hydration payload (`<script id="sveltego-data" type="application/json">…</script>`), then the per-route entry `<script type="module">`. The entry script lands at the **end of `<body>`** (just before `</body>`), matching SvelteKit's `%sveltekit.body%` convention. Putting the entry script after the SSR'd body lets the browser paint the page before any JS chunk executes (better LCP / FCP) and avoids hydration-timing races where modules try to mount markup the browser hasn't finished parsing.

The `modulepreload` hints stay in `<head>` regardless — they are zero-execution, parse-time discovery only, and parallel-fetching the chunks during HTML parse is strictly faster than discovering them at end of body.

For streaming (`kit.Streamed[T]` in a `Load` result) the order is the same: head + SSR body + payload first, then `<script>__sveltego__resolve(…)</script>` patches as each stream resolves, then the entry `<script type="module">` at end of body. The client glue queues incoming resolve calls until the entry module loads.

## adapter-static

`packages/adapter-static` is currently a stub tracked by [#65](https://github.com/binsarjr/sveltego/issues/65). Until it lands, a static-output deploy ships the entire `Prerender: true` route set by running `sveltego build` and uploading the contents of the build's `static/` directory; the SSR Go binary is not needed if every route is `Prerender: true`. This is awkward but accurate — the prerender engine is live; the adapter wrapper is the missing piece.

## See also

- [`packages/sveltego/exports/kit/page_options.go`](../packages/sveltego/exports/kit/page_options.go) — full `PageOptions` surface and field-by-field godoc.
- [ADR 0008 — Pure-Svelte pivot](../tasks/decisions/0008-pure-svelte-pivot.md) — template invariant.
- [ADR 0009 — SSR Option B](../tasks/decisions/0009-ssr-option-b.md) — build-time JS-to-Go transpile that powers the SSR mode.
- [`playgrounds/basic/README.md`](../playgrounds/basic/README.md) — minimal SSR walkthrough.
- [RFC #421](https://github.com/binsarjr/sveltego/issues/421) — Option A/B/C analysis.
