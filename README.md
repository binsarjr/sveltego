# sveltego

> SvelteKit-shape framework for Go. Pure-Svelte templates, Go-only server, zero JS at runtime.

Write your UI in Svelte 5. Write your server in Go. Deploy a single Go binary — no Node, no Bun, no JS engine on the request path. File layout and DX mirror SvelteKit (file-based routing, server-side data loaders, layouts, hooks, form actions).

> Pre-alpha. Expect rough edges. Pin the versions in the quickstart.

## Quickstart

```sh
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@v0.1.0-alpha.1 ./hello
cd hello
go install github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego@v0.1.0-alpha.1
sveltego build && ./build/app                                  # listens on :3000
```

`sveltego build` chains codegen → Vite → `go build` in one step. No separate `go build` invocation needed.

Useful flags on `sveltego-init`:

- `--ai` — emits `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and Copilot rules
- `--tailwind=v4|v3|none` — opt into Tailwind
- `--service-worker` — starter `src/service-worker.ts`

Full annotated walkthrough: [docs/guide/quickstart.md](docs/guide/quickstart.md).

<details><summary>From-source path (clone the repo)</summary>

```sh
git clone https://github.com/binsarjr/sveltego
cd sveltego
go install ./packages/sveltego/cmd/sveltego
go install ./packages/init/cmd/sveltego-init
sveltego-init ./hello
```

</details>

## How it looks

Templates are **100% pure Svelte/JS/TS**. No Go syntax inside `.svelte` files:

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

A sibling Go file owns the data shape:

```go
type PageData struct {
    User  User   `json:"user"`
    Posts []Post `json:"posts"`
}

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{User: fetchUser(ctx), Posts: fetchPosts(ctx)}, nil
}
```

Codegen reads the Go AST and emits a `.svelte.d.ts` declaration so Svelte LSP autocompletes `data.user.name` end to end. JSON tags drive field names at the Go ↔ TypeScript boundary; `kit.Streamed[T]` maps to `Promise<T[]>` for native `{#await}` blocks.

## Render modes

Pick a mode per route by setting fields on `kit.PageOptions` in `_page.server.go`. **SSR is the default.** Layouts cascade; page-level overrides win.

| Mode    | When to use                       | Recipe                                             | Runtime path                                |
|---------|-----------------------------------|----------------------------------------------------|---------------------------------------------|
| **SSR** (default) | Dynamic, fresh data per request   | Default — no opt-in needed                         | Go `Render()` emits HTML; client hydrates    |
| **SSG** | Marketing, docs, blog             | `kit.PageOptions{Prerender: true}`                 | Build-time HTML; static handler at runtime  |
| **SPA** | Authenticated dashboards          | `kit.PageOptions{SSR: false}`                      | App shell + JSON payload; client renders    |
| **Static** | No per-page data                  | No `_page.server.go`; pure `.svelte` only          | App shell + empty payload; client renders   |

```go
// SSR (default)
func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{Posts: fetchPosts(ctx)}, nil
}
```

```go
// SSG
const Prerender = true

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{Title: "About"}, nil
}
```

```go
// SPA
const SSR = false

func Load(ctx kit.LoadCtx) (PageData, error) {
    return PageData{User: currentUser(ctx)}, nil
}
```

Full reference + decision tree: [docs/render-modes.md](docs/render-modes.md).

## Learn more

- [Quickstart](docs/guide/quickstart.md) — annotated end-to-end walkthrough
- [Routing](docs/guide/routing.md) — file-based routes, params, groups
- [Load functions](docs/guide/load.md) — server-side data loading
- [Form actions](docs/guide/actions.md) — progressive-enhancement forms
- [Hooks](docs/guide/hooks.md) — `Handle`, `HandleError`, `HandleFetch`
- [Components & snippets](docs/guide/components.md)
- [Build & deploy](docs/guide/build.md) · [Deploy targets](docs/guide/deploy.md)
- [Migration from SvelteKit](docs/guide/migration.md)
- [FAQ](docs/guide/faq.md)
- [AI-assisted development](docs/ai-development.md)

## Community

- Issues & roadmap: [github.com/binsarjr/sveltego/issues](https://github.com/binsarjr/sveltego/issues)
