# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository state

Pre-alpha. **No Go source yet** — repo holds only specs, RFCs, and a 94-issue roadmap on GitHub at `binsarjr/sveltego`. The first code lands when MVP RFCs (#1–4) are accepted. Until then, work happens in three places:

- `tasks/todo.md` — current execution plan, milestone scope, phase tracking
- `tasks/lessons.md` — design decisions and self-rules captured per session (append-only journal)
- GitHub issues on `binsarjr/sveltego` — every concrete unit of work

Always read both `tasks/` files at session start before proposing changes.

## Project direction (read before planning anything)

This is a **rewrite of SvelteKit's shape in pure Go**, not an embed of SvelteKit-the-JS-server. The earlier "embed JS runtime via goja/v8go/Bun" direction was rejected — see `tasks/lessons.md` "Pivot to Go-native rewrite" for the chain of reasoning. Do not reopen that decision without new evidence.

Key invariants:

- **No JS runtime on the server.** `.svelte` compiles to Go source via codegen (`.gen/*.go`) for SSR. Vite produces the client bundle for hydration only.
- **Mustache expressions are Go, not JS.** `{Data.User.Name}`, `{len(Data.Posts)}`, `nil` not `null`. PascalCase fields. Validated at codegen via `go/parser.ParseExpr`.
- **Svelte 5 only.** Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`). Skip Svelte 4 legacy reactivity.
- **Codegen, not interpretation.** Static decisions at build time, no per-request template walking.
- **Performance target:** 20–40k rps mid-complexity SSR. If a proposal can't reach that, surface it before writing code.

## Roadmap structure

6 milestones, 94 issues. Issue numbering follows execution order:

| Milestone | Issues | Focus |
|---|---|---|
| MVP | #1–23, #76, #77, #83 | RFCs, parser, codegen, runtime, router, CLI to render a page |
| v0.2 | #24–33, #78–82 | Layouts, hooks, error boundaries, form actions, route groups, page options, env |
| v0.3 | #34–42, #84, #85, #87, #88 | Vite client, hydration, SPA router, `$app/navigation`, Snapshot, kit.Link, kit.Asset |
| v0.4 | #43–59, #86, #90 | Svelte 5 full coverage, a11y, `<svelte:options>` |
| v1.0 | #60–69, #89, #91–93 | Bench, docs, streaming, SSG, CSP, sitemap, image opt, deploy adapters |
| v1.1 | #70–75 | LLM tooling: `llms.txt`, MCP server, AI templates, provenance |
| Standalone | #94 | RFC: explicit non-goals |

## Issue conventions

Every issue body follows the same contract — keep it when authoring or editing:

`Summary · Background · Goals · Non-Goals · Detailed Design (with code) · Acceptance Criteria · Testing Strategy · Out of Scope · Risks & Open Questions · Dependencies (Blocks / Blocked by) · References`

Required labels per issue: one `area:*`, one `type:*`, one `priority:*` (`p0` blocker / `p1` important / `p2` nice-to-have). Areas in use: `codegen`, `router`, `runtime`, `cli`, `client`, `forms`, `hooks`, `perf`, `docs`, `infra`, `design`, `llm`. The `blocked` label flags cross-issue waits.

Author/edit issues with `gh issue create --body-file` or `gh issue edit --body-file`, never inline `--body` (heredoc avoids quoting traps). Recent batch scripts (`/tmp/sveltego-issues-*.sh`) show the helper pattern.

## File conventions the framework will implement

When designing, codegen, or runtime work touches these names, treat them as **load-bearing**:

```
src/routes/
  +page.svelte           // SSR template, Go expressions inside {...}
  +page.server.go        // Load(), Actions()
  +layout.svelte         // layout chain
  +layout.server.go      // parent data flow
  +server.go             // REST endpoints (GET, POST, etc.)
  +error.svelte          // error boundary
  (group)/               // route group, no URL segment
  +page@.svelte          // layout reset
  [param]/               // route param
  [[optional]]/          // optional segment
  [...rest]/             // catch-all
src/params/<name>.go     // param matchers
src/lib/                 // shared modules, $lib alias target
src/service-worker.ts    // service worker convention
hooks.server.go          // Handle, HandleError, HandleFetch, Reroute, Init
```

Generated output lives under `.gen/` (gitignored).

## Workflow notes

- The repo is on `main` only. Push directly after a clean commit; no PR flow yet.
- Commits use a short imperative subject. Recent style: `docs: track 19 SvelteKit-parity gap issues across milestones`.
- When adding a new lesson to `tasks/lessons.md`, append a dated section — never rewrite older entries.
- When the issue list expands, also update `README.md` milestones table and `tasks/todo.md` milestone counts in the same commit.

## Out of scope (do not propose)

- Svelte 4 legacy reactivity (`$:`, store autoload).
- Server-side dynamic JS execution.
- A native Go bundler replacing Vite for the client.
- Multi-tenant / RBAC primitives in `kit`.
- Universal (shared client+server) `Load`. Server-only by design.

See issue #94 for the full non-goals RFC once it lands.
