# sveltego — Execution Plan

## Direction

Rewrite the SvelteKit shape (routing, load, actions, hooks, layouts) in pure Go. No JS runtime on the server. `.svelte` files are compiled to Go source via codegen for SSR, and to a Svelte 5 client bundle via Vite for hydration.

This replaces the earlier "embed JS runtime in Go" investigation. That direction was rejected after the runtime survey because every option (goja, v8go, quickjs, wazero, subprocess Bun) bonds CPU to a JS engine and either kills throughput or breaks cross-compile. See [`tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md`](lessons/2026-04-29-pivot-to-go-native-rewrite.md) for the chain of reasoning.

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

### MVP (42 issues) — minimum to render a page

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

Layout chain rendering, `layout.server.go` parent data flow, `Handle` / `HandleError` / `HandleFetch` / `Reroute` / `Init`, `_error.svelte` boundaries, `server.go` REST endpoints, `Actions()` map with form binding (urlencoded + multipart), `kit.Cookies`, `kit.Redirect / Fail / Error` sentinel helpers, route groups `(group)/` + layout reset `@`, page options (`Prerender`, `SSR`, `CSR`, `TrailingSlash`), env var convention (`$env/static`, `$env/dynamic`).

### v0.3 (21 issues) — Client SPA & Hydration

Vite integration for the Svelte client bundle, `window.__sveltego__` hydration payload, client hydrate runtime, SPA router (link interception + history), `__data.json` per-route endpoint, `use:enhance` for forms, prefetch on hover/viewport, precompressed static asset serving, `sveltego dev` with HMR. Full `$app/navigation` API (`goto`, `invalidate`, `preload`, `pushState`), Snapshot API for cross-nav state, typed `kit.Link` with route params, `kit.Asset` with hashed static URLs.

### v0.4 (19 issues) — Svelte 5 Full Coverage

Runes: `$props`, `$state`, `$derived`, `$effect`, `$bindable`. Snippets and `{@render}`. Legacy slots (default + named, with slot props). Special elements: `<svelte:head>`, `<svelte:body>` / `<svelte:window>` / `<svelte:document>`, `<svelte:component>`, `<svelte:options>`. CSS scope hash matching upstream. `{@html}`, `{@const}`, `{#await}`, `{#key}`. Nested component import and rendering. Compile-time a11y warnings.

### v1.0 (28 issues) — Production Ready

Benchmark suite vs adapter-bun with nightly regression gate. Docs site (Vitepress). Blog and dashboard examples. Streaming responses. Prerender / SSG mode. CSP nonce injection. CI (GitHub Actions). Release pipeline (release-please + GoReleaser). LSP for `.svelte` with Go expressions. Sitemap/robots helpers, image optimization (`<Image>`), service worker convention, deploy adapters (server, docker, static, lambda, cloudflare).

### v1.1 (6 issues) — LLM & AI Tooling

`llms.txt` + `llms-full.txt` for AI agents. `sveltego mcp` Model Context Protocol server (`search_docs`, `lookup_api`, `validate_template`, `scaffold_route`). Markdown-first docs with copy-for-LLM buttons. AI assistant project templates (`CLAUDE.md`, `.cursorrules`, `AGENTS.md`, copilot instructions) wired into `sveltego init --ai`. Provenance comments in generated `.gen/*.go`. AI-assisted development guide page.

### v0.5 (23 issues) — SvelteKit-parity catch-up

Tracks upstream `sveltejs/kit` enhancements that landed after the initial roadmap snapshot: `kit.After()`, `HandleAction` global middleware, `Init()` error fallback, short-circuiting `HandleError`, `LoadCtx.Speculative()`, typed `kit.HTTPError`, headers/cookies on error responses, `kit.RedirectReload`, `LoadCtx.RawParam()`, codegen `RouteID` constants, dev-only code stripping, etc. Also includes the cookie-session auth core (`auth/cookiesession`) — separate package modeled on `svelte-kit-cookie-session`.

### v0.6 (40 issues) — Authentication

