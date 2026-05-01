# ADR 0009 — SSR via Mechanical Transpile of `svelte/server` Output (Option B)

- **Status:** Accepted
- **Date:** 2026-05-02
- **Authors:** binsarjr, orchestrator
- **Issue:** [binsarjr/sveltego#421](https://github.com/binsarjr/sveltego/issues/421) (RFC), [#422](https://github.com/binsarjr/sveltego/issues/422) (tracking), [#423](https://github.com/binsarjr/sveltego/issues/423) (this ADR phase)
- **Supersedes:** the SSR-at-runtime portion of [ADR 0008](0008-pure-svelte-pivot.md). The pure-Svelte template invariant in ADR 0008 stays canonical.
- **Related:** [ADR 0005](0005-non-goals.md) (no JS runtime on the server — preserved), Spike #311 (JS runtime cost), Spike #313 (runes evaluator surface), [RFC #379](https://github.com/binsarjr/sveltego/issues/379), [RFC #412](https://github.com/binsarjr/sveltego/issues/412).

## Context

[ADR 0008](0008-pure-svelte-pivot.md) accepted the pure-Svelte pivot: templates became 100% pure Svelte/JS/TS, the server stayed Go-only, and request-time SSR was dropped for non-prerendered routes. Dynamic routes ship a SPA shell + JSON payload and hydrate client-side. ADR 0008 explicitly flagged slower first paint as the tradeoff.

For v1.0 the maintainer wants request-time SSR back **without** reopening the no-JS-runtime invariant. RFC [#421](https://github.com/binsarjr/sveltego/issues/421) evaluated three strategies that preserve every ADR 0008 invariant by doing all heavy lifting at `sveltego build` time:

- **Option A — AST → Go.** Walk Svelte's compiler AST in Go, emit per-route Go render code. Pays for features the Svelte compiler already lowers (control flow, attribute spread, slot composition, head bits). Includes a JS-expression-to-Go transpiler covering the subset Svelte allows in mustache position (~900 LOC). Build-time LOC ~4,400; ~70% JS-expr coverage; medium hydration parity risk.
- **Option B — `svelte/server` compiled JS → Go.** Run `svelte/compiler` in `generate: 'server'` mode at build time, then mechanically pattern-match the resulting JS string-builder program and emit equivalent Go. Svelte's server output is a flat stream of `escape_html`, attribute serialize, head append, etc. — roughly 200 distinct emit shapes, no runtime reactivity. Build-time LOC ~4,000; ~95% feature coverage v1; low hydration parity risk; ~3 days maintenance per Svelte minor.
- **Option C — Pure Go Svelte SSR runtime.** Re-implement Svelte 5 server-side semantics in Go, walk our own AST per request. Build-time LOC ~6,500; runtime LOC ~1,700; ongoing maintenance cost dominates. Already rejected on maintenance grounds in closed spike [#313](https://github.com/binsarjr/sveltego/issues/313).

Option B has the strongest upstream-stability contract — Svelte's *compiled output* is what their own users depend on, while AST shape and runtime semantics are both more volatile. Mechanical pattern-matching means Svelte minor bumps are diffable: re-run the corpus, see new shapes, add pattern entries. No multi-month catch-up.

## Decision

**Adopt Option B for v1.** SSR returns via a build-time pipeline that compiles each `.svelte` to Svelte's server-output JS, mechanically transpiles that JS to Go, and ships the generated `Render(payload, props)` functions in the binary. Runtime delta vs ADR 0008: an extra Go file per route. No JS engine at runtime. Single static binary plus `static/` remains the entire deployable.

Pipeline shape:

```
.svelte → svelte/compiler generate:'server' (Node, build-time)
        → JS module: function _page($$payload, $$props) { ... }
        → acorn.parse (Node, vendored in sidecar)
        → JSON AST (stdout)
        → internal/codegen/svelte_js2go/ (Go) pattern-matches Svelte emit shapes
        → emits .gen/<route>_render.go
        → Go binary at runtime calls Render(payload, props)
```

Three sub-decisions are codified verbatim with this ADR (settled with the maintainer at kickoff on 2026-05-02):

### Sub-decision 1 — Vendored Acorn in the Node sidecar

The Node sidecar that already runs at build time for SSG ([RFC #379](https://github.com/binsarjr/sveltego/issues/379) phase 3, `internal/codegen/svelterender/`) gains an `acorn.parse(jsSource)` step and emits Acorn JSON AST on stdout. The Go side consumes JSON AST and pattern-matches.

Rationale: hand-rolling a JS parser in Go was the largest single line item in the Option B LOC estimate (~1,500 LOC, build-time-only). Vendoring Acorn — Svelte's own parser dependency — drops Option B's build-time LOC from ~4,000 to ~2,500 and removes a class of "we got the JS grammar wrong" bugs. Acorn ships in the sidecar's existing `node_modules`; no new toolchain dependency at runtime.

### Sub-decision 2 — Hard-error build by default for unsupported JS expressions

When the pattern-match emitter encounters a JS expression shape it does not recognise (e.g. a new compiler emit primitive, a JS construct outside the supported subset), `sveltego build` fails loudly with an error pointing at the source `.svelte` file and the unrecognised shape. **No silent fallback to SPA-mode.**

Routes that genuinely need to opt out of SSR (e.g. dynamic shapes the transpiler cannot lower) declare it explicitly via a `// sveltego:ssr-fallback` comment in their `*.server.go` file (Phase 8, [#430](https://github.com/binsarjr/sveltego/issues/430)). The build then routes that page through the existing Node sidecar at request time, with HTML cached by `(route, hash(load_result))`.

Rationale: loud failure beats silent degradation. A route quietly downgraded to SPA-mode looks fine in dev and surfaces as a first-paint regression in prod. Forcing the opt-in to be explicit keeps the SSR coverage map honest. The fallback path is **not** an embedded JS engine — the sidecar already runs at build time for SSG; Phase 8 extends it to a long-running mode for opted-in routes only.

### Sub-decision 3 — Lock one Svelte minor, support N-1 for one release cycle, no chase-HEAD

`package.json` pins one Svelte minor at a time. The golden corpus is regenerated against that pin. When Svelte ships a new minor, the maintainer:

1. Bumps the pin in a dedicated PR.
2. Re-runs the transpile corpus; new emit shapes surface as `unknown shape: <name>` build failures.
3. Adds pattern entries for the new shapes (estimated 3–5 days per minor).
4. Lands the bump.

For one release cycle after a bump, sveltego still supports the previous minor (N-1) — both pins compile cleanly against the same Go pattern set. After that cycle, N-2 falls off support.

Rationale: chase-HEAD against Svelte main means the golden corpus is permanently moving and CI flakes on every upstream commit. Lock-one-minor with a deliberate bump-and-regenerate cadence keeps the maintenance window bounded and predictable. N-1 support gives downstream users a window to upgrade without forced lockstep.

## Consequences

### Enables

- **Request-time SSR for non-prerendered routes.** First paint on dynamic pages stops blocking on client hydration. Target: ≥10k rps p50 on mid-complexity pages (RFC #421 acceptance criterion).
- **Hydration parity by construction.** The same Svelte compiler emits both the client bundle (via Vite + `@sveltejs/vite-plugin-svelte`) and the server output we transpile. Server HTML is a sibling of client HTML, not a re-implementation. Drift surface shrinks.
- **Mechanical Svelte minor upgrades.** New compiler emit shapes surface as build-time `unknown shape: …` errors with a clear pattern-entry remediation. No multi-month catch-up.
- **Per-route fallback.** Routes the transpiler cannot handle opt into the sidecar fallback explicitly via comment, leaving the SSR coverage map honest.

### Drops

- **The "no Node anywhere in the build" framing.** Node was already required at build time post-ADR 0008 for SSG. This ADR extends the sidecar's responsibility (now also: parse JS to JSON AST), but does not introduce Node at request time. Sidecar fallback (Phase 8) runs Node as a long-running build-time companion for opted-in routes; the production deploy remains Go-only.
- **The simpler "all dynamic routes are SPA-mode" mental model.** Routes are now in one of three tiers: transpiled to Go (default, fast path), cached SSR via sidecar (explicit `// sveltego:ssr-fallback` opt-in), or SPA shell + payload (legacy ADR 0008 path retained for transition).

### Preserves

- **No JS runtime on the server at runtime.** [ADR 0005](0005-non-goals.md) invariant intact. The deployed Go binary plus `static/` remains the entire deployable. Sidecar runs only at build time (and at request time only for opted-in fallback routes via the same build-time tooling).
- **Templates are pure Svelte.** ADR 0008's pure-Svelte/JS/TS template invariant unchanged. No Mustache-Go regression. `.svelte` files stay copy-pasteable from the npm Svelte ecosystem.
- **Codegen, not interpretation.** Static decisions at build time. The new `internal/codegen/svelte_js2go/` package extends, not replaces, the existing codegen pipeline.
- **Single static binary deploy.** Go binary plus `static/` is still the entire deployable. No CGO, no JS engine, no subprocess on the request path.
- **Svelte 5 only.** Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) at the template layer. Compiler emit shapes targeted by the transpiler are Svelte 5's `generate: 'server'` output.

### Defers (subsequent issues)

- **Phase 2 — Node sidecar acorn JSON-AST extension** ([#424](https://github.com/binsarjr/sveltego/issues/424)): vendor Acorn in the sidecar; emit JSON AST for compiled-server JS modules.
- **Phase 3 — Go pattern-match emitter scaffold + 30 priority shapes** ([#425](https://github.com/binsarjr/sveltego/issues/425)): `internal/codegen/svelte_js2go/` package; cover `escape_html`, `push_element`, attribute serialize, head append, control-flow lowering, slot calls.
- **Phase 4 — `runtime/svelte/server` helpers package** ([#426](https://github.com/binsarjr/sveltego/issues/426)): Go mirror of Svelte's server helpers (`escape_html`, `attr`, `clsx`, `stringify`, `spread_attributes`, `merge_styles`). ~30 helpers, all pure functions.
- **Phase 5 — property-access lowering** ([#427](https://github.com/binsarjr/sveltego/issues/427)): map JSON tag → Go field via the existing typegen JSON-tag table; rewrite `data.name` → `data.Name` in emitted Go.
- **Phase 6 — pipeline integration** ([#428](https://github.com/binsarjr/sveltego/issues/428)): call generated `Render()` per non-prerendered route; thread through `kit.LoadCtx` → `PageProps`; fall through to SPA-mode only on explicit opt-out.
- **Phase 7 — golden corpus + playground SSR smoke** ([#429](https://github.com/binsarjr/sveltego/issues/429)): per-emit-shape goldens; differential bench vs Node sidecar; Playwright hydration smoke.
- **Phase 8 — sidecar fallback for `// sveltego:ssr-fallback`** ([#430](https://github.com/binsarjr/sveltego/issues/430)): long-running sidecar mode for opted-in routes; HTML cached by `(route, hash(load_result))`.
- **Phase 9 — post-impl docs sync** ([#431](https://github.com/binsarjr/sveltego/issues/431)): README architecture diagram, CLAUDE.md / AGENTS.md project shape, playground READMEs.

## Alternatives Rejected

- **Option A — AST → Go transpile.** Viable but pays for features the Svelte compiler already lowers (control flow, attribute spread, slot composition, head primitives). Tighter coupling to Svelte's AST shape (`Element` shape changed Svelte 4→5; renames likely Svelte 6+). Larger manual surface area for the JS-expression-to-Go transpiler, lower JS-expr coverage (~70% vs Option B's ~85%). See [RFC #421 § Option A](https://github.com/binsarjr/sveltego/issues/421) for detailed LOC and coverage analysis.
- **Option C — Pure Go Svelte SSR runtime.** Re-implements Svelte 5 server-side semantics in Go with no upstream contract. Reactivity-on-server is a tar pit (`$state`/`$derived`/`$bindable` initial values and derived computations); hydration parity bombs because our SSR is an independent implementation; ongoing maintenance perpetually trails upstream (closed spike [#313](https://github.com/binsarjr/sveltego/issues/313)'s "years of catch-up" framing applies). Total writeoff if abandoned. See [RFC #421 § Option C](https://github.com/binsarjr/sveltego/issues/421).

## Open Questions

None — Q1 (unsupported-expression policy) and Q2 (Svelte version pinning) settled at kickoff via Sub-decisions 2 and 3 above. RFC #421's remaining open questions (streaming SSR semantics, hydration parity bar, per-route opt-out semantics) are scoped to implementation phases and tracked in their respective sub-issues.

## References

- [RFC #421 — Go-only SSR runtime for Svelte templates](https://github.com/binsarjr/sveltego/issues/421) — full Option A/B/C evaluation.
- [Tracking issue #422](https://github.com/binsarjr/sveltego/issues/422) — phase plan and dependency graph.
- [Phase 1 issue #423](https://github.com/binsarjr/sveltego/issues/423) — this ADR's tracking issue.
- [ADR 0008 — Pure-Svelte pivot](0008-pure-svelte-pivot.md) — template invariant preserved; SSR-at-runtime portion superseded by this ADR.
- [ADR 0005 — Non-Goals](0005-non-goals.md) — no-JS-runtime-on-server invariant preserved.
- [Closed spike #311 — JS runtime cost](https://github.com/binsarjr/sveltego/issues/311) — embedded-engine path remains rejected.
- [Closed spike #313 — Svelte runes evaluator surface](https://github.com/binsarjr/sveltego/issues/313) — Option C maintenance argument.
- [RFC #412 — SSR error boundaries via svelterender](https://github.com/binsarjr/sveltego/issues/412) — error-boundary rendering routes through Go-emitted code post-Option-B.
- `internal/codegen/svelterender/svelterender.go` — existing Node sidecar; mechanical ancestor of the Phase 2 acorn extension.
- Lesson: [pivot to Go-native rewrite (2026-04-29)](../lessons/2026-04-29-pivot-to-go-native-rewrite.md) — original "no JS runtime on server" rationale, preserved by this ADR.
- Lesson: [SSR Option B decision (2026-05-02)](../lessons/2026-05-02-ssr-option-b-decision.md) — kickoff journey and the three sub-decisions captured at decision time.
