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

### MVP (#1–23) — minimum to render a page

Foundational RFCs (parser strategy, expression syntax, file convention, codegen layout), then bootstrap (Go module, CLI), then the core pipeline:

- Parser: lexer → AST for the Svelte 5 subset we need.
- Codegen: text → element/attribute → expression → `{#if}` → `{#each}` → `<script lang="go">` extraction.
- Runtime: `render.Writer` with `sync.Pool`, escape utilities, `kit.LoadCtx`, Locals.
- Router: scan `src/routes/`, emit manifest, radix-tree match.
- HTTP pipeline: Load → Render → Response.
- CLI: `sveltego build` end-to-end.
- Test harness for golden codegen + a hello-world example.

### v0.2 (#24–33) — Form Actions & Hooks

Layout chain rendering, `+layout.server.go` parent data flow, `Handle` / `HandleError` / `HandleFetch`, `+error.svelte` boundaries, `+server.go` REST endpoints, `Actions()` map with form binding (urlencoded + multipart), `kit.Cookies`, `kit.Redirect / Fail / Error` sentinel helpers.

### v0.3 (#34–42) — Client SPA & Hydration

Vite integration for the Svelte client bundle, `window.__sveltego__` hydration payload, client hydrate runtime, SPA router (link interception + history), `__data.json` per-route endpoint, `use:enhance` for forms, prefetch on hover/viewport, precompressed static asset serving, `sveltego dev` with HMR.

### v0.4 (#43–59) — Svelte 5 Full Coverage

Runes: `$props`, `$state`, `$derived`, `$effect`, `$bindable`. Snippets and `{@render}`. Legacy slots (default + named, with slot props). Special elements: `<svelte:head>`, `<svelte:body>` / `<svelte:window>` / `<svelte:document>`, `<svelte:component>`. CSS scope hash matching upstream. `{@html}`, `{@const}`, `{#await}`, `{#key}`. Nested component import and rendering.

### v1.0 (#60–69) — Production Ready

Benchmark suite vs adapter-bun with nightly regression gate. Docs site (Vitepress). Blog and dashboard examples. Streaming responses. Prerender / SSG mode. CSP nonce injection. CI (GitHub Actions). Release pipeline (release-please + GoReleaser). LSP for `.svelte` with Go expressions.

### v1.1 (#70–75) — LLM & AI Tooling

`llms.txt` + `llms-full.txt` for AI agents. `sveltego mcp` Model Context Protocol server (`search_docs`, `lookup_api`, `validate_template`, `scaffold_route`). Markdown-first docs with copy-for-LLM buttons. AI assistant project templates (`CLAUDE.md`, `.cursorrules`, `AGENTS.md`, copilot instructions) wired into `sveltego init --ai`. Provenance comments in generated `.gen/*.go`. AI-assisted development guide page.

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
- [x] 19 labels, 5 milestones, 69 issues created
- [x] All 69 issues rewritten in English with industry-standard detail
- [x] v1.1 milestone added — LLM & AI tooling (#70–75)
- [ ] Land RFCs (#1–4): parser strategy, expression syntax, file convention, codegen output layout
- [ ] Bootstrap Go module + CLI skeleton (#5, #6)
- [ ] Build the MVP pipeline end-to-end (#7–23)
- [ ] Smoke-test on hello-world example (#23)

## Open questions

- Pinned upstream Svelte commit for CSS hash equivalence (#54) — pick once first build is green.
- Default `Save-Data` behavior for prefetch (#40) — assume conservative-on; revisit after first dogfooding.
- Whether to ship a fixed sanitizer for `{@html}` (#55) — current decision: no, recommend `bluemonday` in docs.

## Out of scope (for now)

- Svelte 4 legacy reactivity (`$:`, stores autoload everywhere).
- Server-side dynamic JS execution.
- Native Go bundler replacing Vite for client.
- View Transitions API.
- Multi-tenant / RBAC primitives in `kit`.
