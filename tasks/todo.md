# sveltego — Execution Plan

## Direction

Rewrite the SvelteKit shape (routing, load, actions, hooks, layouts) in pure Go. No JS runtime on the server. `.svelte` files are compiled to Go source via codegen for SSR, and to a Svelte 5 client bundle via Vite for hydration.

This replaces the earlier "embed JS runtime in Go" investigation. That direction was rejected after the runtime survey because every option (goja, v8go, quickjs, wazero, subprocess Bun) bonds CPU to a JS engine and either kills throughput or breaks cross-compile. See `tasks/lessons.md` for the chain of reasoning.

## Why rewrite

| Concern | Embed JS runtime | Rewrite in Go |
|---|---|---|
| Cross-compile single binary | Hard (v8go/quickjs need CGO) or large (Bun subprocess +50MB) | Native Go cross-compile |
| Throughput | 200–8k rps depending on engine | Target 20–40k rps for mid-complexity SSR |
| CPU profile | Bonded to V8/QuickJS/Bun | Pure Go scheduler, goroutine-native |
| DX vs SvelteKit | Identical (running real code) | Nearly identical (same file conventions, Go expressions) |
| Effort | ~70% spent on Web API polyfills | ~70% spent on parser/codegen + runtime |

The effort is comparable but the rewrite trade buys us performance, deploy simplicity, and a clean concurrency model.

## Milestones

Tracked as GitHub milestones at `binsarjr/sveltego`. Each issue carries Summary, Background, Goals, Non-Goals, Detailed Design, Acceptance Criteria, Testing Strategy, Risks, Dependencies, References.

### MVP (37 issues) — minimum to render a page

