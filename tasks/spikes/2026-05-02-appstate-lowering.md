# Spike: $app/state lowering for SSR Option B (#466)

Date: 2026-05-02
Author: @appstate-lowering
Status: design
Closes: #466

## Problem

`$app/state` runes (`page`, `navigating`, `updated`) are surfaced as a
client-side Svelte 5 module via `internal/vite/state.go` (#463). On the
server-side SSR Option B path (build-time JS-to-Go transpile in
`internal/codegen/svelte_js2go/`), three things prevent these runes from
reaching Go:

1. `recordImport` rejects every import source other than
   `svelte/internal/server` — hits `unknownShape` for `$app/state`.
2. `lowering.go` only knows `data` as a root; expressions like
   `page.url.pathname`, `navigating.current`, `updated.current` flow
   through `recordUnknownRoot` and hard-error.
3. No render-time PageState exists on the Go side. The Render signature
   is `Render(payload, data)` — there's no plumbing to pass URL,
   params, status, etc.

The basic playground escapes this via `<!-- sveltego:ssr-fallback -->`
on `playgrounds/basic/src/routes/appstate/[id]/_page.svelte`, which
routes the request to the long-running Node sidecar (Phase 8). Acceptance
of #466 drops this annotation.

## Surface decisions

### 1. Render signature

Two viable shapes:

**A. Append `pageState` parameter** — the route's typed Render signature
becomes `Render(payload, data PageData, pageState server.PageState)`.

**B. Pull from RenderCtx** — pass nothing extra; have the wire bridge
build a PageState from `ctx *kit.RenderCtx` and pass it as a bare
positional parameter to the typed Render via the bridge.

Going with **A**. Rationale:

- The transpile path doesn't see `ctx`. Render receives `*server.Payload`
  and `data PageData` (and optionally `children` for layouts, per #440).
  Adding a second typed param matches the existing
  `EmitChildrenParam`-shaped extension exactly.
- Layouts already have `EmitChildrenParam` precedent. Same pattern,
  lower friction: a bool option toggles the param.
- The bridge in the manifest (`render__<alias>`) already constructs a
  fresh `server.Payload` per request. Building a PageState alongside it
  from `ctx` is a one-liner.
- Layouts also reference `page.*` runes. The same Render-signature
  extension lives on layouts and error boundaries to cover the full
  surface.

Reject B because: the lowerer must not speak Go-side types like
`server.PageState`. Phase 5's lowering walks identifiers; introducing
implicit identifiers from a non-`scope.dataVar` root smells wrong.
Explicit parameters keep the rewriter simple.

### 2. PageState shape

Mirror the client-side `Page` type from `internal/vite/state.go`. Keep
the field set exactly aligned because the same .svelte template renders
under both paths (server-side initial render + client-side hydration);
divergence here would be a footgun.

```go
// runtime/svelte/server/page_state.go (new)
package server

import "net/url"

// PageState carries the request-scoped state that Svelte's
// `$app/state` runes expose to render-time code. Built from
// kit.RenderCtx by the per-route bridge before each Render call.
//
// Server-side, navigating and updated stay at their idle values
// (Navigation = nil, Updated = false) — both signals belong to the
// client SPA router. The server snapshot is the post-load state.
type PageState struct {
	URL    *url.URL
	Params map[string]string
	Route  PageRoute
	Status int
	Error  *PageError
	// Data is the route's typed PageData widened to any. Codegen
	// passes this through the bridge so `page.data.<x>` resolves
	// to the same map the typed Render parameter saw.
	Data any
	// Form mirrors page.form. Set when an action ran; nil otherwise.
	Form any
	// State is the user-visible history.state.user bag, empty on
	// the server (the server has no history).
	State map[string]any
}

type PageRoute struct {
	ID string // matched route Pattern, "" when no route matched
}

type PageError struct {
	Message string
	Status  int
}
```

Rationale on edge cases:

- `Error` is `*PageError`, not `kit.SafeError` or
  `error`. The page rune surfaces `error.message` and `error.status`;
  using a small named struct keeps the lowering trivial. Error
  boundaries that need richer data still receive `kit.SafeError` via
  the existing `RenderErrorSSR` wire.
- `State` is a `map[string]any` to mirror `Object.keys(page.state)`
  semantics; on the server it's always empty (`Object.keys(...).join`
  → `""`). The lowering must handle `Object.keys(page.state).join`
  later as a separate ESTree pattern (left out here, see Open Questions).
- `Form` is `any`; the same form value the client payload's
  `data.form` carries. Most server renders see `nil`.

### 3. Lowering: page.* as a root

Treat `page` and `navigating` and `updated` as **distinct typed roots**
on the scope, parallel to `data`. The Lowerer's chain walker today
short-circuits when `scope.IsDataRoot(root.Name)` and walks the
`typegen.Shape` for `data`; for `page.*` we walk a hand-coded mapping
because these structures are framework-defined, not user-defined.

Concretely:

```go
// In Lowerer (lowering.go), extend rewriteMember:
//
// 1. If root is "page" → walk a static page field map instead of
//    typegen.Shape.
// 2. If root is "navigating" → static {current: nil-on-server} chain.
// 3. If root is "updated" → static {current: false-on-server} chain.
//
// All three roots emit Go expressions of form `pageState.<Field>...`
// where pageState is the bound parameter name on the Render fn.
```

Field map (matched against the AST property names — JS source identifiers):

| JS chain                     | Go expression                          | Notes                                                      |
| ---------------------------- | -------------------------------------- | ---------------------------------------------------------- |
| `page.url`                   | `pageState.URL`                        | `*url.URL`                                                 |
| `page.url.pathname`          | `pageState.URL.Path`                   | `pathname` → `Path` is the only mismatch                   |
| `page.url.search`            | `pageState.URL.RawQuery` prefixed `?`  | actually map to a helper — see helper section              |
| `page.url.hash`              | `pageState.URL.Fragment`               | similar mismatch                                           |
| `page.url.origin`            | `pageState.URL.Scheme + "://" + Host`  | helper                                                     |
| `page.url.host`              | `pageState.URL.Host`                   |                                                            |
| `page.url.hostname`          | `pageState.URL.Hostname()`             |                                                            |
| `page.url.href`              | `pageState.URL.String()`               | helper                                                     |
| `page.params`                | `pageState.Params`                     | `map[string]string`                                        |
| `page.params.<name>`         | `pageState.Params["<name>"]`           | map indexing for sub-segment access                        |
| `page.route`                 | `pageState.Route`                      | struct                                                     |
| `page.route.id`              | `pageState.Route.ID`                   |                                                            |
| `page.status`                | `pageState.Status`                     |                                                            |
| `page.error`                 | `pageState.Error`                      | `*PageError`                                               |
| `page.error.message`         | `pageState.Error.Message`              | optional chain handles nil                                 |
| `page.error.status`          | `pageState.Error.Status`               |                                                            |
| `page.data`                  | `pageState.Data`                       | typed via existing data param                              |
| `page.data.<x>`              | leave to existing data-root lowering   | `page.data.x` → `data.X` when typed                        |
| `page.form`                  | `pageState.Form`                       | `any`                                                      |
| `page.state`                 | `pageState.State`                      | always empty server-side                                   |
| `navigating.current`         | `pageState.Navigating`                 | always nil server-side; field is `*Navigation`             |
| `navigating.<other>`         | hard error (no other top-level fields) |                                                            |
| `updated.current`            | `pageState.Updated`                    | always false server-side                                   |
| `updated.check()` or others  | hard error                             | server has no version-poll; later phase                    |

Decision on the param name: use **`pageState`** as the Go identifier
the emitter binds — distinct from `page` (the Svelte rune name) so the
lowering rewrites are unambiguous and the local doesn't shadow a JS
identifier in scope. A short name (`ps`) was tempting but readability
wins; the gen file is rarely read by humans but when it is, the long
form helps.

### 4. URL field name mismatches

JS `URL.pathname` → Go `URL.Path`. JS `URL.search` includes the leading
`?`; Go `URL.RawQuery` does not. Handle these via the lowerer's special-
case list rather than dragging in a helper. Keep helpers for shapes that
do require code (origin = `Scheme + "://" + Host`).

For v1 we'll cover the high-value subset: `pathname`, `host`, `hostname`,
`href`, `origin`, `search`, `hash`. `searchParams.<x>` is out of scope —
it's a methods-on-object surface that doesn't lower cleanly.

### 5. Recording the imports

Today `recordImport` rejects everything that isn't
`svelte/internal/server`. Extend to a small whitelist:

```go
switch decl.Source.LitStr {
case "svelte/internal/server":
    // existing namespace-import path
case "$app/state":
    // record imported names so the emitter knows which roots to
    // pre-bind in scope. Each ImportSpecifier names one of:
    //   page | navigating | updated
    // Reject any other specifier name as unknownShape.
case "$app/navigation":
    // record the goto/invalidate/etc names. v1 lowering does not
    // emit calls to these — they're client-only — but recognising
    // the import keeps the build green for routes that reference
    // them in handlers (those handlers don't run server-side).
default:
    return unknownShape(...)
}
```

For the v1 cut, `$app/navigation` imports are **accepted but inert**:
the imported names get registered as `LocalCallback` so any call site
(`goto(target)` inside a function declaration) lowers to a Go function-
value invocation. Server-side those calls aren't reachable from Render
because the function declarations aren't called from Render's body —
they're handler closures. If a render-time call to one of these does
appear, the lowerer falls back to `unknownShape` with a message pointing
at the SSR fallback annotation.

### 6. New scope kinds

Add `LocalAppState` to the LocalKind enum. Mark `page`, `navigating`,
`updated` as `LocalAppState`. This:

- Tells `Scope.Lookup` they're not user-data roots (so default JSON-tag
  rewriting skips them).
