# ADR 0008 — Pivot to Pure-Svelte Templates

> **Update 2026-05-02:** SSR-at-runtime is restored via [ADR 0009](0009-ssr-option-b.md) (Option B — mechanical transpile of `svelte/server` compiled JS to Go). The pure-Svelte template invariant in this ADR remains canonical; only the "all dynamic routes ship as SPA shell" framing in the runtime section below is superseded.

- **Status:** Accepted (SSR-at-runtime portion superseded by [ADR 0009](0009-ssr-option-b.md), 2026-05-02)
- **Date:** 2026-05-01
- **Authors:** binsarjr, orchestrator
- **Issue:** [binsarjr/sveltego#379](https://github.com/binsarjr/sveltego/issues/379)
- **Supersedes:** [ADR 0007](0007-svelte-semantics-revisit.md) (Proposed); the "Mustache expressions are Go" half of [ADR 0001](0001-parser-strategy.md) and [ADR 0002](0002-expression-syntax.md). Codegen-shape and file-convention ADRs (0003, 0004) remain in force, with template-emit lowered to typegen + Vite passthrough.
- **Superseded by:** [ADR 0009](0009-ssr-option-b.md) for the SSR-at-runtime piece only. Template invariant unchanged.
- **Related:** [ADR 0005](0005-non-goals.md) (no JS runtime on the server — preserved), RFC #309 (Svelte semantics revisit), Spike #311 (JS runtime survey), Spike #313 (runes evaluation surface).

## Context

ADR 0001/0002 made `.svelte` files **Go-decorated templates**: mustaches held Go expressions (`{Data.User.Name}`, `{len(Data.Posts)}`), validated at codegen via `go/parser.ParseExpr`. ADR 0005 locked "no JS runtime on the server" to keep the Go binary the entire deployable.

Three pressure points surfaced in the 2026-05-01 user conversation that ADR 0007 had only proposed (not resolved):

1. **Two-language friction inside one file.** Devs read `.svelte`, expected Svelte/JS expressions, and wrote `{user.name}` then debugged `{Data.User.Name}` errors. The cognitive switch happened mid-template, every template.
2. **Svelte ecosystem incompatibility.** Off-the-shelf `.svelte` components from npm assume JS expressions. Sveltego forked the dialect; reuse meant rewriting every component.
3. **SvelteKit migration friction.** `docs/why-sveltego.md` named "zero-curve migration from SvelteKit" as a goal. Mustache-Go templates contradict that goal — every page needs a rewrite, not a port.

Five reinterpretations were on the table in ADR 0007 (status quo, JS-runtime SSR, CSR-only, Go VDOM, sugar layer). None preserved the SvelteKit migration goal **and** the no-JS-runtime invariant **and** the Svelte ecosystem reuse goal simultaneously. The user picked a sixth path that does: keep templates 100% Svelte/JS/TS at authoring time, push all Go server-side, and split runtime into hybrid SSG (Node only at build time) + SPA (Go-only at runtime).

## Decision

Pivot `.svelte` files from Go-decorated templates to **100% pure Svelte/JS/TS**. The Go binary still has zero JS engine at runtime. Single static binary deploy stays the invariant.

Concretely:

1. **Templates are pure Svelte.** `.svelte` files contain only Svelte/JS/TS — runes, JS expressions, lowercase props (`{data.user.name}`). **Zero Go syntax** in mustaches, blocks, or `<script>`. Svelte LSP and the wider Svelte ecosystem work without a fork.
2. **Go owns the server.** A Go-only file (the project's `*.server.go` route file under `src/routes/...`) is the only place Go touches a route. It returns a typed data shape from `Load(ctx kit.LoadCtx)`; that shape becomes `data` in client `$props()`.
3. **Codegen emits TypeScript declarations.** A new Go-AST → TypeScript walker (`internal/codegen/typegen/`) reads each route's Go `Load` return type and emits a sibling `.svelte.d.ts` file per route. JSON tags drive field names; the type table maps `kit.Streamed[T]` → `Promise<T[]>`, `*T` → `T | null`, `time.Time` → `string`, etc. IDE autocomplete via Svelte LSP / vscode-svelte; drift caught by typegen golden tests.
4. **Hybrid SSG + SPA runtime.** Routes opting into `kit.PageOptions{Prerender: true}` are rendered to static HTML at `sveltego build` via a one-shot Node process invoking `svelte/server`. Routes without `Prerender` ship the SPA shell (`app.html` + JSON payload) and hydrate client-side. **Node is required at build time only.** The deployable is the Go binary plus `static/`.
5. **Streaming uses existing primitives.** `kit.Streamed[T]` flows through the existing `__sveltego__resolve` patch protocol (PR #366). The client-side wrapper exposes a `Promise<T[]>` to `$props()`; `.svelte` templates use Svelte's native `{#await}` block.
6. **Mustache-Go codegen is removed.** Roughly 80% of the current template parser, element handlers, block emitters, and scoped-CSS Go-side codegen are deleted. Vite + svelte-preprocess take over those responsibilities for the client bundle. Pre-alpha; no compatibility shim.

Migration runs across six phases tracked in `binsarjr/sveltego#380–#385`. Phase 1 (this ADR + doc realignment) is the approval gate; Phases 2–6 implement typegen, parallel pure-Svelte pipeline, playground/AI-template rewrite, deletion of Mustache-Go, and an optional perf re-bench.

## Consequences

### Enables

- **Direct Svelte ecosystem reuse.** `.svelte` components from npm work without a dialect rewrite.
- **SvelteKit-shaped migration.** Porting a SvelteKit app means swapping `+page.ts` loaders for `*.server.go` and accepting the type-mapping table; templates copy over verbatim.
- **Lower mental load.** One language inside a template. JS expressions for client, Go for server, JSON-tag boundary between them.
- **SSG-grade marketing routes.** `Prerender: true` produces static HTML at build time — no per-request work, optimal LCP for content pages.

### Drops

- **Build-time validation of template expressions.** Today, `{Data.User.NamE}` is a Go compile error. Post-pivot, `{data.user.namE}` typos surface only via Svelte LSP + TypeScript checking against the generated `.d.ts`. JS-only authors get less safety than Go-typed authors.
- **The current SSR throughput proposition.** SPA-mode first paint is slower than the Mustache-Go SSR path because the client must boot before render. The 20–40k rps target now applies to JSON-payload responses (server side) and to SSG output (zero per-request cost), not to per-request HTML rendering. Marketing-style routes use `Prerender: true`; dynamic routes accept the SPA tradeoff.
- **~80% of the existing Mustache-Go codegen.** Mustache parser, element handlers, block emitters, scoped-CSS Go-side codegen, expression validation, and the `.gen/*.go` template artifacts are deleted in Phase 5.
- **Backward compatibility.** Pre-alpha; users rewrite their templates. No deprecation window for Mustache-Go.

### Preserves

- **No JS runtime on the server at runtime.** ADR 0005 invariant intact. Node runs only during `sveltego build` for SSG; the deployed Go binary has no JS engine.
- **Svelte 5 only.** Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`). Svelte 4 legacy reactivity remains out of scope.
- **Codegen, not interpretation.** The codegen pipeline narrows but does not disappear: route scan, manifest, server-side `Load` AST extraction, `.d.ts` emission, prerender orchestration, hydration payload all remain compile-time work.
- **File convention.** Route file names, `//go:build sveltego` tags on user Go files, `.gen/` provenance, `$lib`/`hooks.server.go`/`+error.svelte` semantics unchanged. ADR 0003 stands.
- **Single static binary deploy.** Go binary plus `static/` is still the entire deployable.

### Defers (subsequent issues)

- **Phase 2 — typegen scaffold** (`#381`): Go AST → TypeScript walker, type-mapping table, golden tests, `.d.ts` emission alongside the legacy pipeline.
- **Phase 3 — parallel pure-Svelte pipeline** (`#382`): per-route opt-in via `kit.PageOptions{Templates: "svelte"}`; SPA runtime; SSG via `svelte/server` at build time; smoke test on `playgrounds/basic`.
- **Phase 4 — playground + AI template rewrite** (`#383`): rewrite all three playgrounds and `templates/ai/*` to pure Svelte; refresh quickstart + reference docs.
- **Phase 5 — drop Mustache-Go path** (`#384`): delete the legacy parser, handlers, golden files, and the `Templates: "go-mustache"` option; `STABILITY.md` per package logs the breaking change.
- **Phase 6 — bench + perf gate** (`#385`, optional): re-run bench harness; update `docs/perf.md`; surface regressions > 20% for discussion.

## Supersedes

- [ADR 0007 — Svelte Semantics Revisit](0007-svelte-semantics-revisit.md) — moves from `Proposed` → `Superseded by 0008` on this date. Option A (status quo Mustache-Go) and Options B/C/D/E (JS runtime, CSR-only, Go VDOM, sugar) are all rejected; the chosen path is pure-Svelte templates with Go-AST → `.d.ts` codegen plus hybrid SSG+SPA runtime.
- The "expressions are Go" half of [ADR 0001](0001-parser-strategy.md) and [ADR 0002](0002-expression-syntax.md). The hand-rolled Svelte parser and Go expression validator give way to Vite + svelte-preprocess for templates and `go/ast` walking for the server-side `Load` type. Codegen-shape ([ADR 0004](0004-codegen-shape.md)) and file-convention ([ADR 0003](0003-file-convention.md)) ADRs survive intact.

## References

- [RFC #379 — pivot to pure-Svelte client templates + Go-only server](https://github.com/binsarjr/sveltego/issues/379) — full design, decision tree, migration phases, AskUserQuestion answers from 2026-05-01.
- [Phase 1 issue #380](https://github.com/binsarjr/sveltego/issues/380) — this ADR's tracking issue.
- [ADR 0007 — Svelte Semantics Revisit](0007-svelte-semantics-revisit.md) — superseded by this ADR.
- [ADR 0005 — Non-Goals](0005-non-goals.md) — no-JS-runtime invariant preserved.
- [RFC #309 — revisit Svelte semantics](https://github.com/binsarjr/sveltego/issues/309) — Option A vs B/C/D/E discussion that this ADR resolves.
- Lesson: [pivot to Go-native rewrite (2026-04-29)](../lessons/2026-04-29-pivot-to-go-native-rewrite.md) — original "no JS runtime on server" rationale, still load-bearing.
- PR #352 — hydration payload runtime; reused for SPA-mode JSON payload.
- PR #360 — SPA router runtime; reused for client-side navigation post-pivot.
- PR #366 — streaming hydration payload; `kit.Streamed[T]` → `Promise<T[]>` flows through this protocol.
- PR #372 — prerender bundle; becomes the SSG mode in Phase 3.
- Parent project direction: `CLAUDE.md` "Project direction" section, `AGENTS.md` "Project shape" section.
