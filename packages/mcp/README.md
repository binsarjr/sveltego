# mcp

Model Context Protocol server for sveltego docs and APIs. AI clients
(Claude Desktop, Cursor, Claude Code, Continue) launch this binary and
exchange JSON-RPC messages over stdio to query sveltego documentation,
look up runtime APIs, fetch playground examples, scaffold routes, and
validate template snippets.

Status: pre-alpha. See repo root [`README.md`](../../README.md) for the
project overview and [`STABILITY.md`](./STABILITY.md) for API tiers.

## Build

```sh
cd packages/mcp
go build -o sveltego-mcp ./cmd/sveltego-mcp
```

The binary speaks line-delimited JSON-RPC 2.0 (one message per line) on
stdin/stdout. There is no HTTP transport yet.

## Tools

| Tool | Status | Notes |
|---|---|---|
| `search_docs(query, limit)` | implemented | Substring search across `documentation/docs/**/*.md`. |
| `get_doc_page(slug)` | implemented | Reads one markdown page by slug (with or without `.md`). |
| `lookup_api(symbol)` | implemented | Parses `packages/sveltego/exports/kit/*.go` via `go/parser` + `go/doc`; returns signature and godoc. |
| `get_example(name)` | implemented | Concatenates `playgrounds/<name>/**` source, capped at 100KB. |
| `scaffold_route(path, kind)` | implemented | Returns boilerplate for `page`, `layout`, `server`, or `error` files honouring ADR 0003. |
| `validate_template(source)` | stub | Parser lives behind Go's `internal/` rule; follow-up issue tracks a thin re-export. |

## Client setup

The MCP server expects to launch from the sveltego repo root so it can
locate `documentation/docs`, `packages/sveltego/exports/kit`, and
`playgrounds`. Override with flags:

```sh
sveltego-mcp --root /path/to/sveltego \
             --docs /path/to/sveltego/documentation/docs \
             --kit /path/to/sveltego/packages/sveltego/exports/kit \
             --playgrounds /path/to/sveltego/playgrounds
```

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`
(macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "sveltego": {
      "command": "/absolute/path/to/sveltego-mcp",
      "args": ["--root", "/absolute/path/to/sveltego"]
    }
  }
}
```

Restart Claude Desktop to pick up the config.

### Cursor

Add to `~/.cursor/mcp.json` (or the project-local `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "sveltego": {
      "command": "/absolute/path/to/sveltego-mcp",
      "args": ["--root", "/absolute/path/to/sveltego"]
    }
  }
}
```

### Continue (VS Code / JetBrains)

Add to your Continue config under `experimental.modelContextProtocolServers`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "/absolute/path/to/sveltego-mcp",
          "args": ["--root", "/absolute/path/to/sveltego"]
        }
      }
    ]
  }
}
```

### Manual test

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scaffold_route","arguments":{"path":"about","kind":"page"}}}' \
  | ./sveltego-mcp
```

## Notes

- This binary is currently shipped standalone. Integration as
  `sveltego mcp` subcommand is tracked as a follow-up; that work edits
  `cmd/sveltego/` which is owned by other phases.
- The server makes no network calls — every tool reads local files only.
- Resources are advertised in `initialize` capabilities but
  `resources/list` returns an empty list and `resources/read` is a
  follow-up.
