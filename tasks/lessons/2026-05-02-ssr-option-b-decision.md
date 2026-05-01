## 2026-05-02 — SSR Option B decision

### Insight

- ADR 0008 (pure-Svelte pivot, 2026-05-01) bought ecosystem fit and SvelteKit migration shape at the cost of request-time SSR for non-prerendered routes. Dynamic routes shipped a SPA shell and hydrated client-side; first paint regressed.
- The "no JS runtime on the server at runtime" invariant from [ADR 0005](../decisions/0005-non-goals.md) was load-bearing — re-opening it would have re-opened the closed spike #311 (JS runtime cost) trade space and pulled the project back toward goja/v8go/Bun discussions that were already settled.
- The unexplored gap from spikes #311 and #313 was **transpilation at build time**. Both spikes asked "can a JS engine live on the server" or "can we re-implement Svelte runtime semantics in Go" — neither asked "what if we let Svelte's own compiler do the work and just translate its output?"
- Svelte's `generate: 'server'` emits a flat string-builder program with no runtime reactivity. Roughly 200 distinct emit shapes (`escape_html`, `push_element`, `attr`, head append, control-flow lowering). The compiler already lowered everything we'd otherwise have to re-implement.
- The JS parser was the largest line item in the original Option B LOC estimate (~1,500 build-time LOC). Vendoring Acorn — Svelte's own parser dependency — in the Node sidecar that already runs at build time for SSG turned that line item into ~50 LOC of Node glue. The Go side consumes JSON AST.
- "Hard-error build by default" beats silent fallback for SSR coverage honesty. A route quietly downgraded to SPA-mode looks fine in dev and surfaces as a first-paint regression in prod. Forcing `// sveltego:ssr-fallback` to be explicit keeps the SSR coverage map auditable.
- "Lock one Svelte minor, support N-1 for one cycle, no chase-HEAD" keeps the maintenance window bounded. Chase-HEAD against Svelte main means the golden corpus permanently moves and CI flakes on every upstream commit.

### Self-rules

1. When a hard architectural invariant has been settled (ADR-level), do not propose paths that reopen it. Instead, ask: "is there a path that achieves the goal *under* the invariant?" Build-time transpilation lived under the no-JS-runtime invariant; embedding a JS engine did not.
2. When evaluating a transpilation strategy, prefer the layer with the strongest upstream-stability contract. Compiled output > AST shape > runtime semantics. Svelte's compiled output is what their users depend on; AST and runtime semantics churn faster.
3. When a build-time tool (Node sidecar) already exists for an adjacent purpose (SSG), extending it is cheaper than introducing a new toolchain dependency. Vendor inside the existing sidecar before reaching for a Go-side reimplementation.
4. For unknown-input policies (unsupported JS expressions, unsupported emit shapes), prefer hard-error + explicit opt-out comment over silent fallback. Coverage maps stay honest only when degradation is opt-in.
5. Pin one upstream minor at a time with a deliberate bump-and-regenerate cadence; never chase HEAD on a code-generation contract. The golden corpus is the contract.
6. Document the *three* sub-decisions that ride on a major decision verbatim in the ADR. Future maintainers will want to know which calls were settled at decision time vs left open for implementation phases.

### Decisions captured (2026-05-02)

- **Option B chosen** over A (AST→Go) and C (pure Go runtime). Rationale: ~95% feature coverage v1, ~3 days maintenance per Svelte minor, low hydration parity risk, strongest upstream-stability contract. RFC [#421](https://github.com/binsarjr/sveltego/issues/421) carries the matrix.
- **Sub-decision 1 — Vendored Acorn in the Node sidecar.** Drops Option B build-time LOC ~4,000 → ~2,500. No hand-rolled JS parser in Go.
- **Sub-decision 2 — Hard-error build by default for unsupported JS expressions.** Routes that genuinely need SPA-mode opt in via `// sveltego:ssr-fallback` comment in `*.server.go` (Phase 8, [#430](https://github.com/binsarjr/sveltego/issues/430)).
- **Sub-decision 3 — Lock one Svelte minor, support N-1 for one release cycle, no chase-HEAD.** Bump-and-regenerate is a deliberate maintainer action, not a CI-driven event.
- ADR 0008 stays canonical for the pure-Svelte template invariant; only its SSR-at-runtime framing is superseded by [ADR 0009](../decisions/0009-ssr-option-b.md).
- Implementation tracked under [#422](https://github.com/binsarjr/sveltego/issues/422); 9 phase issues #423–#431. Phase 1 (this ADR) gates the rest.
