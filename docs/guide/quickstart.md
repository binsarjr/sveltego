---
title: Quickstart
order: 10
summary: Install the sveltego CLI, scaffold a project, run dev, build, ship.
---

# Quickstart

sveltego is pre-alpha. Expect rough edges and breaking changes. The shape below tracks the v1.0 milestone; commands and flags may shift.

## Scaffold

One Go command from any terminal — no clone, no `go install` ceremony:

```sh
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./hello
cd hello
```

`go run @latest` resolves the scaffold engine through the Go module proxy and discards it after a single use. The same flag surface is available under [`packages/init/README.md`](https://github.com/binsarjr/sveltego/blob/main/packages/init/README.md). Common opts:

- `--ai` — also write `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and `.github/copilot-instructions.md` from the bundled templates.
- `--tailwind=v4|v3|none` — opt into Tailwind (`v4` is the bare-flag default).
- `--service-worker` — emit a starter `src/service-worker.ts` (#89).
- `--module example.com/x` — override the generated Go module path (defaults to the directory base name).
- `--force` — overwrite existing files.
- `--non-interactive` — never prompt; default `--ai` to `false` when unset.

## Install the framework CLI

Until [#368](https://github.com/binsarjr/sveltego/issues/368) ships release binaries, install the build CLI from source:

```sh
go install github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego@latest
sveltego version
```

`sveltego` (build / compile / routes / version) is the only binary you need on `PATH`. The scaffold engine (`sveltego-init`) only runs through `go run @latest` as shown above. No Node runtime is required to run a built app; Node is only needed at build time for the Vite client bundle.

## Scaffold layout

`go run @latest` writes the baseline tree:

```
hello/
  src/
    routes/
      _page.svelte           # pure Svelte/JS/TS
      _page.server.go        # `_` prefix; Go toolchain skips
      _layout.svelte
    lib/                     # $lib alias target ($lib/.gitkeep)
  hooks.server.go            # //go:build sveltego (no `_` prefix)
  sveltego.config.go         # //go:build sveltego
  go.mod
  README.md
  .gitignore
```

The scaffold writes `cmd/app/main.go`, `app.html`, `package.json`, and `vite.config.js` alongside the tree above, so `sveltego build && ./build/app` runs end-to-end.

## A minimal route

`src/routes/_page.svelte` (pure Svelte — copy across from any SvelteKit project unchanged):

```svelte
<script lang="ts">
  let { data } = $props();
</script>

<h1>{data.greeting}</h1>
```

`src/routes/_page.server.go` (the `_` prefix hides the file from Go's default toolchain — no `//go:build sveltego` tag needed):

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
  Greeting string `json:"greeting"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
  return PageData{Greeting: "hello, sveltego"}, nil
}
```

Codegen reads the `Load` return type and emits a sibling `_page.svelte.d.ts` so `data.greeting` autocompletes in your editor with no manual type duplication.

## Develop

`sveltego dev` is a stub today (deferred to v0.3, [#42](https://github.com/binsarjr/sveltego/issues/42)). For now: re-run `sveltego compile` after editing `.svelte` or user `.go` files, then re-run the binary.

```sh
sveltego compile         # regenerate manifest + .svelte.d.ts
go run ./cmd/app         # boot the server
```

## Build

```sh
sveltego build           # codegen + Vite + go build, in one step
./build/app              # listens on :3000
```

`sveltego build` runs codegen, runs Vite for the client bundle, then chains directly into `go build -o build/app ./cmd/app`. You do not need a separate `go build` invocation. Deploy `./build/app` plus the Vite `dist/` output (static assets) as a single artifact.

## Next steps

- [Routing](/guide/routing) — `_page.svelte`, params, groups, optional and rest segments.
- [Load](/guide/load) — server-side data loading, parent layouts, fetch.
- [Form actions](/guide/actions) — POST handlers tied to a page.
- [Hooks](/guide/hooks) — `Handle`, `HandleError`, `HandleFetch`, `Reroute`, `Init`.
- [Migration from SvelteKit](/guide/migration) — what carries over and what does not.
