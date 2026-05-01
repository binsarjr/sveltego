---
title: Working with AI assistants
order: 100
summary: Configure Claude Code, Cursor, Copilot, and Continue for sveltego projects.
---

# Working with AI assistants

sveltego is built with AI-assisted development in mind. Project templates ship `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and `.github/copilot-instructions.md`; the docs site exposes `llms.txt` and a "Copy for LLM" button on every page; an MCP server is on the v1.1 roadmap (#71).

This page collects the setup snippets and the gotchas worth knowing.

## Quick setup

### Claude Code / Claude Desktop

Project-level instructions live at the repo root in `CLAUDE.md`. Claude Code reads it automatically. The shipped `CLAUDE.md` includes the read-this-first list, the pure-Svelte template invariants (per [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md)), and the verification gates.

For Claude Desktop's MCP integration:

```jsonc
// ~/.config/claude/mcp.json
{
  "mcpServers": {
    "sveltego": { "command": "sveltego", "args": ["mcp"] }
  }
}
```

The `sveltego mcp` command lands with #71. Until then, the Copy-for-LLM buttons on this site cover the same ingestion path.

### Cursor

Drop `.cursorrules` from `sveltego-init --ai` into your project root. Cursor reads it automatically and applies it to every chat.

For MCP: Cursor Settings → MCP → add `sveltego mcp`.

### GitHub Copilot

`sveltego-init --ai` writes `.github/copilot-instructions.md`. Copilot Chat picks it up automatically; Copilot inline completion uses it as additional context where supported.

### Continue

```jsonc
// .continue/config.json
{
  "rules": ["./AGENTS.md"]
}
```

Continue resolves `AGENTS.md` as the master rules file; the project keeps `.cursorrules` and `copilot-instructions.md` in sync via a simple sync step (RFC #103).

## Project templates

```sh
sveltego-init --ai ./my-app
```

Adds `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.github/copilot-instructions.md` to the scaffold. `AGENTS.md` is the master; the other three are generated from it via `scripts/sync-ai-docs.sh`. Edit `AGENTS.md` and re-run the sync to update the rest.

To copy AI templates into an existing project (without scaffolding a new tree), use the same binary against the existing dir — non-template files are not touched, conflicts skip-by-default.

## Prompting tips

### sveltego-specific gotchas

LLMs default to SvelteKit-with-Node-server. Remind them:

> Templates are pure Svelte/JS/TS — `let { data } = $props()`, `{data.user.name}`, `null` not `nil`. Server-side data lives in `_page.server.go` (Go), not `+page.server.ts`. Files under `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`) so Go's default toolchain skips them. `src/params/**` and `hooks.server.go` still need `//go:build sveltego`.

The shipped templates do this for you; the prompts below assume the rules are loaded.

### Three example prompts that work

**Scaffold a route**

> Create a `src/routes/blog/[slug]` route with `_page.svelte` (pure Svelte) and `_page.server.go` (Go). The page shows a `Post` fetched by slug from `db.PostBySlug(ctx, slug)`. Fields: `title`, `body` (HTML), `publishedAt` (ISO string). On the Go side: `Title string \`json:"title"\``, `Body string \`json:"body"\``, `PublishedAt time.Time \`json:"publishedAt"\``.

**Write a form action**

> Add a comment form to `src/routes/blog/[slug]/_page.svelte` (use `<form method="POST" use:enhance>`) and an action in `_page.server.go` that validates body is non-empty, appends to `db.Comments`, and redirects back to the page on success or returns `kit.ActionFail(422, ...)` on validation failure.

**Add a hook**

> Add `hooks.server.go` (with `//go:build sveltego`) with a `Handle` that reads cookie `session`, calls `auth.LookupUser(token)`, and attaches `*User` to `ev.Locals["user"]`. Use `kit.Sequence` so we can chain another handler later.

## Watch out for

LLMs reach for stale or wrong patterns. Reject these:

- `export let prop` → use `let { prop } = $props()`.
- `<script lang="go">` inside `.svelte` → templates are 100% pure Svelte/JS/TS now (per [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md)). Server-side Go lives in `_page.server.go`.
- `{Data.User.Name}` (PascalCase, Go-style) → `{data.user.name}` (camelCase via JSON tags). The legacy Mustache-Go dialect is removed.
- `kit.json(...)` (lowercase) → actual API is `kit.JSON(status, body)`.
- `throw redirect(303, ...)` (in Go) → `return data, kit.Redirect(303, ...)`.
- `data.user?.name` in `.svelte` → fine on the client (standard JS optional chaining); plan for it on the Go side by leaving `User` as a pointer or omitempty field.
- `useState`, `writable`, `derived` from Svelte stores → use runes (`$state`, `$derived`).
- Universal Load (`+page.ts`) → server-only by design; not supported.
- `+page.server.ts` filenames → use `_page.server.go`. The `_` prefix hides the file from Go's default toolchain (RFC #379 phase 1b).

## Markdown access for assistants

Every page on this site is reachable as raw markdown:

- `https://sveltego.dev/guide/routing` → HTML
- `https://sveltego.dev/guide/routing.md` → markdown

Two buttons sit at the top of every page:

- **Copy as Markdown** — copies the page body without frontmatter.
- **Copy for LLM** — prepends a small context header (page title and source URL) so the assistant has provenance.

Two single-fetch endpoints for whole-site ingestion:

- `https://sveltego.dev/llms.txt` — curated index with one-line summaries.
- `https://sveltego.dev/llms-full.txt` — every page concatenated as markdown.

Both are generated at docs build, ordered by frontmatter `order`.

## More

- AGENTS.md spec: RFC #103.
- llms.txt convention: <https://llmstxt.org/>.
- Project conventions live in `CLAUDE.md` at the repo root and in per-package `CLAUDE.md` files where helpful.
