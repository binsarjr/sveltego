# Issue #518 plan — preserve layout state across cross-route same-chain SPA navs

## Decision

**Option B (sidecar/transpile-rendered shared wrapper module).**

Why: lower risk than Option A's transpiler-side changes, hydration parity provable in a 5-line spike (`/tmp/svelte-spike/`), and only needs a one-line marker emit around `page()` in the chain composer.

## Spike result

Compiled `<Page data={...}/>` where `Page` is a `$props()` variable using svelte 5.55.5:

```js
if (Page) {
  $$renderer.push('<!--[-->');
  Page($$renderer, { data });
  $$renderer.push('<!--]-->');
} else {
  $$renderer.push('<!--[!-->');
  $$renderer.push('<!--]-->');
}
```

Svelte 5 treats `<Page />` where `Page` is bound to a runtime value as a dynamic
component AUTOMATICALLY — no `<svelte:component>` syntax needed (and that
syntax is deprecated upstream). Hydration walker reads
`<!--[-->...<!--]-->` markers. Wrapper-store rune holds the page module
reference and the SSR pipeline must emit those markers around the
page-body slot for hydration parity.

## Architecture

1. **One wrapper per chainKey** (was per route). Lives at
   `.gen/client/__chain/<chainKey>/wrapper.svelte`. Imports layouts
   statically (chain is fixed per key). Renders the page slot via
   `<Page data={...} form={...}/>` where `Page` is read from the
   wrapper-state rune. No static page import.
2. **Wrapper-state rune extended** with a `Page` field
   (`Component | null`).
3. **Per-route entry.ts** imports the page module directly, seeds
   `_setWrapperState({Page, data, layoutData, form})` BEFORE
   hydrate/mount, then mounts the chain wrapper. (Same-chain entries
   import the same wrapper module, only the page module differs.)
4. **Router** owns two loader maps: `wrapperLoaders[chainKey]` and
   `pageLoaders[routeId]`. On nav within the same chain → load page
   module → write `{Page, data, ...}` to wrapper-state. NO unmount.
   Across chains → unmount + mount destination chain wrapper. Routes
   with no chain mount the page directly (unchanged).
5. **SSR pipeline** — `emitChainBody` writes
   `w.WriteString(server.BlockOpen)` before the inner `page()` call and
   `w.WriteString(server.BlockClose)` after. Wraps both transpiled and
   fallback page bodies uniformly (fallback's `StripFragmentMarkers`
   keeps removing the sidecar's own outer markers so we add ours
   exactly once around the stripped body).

## Files touched

- `packages/sveltego/internal/vite/wrapper.go` — rewrite for
  per-chain wrapper, extend wrapper-store with `Page` field, add
  `GenerateChainWrapper` helper.
- `packages/sveltego/internal/vite/wrapper_test.go` — update + add
  cross-route preservation cases.
- `packages/sveltego/internal/vite/cliententry.go` — entry imports
  page directly; seeds `Page` in wrapper-state.
- `packages/sveltego/internal/vite/cliententry_test.go` — adapt.
- `packages/sveltego/internal/vite/router.go` — replace single
  loader map with `wrapperLoaders` + `pageLoaders` + chain-aware
  swap path.
- `packages/sveltego/internal/vite/router_test.go` — adapt.
- `packages/sveltego/internal/codegen/build.go` — emit shared chain
  wrappers under `.gen/client/__chain/<chainKey>/`; client entry math
  changes; pass `pageLoaders` map distinct from `wrapperLoaders`.
- `packages/sveltego/internal/codegen/build_test.go` — adapt.
- `packages/sveltego/internal/codegen/manifest.go` — `emitChainBody`
  emits `BlockOpen` / `BlockClose` around the page() call.
- `packages/sveltego/internal/codegen/wire_test.go` (and any chain
  golden tests) — adapt.
- `packages/sveltego/runtime/svelte/server` — `BlockOpen`/`BlockClose`
  consts already exist; nothing new.
- Kitchen-sink layout — add `let count = $state(0)` + `<button>` for
  E2E proof.
- (Optional) `scripts/hydration-smoke.mjs` — extend with cross-route
  nav assertion.

## Acceptance criteria mapping

1. `let count = $state(0)` in `_layout.svelte` survives `/post/1 →
   /post/2`: chain wrapper instance is reused, layout's `$state` runes
   persist.
2. SSR markup matches client hydration: chain composer emits the
   `<!--[-->...<!--]-->` markers the dynamic page slot's hydration
   walker expects.
3. Same-route refresh still works: the existing `_setWrapperState`
   write-through path is unchanged; we just additionally write `Page`
   in the seed (passing the same module reference is a no-op).
4. Cross-chain nav still remounts: nav code checks
   `nextChainKey !== currentChainKey` → unmount path.
