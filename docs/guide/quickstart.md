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
sveltego version
```

The CLI is a single binary. No Node runtime is required to run a built app; Node is only needed at build time for the Vite client bundle.

## Scaffold

```sh
mkdir hello && cd hello
sveltego init
```

This produces:

```
hello/
  src/
    routes/
      +page.svelte
      page.server.go       # //go:build sveltego (no `+` prefix on user .go files)
    hooks.server.go        # //go:build sveltego (optional)
    lib/                   # $lib alias target
  go.mod
  package.json             # Vite client bundle
  sveltego.config.go
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

```sh
sveltego dev
```

This starts the dev server, watches `src/routes/**`, regenerates `.gen/*.go`, and proxies the Vite dev server for the client bundle.

## Build

```sh
sveltego build
go build -o app ./cmd/app
./app
```

The `build` step writes `.gen/*.go`; the standard `go build` produces a single binary. Deploy that binary plus the `dist/` Vite output as static assets.

## Next steps

- [Routing](/guide/routing) — `+page.svelte`, params, groups, optional and rest segments.
- [Load](/guide/load) — server-side data loading, parent layouts, fetch.
- [Form actions](/guide/actions) — POST handlers tied to a page.
- [Hooks](/guide/hooks) — `Handle`, `HandleError`, `HandleFetch`, `Reroute`, `Init`.
- [Migration from SvelteKit](/guide/migration) — what carries over and what does not.