- Tells the Lowerer to walk the static map instead of the typegen
  Shape.

Alternatively a new `Scope.AppStateRoots` set could carry the same info,
but reusing the existing kind machinery is one line of code and zero
new fields on Scope.

### 7. Wire / signature plumbing

Three layers change shape:

1. **`svelte_js2go.Options.EmitPageStateParam bool`** — when true, the
   emitter:
   - Adds `pageState server.PageState` to the Render signature (after
     `data` and after `children` if both are set).
   - Pre-declares `page`, `navigating`, `updated` in the render scope
     as `LocalAppState`.
   - For each appstate-import that records `goto`, `invalidate`,
     `pushState`, etc., declares the local as `LocalCallback`.

2. **`internal/codegen/ssr.go`** — flips `EmitPageStateParam: true` for
   every `Transpile` call (page, layout, error-boundary). All Svelte
   templates can read `$app/state`; consistency is cheaper than
   per-route detection.

3. **`internal/codegen/wire.go`** — `emitSSRWire`,
   `emitSSRLayoutWire`, `emitSSRErrorWire` extend their bridge wrappers
   to take a `server.PageState` param and forward to the typed Render.
   The wrapper signature becomes
   `RenderSSR(payload, data, pageState server.PageState) error` etc.

4. **`internal/codegen/manifest.go`** — `render__<alias>`,
   `renderLayout__<alias>`, `renderError__<alias>` build a PageState
   from `*kit.RenderCtx` (a 5-line helper in
   `runtime/svelte/server/page_state.go`) and forward to the wire
   bridge.

