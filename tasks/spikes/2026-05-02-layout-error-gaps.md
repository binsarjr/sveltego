# 2026-05-02 — layout/error SSR-transpile gap state (#478)

Re-verification of the gap matrix from #478 against `main` at
`54a3da4` (PR #482 `$app/state` rune lowering merged on top of PR #477
LayoutChain retire).

## Method

For each playground:

```sh
cd playgrounds/<name> && rm -rf .gen build && go run \
    github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
```

Then:

- list `.gen/layoutsrc/**` and `.gen/errorsrc/**` — populated means the
  Option B SSR transpile fired
- grep `.gen/manifest.gen.go` for `Layout{}.Render` / `ErrorPage{}.Render`
  — non-zero hits mean the manifest is still calling Mustache-Go output
- list `.gen/routes/**/layout.gen.go` and `.gen/routes/**/error.gen.go`
  — present means Mustache-Go is still emitting (potentially dead
  artifacts even when SSR adapter wins)

## State (post-PR #482)

| Playground | layoutsrc | errorsrc | Mustache refs in manifest | Stale Mustache `.gen` files | Verdict |
|---|---|---|---|---|---|
| basic       | populated         | populated | none                       | `routes/layout.gen.go`         | **clean** — chain pulled in via non-annotated `/post/[id]` and `/appstate/[id]` |
| blog        | EMPTY             | EMPTY     | YES (`Layout{}.Render` x1) | `routes/layout.gen.go`         | **broken** — both routes (`/`, `/[slug]`) ssr-fallback annotated, gap-1 fires |
| dashboard   | populated         | EMPTY (no `_error.svelte`) | none           | `routes/layout.gen.go`         | **clean** — `/` and `/login` non-annotated cover root layout |
| ssr-stress  | populated (4 layouts) | populated (1 error) | none                  | 4× stale `layout.gen.go`       | **clean** — all routes non-annotated |

## Findings

### Gap-1 cascade is real, but only fires on blog

Per ssr.go:562/629, `planSSRLayouts(scan, transpilePlan)` and
`planSSRErrors(scan, transpilePlan)` enumerate from the page-transpile
plan. A page that is `<!-- sveltego:ssr-fallback -->` annotated routes
into `fallback`, NOT `transpilePlan`. Their layouts/errors are skipped.

For blog this means BOTH routes are annotated, so the root layout never
enters the layout plan, the manifest emits the Mustache-Go
`Layout{}.Render` adapter (manifest.go:743 `!li.hasSSR` branch), and
`.gen/layoutsrc/` is empty.

For basic the cascade is masked: `/` and `/version-poll` are both
annotated, but `/post/[id]` and `/appstate/[id]` aren't, so the chain-
mate route pulls the root layout into `transpilePlan` indirectly.

### Gap-2 (Vite stage cascade) is closed by PR #482

Issue #478 mentioned `$app/state` aborting Vite. PR #482 lowered
`$app/state` runes through svelte_js2go, so basic now compiles end to
end without Vite errors. No further action needed.

### Stale Mustache `.gen/routes/<route>/layout.gen.go` artifacts

Even when SSR transpile wins (and the manifest does NOT call
`Layout{}.Render`), `emitLayout()` in build.go:728 still writes the
file. Compare with `emitErrorPage()` at build.go:217, which is gated on
`ssrErrorPkgs`. The gate exists for errors (because lowercase
`data.code` access would compile-collide), not for layouts.

These stale layout artifacts are dead — the manifest does not import
them — but they remain on disk. Cleanup is a separate task: deferred
until the broader Mustache-Go atomic delete (#468 PR2) lands. Including
it in this PR would conflict with #468 PR2's surface.

## Fix shape

**Decouple layout/error eligibility from page-fallback annotation.**

`planSSRLayouts` / `planSSRErrors` enumerate from a separate eligibility
predicate, not `transpilePlan`. A route's layouts and error boundaries
should SSR-transpile when the route is a pure-Svelte SSR-template route
(Templates=svelte, SSR=true OR Prerender), **regardless of whether the
page itself routes via the Phase 8 sidecar fallback**. Fallback opts
the page body out of build-time transpile; chain-mate layouts and
errors still render Go-side in both Phase 8 and Phase 6 paths.

To implement this, both planners need access to `routeOptions`:

```go
func planSSRLayouts(scan *routescan.ScanResult,
    routeOptions map[string]kit.PageOptions) []layoutPlan { ... }

func planSSRErrors(scan *routescan.ScanResult,
    routeOptions map[string]kit.PageOptions) []errorPlan { ... }
```

The eligibility predicate inside both functions:

```go
opts, ok := routeOptions[r.Pattern]
if !ok || opts.Templates != kit.TemplatesSvelte {
    continue
}
if !(opts.SSR || opts.Prerender || opts.PrerenderAuto) {
    continue
}
// fall through regardless of r.SSRFallback
```

Existing `pageRoutes` membership filter goes away. Dedup by
`pkgPath` (layouts) / `ErrorBoundaryPackagePath` (errors) stays.

## Out of scope

- Stale `.gen/routes/<route>/layout.gen.go` cleanup — deferred to #468
  PR2's atomic Mustache-Go delete.
- Children-callback ABI for snippets (#443) — current playgrounds use
  trivial `{@render children()}` which lowers fine without #443.
- Lowerer per-feature gaps — none surfaced in the four playgrounds.
  blog's root layout is `let { children } = $props(); {@render children()}`
  — already passes when chained in.

## Acceptance

Post-fix, blog rebuild produces:

- `.gen/layoutsrc/routes/layout_render.gen.go` with the lowered
  `Render` body
- `.gen/manifest.gen.go` with zero `page_routes.Layout{}.Render`
  references
- `render__layout__page_routes` adapter calling
  `page_routes.RenderLayoutSSR(...)` (Option B bridge form)
- All other 3 playgrounds remain clean
