---
title: CLI
order: 230
summary: sveltego command reference — build, compile, dev, check, routes, version.
---

# CLI

`sveltego` is the project's command-line entry point. Install with:

```sh
go install github.com/binsarjr/sveltego/cmd/sveltego@latest
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

Full pipeline: codegen → Vite client bundle → `go build`.

### `sveltego compile`

Codegen only. Writes `.gen/*.go`. Use this when iterating on templates without rebuilding the binary.

### `sveltego dev`

Watch + regenerate. Restarts the server on `.gen/` change. Proxies the Vite dev server for client HMR.

### `sveltego check`

Validate without writing output. Runs the parser and Go expression validator over every `+page.svelte` / `+layout.svelte`. Use this in CI before `build`.

### `sveltego routes`

Print the route table. One line per route with: pattern, file, presence of `Load`, presence of `Actions`, resolved `PageOptions`.

### `sveltego version`

Print version, commit, build date.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | command-level error (parse, codegen, build) |
| 2 | internal panic — file an issue |

The CLI silences cobra's automatic usage-on-error since most failures are user-fixable parse errors that benefit from a clean message, not a usage dump.
