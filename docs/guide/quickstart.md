---
title: Quickstart
order: 10
summary: Install the sveltego CLI, scaffold a project, run dev, build, ship.
---

# Quickstart

sveltego is pre-alpha. Expect rough edges and breaking changes. The shape below tracks the v1.0 milestone; commands and flags may shift.

## Install

```sh
go install github.com/binsarjr/sveltego/cmd/sveltego@latest
go install github.com/binsarjr/sveltego/init/cmd/sveltego-init@latest
sveltego version
```

Two binaries today: `sveltego` (build / compile / routes / version) and `sveltego-init` (project scaffolder, separate module under `packages/init`). No Node runtime is required to run a built app; Node is only needed at build time for the Vite client bundle.

## Scaffold

```sh
sveltego-init ./hello
cd hello
```

This writes the baseline tree:

```
hello/
  src/
    routes/
      +page.svelte
      page.server.go         # //go:build sveltego (no `+` prefix on user .go files)
      +layout.svelte
    lib/                     # $lib alias target ($lib/.gitkeep)
  hooks.server.go            # //go:build sveltego
  sveltego.config.go         # //go:build sveltego
  go.mod
  README.md
  .gitignore
```

::: warning Scaffold gap (#356)
The scaffold does not yet emit `cmd/app/main.go`, `app.html`, `package.json`, or `vite.config.js`. To run the project end-to-end today, copy those four files from [`playgrounds/basic/`](https://github.com/binsarjr/sveltego/tree/main/playgrounds/basic) into your scaffold output. Tracked in [#356](https://github.com/binsarjr/sveltego/issues/356).
:::

Add `--ai` to also write `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and `.github/copilot-instructions.md` from the bundled AI templates:

```sh
sveltego-init --ai ./hello
```

## A minimal route

`src/routes/+page.svelte`:

```svelte
<script lang="go">
  type PageData struct {
    Greeting string
  }
</script>

<h1>{Data.Greeting}</h1>
```

`src/routes/page.server.go` (no `+` prefix — user `.go` files use the `//go:build sveltego` tag instead, so the standard Go toolchain ignores them):

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (PageData, error) {
  return PageData{Greeting: "hello, sveltego"}, nil
}
```

## Develop

`sveltego dev` is a stub today (deferred to v0.3, [#42](https://github.com/binsarjr/sveltego/issues/42)). For now: re-run `sveltego compile` after editing `.svelte` or user `.go` files, then re-run the binary.

```sh
sveltego compile         # regenerate .gen/*.go
go run ./cmd/app         # boot the server
```

## Build

```sh
sveltego build           # codegen + Vite + go build, in one step
./build/app              # listens on :3000
```

`sveltego build` runs codegen, runs Vite for the client bundle, then chains directly into `go build -o build/app ./cmd/app`. You do not need a separate `go build` invocation. Deploy `./build/app` plus the Vite `dist/` output (static assets) as a single artifact.

## Next steps

- [Routing](/guide/routing) — `+page.svelte`, params, groups, optional and rest segments.
- [Load](/guide/load) — server-side data loading, parent layouts, fetch.
- [Form actions](/guide/actions) — POST handlers tied to a page.
- [Hooks](/guide/hooks) — `Handle`, `HandleError`, `HandleFetch`, `Reroute`, `Init`.
- [Migration from SvelteKit](/guide/migration) — what carries over and what does not.
