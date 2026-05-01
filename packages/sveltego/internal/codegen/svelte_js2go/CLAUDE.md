# svelte_js2go — package notes

The Phase-3 emitter for [ADR 0009](../../../../../tasks/decisions/0009-ssr-option-b.md):
takes the Acorn JSON AST emitted by the Phase 2 sidecar
(`internal/codegen/svelterender/sidecar/`), pattern-matches Svelte 5's
`generate: 'server'` output shapes, and writes equivalent Go to
`.gen/<route>_render.go`.

For the data model + envelope contract, read
[`tasks/specs/ssr-json-ast.md`](../../../../../tasks/specs/ssr-json-ast.md)
first. Reality deltas vs the original spec are baked into that doc as
of the Phase 9 sync (2026-05-02). Read it before adding patterns.

## Mental model

Pattern matching is **intentionally narrow**. The closed set of ESTree
shapes recognised in v1 is documented in the spec; anything else is
turned into a build-time `unknown shape: <kind>: <snippet>` error
(ADR 0009 sub-decision 2). The emitter never silently lowers an
unknown shape to a fallback — silent degradation breaks the SSR
coverage map.

Routes that genuinely cannot be transpiled opt out via the
`<!-- sveltego:ssr-fallback -->` annotation in their `_page.svelte`,
which the Phase 8 (`runtime/svelte/fallback`) path handles at runtime.

## Spec-vs-reality deltas (Svelte 5.55.5)

Carry-over surprises from Phase 3 that anyone adding patterns must
keep in mind. The spec doc has the full table; the most load-bearing
ones:

1. **Renderer-method, not `payload.out` assignment, is the dominant
   shape.** Real fixtures emit `$$renderer.push(template_literal)`
   and `$$renderer.component(arrowFn)`. The spec-original
   `$$payload.out += '...'` form is also dispatched but rarely seen.
2. **Escape helper is `$.escape`, not `$.escape_html`.** Both names
   map to `runtime/svelte/server.EscapeHTML` in `patterns.go`; only
   `$.escape` shows up in fixtures.
3. **`{#each}` is a hand-rolled C-style `for` loop**, not a
   `$.each(...)` helper. Lowering uses
   `$.ensure_array_like(arr)` plus `let $$index = 0, $$length = arr.length`.
   The multi-declarator init has to be hoisted to preceding lines
   because Go's `for` init is single-statement.
4. **Identifier mangling.** `$$index` / `$$length` / `$$renderer`
   become `ssvar_index` / `ssvar_length` / `ssvar_renderer` in Go
   (the `$` is illegal). Phase 5 must NOT relower these — they are
   emitter-internal locals, not user data.

## How to add a new pattern

The emitter dispatches on ESTree node `type` strings via a
registry-style switch. To add a new shape:

1. **Confirm it's actually new.** Run the corpus regen against the
   pinned Svelte minor and see whether the `unknown shape: …` build
   error is the only thing failing. If so, capture the offending
   AST snippet from the JSON envelope.
2. **Read the spec doc.** Decide whether the new shape is a top-level
   render-function pattern, a helper-call pattern, or an
   expression-walker pattern.
3. **Add a fixture.** Drop a minimal `.svelte` source under
   `testdata/golden/shapes_extended/<name>/` (or `shapes/` if it's
   priority-set) and let the corpus runner emit the expected
   `.golden.go`. Goldens are byte-identical; the corpus tooling
   regenerates them with `go test ./internal/codegen/svelte_js2go/ -args -update`.
4. **Wire the dispatch.** Extend the relevant case in `emitter.go`
   (top-level), `patterns.go` (helper / call), or `expr.go`
   (expression). Mirror existing patterns; do not invent a new
   structural style.
5. **Helper symbol mapping.** New `$.<helper>` calls land in
   `patterns.go` with the corresponding entry in
   `runtime/svelte/server/` (Phase 4 helpers package; pure functions,
   no state). Helpers explicitly skipped for v1 (`store_get`,
   `derived`, `bind_props`, `css_props`, `snapshot`, DEV-only
   validators) are listed in the server STABILITY.md.
6. **Run the cross-check.** `cases_test.go` and
   `cases_extended_test.go` exercise the corpus end-to-end; the
   Phase 7 cross-check fixtures (`runtime/svelte/server/testdata/cross/`)
   diff Go output against captured Node output.

## LocalKind enum semantics

`emitter.go` defines a closed `LocalKind` enum used by Phase 5
property-access lowering (`lowering.go`) to decide which identifiers
to rewrite via the JSON-tag → Go-field map. The kinds:

| Kind | Origin | Phase 5 behavior |
|---|---|---|
| `LocalUnknown` | Default. Name was never bound in scope. | Treat as a free reference; **subject to lowering** (root data). |
| `LocalProp` | Destructured prop (`let { data } = $$props`, `params`, `url`, `form`, etc.). | **Subject to lowering**; root of a user-data subtree. |
| `LocalEach` | `{#each items as item}` — the per-iteration alias. | Leave JS-style; lowering would point at a nonexistent struct field. |
| `LocalScratch` | Emitter-introduced bookkeeping (`ssvar_index`, `ssvar_length`, `each_array`). | Leave JS-style; not user data. |
| `LocalSnippet` | `{#snippet name(args)}` closure. | Leave JS-style; closure-local scope. |

Anything that walks expressions and might produce identifier
references **must consult the scope before lowering**. Look at
`Scope.Lookup` and `Scope.IsDataRoot` in `emitter.go`.

## Determinism rules

- Pattern dispatch must be order-independent (no map-iteration
  dependence).
- `TestTranspile_Determinism` asserts byte-identical output across
  runs. Any new pattern that introduces ordering (e.g. iterating a
  Go `map`) must sort first.
- Emitted Go is gofumpt + goimports clean; the emitter never relies
  on a follow-up `gofmt` pass.

## Files

```
ast.go                  — Envelope + Node decode
expr.go                 — expression walker (calls into rewriter for Phase 5)
emitter.go              — top-level render-function dispatch, scope, options
patterns.go             — helper-call (`$.escape`, `$.attr`, …) dispatch
identifier.go           — `$$x` → `ssvar_x` mangling
lowering.go             — Phase 5 property-access lowering (rewriter impl)
runtime_companion.go    — Go boilerplate emitted alongside Render()
json.go                 — JSON helpers
testdata/golden/        — corpus goldens (shapes/, shapes_extended/, lowered/)
```

## Cross-references

- [ADR 0009](../../../../../tasks/decisions/0009-ssr-option-b.md) —
  decision and three sub-decisions.
- [`tasks/specs/ssr-json-ast.md`](../../../../../tasks/specs/ssr-json-ast.md) —
  envelope schema + reality deltas.
- `internal/codegen/svelterender/sidecar/CLAUDE.md` — sidecar's two
  operating modes (build-time AST emit + long-running fallback).
- `runtime/svelte/server/` — Phase 4 helpers, the Go side of the
  pattern map.
- `runtime/svelte/fallback/` — Phase 8 escape hatch for routes this
  emitter cannot handle.
