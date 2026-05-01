---
title: Quickstart
order: 10
summary: Install the sveltego CLI, scaffold a project, run dev, build, ship.
---

# Quickstart

sveltego is pre-alpha. Expect rough edges and breaking changes. The shape below tracks the v1.0 milestone; commands and flags may shift.

## Scaffold

One command from any terminal. Pick whichever you have:

```sh
# Node >= 20 — downloads the scaffold engine on demand
npm create sveltego@latest ./hello

# Go >= 1.23 — resolves the binary via the Go module proxy
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./hello
```

Both paths produce the same project tree. `npm create sveltego@latest` falls back to `go run @latest` automatically when Go is on `PATH` and a release binary is not yet available (release binaries are pending [#368](https://github.com/binsarjr/sveltego/issues/368)).

## Install the framework CLI

```sh
go install github.com/binsarjr/sveltego/cmd/sveltego@latest
sveltego version
```

`sveltego` (build / compile / routes / version) is the only binary you need installed locally; the scaffold engine (`sveltego-init`) runs through `npm create` or `go run @latest` as shown above. No Node runtime is required at run time; Node is only needed at build time for the Vite client bundle.

## Scaffold flags

```sh
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

Add `--ai` to also write `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and `.github/copilot-instructions.md` from the bundled AI templates:

```sh
npm create sveltego@latest --ai ./hello
# or:
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest --ai ./hello
```

Other flags supported by both paths: `--tailwind=v4|v3|none` (Tailwind opt-in), `--service-worker` (starter `src/service-worker.ts`), `--module <path>` (override the generated Go module path), `--force` (overwrite existing files), `--non-interactive` (never prompt).

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