5. **`packages/sveltego/server/pipeline.go`** — no change. The page
   state is constructed inside the bridge functions (which are part of
   the manifest emit), so the pipeline keeps its current
   `route.RenderChain(...)` / `route.Page(...)` signatures.

   Wait — the bridge functions need data the pipeline owns: status code
   from form, error from in-flight render. Let me reconsider.

   Actually the simpler shape: PageState is built _in the bridge_ from
   ctx (URL, Params come straight off `*kit.RenderCtx`). Status defaults
   to 200 because PageState is what user code reads during success
   render; the error path uses `RenderError` which has a different shape
   (kit.SafeError). Form similarly comes from a route-known-only-to-
   pipeline path; for now `Form: nil` is the safe default. We can
   thread Form through later by extending the PageHandler signature,
   but #466's acceptance only asks for `page.url`, `page.params`,
   `page.route`, `page.status`, `page.error`, `page.data`, `page.form`,
   `page.state` to *lower* — runtime correctness for `page.form` and
   `page.state` is "always nil/empty server-side" which the bridge can
   honour without pipeline help.

   Verdict: pipeline stays untouched. Bridge constructs PageState from
   ctx + data. `page.form` and `page.state` always render their server
   defaults; the client SPA hydrates with the actual values via
   `_setPage`.

### 8. Goldens

