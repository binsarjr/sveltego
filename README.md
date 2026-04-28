# sveltego

> SvelteKit-shape framework for Go. Native runtime, zero JS server.

Rewritten from scratch in Go. File layout and DX mirror SvelteKit (file-based routing, `+page.server.go`, layouts, hooks, form actions). Svelte components are compiled to Go source via codegen — no JS runtime on the server. The CPU bonds to Go, not V8.

## Status

🚧 Pre-alpha. Spec and RFC phase. See [GitHub issues](https://github.com/binsarjr/sveltego/issues) for the roadmap.

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

## Architecture

```
.svelte (UI)            ──┬─→ codegen → .gen/*.go    (server SSR)
                          └─→ Vite build → JS bundle (client hydration)
+page.server.go         ──→  Load(), Actions()
+layout.server.go       ──→  Load() with parent data flow
hooks.server.go         ──→  Handle, HandleError, HandleFetch
+server.go              ──→  REST endpoints
                          ↓
                  sveltego CLI (pure Go)
                          ↓
                       go:embed
                          ↓
                   single binary deploy
```

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

5 milestones tracked on GitHub:

| Milestone | Scope |
|-----------|-------|
| **MVP** (#1–23) | Parser, codegen, runtime, router, CLI — minimum to render a page |
| **v0.2** (#24–33) | Layouts, hooks, error boundaries, form actions, cookies |
| **v0.3** (#34–42) | Vite client bundle, hydration, SPA router, dev server |
| **v0.4** (#43–59) | Svelte 5 runes, slots, snippets, special elements, scoped CSS |
| **v1.0** (#60–69) | Benchmarks, docs, examples, streaming/SSG/CSP, CI/release/LSP |

## See also

- [tasks/todo.md](tasks/todo.md) — current execution plan and phase tracking
- [tasks/lessons.md](tasks/lessons.md) — design decisions and trade-offs
- [GitHub issues](https://github.com/binsarjr/sveltego/issues) — milestone breakdown
