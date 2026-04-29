# ADR 0005 — Non-Goals

- **Status:** Accepted
- **Date:** 2026-04-29
- **Issue:** [binsarjr/sveltego#94](https://github.com/binsarjr/sveltego/issues/94)

## Decision

Eleven enumerated non-goals for v1.0. Each is something SvelteKit ships (or is widely expected) that sveltego deliberately excludes from core, with a documented alternative. The Cloudflare Workers adapter is **in scope** (`packages/adapter-cloudflare`); only Vercel/Netlify Functions adapters are excluded. Re-evaluation cadence: yearly, or sooner via a superseding RFC.

## Rationale

- **No JS runtime on the server.** Universal load, `<script context="module">`, runtime template interpretation, JSDoc-driven type discovery, and `vitePreprocess` all require either a JS runtime or a JS-plugin host on the server. They contradict the core invariant (see ADR 0001, ADR 0002).
- **BYO over framework primitive.** i18n, form validation, and WS/SSE shapes vary so much per app that any first-party choice is either too narrow or too generic. Go ecosystem already has strong libraries (`go-i18n`, `go-playground/validator`, `gorilla/websocket`).
- **Browser-only features deferred.** View Transitions is a JS-only DOM API. It only makes sense after the SPA router (#37) ships in v0.3; revisit post-v1.0.
- **Per-route code splitting is enough.** Deeper dynamic-import splitting hits 5% of cases; defer to a future RFC if demand surfaces.
- **Generic Go runtime > platform-specific JS adapters.** Vercel/Netlify Functions adapters reduce to "deploy a Go binary" — both platforms already support that generically. Cloudflare Workers, by contrast, has unique edge runtime constraints worth a dedicated adapter.

## Locked sub-decisions

1. **All 8 original categories retained unchanged.**

   Universal load (`+page.ts` / `+layout.ts`), `<script context="module">`, WS/SSE primitives, Vercel/Netlify Functions adapters, `vitePreprocess` pipeline, JSDoc-driven type discovery, deep dynamic-import code splitting, runtime template interpretation. Reasoning frozen as published in #94.

2. **Added category 9 — View Transitions API.**

   SvelteKit equivalent: `onNavigate` + browser View Transitions. Pure browser API requiring JS-only DOM diffing across SPA navigation. Sveltego's hydration story is server-rendered HTML + minimal client; layered View Transitions would couple hydration to a JS-only feature. Do instead: full-page reloads with cached SSR; revisit post-v1.0 if SPA router (#37) ships and demand emerges.

3. **Added category 10 — Built-in i18n primitives.**

   No SvelteKit-core equivalent (community uses `paraglide-js`, `inlang`). i18n shape varies wildly per app — first-party choice would either be too narrow or reimplement Go ecosystem libs. Do instead: BYO — `nicksnyder/go-i18n` server side, `paraglide-js` or static catalogs client side. Standard `Accept-Language` parsing in `+server.go` / `hooks.server.go`.

4. **Added category 11 — Built-in form-validation library.**

   No SvelteKit-core equivalent (community uses `sveltekit-superforms` + `zod`). Validation belongs in Go domain code, not a framework primitive. Do instead: `go-playground/validator/v10` for struct validation; return field-tagged errors from `Actions()`; Svelte components read tagged error map. Pattern documented in #21 (form actions issue).

5. **Cloudflare adapter clarified as in-scope.**

   Risk note rewritten in #94: "The Cloudflare Workers adapter is in scope and ships in `packages/adapter-cloudflare`; only Vercel/Netlify Functions are excluded." Cloudflare Workers' edge runtime constraints (CPU limits, fetch-event model, Wasm) justify a dedicated adapter; Vercel/Netlify reduce to "run a Go binary" which both platforms already support generically.

6. **Re-evaluation cadence: yearly**, or sooner on a maintainer-led RFC superseding this one. Append-only — non-goals graduate via a superseding ADR, never by silent edit.

## Implementation outline

- **Universal load (#1)** — codegen rejects `+page.ts` / `+layout.ts` files; only `+page.server.go` / `+layout.server.go` recognized. Cross-ref ADR 0003 (file convention).
- **`<script context="module">` (#2)** — parser does not recognize the `context="module"` attribute; user code goes in `src/lib/`.
- **WS/SSE (#3)** — no `kit.WebSocket` or `kit.EventStream` package. Users wire `gorilla/websocket` directly inside `+server.go`.
- **Vercel/Netlify Functions (#4)** — `packages/` ships `adapter-server`, `adapter-static`, `adapter-cloudflare`, `adapter-lambda`, `adapter-docker`. No `adapter-vercel` / `adapter-netlify`.
- **`vitePreprocess` (#5)** — codegen does not accept preprocessor plugins. Tailwind/PostCSS/SCSS run through Vite for the client bundle only.
- **JSDoc-driven types (#6)** — codegen reads Go types via `go/ast`. No JSDoc parser, no `.d.ts` emission.
- **Deep dynamic-import splitting (#7)** — Vite per-route chunk only. No `import('./Heavy.svelte')` AST rewriting in codegen.
- **Runtime template interpretation (#8)** — no template walker on the request path. All decisions lowered to Go control flow at codegen.
- **View Transitions (#9)** — no `kit.OnNavigate` API, no codegen path. SPA router (#37) ships without it; revisit post-v1.0.
- **i18n (#10)** — no `kit.I18n` package. CONTRIBUTING.md will document the recommended `go-i18n` + `Accept-Language` pattern when forms/hooks land.
- **Form validation (#11)** — no `kit.Validate` package. Form actions (#21) document the `go-playground/validator` + tagged error map pattern.

## References

- [Issue #94 — RFC: explicit non-goals](https://github.com/binsarjr/sveltego/issues/94)
- [ADR 0001 — Parser Strategy](0001-parser-strategy.md)
- [ADR 0002 — Template Expression Syntax](0002-expression-syntax.md)
- [ADR 0003 — File Convention](0003-file-convention.md)
- [ADR 0004 — Codegen Output Shape](0004-codegen-shape.md)
- SvelteKit page options (compare): https://svelte.dev/docs/kit/page-options
- `go-i18n`: https://github.com/nicksnyder/go-i18n
- `go-playground/validator`: https://github.com/go-playground/validator
- `gorilla/websocket`: https://github.com/gorilla/websocket
