# sveltego — VS Code

VS Code language client scaffold for the sveltego LSP. Launches `sveltego-lsp`
over stdio against any open `.svelte` file.

Status: scaffold. The server side ships `initialize`/`shutdown` plus stub
`hover`, `definition`, `references`. Full hover / go-to-definition / diagnostics
land when the gopls proxy follow-up to issue [#69] merges.

## Build

```
cd editor/vscode
npm install
npm run compile
```

## Run

The extension expects `sveltego-lsp` on `PATH`, or override via the
`sveltego.lsp.path` setting. Build the binary from the repo root:

```
go build -o sveltego-lsp ./packages/lsp/cmd/sveltego-lsp
```

## Configuration

| Setting | Default | Purpose |
|---|---|---|
| `sveltego.lsp.path` | `sveltego-lsp` | Path to the language server binary. |
| `sveltego.trace.server` | `off` | LSP message tracing in the Output panel. |

[#69]: https://github.com/binsarjr/sveltego/issues/69
