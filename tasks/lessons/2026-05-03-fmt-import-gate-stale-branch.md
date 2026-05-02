# 2026-05-03 — Stale `usesFmt` layout branch in manifest emitter (#485)

## Insight

`internal/codegen/manifest.go` had an `usesFmt` accumulator that gated
the emitted `"fmt"` import on whether any adapter actually wrote
`fmt.Errorf` into the generated body. The page-side check correctly
tracked the only remaining emit site (the legacy Mustache-Go typed-data
type-assert bridge at `emitRenderAdapters`). The layout-side check —
"set `usesFmt=true` if any layout has `!hasSSR`" — was a leftover from
an earlier era when `emitLayoutAdapters` switched between a Mustache-Go
form (`fmt.Errorf` for type-assert) and an SSR payload-bridge form
(no fmt) based on the per-layout `hasSSR` flag.

When the SSR layout-chain wiring (#456 / commit `e876b66`) unified the
emitter so `emitLayoutAdapters` always calls `emitSSRLayoutAdapter`
regardless of `li.hasSSR`, the Mustache-Go layout branch disappeared.
The accumulator that fed off `!li.hasSSR` was never rewired and stayed
as the only path through `usesFmt` whenever the page-side check missed.
In production the dead branch did not fire because `planSSRLayouts`
always marks every Svelte SSR route's layouts with `hasSSR=true` —
but any direct `GenerateManifest` caller that omits `SSRRenderLayouts`
(unit tests, future codegen paths, hand-rolled fixtures) hits the bug:
`fmt` is imported, no `fmt.` reference lands, `gofmt` does not strip
unused imports (`goimports` does), and `go build` fails with `"fmt"
imported and not used`.

The page-options golden was carrying the dead `"fmt"` import line for
this reason — it had no fmt reference but the layout-side branch tripped
the gate.

## Self-rules

1. When unifying two emitter forms into one (e.g. `emitLayoutAdapters`
   collapsing Mustache-Go + SSR into a single SSR call), audit every
   import-gate accumulator that the dropped form fed. Stale `if`
   branches over removed flags are silent producers of wrong imports.
   Follow the chain: removed emit site → removed `fmt.X` reference →
   accumulator branch that tracked it must also be removed.

2. Rule of thumb for codegen import gates: a "X imports Y" decision
   is correct iff every branch that sets the gate corresponds to an
   actual `b.Line(...)` call that writes a `Y.` reference into the
   buffer. After a refactor, scan each accumulator branch and trace
   back to the emit site it claims to track. If the emit site is
   gone, the branch is dead — delete it.

3. `gofmt` (and `go/format.Source`) does NOT strip unused imports,
   only `goimports` does. Codegen that runs through `format.Source`
   alone must be import-correct on its own — no compensating
   post-pass. Any reliance on `goimports -w .gen` as a build step is
   a foot-gun: pin the test fixture to bare `go build` to catch this.

4. Direct `GenerateManifest` test callers exercise import gates with
   inputs the production cascade may never produce. Keep at least one
   regression test per import-block branch that sets up a
   "production-impossible but locally-broken" combination — these
   catch latent gate bugs before they migrate into the production
   path via a future cascade change.
