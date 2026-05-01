# sveltego

> SvelteKit-shape framework for Go. Native runtime, zero JS server.

Rewritten from scratch in Go. File layout and DX mirror SvelteKit (file-based routing, `page.server.go`, layouts, hooks, form actions). Svelte components are compiled to Go source via codegen — no JS runtime on the server. The CPU bonds to Go, not V8.

## Status

🚧 Pre-alpha. MVP closed; v0.2 (form actions, hooks), v0.4 (Svelte 5 runes), and v1.1 (LLM tooling) shipped. v0.3 (client SPA + hydration), v0.5 (SvelteKit-parity catch-up), v0.6 (auth), and v1.0 (production hardening) in flight. See [GitHub issues](https://github.com/binsarjr/sveltego/issues) for the live roadmap.

## Goals

- Go-level performance — target **20–40k rps** for mid-complexity SSR
- Goroutine-native concurrency, no JS worker pool
- DX nearly identical to SvelteKit (file structure and conventions)
- Single Go binary deploy, no Node or Bun runtime
- Svelte 5 (runes) as the UI source of truth, dual-target codegen (server Go + client JS)

## Non-Goals

- 100% compatibility with SvelteKit JS plugins or libraries
- Svelte 4 legacy syntax
- Dynamic JS execution on the server

Full enumerated list with reasoning: [ADR 0005 — Non-goals](tasks/decisions/0005-non-goals.md) (mirrors [issue #94](https://github.com/binsarjr/sveltego/issues/94)).

## Architecture

```
.svelte (UI)            ──┬─→ codegen → .gen/*.go    (server SSR)
                          └─→ Vite build → JS bundle (client hydration)
page.server.go          ──→  Load(), Actions()           (//go:build sveltego)
layout.server.go        ──→  Load() with parent data flow (//go:build sveltego)
hooks.server.go         ──→  Handle, HandleError, HandleFetch
server.go               ──→  REST endpoints              (//go:build sveltego)
                          ↓
                  sveltego CLI (pure Go)
                          ↓
                       go:embed
                          ↓
                   single binary deploy
```

## Quickstart

Pre-alpha — expect rough edges. One command from any terminal — no clone, no global install:

```sh
# Recommended: npm/npx (Node >= 20)
npm create sveltego@latest ./hello
cd hello
sveltego build && ./build/app           # listens on :3000

# Or, if you have Go on PATH:
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./hello
```

`npm create sveltego@latest` downloads the scaffold engine (or falls back to `go run @latest` when Go is on PATH) and produces the same project tree either way. Add `--ai` for `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and the Copilot rules; `--tailwind=v4|v3|none` to opt into Tailwind; `--service-worker` for a starter `src/service-worker.ts`.

Build the framework CLI separately (until release binaries land in [#368](https://github.com/binsarjr/sveltego/issues/368)):

```sh
go install github.com/binsarjr/sveltego/cmd/sveltego@latest
sveltego version
```

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

## Expression philosophy

Inside `.svelte`, `{...}` mustaches are **Go expressions**, not JS:

```svelte
<script lang="go">
  import "strconv"
</script>

<h1>{Data.User.Name}</h1>
{#if len(Data.Posts) > 0}
  {#each Data.Posts as p}
    <li>{p.Title}</li>
  {/each}
{/if}
```

Field names are PascalCase (Go exported). `nil` not `null`, `len(x)` not `x.length`, `strconv.Itoa(n)` for explicit number formatting.

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
| **v1.0** | 25 | Benchmarks, docs, examples, streaming/SSG/CSP, sitemap, image opt, deploy adapters, CI/release/LSP, service worker, post-merge code-quality follow-ups |
| **v1.1** | 6 | LLM tooling: `llms.txt`, MCP server, copy-for-LLM, AI templates, provenance |

## Repository layout

This is a Go workspace (`go.work`) with one module per package:

```
packages/
  sveltego/             # Core: parser, codegen, runtime, kit, router, server, CLI
  auth/                 # First-party auth library (ADR 0006; #216–#255)
  init/                 # `sveltego-init` scaffolder (standalone binary)
  lsp/                  # Language server for `.svelte` with Go expressions
  mcp/                  # Model Context Protocol server (search_docs, lookup_api, …)
  enhanced-img/         # Image optimization helpers
  create-sveltego/      # `npm create sveltego` bridge
  adapter-server/       # Bare HTTP binary deploy
  adapter-docker/       # Multi-stage Dockerfile + distroless runtime
  adapter-lambda/       # AWS Lambda via aws-lambda-go-api-proxy
  adapter-static/       # SSG output (stub; tracks #65)
  adapter-cloudflare/   # Cloudflare Workers (stub; tracks Workers Go runtime)
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
