---
title: FAQ
order: 95
summary: Common questions and the canonical non-goals list.
---

# FAQ

## Why not embed a JS runtime?

Adapters layered on top of SvelteKit-the-JS-server inherit every limitation of the chosen runtime: foreign concurrency model, no goroutines, no `context.Context`, IPC overhead per request. Going faster than the runtime is impossible.

The SvelteKit *shape* (file convention, mental model) is what users want — not the SvelteKit *implementation*. Rewriting the shape in Go unlocks the standard library, goroutines, and a target throughput of 20–40k rps for mid-complexity SSR.

## Why Svelte 5 only?

Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) are easier to compile to Go than legacy `$:` reactivity. Locking to Svelte 5 keeps the codegen surface tractable and avoids a forever-deprecation tail.

## Can I write TypeScript in `<script>`?

No. `<script lang="go">` is the only supported language. The expression validator runs `go/parser` on every mustache.

## Can I use my SvelteKit components?

Markup mostly. Mustaches must be rewritten to Go. Component scripts must be rewritten to Go. The result is a port, not a drop-in.

## Where is hot reload?

`sveltego dev` is the planned watch + regenerate + HMR proxy command, but today it is a stub (deferred to v0.3, [#42](https://github.com/binsarjr/sveltego/issues/42)). For now, re-run `sveltego compile` after editing templates and restart the server. Once it ships, the Go server restart will be the primary feedback loop, with browser HMR for the Vite client bundle handled by Vite as in any other project.

## How do I serve static files?

`http.FileServer` against the Vite output (`dist/`). sveltego does not bundle a static handler; you pick.

## How do I add WebSockets?

Bring `gorilla/websocket` (or any other library). Mount the upgrade handler in your server. WebSocket primitives are not in the `kit` package by design.

## How do I add i18n?

Bring `go-i18n`. Pass the localizer through `Locals` from `Handle`. i18n is not in `kit` by design.

## Non-goals

The following are explicitly **not** going to ship in `kit` or core. See `tasks/decisions/0005-non-goals.md` and the project README for canonical reasoning.

- Universal (shared client+server) `Load` — server-only by design.
- `<script context="module">`.
- WebSocket / SSE primitives in core.
- Vercel / Netlify Functions adapters (Cloudflare Workers adapter is in scope).
- vitePreprocess / arbitrary preprocessor pipeline.
- JSDoc-driven type discovery (Go types only).
- Deep dynamic-import code splitting beyond per-route.
- Runtime template interpretation.
- View Transitions API in core.
- Built-in i18n primitives.
- Built-in form-validation library.
- Svelte 4 legacy reactivity (`$:`, store autoload).
- Server-side dynamic JS execution.
- Native Go bundler replacing Vite for the client.
- Multi-tenant / RBAC primitives in `kit`.