Add a new `lowered/` golden subdirectory entry per page-state pattern.
Existing 30+ priority and 50+ extended goldens stay byte-identical
because `EmitPageStateParam` defaults to false.

Run the corpus regen to confirm:

```
go test ./packages/sveltego/internal/codegen/svelte_js2go/ -args -update
```

Add per-pattern fixture goldens for:

- `page-url-pathname` (`page.url.pathname`)
- `page-params-id` (`page.params.id`)
- `page-route-id` (`page.route.id`)
- `page-status` (`page.status`)
- `page-error-message` (`page.error?.message`)
- `page-data` (`page.data` returns the typed data root)
- `navigating-current` (`navigating.current`)
- `updated-current` (`updated.current`)
- `appstate-mixed` — the appstate playground source itself

### 9. Playground annotation drop

`playgrounds/basic/src/routes/appstate/[id]/_page.svelte` currently
opens with `<!-- sveltego:ssr-fallback -->`. Drop the line; codegen
should now transpile the route into Go without hitting unknownShape.

### 10. `Object.keys(page.state).join(',')` etc

The fixture page uses `Object.keys(page.state).join(',') || 'none'`.
This is an Object method call on a map — out of scope for v1 lowering.
Two options:

- **A.** Rewrite the fixture to a Go-friendlier shape:
  `{stateKeysCount}` based on `page.state.length` (also out of scope).
- **B.** Add an Object/builtins helper in patterns.go. Bigger surface;
  defer.

Going with A: simplify the fixture so it lowers cleanly. The
playground exists to demonstrate the rune surface, not to exercise
Object.* methods. Rewrite to a server-renderable expression while
keeping the smoke-test contract: render every field of `page` and
`navigating` so a human can eyeball the result.

Concrete fixture rewrite:

```svelte
<script lang="ts">
  import { page, navigating, updated } from '$app/state';
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
  const errorMessage = page.error ? page.error.message : 'null';
  const navigatingType = navigating.current ? navigating.current.type : 'idle';
  const formLabel = page.form === null ? 'null' : 'set';
  const routeId = page.route.id ?? 'null';
  const paramId = page.params.id ?? 'null';
</script>
```

Drop the `stateKeys` line. Server side doesn't render it because state
is always empty. Client-side hydration will still show a `state` field
in `page.state` (an empty object) — fine, the smoke test asserts on
URL/route/params/status fields which are non-empty server-side.

Actually re-reading the fixture: `page.state` lowering is in-scope for
acceptance; what's *not* in scope is `Object.keys`. So leave the
expression as `'none'` server-side (since `page.state` is always empty
on the server, the `|| 'none'` branch wins). Even simpler: drop the
state-keys line entirely from server render and let the client repaint
it after hydration. Keep the fixture honest by rendering a single line
that shows the rune *fired*: `state object: empty`.

### 11. Layouts and error boundaries

Layouts also carry `$app/state` runes (e.g. nav components in
`_layout.svelte` showing the current path). Same lowering, same
parameter — `EmitPageStateParam` toggles independently of
`EmitChildrenParam`. Set both for layouts.

Error boundaries get the same param. The bridge passes a PageState
constructed from the in-flight ctx; status comes from the SafeError.

### 12. Out of scope (for #466 v1)