Full `sveltego-auth` package modeled on the better-auth feature set: master plan (#155), package scaffold, storage adapters (memory, `database/sql`, pgx), session token format + DB-backed strategy, encrypted-cookie session strategy, password hashing (argon2id), email/password flows, email verification, password reset, mailer + SMS adapters, magic-link, OTP, TOTP, OAuth core + provider adapters, RBAC primitives, audit log, 2FA, account linking, etc.

### Standalone

- #94 (closed) — non-goals RFC retained as ADR 0005; the GitHub issue is unmilestoned.

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
- [x] 20 labels, 6 milestones, 105 issues created (initial roadmap snapshot; live counts now 8 milestones / 194 issues — see milestone table above)
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
- [x] Land HTTP server pipeline (#20) — landed Phase 0h 2026-04-30; `packages/sveltego/server/` exposes `New(Config) (*Server, error)`, `ListenAndServe`, `Shutdown`, `ServeHTTP`; pipeline is Match → _server.go branch / 405 / Load / Render / Response with shell template parsed once at boot; race-safe under 100×100 concurrent load; ~163 ns/op in-process p50 on M1 Pro
- [x] Land CLI build orchestrator + `$lib` alias (#21, #83) — landed Phase 0i 2026-04-30; `internal/codegen.Build` walks `src/routes/` → `.gen/routes/.../page.gen.go` + `.gen/manifest.gen.go` + conditional `.gen/embed.go`; CLI `build`/`compile` resolve project root via go.mod walk, `build` wraps `go build -o`; `$lib` import literals rewritten using user's go.mod module path; integration test (`-tags=integration`) builds fixture binary
- [x] Phase 0i-fix — convention amend, manifest adapter, wire emit (closes #106, #107, #108) — landed 2026-04-30; user `.go` files drop `+` prefix and require `//go:build sveltego`; codegen emits `.gen/usersrc/<encoded>/` mirror tree so wire glue can import by Go-valid path; manifest emits per-route `render__<alias>` adapters that widen `Page{}.Render(data PageData)` to `router.PageHandler`; `wire.gen.go` re-exports user Load (and stubs Actions when absent); ADR 0003 amended; routescan emits a warning diagnostic when the build constraint is missing
- [x] Phase 0j (partial) — `playgrounds/basic` authored under the new convention; non-user-go portion (svelte templates, app.html, cmd/app, README, CI workflow) committed; user `.go` files (`page.server.go` under `src/routes/` and `[id]/`) deferred to Phase 0j-fix because the existing pre-commit hook rejects them (#110). Codegen + binary-build pipeline validated locally before the partial commit. Filed: #109 (runtime PageData type-assertion mismatch), #110 (pre-commit hook gap on `[bracket]` paths and all-tagged dirs).
- [x] Phase 0j-fix — closed #109 (`emitPageDataStruct` now emits `type PageData = struct{...}` as a type alias so the user's anonymous struct literal returned by Load() is type-identical to `PageData`, satisfying the manifest adapter's `data.(PageData)` assertion); closed #110 (`.githooks/pre-commit` skips user `.go` files under `src/{routes,params}/`, `hooks.server.go`, and any file whose first line is `//go:build ... sveltego ...`); landed the deferred user `.go` files (`playgrounds/basic/src/routes/_page.server.go` and `.../post/[id]/page.server.go`); ungated `playground-smoke` CI job. Local smoke renders `/` (Hello, sveltego!) and `/post/123` (Post 123) against the compiled binary. Closes #23. **MVP scope CLOSED — all 24 MVP feature issues + 11 foundation issues (#95–105) shipped.**
- [x] Phase 0p — CI budget split (2026-04-30). `.github/workflows/ci.yml` split lint-and-test by event: PR runs `ubuntu-latest, go1.23.x` only (~1 job); push-to-main runs full 3 OS × 2 Go matrix (6 jobs). `isolated-modules` and `playground-smoke` gated to push-only. `paths-ignore` for `**.md`, `tasks/**`, `tasks/decisions/**`, `docs/**` skips CI on docs-only commits. `concurrency: cancel-in-progress` on both ci.yml and bench.yml. `bench.yml` PR trigger removed; now runs on push-to-main + nightly schedule + manual `workflow_dispatch`. PR-time job count drops from ~11 to ~3-4 (~65% cut); main-time coverage unchanged.
- [x] Phase 0q — hooks group (#26 Handle + Sequence, #27 HandleError + HandleFetch, #80 Reroute, #81 Init) (2026-04-30). `kit.RequestEvent`, `kit.Response`, `kit.HandleFn/ResolveFn/HandleErrorFn/HandleFetchFn/RerouteFn/InitFn`, `kit.SafeError`, `kit.Sequence`, `kit.Hooks` bundle + `WithDefaults`, `kit.IdentityHandle/IdentityHandleError/IdentityHandleFetch/IdentityReroute/IdentityInit`. Codegen `internal/codegen/hookscan.go` + `hooks_emit.go` discover `src/hooks.server.go`, validate signatures, mirror to `.gen/hookssrc/`, emit `.gen/hooks.gen.go` `Hooks() kit.Hooks` factory. Server pipeline rebuilt around `RequestEvent`: Reroute before match, Handle wraps resolve, HandleFetch on `ev.Fetch`, HandleError sanitizes uncaught errors (sentinel branches preserved), Init runs before `ListenAndServe` binds. Panic recovery at the framework boundary funnels into HandleError. 12 hook-integration tests + 4 hook codegen goldens + signature-mismatch + build-test coverage.
- [x] Phase 0x — AI assistant templates (#73) (2026-04-30). `templates/ai/` shipped as a standalone Go module exposing `embed.FS` with `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.github/copilot-instructions.md`. Templates teach Go expressions in mustaches (PascalCase, `nil`, `len()`), file conventions (`page.server.go` no `+`, `//go:build sveltego`), and the kit API (`kit.Redirect`, `kit.ActionMap`, `kit.SafeError`). Snapshot test asserts anchor strings + cross-links, not byte-equality. CLI `init --ai` deferred — no `init` command yet, follow-up issue filed.
- [x] Phase 0ii — deploy adapters (#93) (2026-04-30). `packages/adapter-{server,docker,lambda,static,cloudflare,auto}` all populated: server copies pre-built binary + assets; docker emits multi-stage Dockerfile + .dockerignore (distroless runtime, healthcheck stanza); lambda emits `.gen/lambda/main.go` wrapping the user's router with `aws-lambda-go-api-proxy/httpadapter` + SAM template stub; static and cloudflare ship as stubs returning `ErrNotImplemented` (blocked on #65 and Workers Go runtime gaps respectively). `packages/adapter-auto` dispatches by target name + auto-detects from env (`SVELTEGO_ADAPTER`, `AWS_LAMBDA_RUNTIME_API`, `CF_PAGES`). Standalone CLI binary at `packages/adapter-auto/cmd/sveltego-adapter` (mirrors `packages/lsp/cmd/sveltego-lsp` per Phase 0ff lesson #1). `cmd/sveltego` `--target` flag integration deferred to follow-up (Phase 0ee owns that path).
- [x] v0.2 milestone — layouts (#24), hooks (#25), error boundaries (#26), form actions (#27–#29), route groups (#30), page options (#31, #80), env (#32, #33), `kit.Cookies/Redirect/Fail/Error` (#78, #79, #81, #82). 15 issues total. **CLOSED.**
- [x] v0.4 milestone — Svelte 5 runes (#43–#47), element handlers (#49, #50, #52, #53), snippets / `{@render}` (#48), template blocks (`{@html}`, `{@const}`, `{#await}`, `{#key}` — #55–#58), scoped CSS + `<svelte:options>` (#54, #90), `<svelte:head>` + nested component invocation (#51, #59), compile-time a11y warnings (#86). 19 issues total. **CLOSED.**
- [x] v1.1 milestone — Vitepress docs site + `llms.txt` + copy-for-LLM + AI guide (#70, #71, #72, #73, #74, #75). `sveltego mcp` scaffold (#71), AI templates `templates/ai/` (#73), `llms.txt` + `llms-full.txt`, markdown guide. 6 issues total. **CLOSED.**
- [x] v0.5 wave (partial, ongoing) — SvelteKit-parity catch-up: `kit.After()`, `HandleAction`, `Init()` error fallback, `HandleError` short-circuit, `LoadCtx.Speculative()`, `kit.HTTPError`, headers/cookies on error responses, `kit.RedirectReload`, `LoadCtx.RawParam()`, codegen `RouteID` constants, dev-only code stripping, `kit.Header` Set vs Add, MIME type registration, `kit.Error` default message, `static asset precompression`. Auth core (`packages/auth`) scaffold + storage + mailer + SMS adapters (#216, #217, #226, #227). ADR 0006 (auth master plan), ADR 0007 (Svelte semantics revisit). STABILITY.md per package. 23 issues tracked; in flight.
- [ ] v0.5 remaining — `kit.After`, `HandleAction` wire-up, `iter.Seq` streamed, kit.Cron, child-component pkg gen, cookiesession (#160, #161, #166, #167, #173, etc.) — 4 open issues.
- [ ] v0.6 in-flight — auth: session strategy (#220), password hashing (#222), email verification (#224), magic-link (#229), OTP (#230), TOTP (#231), OAuth core (#236), RBAC (#245), and many more. 30 open issues.

## Open questions

- Pinned upstream Svelte commit for CSS hash equivalence (#54) — pick once first build is green.
- Default `Save-Data` behavior for prefetch (#40) — assume conservative-on; revisit after first dogfooding.
- Whether to ship a fixed sanitizer for `{@html}` (#55) — current decision: no, recommend `bluemonday` in docs.

## Out of scope (for now)

Canonical list: [ADR 0005 — Non-goals](decisions/0005-non-goals.md). [ADR 0007 — Svelte Semantics Revisit](decisions/0007-svelte-semantics-revisit.md) covers the Full-Svelte vs Go-mustache question (Status: Proposed). Quick reference:

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