Foundation layer first: monorepo layout RFC (#95), code style + stability RFCs (#96, #97), lint + hooks + release-please + CI + PR template + AI doc sync + golden tests + bench gate (#98–105). Then technical RFCs (parser strategy, expression syntax, file convention, codegen layout — #1–4), then bootstrap (Go module, CLI), then the core pipeline:

- Parser: lexer → AST for the Svelte 5 subset we need.
- Codegen: text → element/attribute → expression → `{#if}` → `{#each}` → `<script lang="go">` extraction.
- Runtime: `render.Writer` with `sync.Pool`, escape utilities, `kit.LoadCtx`, Locals.
- Router: scan `src/routes/`, emit manifest, radix-tree match. Param matchers (`src/params/<name>.go`), optional `[[param]]` and rest `[...rest]` segments.
- Build: `$lib` alias and shared modules under `src/lib/`.
- HTTP pipeline: Load → Render → Response.
- CLI: `sveltego build` end-to-end.
- Test harness for golden codegen + a hello-world example.

### v0.2 (15 issues) — Form Actions & Hooks

Layout chain rendering, `+layout.server.go` parent data flow, `Handle` / `HandleError` / `HandleFetch` / `Reroute` / `Init`, `+error.svelte` boundaries, `+server.go` REST endpoints, `Actions()` map with form binding (urlencoded + multipart), `kit.Cookies`, `kit.Redirect / Fail / Error` sentinel helpers, route groups `(group)/` + layout reset `@`, page options (`Prerender`, `SSR`, `CSR`, `TrailingSlash`), env var convention (`$env/static`, `$env/dynamic`).

### v0.3 (13 issues) — Client SPA & Hydration

Vite integration for the Svelte client bundle, `window.__sveltego__` hydration payload, client hydrate runtime, SPA router (link interception + history), `__data.json` per-route endpoint, `use:enhance` for forms, prefetch on hover/viewport, precompressed static asset serving, `sveltego dev` with HMR. Full `$app/navigation` API (`goto`, `invalidate`, `preload`, `pushState`), Snapshot API for cross-nav state, typed `kit.Link` with route params, `kit.Asset` with hashed static URLs.

### v0.4 (19 issues) — Svelte 5 Full Coverage

Runes: `$props`, `$state`, `$derived`, `$effect`, `$bindable`. Snippets and `{@render}`. Legacy slots (default + named, with slot props). Special elements: `<svelte:head>`, `<svelte:body>` / `<svelte:window>` / `<svelte:document>`, `<svelte:component>`, `<svelte:options>`. CSS scope hash matching upstream. `{@html}`, `{@const}`, `{#await}`, `{#key}`. Nested component import and rendering. Compile-time a11y warnings.

### v1.0 (14 issues) — Production Ready

Benchmark suite vs adapter-bun with nightly regression gate. Docs site (Vitepress). Blog and dashboard examples. Streaming responses. Prerender / SSG mode. CSP nonce injection. CI (GitHub Actions). Release pipeline (release-please + GoReleaser). LSP for `.svelte` with Go expressions. Sitemap/robots helpers, image optimization (`<Image>`), service worker convention, deploy adapters (server, docker, static, lambda, cloudflare).

### v1.1 (6 issues) — LLM & AI Tooling

`llms.txt` + `llms-full.txt` for AI agents. `sveltego mcp` Model Context Protocol server (`search_docs`, `lookup_api`, `validate_template`, `scaffold_route`). Markdown-first docs with copy-for-LLM buttons. AI assistant project templates (`CLAUDE.md`, `.cursorrules`, `AGENTS.md`, copilot instructions) wired into `sveltego init --ai`. Provenance comments in generated `.gen/*.go`. AI-assisted development guide page.

### Standalone

- #94 RFC: explicit non-goals (universal load, WS server, vercel-style adapter, etc.) — sets boundaries before contributors propose them.

## Decision log (high-level)

| Decision | Rationale |
|---|---|
| Pure Go build tool | No Node/Bun runtime dependency in production server; Vite only at build time for client bundle |
| Go-native expression syntax | PascalCase fields, `nil`, `len()` — avoids a JS-to-Go translator and lets `go/parser.ParseExpr` validate at codegen |
| Svelte 5 only | Runes are the future; Svelte 4 legacy syntax inflates parser surface |
| Codegen, not interpretation | Interpreting templates per-request would defeat the perf goal; static codegen lowers to plain Go |
| Same DX as SvelteKit | File conventions and runtime concepts (`Load`, `Actions`, hooks) directly mapped to Go equivalents |

## Phase tracking

- [x] Direction confirmed (Go-native rewrite, not JS embed)
- [x] Repo created at `binsarjr/sveltego`
- [x] Issue templates (feature, RFC, bug)
- [x] 20 labels, 6 milestones, 105 issues created
- [x] All 69 original issues rewritten in English with industry-standard detail
- [x] v1.1 milestone added — LLM & AI tooling (#70–75)
- [x] SvelteKit-parity gap audit + 19 issues filed with priorities (#76–94)
- [x] Foundation issues filed (#95–105): monorepo layout, conventions, stability, lint, hooks, release-please, CI, PR template, AI sync, golden tests, bench gate
- [x] Land foundation RFCs (#95–97) and setup tasks (#98–103) — landed Phase 0a 2026-04-29
- [x] Land golden harness + bench gate (#104, #105) — landed Phase 0b 2026-04-29
- [x] Lock RFC #94 (non-goals) — ADR 0005, landed Phase 0c 2026-04-29
- [x] Lock RFCs #1–4 (parser, expression, file convention, codegen) — ADRs 0001–0004, locked 52f96da
- [x] Bootstrap cobra CLI (#5, #6) — landed Phase 0d 2026-04-29 (e9f7263)
- [x] Land parser foundation (#7 lexer + #8 AST/parser) — landed Phase 0e 2026-04-29; split layout `internal/lexer/`, `internal/ast/`, `internal/parser/`; multi-error model supersedes ADR 0001 sub-decision Q1
- [ ] Build the rest of the MVP pipeline end-to-end (#9–23, #76, #77, #83)
- [ ] Smoke-test on hello-world example (#23)

## Open questions

- Pinned upstream Svelte commit for CSS hash equivalence (#54) — pick once first build is green.
- Default `Save-Data` behavior for prefetch (#40) — assume conservative-on; revisit after first dogfooding.
- Whether to ship a fixed sanitizer for `{@html}` (#55) — current decision: no, recommend `bluemonday` in docs.

## Out of scope (for now)

Canonical list: [ADR 0005 — Non-goals](decisions/0005-non-goals.md). Quick reference:

- Universal `+page.ts` / `+layout.ts` loads.
- `<script context="module">` (Svelte 5 deprecated upstream too).
- WebSocket / SSE primitives in core.
- Vercel / Netlify Functions adapters (generic Go runtime via container/Lambda is supported).
- vitePreprocess / arbitrary preprocessor pipeline in codegen.
- JSDoc-driven type discovery (we use Go types).
- Deep dynamic-import code splitting beyond per-route.
- Runtime template interpretation.
- View Transitions API.
- Built-in i18n primitives.
- Built-in form-validation library.

Re-evaluated yearly via superseding RFC.