- `goto`, `invalidate`, `pushState`, `disableScrollHandling` — client-
  only APIs. v1 accepts the import, registers the names as
  LocalCallback, but emits hard errors if any is called from render
  body (function-decl bodies are fine; they're not invoked in render).
- `URL.searchParams.*` — methods-on-object surface; defer.
- `Object.keys`, `Object.values`, `Object.entries` — JS builtins; defer.
- `page.state` writes (`pushState({state})` mutates state) — write-
  surface stays client-only.
- `updated.check()` — server has no version-poll; the surface returns
  `false` Promise on the client and is unreachable on the server. v1
  treats `updated.check` as unknownShape (sidecar fallback if hit).

## Files touched

Per #466 acceptance:

1. `packages/sveltego/internal/codegen/svelte_js2go/emitter.go`
   - Extend `Options` with `EmitPageStateParam bool`.
   - Extend `LocalKind` with `LocalAppState`.
   - Extend `recordImport` whitelist for `$app/state`,
     `$app/navigation`.
   - In `emitRenderFunction` pre-declare appstate roots when
     `EmitPageStateParam` is on.
   - Extend the function-signature switch (4-way already; becomes 8-way
     with the page-state axis) — refactor into a builder to keep the
     case explosion sane.

2. `packages/sveltego/internal/codegen/svelte_js2go/lowering.go`
   - In `rewriteMember`, branch on root name: `page`, `navigating`,
     `updated` → `lowerAppStateChain`.
   - Add `lowerAppStateChain` with the static field map from §3.

3. `packages/sveltego/runtime/svelte/server/page_state.go` (NEW)
   - Define `PageState`, `PageRoute`, `PageError` types.
   - Provide `NewPageState(ctx *kit.RenderCtx, route, data, form any) PageState`.

4. `packages/sveltego/internal/codegen/wire.go`
   - `emitSSRWire`, `emitSSRLayoutWire`, `emitSSRErrorWire` extend
     their wrapper signatures to take and forward `pageState`.

5. `packages/sveltego/internal/codegen/ssr.go`
   - Pass `EmitPageStateParam: true` in all three Transpile sites.

6. `packages/sveltego/internal/codegen/manifest.go`
   - `render__<alias>`, `renderLayout__<alias>`,
     `renderError__<alias>` construct a `server.PageState` from ctx
     and forward to RenderSSR/RenderLayoutSSR/RenderErrorSSR. Note the
     wrap signatures change.

7. `packages/sveltego/internal/codegen/typegen` — no change. PageState
   is framework-defined, not from user types.

8. `playgrounds/basic/src/routes/appstate/[id]/_page.svelte`
   - Drop the `<!-- sveltego:ssr-fallback -->` annotation.
   - Trim the `stateKeys` line (or simplify to a server-renderable
     expression).

## Open questions

1. **Is `pageState` the right Go param name?** `page` clashes with the
   Svelte rune local (we want `page.url` to resolve via the lowerer to
   `pageState.URL`, not collide with a Go variable named `page`).
   `pageState` is unambiguous and self-documenting.

2. **PageState as struct vs pointer?** Pointer (`*server.PageState`)
   would be one alloc-saving pass at the bridge. Struct (no pointer)
   pays nothing in the function call (Go inlines small struct copies).
   Going with **value** because it's simpler and the struct is small
   (~10 fields, <80 bytes).

3. **Where to derive `Status`?** RenderCtx doesn't carry it. Default to
   200 in the bridge. Form-action paths that mutate status flow through
   a separate path (the action runs, status is set on the response,
   PageState's Status is what the *load chain* would read at render
   time). 200 is the right value for the success render path.

4. **`page.form` payload?** Form data lives in pipeline-local state
   (`*formData`), not on RenderCtx. v1 sets `Form: nil` server-side;
   client hydrates the actual form payload via `_setPage`. If a route
   reads `page.form` server-side (uncommon — usually `let { form }`
   destructure of `data.form`), it always sees nil server-side. That
   matches SvelteKit's actual behaviour because `page.form` on first
   render is set by client-side action submission, not by server SSR.
   Validated against SvelteKit docs: `page.form` is a **client-side
   reflection of the action result**, undefined during SSR initial
   render.

   So `Form: nil` is correct.

## Phasing

- Phase 1: Core lowering (emitter Option, recordImport whitelist,
  Lowerer chain), synthetic `runtime/svelte/server/page_state.go`,
  AST-builder tests for new patterns. No bridge / wire / manifest
  changes yet — tests use programmatic AST + raw Transpile.

- Phase 2: Wire bridge wrappers extended (wire.go, ssr.go flip to pass
  EmitPageStateParam, manifest.go bridges build PageState).

- Phase 3: Drop the playground annotation; smoke route locally. Run
  full local gate: gofumpt, goimports, vet, race tests, build,
  golangci-lint.

- Phase 4: Open PR, self-merge after CI green per
  `feedback_no_separate_reviewer.md`.

Each phase is one or two commits.

## Verification gate

- `gofumpt -l .` clean
- `goimports -l -local github.com/binsarjr/sveltego .` clean
- `go vet ./...` clean across go.work modules
- `go test -race ./...` baseline (~1717 tests)
- `golangci-lint run` clean
- `go build ./...` across go.work
- 4× playground-smoke green on CI
- `playgrounds/basic` route `/appstate/42` SSRs and renders the rune
  surface without the fallback annotation
