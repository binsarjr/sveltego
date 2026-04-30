# lsp

Language Server Protocol implementation for `.svelte` with Go expressions.

Status: pre-alpha scaffold. The `sveltego-lsp` binary speaks LSP over stdio
and answers the `initialize`/`shutdown` handshake plus stub `hover`,
`definition`, and `references`. Hover/definition wiring through gopls is the
follow-up to [#69](https://github.com/binsarjr/sveltego/issues/69).

## Build

```
go build -o sveltego-lsp ./cmd/sveltego-lsp
```

## Layout

- `cmd/sveltego-lsp/` — binary entry point.
- `internal/server/` — JSON-RPC framing, dispatcher, method handlers.
- `internal/sourcemap/` — `.svelte` ↔ `.gen/*.go` position translation.

## Editor integration

`editor/vscode/` hosts the VS Code language-client scaffold. See
[`STABILITY.md`](./STABILITY.md) for API tiers.
