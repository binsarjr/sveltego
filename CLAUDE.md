# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Read this first (mandatory at session start)

To avoid hallucinating conventions, layout, or APIs, read **in this order** before any non-trivial action:

1. `README.md` — what the project is.
2. **This file** (`CLAUDE.md`) — Claude Code-specific entry point.
3. `tasks/todo.md` — current execution plan, milestone scope, phase tracking.
4. `tasks/lessons.md` — design decisions, self-rules, append-only journal of why things are the way they are.
5. **Foundation issues #95–105** on GitHub `binsarjr/sveltego` — they define the entire project's conventions. Do not invent rules; consult these.
6. `AGENTS.md` — master AI rules synced to `.cursorrules` and copilot instructions per RFC #103.
7. `CONTRIBUTING.md` — code style, error handling, logging, ctx propagation.
8. `STABILITY.md` per package — what's safe to change (RFC #97).
9. Any package-local `CLAUDE.md` for scope-specific patterns (e.g. `packages/sveltego/internal/codegen/CLAUDE.md` once it lands).

If a doc is missing, check the corresponding issue body — body-files in `/tmp/setup-bodies/*.md` exist locally during the bootstrap phase.

### Foundation issue index

| # | Topic | Why you'd read it |
|---|---|---|
| #95 | Monorepo workspace layout | Where files go, module path naming, `go.work` setup |
| #96 | Code style conventions | gofumpt, goimports, error wrapping, slog, ctx, naming, forbidden patterns |
| #97 | API stability and versioning | Tier rules (stable/experimental/deprecated), breaking change procedure |
| #98 | golangci-lint config | What lints run, how to fix violations, lint-fix workflow |
| #99 | Pre-commit hooks + commit-msg | Conventional Commits format, hook install, scope = package name |
| #100 | release-please multi-package | Tag format, CHANGELOG generation, per-package versioning |
| #101 | CI matrix | Required checks before merge, race detector, workspace vs isolated mode |
| #102 | PR template + Definition of Done | Checklist every PR must satisfy |
| #103 | AGENTS.md + AI doc sync | Single source of truth for AI agent rules |
| #104 | Codegen golden testing | `-update` flag flow, determinism rules, review discipline |
| #105 | Bench regression gate | benchstat thresholds, CI integration, override mechanism |

When in doubt about a convention: open the relevant issue with `gh issue view <N> --repo binsarjr/sveltego`. Do not guess.

## Repository state

Pre-alpha. MVP closed; v0.2, v0.4, and v1.1 shipped; v0.3, v0.5, v0.6, and v1.0 in flight. The Go workspace already hosts the core (`packages/sveltego`), auth (`packages/auth`), tooling (`packages/lsp`, `packages/mcp`, `packages/init`, `packages/enhanced-img`), five deploy adapters plus `adapter-auto`, the bench harness (`bench/`, `benchmarks/`), AI templates (`templates/ai/`), and end-to-end playgrounds. New work continues to flow through:

- `tasks/todo.md` — current execution plan, milestone scope, phase tracking
- `tasks/lessons.md` — design decisions and self-rules captured per session (append-only journal)
- GitHub issues on `binsarjr/sveltego` — every concrete unit of work (see milestone counts below for live totals)

Always read both `tasks/` files at session start before proposing changes.

## Project direction (read before planning anything)

This is a **rewrite of SvelteKit's shape in pure Go**, not an embed of SvelteKit-the-JS-server. The earlier "embed JS runtime via goja/v8go/Bun" direction was rejected — see [`tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md`](tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md) for the chain of reasoning. Do not reopen that decision without new evidence.

