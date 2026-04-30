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

Layout chain rendering, `layout.server.go` parent data flow, `Handle` / `HandleError` / `HandleFetch` / `Reroute` / `Init`, `+error.svelte` boundaries, `server.go` REST endpoints, `Actions()` map with form binding (urlencoded + multipart), `kit.Cookies`, `kit.Redirect / Fail / Error` sentinel helpers, route groups `(group)/` + layout reset `@`, page options (`Prerender`, `SSR`, `CSR`, `TrailingSlash`), env var convention (`$env/static`, `$env/dynamic`).

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
- [x] Land render + kit + codegen pipeline (#9–#15) — landed Phase 0f 2026-04-29; render `Writer` w/ sync.Pool + WriteEscape/WriteEscapeAttr; kit RenderCtx/LoadCtx/Cookies stubs; codegen text/element/mustache/if/each/script-hoist/PageData-inference; ADR 0004 amended (drop WriteAttr, lock struct-literal-only PageData)
- [x] Land router foundation (#18 scan + emit, #19 radix matcher, #76 param matchers + built-ins, #77 optional + rest segments) — landed Phase 0g 2026-04-30; runtime/router/ + internal/routescan/ + exports/kit/params/ + internal/codegen/manifest.go; integration smoke compiles end-to-end
- [x] Land HTTP server pipeline (#20) — landed Phase 0h 2026-04-30; `packages/sveltego/server/` exposes `New(Config) (*Server, error)`, `ListenAndServe`, `Shutdown`, `ServeHTTP`; pipeline is Match → +server.go branch / 405 / Load / Render / Response with shell template parsed once at boot; race-safe under 100×100 concurrent load; ~163 ns/op in-process p50 on M1 Pro
- [x] Land CLI build orchestrator + `$lib` alias (#21, #83) — landed Phase 0i 2026-04-30; `internal/codegen.Build` walks `src/routes/` → `.gen/routes/.../page.gen.go` + `.gen/manifest.gen.go` + conditional `.gen/embed.go`; CLI `build`/`compile` resolve project root via go.mod walk, `build` wraps `go build -o`; `$lib` import literals rewritten using user's go.mod module path; integration test (`-tags=integration`) builds fixture binary
- [x] Phase 0i-fix — convention amend, manifest adapter, wire emit (closes #106, #107, #108) — landed 2026-04-30; user `.go` files drop `+` prefix and require `//go:build sveltego`; codegen emits `.gen/usersrc/<encoded>/` mirror tree so wire glue can import by Go-valid path; manifest emits per-route `render__<alias>` adapters that widen `Page{}.Render(data PageData)` to `router.PageHandler`; `wire.gen.go` re-exports user Load (and stubs Actions when absent); ADR 0003 amended; routescan emits a warning diagnostic when the build constraint is missing
- [x] Phase 0j (partial) — `playgrounds/basic` authored under the new convention; non-user-go portion (svelte templates, app.html, cmd/app, README, CI workflow) committed; user `.go` files (`page.server.go` under `src/routes/` and `[id]/`) deferred to Phase 0j-fix because the existing pre-commit hook rejects them (#110). Codegen + binary-build pipeline validated locally before the partial commit. Filed: #109 (runtime PageData type-assertion mismatch), #110 (pre-commit hook gap on `[bracket]` paths and all-tagged dirs).
- [x] Phase 0j-fix — closed #109 (`emitPageDataStruct` now emits `type PageData = struct{...}` as a type alias so the user's anonymous struct literal returned by Load() is type-identical to `PageData`, satisfying the manifest adapter's `data.(PageData)` assertion); closed #110 (`.githooks/pre-commit` skips user `.go` files under `src/{routes,params}/`, `hooks.server.go`, and any file whose first line is `//go:build ... sveltego ...`); landed the deferred user `.go` files (`playgrounds/basic/src/routes/page.server.go` and `.../post/[id]/page.server.go`); ungated `playground-smoke` CI job. Local smoke renders `/` (Hello, sveltego!) and `/post/123` (Post 123) against the compiled binary. Closes #23. **MVP scope CLOSED — all 24 MVP feature issues + 11 foundation issues (#95–105) shipped.**
- [ ] v0.2 milestone — layouts (#24), hooks (#25), error boundaries (#26), form actions (#27–#29), route groups (#30), page options (#31, #80), env (#32, #33), `kit.Cookies/Redirect/Fail/Error` (#78, #79, #81, #82). 15 issues total.

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
