# basic

Hello-world playground exercising the post-pivot pipeline end-to-end:
parser → typegen → svelte_js2go SSR transpile → manifest adapters →
router → server → SSR `Render()` per request.

After the SSR Option B track ([RFC #421](https://github.com/binsarjr/sveltego/issues/421))
shipped (2026-05-02), this playground returns full SSR HTML on
`curl /` rather than the bare SPA shell that ADR 0008 originally
targeted. No JS engine on the request path — Node only ran during
`sveltego build` to produce the per-route `Render()` Go functions.

## Render mode

This playground runs every route in **SSR mode** — the default. `_page.server.go` files declare a `Load(ctx)` and emit no `Prerender` / `SSR` constants, so codegen seeds them with `kit.DefaultPageOptions()` (`SSR: true`). To switch a route into another mode, edit its `_page.server.go`:

- **SSG** — add `const Prerender = true` to render once at build time.
- **SPA** — add `const SSR = false` to ship the app shell + JSON payload only.
- **Static** — delete `_page.server.go` entirely; only `_page.svelte` remains.

See [`docs/render-modes.md`](../../docs/render-modes.md) for the full reference and decision tree.

## Layout

```
playgrounds/basic/
├── go.mod                    # require + replace points at packages/sveltego
├── app.html                  # shell with %sveltego.head% / %sveltego.body%
├── src/routes/
│   ├── _layout.svelte        # layout chain (slot-only is fully supported;
│   │                         #   children-callback ABI still tracked in #440)
│   ├── _page.svelte          # pure Svelte/JS/TS — `let { data } = $props()`,
│   │                         #   `{data.greeting}`, `{#each data.posts as post}`
│   ├── _page.server.go       # //go:build sveltego; Load(ctx) returns PageData
│   └── post/
│       └── [id]/
│           ├── _page.svelte
│           └── _page.server.go
└── cmd/app/main.go           # boots server with gen.Routes()
```

## Conventions

- `_page.svelte` carries Svelte 5 runes only — `$props`, `$state`,
  `$derived`. Lowercase props (`data.user.name`); zero Go syntax in
  mustaches. ADR 0008 governs.
- `_page.server.go` declares a typed `PageData` struct; codegen reads
  the Go AST and emits a sibling `.svelte.d.ts` for IDE
  autocompletion. Server-side fields lower into `data.User.Name` Go
  field access in the SSR `Render()` via the JSON-tag map (Phase 5,
  [#427](https://github.com/binsarjr/sveltego/issues/427)).
- User `.go` files under `src/routes/**` use the `_` prefix
  (`_page.server.go`) so the default Go toolchain skips them; codegen
  reads them through `go/parser` directly. ADR 0003 amendment.

## SSR walkthrough (post-Option-B)

1. `sveltego compile` walks `src/routes/`, runs the Node sidecar in
   `--mode=ssr` once per route (compile via Svelte → Acorn → JSON AST
   to `.gen/svelte_js2go/<route>/ast.json`), then runs the Go-side
   `internal/codegen/svelte_js2go/` transpiler to emit
   `.gen/<route>_render.go` functions. The manifest wires each route
   through a `render__<alias>` adapter that calls into the generated
   `Render()`.
2. `sveltego build` chains compile → Vite (client bundle) →
   `go build`. Output binary is self-contained.
3. At request time the server runs `Load(ctx) → data → Render(payload, data)`
   in pure Go. No Node, no JS engine.

## Run

```bash
cd playgrounds/basic
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app           # listens on :3000
curl http://localhost:3000/
curl http://localhost:3000/post/123
```

`curl /` returns a populated HTML body (greeting + post list); `curl
/post/123` returns the per-post view. The CI playground-smoke job
asserts non-empty bodies on both paths.

## Hydration-parity smoke (#446)

CI runs `scripts/hydration-smoke.mjs` against `/` and `/post/123` after
building the playground with the client bundle. The script loads each
route in headless Chromium, waits for `window.__sveltego_hydrated`, and
fails on Svelte `hydration_mismatch` / `hydration_attribute_changed`
warnings. To run locally:

```bash
cd playgrounds/basic
npm install
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app &
npx playwright install --with-deps chromium
node ../../scripts/hydration-smoke.mjs --base http://localhost:3000 --routes /,/post/123
```

## SSR fallback (opt-in)

Routes whose JS the build-time transpiler cannot lower may opt out of
SSR-Go and route through the Node sidecar at request time by adding a
single line to their `_page.svelte`:

```svelte
<!-- sveltego:ssr-fallback -->
<script lang="ts">
  let { data } = $props();
</script>
...
```

The build prints which routes are annotated; the supervisor in
`runtime/svelte/fallback/` boots Node only if at least one is
present. See
[`runtime/svelte/fallback/STABILITY.md`](../../packages/sveltego/runtime/svelte/fallback/STABILITY.md)
for the contract.

## References

- [#23](https://github.com/binsarjr/sveltego/issues/23) — original
  hello-world spec.
- [ADR 0003](../../tasks/decisions/0003-file-convention.md) — file
  convention + Phase 0i-fix amendment.
- [ADR 0008](../../tasks/decisions/0008-pure-svelte-pivot.md) —
  pure-Svelte template invariant.
- [ADR 0009](../../tasks/decisions/0009-ssr-option-b.md) — SSR Option
  B (build-time JS-to-Go transpile).
- [#440](https://github.com/binsarjr/sveltego/issues/440) — open:
  layout-chain children-callback ABI.
