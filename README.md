# sveltego

> SvelteKit-shape framework for Go. Native runtime, zero JS server.

Ditulis ulang dari nol di Go. Struktur file & DX mirroring SvelteKit (file-based routing, `+page.server.go`, layouts, hooks, form actions). Render Svelte component via codegen `.svelte` → Go source. Tanpa JS runtime di server. CPU bound ke Go, bukan V8.

## Status

🚧 Pre-alpha. Spec & RFC phase. Lihat issues untuk roadmap.

## Goal

- Performa level Go pure — target **20-40k rps** SSR mid-complexity
- Concurrency goroutine-native, tanpa worker pool JS
- DX hampir identik SvelteKit (file structure, conventions)
- Single Go binary deploy, no Node/Bun runtime
- Svelte 5 (runes) sebagai UI source-of-truth, dual-target codegen

## Bukan Goal

- 100% kompatibilitas SvelteKit JS plugin/lib
- Mendukung Svelte 4 legacy syntax
- Dynamic JS execution di server

## Arsitektur singkat

```
.svelte (UI)            ──┬─→ codegen → .gen/*.go    (server SSR)
                           └─→ Vite build → JS bundle  (client hydration)
+page.server.go         ──→  Load(), Actions()
hooks.server.go         ──→  Handle, HandleError, HandleFetch
+server.go              ──→  REST endpoints
                           ↓
                  sveltego CLI (pure Go)
                           ↓
                       go:embed
                           ↓
                   single binary deploy
```

## Filosofi expression

Dalam `.svelte`, expression dalam `{...}` adalah **Go expression**, bukan JS:

```svelte
<script lang="go">
  import "strconv"
</script>

<h1>{data.User.Name}</h1>
{#if len(data.Posts) > 0}
  {#each data.Posts as p}
    <li>{p.Title}</li>
  {/each}
{/if}
```

## Lihat juga

- [tasks/todo.md](tasks/todo.md) — feasibility report & R&D notes
- [tasks/lessons.md](tasks/lessons.md) — design decisions & trade-offs
- GitHub issues — roadmap & MVP breakdown
