# Stability — sveltego

Last updated: 2026-05-02 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

## Stable

### `kit.PageOptions` — per-route render-mode selector

The mode-selecting fields are **stable** within the pre-alpha window. Behavioral defaults match SvelteKit:

- `kit.DefaultPageOptions()` returns `SSR: true, CSR: true, CSRF: true, TrailingSlash: TrailingSlashNever, Templates: "svelte"`. **SSR is the framework default** for any route that omits the relevant constant.
- `Prerender bool` — opt into SSG. Build-time HTML written to `static/`; runtime serves the file directly.
- `SSR bool` — opt out of server-side rendering by setting `false`. Server returns the app shell + JSON payload; client renders.
- `SSROnly bool` — render server-side and ship no client bundle. Page is non-interactive.
- `CSR bool` — disable client hydration entirely. Server renders; client receives static markup only.
- `PrerenderAuto bool` — opportunistic SSG: prerender only when the route has no dynamic params and no `Load`; otherwise fall through to SSR.
- `PrerenderProtected bool` ([#187](https://github.com/binsarjr/sveltego/issues/187)) — emit static HTML at build time but gate it behind `PrerenderAuthGate` at request time.
- `Templates string` — pipeline pick. `"svelte"` (the default and only supported value as of RFC #379 phase 5) routes through Vite + Svelte for the client and `svelte_js2go` for SSR.
- `CSRF bool` — toggle CSRF protection for form actions.
- `TrailingSlash TrailingSlash` — request-path trailing-slash normalization.

The four render modes the framework supports — **SSR** (default), **SSG** (`Prerender: true`), **SPA** (`SSR: false`), **Static** (no `_page.server.go`) — are stable selections backed by these fields. See [`docs/render-modes.md`](../../docs/render-modes.md) for the full reference.

Layout-level values cascade to descendants per field; page-level values override the cascade. The cascade contract is stable.

## Experimental

### `server.Server.Routes()` and `server.RouteSummary` ([#455](https://github.com/binsarjr/sveltego/issues/455))

Adapter-facing accessor exposing the runtime route table:

- `Routes() []RouteSummary` — projects every registered route into a `RouteSummary{Pattern, Prerender, SSR, DynamicParams, PageOptions}` snapshot. Returned slice is a copy; callers may sort or filter freely.
- `RoutesManifestFilename` (`"routes.json"`) — sidecar written next to `manifest.json` when the user binary runs under `MaybePrerenderFromEnv`. Adapter-static's subprocess runner reads it to enumerate dynamic routes through the binary boundary.

The shape is provisional. The struct may grow new fields and the JSON field names may change once tooling consumers stabilize.

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

(none yet)