As of 2026-05-01, [ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md) supersedes [ADR 0007](tasks/decisions/0007-svelte-semantics-revisit.md): templates pivot to **100% pure Svelte/JS/TS**. Go expressions in mustaches are out. Server-side Go files own data; codegen reads their Go AST and emits TypeScript declaration files for IDE autocompletion. Runtime is hybrid: build-time SSG (Node only at build time) for prerendered routes, runtime SPA (Go-only) for everything else. Phases 2–6 land via [#381](https://github.com/binsarjr/sveltego/issues/381)–[#385](https://github.com/binsarjr/sveltego/issues/385); see [RFC #379](https://github.com/binsarjr/sveltego/issues/379) for the full plan.

Key invariants (post-ADR 0008):

- **No JS runtime on the server at runtime.** Node may run during `sveltego build` to prerender SSG routes via `svelte/server`. The deployed Go binary plus `static/` is the entire deployable; no JS engine on the request path.
- **Templates are pure Svelte.** `.svelte` files contain only Svelte/JS/TS — runes, JS expressions, lowercase props (`{data.user.name}`). Zero Go syntax in mustaches, blocks, or `<script>`. Svelte LSP and the npm Svelte ecosystem work without a fork.
- **Go owns the server.** Server-side Go files return a typed data shape from `Load(ctx kit.LoadCtx)`; that shape becomes `data` in client `$props()`. JSON tags drive the Go ↔ TypeScript boundary.
- **Codegen, not interpretation.** Static decisions at build time. Codegen now emits `.svelte.d.ts` declarations (Go AST → TypeScript) plus prerendered HTML for SSG routes, instead of the old `.gen/*.go` template artifacts. No per-request template walking.
- **Svelte 5 only.** Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`). Skip Svelte 4 legacy reactivity.
- **Performance target:** 20–40k rps for SSG (zero per-request work) and JSON-payload responses for SPA-mode dynamic routes. If a proposal can't reach that, surface it before writing code.

## Working rules (do not skip)

These are the project-explicit form of the global agent directives. They override default biases toward "minimal, fast, often broken" output.

### 1. Plan before build

- For any task with 3+ steps or architectural impact: **plan first**, get approval, then execute. Do not improvise mid-task.
- Plans live in `tasks/todo.md` with checkable items. Update progress as you go; mark complete only after verification.
- "Plan" and "build" are separate steps. When asked to plan, **output only the plan**. No code until "yes / do it / push".
- Vague instructions like "add a settings page" warrant a written plan, not immediate code.
- **Spec-Based Development:** for non-trivial features, use `AskUserQuestion` to interview the user about technical approach, UX, concerns, and tradeoffs before writing code. Strip every assumption before touching code. The spec is the contract — execute against it, not against guesses.
- If something goes wrong mid-execution: **stop and re-plan**. Do not continue blindly.
- If the user provides a written plan: follow it exactly. If you spot a real problem, flag it and wait — do not improvise around it.
- **Follow References, Not Descriptions.** When the user points to existing code as a reference, study it thoroughly before building. Match its patterns exactly. Working code is a better spec than English description.

### 2. Verification before done (mandatory cross-check)

A task is **not done** when bytes hit disk. It is done when proven correct. Before declaring "selesai" / "done":

- **Re-read every file you edited.** Edit tool reports success on byte-write, not on correctness. Stale context produces silent corruption.
- Run the full local gate:
  - `gofumpt -l .` (no output = clean)
  - `goimports -l -local github.com/binsarjr/sveltego .`
  - `golangci-lint run` (when `.golangci.yml` lands)
  - `go vet ./...`
  - `go test -race ./...`
  - `go build ./...` across all `go.work` packages
- If any check is missing tooling (e.g. lint config not yet in repo), **say so explicitly**. Never claim success when a gate is unverified.
- For codegen work: run golden tests and review the diff line-by-line before approving an `-update`.
- For PR-flow work (when it starts): wait for CI green before declaring done. Do not push and walk away.
- Ask yourself: **"Would a staff engineer approve this in code review?"** If no, fix before declaring done.
- After fixing a bug: **diff behavior between `main` and your change** to verify the fix actually fires on the failing input.
- Cross-check artifacts: when issue counts change, README + `tasks/todo.md` + CLAUDE.md milestone tables must all match. Run `gh issue list --milestone X --state all --json number | jq length` to verify.

### 3. Phased execution (multi-file work)

- Never refactor across >5 files in one response. Break into phases.
- Each phase: complete → verify → wait for "ok lanjut" → next phase.
- For >5 independent files, **launch parallel subagents** (5–8 files per agent). One agent per task. Use `Agent` with `subagent_type=Explore` for research, `general-purpose` for execution.
- **Step 0 before any structural refactor:** delete dead code first (unused exports, unused imports, debug logs). Commit cleanup separately.

### 4. Edit safety

- **Re-read files after 10+ messages** before editing — context decay corrupts memory of file contents.
- Re-read **before every edit**, re-read **after every edit** to confirm the change applied.
- Never batch >3 edits to the same file without an intervening read.
- When renaming a symbol, grep for: direct calls, type references, string literals, dynamic imports, re-exports, test files, mocks. **Assume the first grep missed something.**
- One source of truth. If tempted to copy state to fix a display bug, the fix is in the wrong place — find the real source.

### 5. Senior dev override

- Ignore default "minimal change, simplest approach" bias when it produces band-aids.
- If architecture is flawed, state is duplicated, or patterns are inconsistent: **propose a structural fix**, do not patch around it.
- Ask: **"What would a senior, experienced, perfectionist dev reject in code review?"** Fix all of it.
- For non-trivial changes, pause and ask: **"Is there a more elegant way?"** If a fix feels hacky, implement the clean solution you would choose knowing everything you know now.
- **Two-Perspective Review:** when evaluating your own work, present both views. What a perfectionist would criticize, what a pragmatist would accept. Let the user pick the tradeoff.
- **Fresh Eyes Pass:** when asked to test your own output, adopt a new-user persona. Walk through as if you've never seen the project. Flag anything confusing, friction-heavy, or unclear.
- After fixing a bug: **autopsy.** Why did it happen? What category of bug is this? Add a self-rule to `tasks/lessons.md` so the same category cannot return.
- After 2 failed attempts at the same problem: **stop**. Re-read the relevant section top-down. Say where your mental model was wrong. Propose something fundamentally different.
- If the user says "step back" or "we're going in circles": drop everything, rethink from scratch, propose something fundamentally different.

### 6. Mistake logging (lessons.md)

- After **any user correction**, create a new file `tasks/lessons/YYYY-MM-DD-<topic>.md` with:
  - `## YYYY-MM-DD — <topic>` heading.
  - "Insight" — what was wrong, with the underlying pattern named.
  - "Self-rules" — numbered, future-tense, prevent the category.
- Add a bullet to the top of `tasks/lessons.md` index pointing at the new file.
- Never rewrite or delete existing lesson files. Append-only journal.
- Read recent lessons at session start when relevant.

### 7. Don't over-engineer

- No imaginary scenarios. If nobody asked for the scenario, do not handle it.
- No fallbacks for cases that can't happen. Trust framework guarantees and internal invariants. Validate at boundaries only (HTTP input, external APIs, file I/O).
- Three similar lines is better than a premature abstraction.
- No half-finished implementations. If you cannot complete a feature in this phase, do not stub it; file an issue and stop.

### 8. Comments and code style

- **Default: write no comment.** Code with named identifiers explains itself.
- Only add a comment when the WHY is non-obvious: a hidden constraint, a workaround for a specific bug, surprising behavior.
- Never reference the current task or PR (`// added for issue #42`) — those rot.
- Never write multi-paragraph docstrings or ASCII-art section headers in code.
- Godoc comments on exported symbols: one sentence starting with the symbol name.

### 9. Destructive action safety

- Never `git reset --hard`, `git push --force`, `git branch -D`, `git clean -f`, `rm -rf` without explicit user authorization in the same conversation. Authorization once does not stand for next time.
- Never bypass hooks (`--no-verify`, `--no-gpg-sign`) unless the user explicitly asks. If a hook fails, fix the cause.
- Never amend a published commit. New commit, never `--amend` after push.
- Investigate before deleting unfamiliar files or branches — they may be in-progress work.
- Resolve merge conflicts; do not discard.

### 10. File system as state

- Do not hold large data in context. Save to disk, grep/jq/awk it.
- Write intermediate results to files for multi-pass work.
- Use `tasks/lessons.md` (index) + `tasks/lessons/` (per-entry files), `tasks/todo.md`, and per-package `CLAUDE.md` as durable memory. Not chat history.
- `/tmp/` is fine for batch scripts and bodies; just remember they are ephemeral across sessions.

### 11. Tone and reporting

- Match response size to task. A simple question gets a direct answer, not a section-headed essay.
- End-of-turn summary: 1–2 sentences. What changed, what's next.
- When uncertain, say so. Do not invent file paths, function names, or library APIs.
- Trust raw data (logs, error output, file contents) over your memory or theories. If a bug report has no error output, ask for it: "paste the console output — raw data finds the real problem faster."
- **One-Word Mode:** when the user says "yes", "do it", "push", "lanjut", "ok": **execute**. Do not repeat the plan. Do not add commentary. Context is loaded; the message is just the trigger.
- **Caveman mode** is active project-wide via session hook (~/.claude). Drop articles, fillers, pleasantries. Fragments OK. Code blocks and commit messages stay normal English.

### 12. Cross-doc consistency on every doc PR

When you edit ANY of these, check the others against the change:

- `README.md`
- `CLAUDE.md`
- `AGENTS.md` (when it exists)
- `tasks/todo.md`
- `tasks/lessons.md`
- Issue counts in milestone tables
- `.cursorrules`, `.github/copilot-instructions.md` (auto-synced from AGENTS.md, but verify)

If counts, file lists, or roadmap stages disagree across these, the doc set is broken. Fix all in the same commit.

### 13. Tool mechanics (avoid silent corruption)

- **File Read Budget.** Read tool capped at 2,000 lines. For files >500 LOC, use `offset` + `limit` to read in chunks. Never assume one read showed the whole file.
- **Tool Result Blindness.** Bash output >50,000 chars is silently truncated to a 2,000-byte preview. If a search returns suspiciously few hits, re-run with narrower scope (single dir, stricter glob). State explicitly when you suspect truncation.
- **Edit tool fails silently** when `old_string` doesn't match exactly (whitespace, line numbers, stale context). Re-read after every edit to confirm.
- **Background subagents:** do not poll their output mid-run — pulls noise into your context. Wait for completion notification.
- **Proactive compaction.** If you notice context degradation (forgetting file structure, referencing nonexistent vars), run `/compact` and write summary to `context-log.md`. Do not wait for auto-compact at ~167K.
- **Prompt cache awareness.** Do not switch models mid-session — invalidates cache prefix. Delegate to subagent if a different model needed.

### 14. Autonomous bug fixing

- Given a bug report: **just fix it**. Trace logs, errors, failing tests, resolve.
- Do not require context switching from the user. No "can you check X for me?" when you can check it yourself.
- Fix failing CI proactively when it shows up.
- Offer a checkpoint commit before risky changes.
- Flag files that grow unwieldy (>500 LOC for non-generated code) — suggest splitting.

## Roadmap structure

8 milestones. Counts below match the live `gh api repos/binsarjr/sveltego/milestones` output — re-verify with `gh issue list --milestone <N> --state all --json number | jq length` whenever you edit this table.

| Milestone | Issues | Focus |
|---|---|---|
| MVP | 42 (#1–23, #76, #77, #83, #95–110) | Foundation RFCs + setup, parser, codegen, runtime, router, CLI to render a page |
| v0.2 | 15 (#24–33, #78–82) | Layouts, hooks, error boundaries, form actions, route groups, page options, env |
| v0.3 | 21 (#34–42, #84, #85, #87, #88, plus #123, #172, #181, #183, #204, #206–208) | Vite client, hydration, SPA router, `$app/navigation`, Snapshot, kit.Link, kit.Asset |
| v0.4 | 19 (#43–59, #86, #90) | Svelte 5 full coverage, a11y, `<svelte:options>` |
| v0.5 | 23 (SvelteKit-parity catch-up + cookie-session core) | Upstream-tracked enhancements (`kit.After`, `HandleAction`, `RawParam`, `RouteID`, …) and the cookie-session auth core |
| v0.6 | 40 (auth track) | `sveltego-auth` master plan (#155), storage adapters, sessions, password / magic-link / OTP / OAuth |
| v1.0 | 25 (#60–69, #89, #91–93, plus post-merge code-quality follow-ups) | Bench, docs, streaming, SSG, CSP, sitemap, image opt, deploy adapters, post-Wave-1 hardening |
| v1.1 | 6 (#70–75) | LLM tooling: `llms.txt`, MCP server, AI templates, provenance |

Closed standalone issues (e.g. #94 non-goals RFC) live unmilestoned — search `gh issue list --search "no:milestone state:closed"` if needed.

## Issue conventions

Every issue body follows the same contract — keep it when authoring or editing:

`Summary · Background · Goals · Non-Goals · Detailed Design (with code) · Acceptance Criteria · Testing Strategy · Out of Scope · Risks & Open Questions · Dependencies (Blocks / Blocked by) · References`

Required labels per issue: one `area:*`, one `type:*`, one `priority:*` (`priority:p0` blocker / `priority:p1` important / `priority:p2` nice-to-have). Areas in use: `auth`, `cli`, `client`, `codegen`, `design`, `docs`, `forms`, `hooks`, `infra`, `llm`, `perf`, `router`, `runtime`, `tooling`. The `blocked` label flags cross-issue waits.

Author/edit issues with `gh issue create --body-file` or `gh issue edit --body-file`, never inline `--body` (heredoc avoids quoting traps). Recent batch scripts (`/tmp/sveltego-issues-*.sh`) show the helper pattern.

## File conventions the framework will implement

When designing, codegen, or runtime work touches these names, treat them as **load-bearing**:

```
src/routes/
  +page.svelte           // SSR template, Go expressions inside {...}
  page.server.go         // Load(), Actions()           — needs //go:build sveltego
  +layout.svelte         // layout chain
  layout.server.go       // parent data flow            — needs //go:build sveltego
  server.go              // REST endpoints (GET, POST)  — needs //go:build sveltego
  +error.svelte          // error boundary
  (group)/               // route group, no URL segment
  +page@.svelte          // layout reset
  [param]/               // route param
  [[optional]]/          // optional segment
  [...rest]/             // catch-all
src/params/<name>.go     // param matchers              — needs //go:build sveltego
src/lib/                 // shared modules, $lib alias target
src/service-worker.ts    // service worker convention
hooks.server.go          // Handle, HandleError, HandleFetch, Reroute, Init
```

Generated output lives under `.gen/` (gitignored). User `.go` files under
`src/routes/**` and `src/params/**` MUST start with `//go:build sveltego`
so Go's default toolchain (build/vet/lint) skips them; codegen reads them
through `go/parser` directly. See ADR 0003 amendment (Phase 0i-fix).

## Workflow notes

- Feature work lands via short-lived branches + PRs merged to `main`. Branch naming: `phase/<slug>` for feature phases, `fix/<slug>` for bug fixes, `docs/<slug>` for doc-only changes.
- Commits use a short imperative subject. Recent style: `docs: track 19 SvelteKit-parity gap issues across milestones`.
- When adding a new lesson, create `tasks/lessons/YYYY-MM-DD-<topic>.md` and prepend a bullet to the `tasks/lessons.md` index — never edit existing lesson files.
- When the issue list expands, also update `README.md` milestones table and `tasks/todo.md` milestone counts in the same commit.

## Out of scope (do not propose)

See [ADR 0005](tasks/decisions/0005-non-goals.md) for the canonical list and reasoning. [ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md) is the live decision on template semantics (pure Svelte/JS/TS, no Go in mustaches); [ADR 0007](tasks/decisions/0007-svelte-semantics-revisit.md) is superseded — consult ADR 0008 before proposing a JS-runtime-on-server, Go-mustache, or Go-VDOM alternative.

- Universal (shared client+server) `Load` (`+page.ts` / `+layout.ts`). Server-only by design.
- `<script context="module">` (deprecated upstream).
- WebSocket / SSE primitives in core (BYO `gorilla/websocket`).
- Vercel / Netlify Functions adapters (Cloudflare Workers adapter **is** in scope).
- vitePreprocess / arbitrary preprocessor pipeline in codegen.
- JSDoc-driven type discovery (Go types only).
- Deep dynamic-import code splitting beyond per-route.
- Runtime template interpretation.
- View Transitions API.
- Built-in i18n primitives (BYO `go-i18n`).
- Built-in form-validation library (BYO `go-playground/validator`).
- Svelte 4 legacy reactivity (`$:`, store autoload) — predates ADR 0005, still excluded.
- Server-side dynamic JS execution.
- Native Go bundler replacing Vite for the client.
- Multi-tenant / RBAC primitives in `kit`.
