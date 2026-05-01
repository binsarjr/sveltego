---
title: CLI
order: 230
summary: sveltego command reference — build, compile, dev, check, routes, version.
---

# CLI

`sveltego` is the project's primary command-line entry point. Project scaffolding lives in a separate binary, `sveltego-init`, that is normally invoked through `go run @latest` rather than installed globally. Install the framework CLI with:

```sh
go install github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego@latest
```

## Global flags

| Flag | Description |
|---|---|
| `-c, --config <path>` | Path to sveltego config (no-op until #21). |
| `--cwd <path>` | Working directory override (no-op until #21). |
| `-v, -vv, -vvv` | Increase log verbosity (info, debug, debug+source). |

Verbosity defaults to warn-level. Logs use `slog` with a text handler on stderr.

## Subcommands

### `sveltego build`

Full pipeline: codegen → Vite client bundle → `go build -o build/app ./cmd/app`. Single command; no separate `go build` step needed.

### `sveltego compile`

Codegen only. Writes `.gen/*.go`. Use this when iterating on templates without rebuilding the binary.

### `sveltego dev`

**Stub today** — deferred to v0.3 ([#42](https://github.com/binsarjr/sveltego/issues/42)). Will watch `src/routes/**`, regenerate `.gen/*.go` on change, restart the server, and proxy the Vite dev server for client HMR.

### `sveltego check`

**Stub today.** Will validate without writing output: parser pass + Go expression validator over every `+page.svelte` / `+layout.svelte`, intended for CI before `build`. Milestone TBD.

### `sveltego routes`

Print the route table. One line per route with: pattern, file, presence of `Load`, presence of `Actions`, resolved `PageOptions`.

### `sveltego version`

Print version, Go runtime version, OS, and architecture.

## `sveltego-init` (zero-clone scaffolder)

Project scaffolder. The canonical entry point is `go run @latest` — no clone, no global install. See [`packages/init/README.md`](https://github.com/binsarjr/sveltego/blob/main/packages/init/README.md) for the full flag list.

```sh
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./my-app
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest --ai ./my-app
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest --tailwind=v4 --module example.com/x ./my-app
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest --service-worker ./my-app
```

Want a local install instead? `go install github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest` puts a `sveltego-init` binary on `$GOPATH/bin`. Use `go run` for one-off scaffolds; install only if you scaffold often enough that fetch latency matters.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | command-level error (parse, codegen, build) |
| 2 | internal panic — file an issue |

The CLI silences cobra's automatic usage-on-error since most failures are user-fixable parse errors that benefit from a clean message, not a usage dump.
