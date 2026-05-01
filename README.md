# sveltego

> SvelteKit-shape framework for Go. Pure-Svelte templates, Go-only server, zero JS at runtime.

Rewritten from scratch in Go. File layout and DX mirror SvelteKit (file-based routing, server-side Go data loaders, layouts, hooks, form actions). Templates are 100% pure Svelte/JS/TS; Go owns the server and emits TypeScript declarations for IDE autocompletion. The runtime is hybrid: build-time static prerender (SSG) for marketing-style routes, client-side render (SPA) for everything else. The deployed Go binary has no JS engine.

## Status

🚧 Pre-alpha. MVP closed; v0.2 (form actions, hooks), v0.4 (Svelte 5 runes), and v1.1 (LLM tooling) shipped. v0.3 (client SPA + hydration), v0.5 (SvelteKit-parity catch-up), v0.6 (auth), and v1.0 (production hardening) in flight. See [GitHub issues](https://github.com/binsarjr/sveltego/issues) for the live roadmap.

[ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md) (2026-05-01) pivots templates from Go-decorated mustaches to **100% pure Svelte/JS/TS**. Migration phases land via [RFC #379](https://github.com/binsarjr/sveltego/issues/379) → [#380](https://github.com/binsarjr/sveltego/issues/380)–[#385](https://github.com/binsarjr/sveltego/issues/385).

## Goals

- Go-level performance — target **20–40k rps** for SSG output (zero per-request work) and JSON-payload responses on SPA-mode dynamic routes
- Goroutine-native concurrency, no JS worker pool
- DX identical to SvelteKit at the template layer — copy `.svelte` files between projects unchanged
- Single Go binary deploy, no Node or Bun runtime at request time (Node runs only during `sveltego build` for SSG)
- Svelte 5 (runes) as the UI source of truth; Go AST → TypeScript declaration codegen for type-safe `data` props in templates

## Non-Goals

- A server-side JS runtime at request time
- Svelte 4 legacy syntax
- Backward compatibility with the previous Mustache-Go template dialect (pre-alpha; users rewrite)

Full enumerated list with reasoning: [ADR 0005 — Non-goals](tasks/decisions/0005-non-goals.md) (mirrors [issue #94](https://github.com/binsarjr/sveltego/issues/94)). Template semantics: [ADR 0008 — Pure-Svelte pivot](tasks/decisions/0008-pure-svelte-pivot.md).

## Architecture

Pure-Svelte templates on the client, Go-only on the server, hybrid runtime.

```
.svelte (UI, 100% Svelte/JS/TS)  ──→ Vite build → JS bundle   (client hydration)
                                  └─→ svelte/server (build time only) → static HTML (SSG)
server-side Go (route data)      ──→ Load(), Actions()        (//go:build sveltego)
                                  └─→ codegen → .svelte.d.ts  (Go AST → TypeScript types)
hooks.server.go                  ──→ Handle, HandleError, HandleFetch
                                          ↓
                                  sveltego CLI (pure Go)
                                          ↓
                                       go:embed
                                          ↓
                                  single binary deploy
                                  + static/ (SSG output, optional)
```

Routes opting into `kit.PageOptions{Prerender: true}` ship as static HTML rendered at build time via `svelte/server`. Everything else ships as a SPA shell + JSON payload at runtime; the client mounts and renders. **Node is required at build time only.** The deployed Go binary plus `static/` is the entire deployable.

## Quickstart

Pre-alpha — expect rough edges. One Go command from any terminal, no clone, no global install:

```sh
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./hello
cd hello
go install github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego@latest    # build CLI (until #368 ships release binaries)
sveltego build && ./build/app                                  # listens on :3000
```

Add `--ai` for `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and the Copilot rules; `--tailwind=v4|v3|none` to opt into Tailwind; `--service-worker` for a starter `src/service-worker.ts`.

`sveltego build` chains codegen → Vite → `go build` in one step; you do not need a separate `go build` invocation. The full quickstart with annotated layout lives in [docs/guide/quickstart.md](docs/guide/quickstart.md).

<details><summary>From-source path (clone the repo)</summary>

```sh
git clone https://github.com/binsarjr/sveltego
cd sveltego
go install ./packages/sveltego/cmd/sveltego
go install ./packages/init/cmd/sveltego-init
sveltego-init ./hello
```

</details>

## Template philosophy

Templates are **100% pure Svelte/JS/TS** ([ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md)). No Go syntax inside `.svelte` files; everything reads like SvelteKit:

```svelte
<script lang="ts">
  let { data } = $props();
</script>

<h1>Hello {data.user.name}</h1>
{#if data.posts.length > 0}
  <ul>
    {#each data.posts as post}
      <li>{post.title}</li>
    {/each}
  </ul>
{/if}
```

Server-side, a Go file returns the `data` shape:

```go
//go:build sveltego

type PageData struct {
    User  User   `json:"user"`
    Posts []Post `json:"posts"`
}

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{User: fetchUser(ctx), Posts: fetchPosts(ctx)}, nil
}
```

Codegen reads the Go AST and emits a sibling `.svelte.d.ts` declaration so Svelte LSP / vscode-svelte autocomplete `data.user.name` end to end. JSON tags drive field names at the Go ↔ TypeScript boundary; `kit.Streamed[T]` maps to `Promise<T[]>` for native `{#await}` blocks.

## Roadmap

8 milestones tracked on GitHub (counts as of the latest doc-drift sync):

| Milestone | Issues | Scope |
|-----------|--------|-------|
| **MVP** | 42 | Foundation RFCs (#95–97) + setup (#98–105: lint, hooks, release-please, CI, PR template, AI sync, golden tests, bench gate), parser, codegen, runtime, router (incl. param matchers, optional/rest), `$lib` alias, CLI, Phase 0i-fix bugs (#106–110) |
| **v0.2** | 15 | Layouts, hooks (incl. `Reroute`/`Init`), error boundaries, form actions, cookies, route groups, page options, `$env` |
| **v0.3** | 21 | Vite client bundle, hydration, SPA router, full `$app/navigation`, Snapshot, typed `kit.Link`, hashed `kit.Asset`, dev server |
| **v0.4** | 19 | Svelte 5 runes, slots, snippets, special elements, `<svelte:options>`, scoped CSS, a11y warnings |
| **v0.5** | 23 | SvelteKit-parity catch-up: upstream-tracked enhancements (`kit.After`, `HandleAction`, `RawParam`, `RouteID`, etc.) and the cookie-session auth core |
| **v0.6** | 40 | Authentication: `sveltego-auth` master plan (#155), storage adapters, sessions, password / magic-link / OTP / OAuth flows |
| **v1.0** | 28 | Benchmarks, docs, examples, streaming/SSG/CSP, sitemap, image opt, deploy adapters, CI/release/LSP, service worker, post-merge code-quality follow-ups |
| **v1.1** | 6 | LLM tooling: `llms.txt`, MCP server, copy-for-LLM, AI templates, provenance |

## Repository layout

This is a Go workspace (`go.work`) with one module per package:

```
packages/
  sveltego/             # Core: parser, codegen, runtime, kit, router, server, CLI
  auth/                 # First-party auth library (ADR 0006; #216–#255)
  init/                 # `sveltego-init` scaffolder (standalone binary)
  lsp/                  # Language server for sveltego routes (Go-side `Load` + emitted `.svelte.d.ts`)
  mcp/                  # Model Context Protocol server (search_docs, lookup_api, …)
  enhanced-img/         # Image optimization helpers
  adapter-server/       # Bare HTTP binary deploy
  adapter-docker/       # Multi-stage Dockerfile + distroless runtime
  adapter-lambda/       # AWS Lambda via aws-lambda-go-api-proxy
  adapter-static/       # SSG output (stub; tracks #65)
  adapter-cloudflare/   # Cloudflare Workers (stub; tracks Workers Go runtime)
  adapter-fastly/       # Fastly Compute@Edge (Wasm; #298)
  adapter-auto/         # Dispatch by env / target name + standalone CLI
bench/                  # Benchmark harness vs adapter-bun (RFC #105)
benchmarks/             # Per-package microbenchmarks
playgrounds/            # End-to-end example apps (basic, blog, dashboard, …)
templates/ai/           # Embedded AGENTS.md / CLAUDE.md / .cursorrules / copilot
docs/                   # VitePress site (guide + reference)
tasks/                  # Execution plan, lessons, ADRs
```

Per-package `STABILITY.md` and `CHANGELOG.md` are the authoritative source for what is safe to import.

## See also

- [tasks/todo.md](tasks/todo.md) — current execution plan and phase tracking
- [tasks/lessons.md](tasks/lessons.md) — design decisions and trade-offs
- [GitHub issues](https://github.com/binsarjr/sveltego/issues) — milestone breakdown
