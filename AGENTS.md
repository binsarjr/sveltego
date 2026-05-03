# Agent instructions for sveltego

Master ruleset for AI agents working in this repo. This file is the **single source of truth**; `.cursorrules` and `.github/copilot-instructions.md` are auto-generated from it via `scripts/sync-ai-docs.sh`. Do not edit those files directly.

Per-tool entry points:

- Claude Code → [`CLAUDE.md`](./CLAUDE.md) (full working rules, layered on top of this file).
- Cursor → `.cursorrules` (auto-generated).
- GitHub Copilot → `.github/copilot-instructions.md` (auto-generated).
- Aider / generic agents → this file.

If a per-package `CLAUDE.md` exists (e.g. `packages/sveltego/internal/codegen/CLAUDE.md` once it lands), it wins for that package's scope. Cross-cutting rules live here.

---

## 1. Project shape

`sveltego` is a **rewrite of SvelteKit's shape in pure Go**, not an embedding of SvelteKit-the-JS-server. Pre-alpha. The Go workspace already hosts the core (`packages/sveltego` — including the `internal/codegen/svelte_js2go/` JS-to-Go SSR transpiler and `runtime/svelte/{server,fallback}/` helper packages), auth (`packages/auth`, `packages/cookiesession`), tooling (`packages/lsp`, `packages/mcp`, `packages/init`, `packages/enhanced-img`), six deploy adapters plus `adapter-auto`, the bench harness (`bench/`, `benchmarks/`), AI templates, and end-to-end playgrounds (`basic`, `blog`, `dashboard`, `ssr-stress`, `static`). MVP, v0.2, v0.3, v0.4, and v1.1 milestones have shipped; v0.5 (4 open / 19 closed), v0.6 (31 open / 9 closed), and v1.0 (3 open / 59 closed) are in flight on `binsarjr/sveltego`. The SSR Option B track ([RFC #421](https://github.com/binsarjr/sveltego/issues/421), 9 phases #423–#431) closed 2026-05-02; SSR is live end-to-end. The legacy Mustache-Go template emitter was atomically deleted via [#486](https://github.com/binsarjr/sveltego/issues/486); only the pure-Svelte + transpiled-SSR path remains.

As of 2026-05-01, [ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md) supersedes [ADR 0007](tasks/decisions/0007-svelte-semantics-revisit.md): templates are **100% pure Svelte/JS/TS**, server-side Go files own data, and codegen emits TypeScript declaration files (Go AST → `.d.ts`) for IDE autocompletion. As of 2026-05-02, [ADR 0009](tasks/decisions/0009-ssr-option-b.md) supersedes the SSR-at-runtime portion of ADR 0008: SSR returns via build-time mechanical transpile of `svelte/server` compiled JS to Go (Option B per [RFC #421](https://github.com/binsarjr/sveltego/issues/421)). Runtime is hybrid: build-time SSG sidecar for prerendered routes, build-time JS-to-Go transpile (via vendored Acorn in the same sidecar) emitting per-route `Render()` Go functions for the dominant SSR path, and a long-running Node sidecar at request time **only** for routes that opt in via the `<!-- sveltego:ssr-fallback -->` HTML comment in `_page.svelte` (HTML cached LRU+TTL by `route|hash(data)`). Hard-error-at-build is the default for non-annotated routes the lowerer cannot handle (ADR 0009 sub-decision 2). Phases 2–6 of the pure-Svelte pivot land via [#381](https://github.com/binsarjr/sveltego/issues/381)–[#385](https://github.com/binsarjr/sveltego/issues/385); SSR phases 1–9 closed via [#423](https://github.com/binsarjr/sveltego/issues/423)–[#431](https://github.com/binsarjr/sveltego/issues/431) under tracking [#422](https://github.com/binsarjr/sveltego/issues/422). Original SSR carryovers ([#440](https://github.com/binsarjr/sveltego/issues/440), [#443](https://github.com/binsarjr/sveltego/issues/443), [#435](https://github.com/binsarjr/sveltego/issues/435)) all closed; only [#492](https://github.com/binsarjr/sveltego/issues/492) (`<Image>` element lowering) remains open. Hydration-payload size vs SvelteKit measured in [`tasks/spikes/2026-05-03-hydration-payload-size.md`](./tasks/spikes/2026-05-03-hydration-payload-size.md) (closes [#315](https://github.com/binsarjr/sveltego/issues/315)).

Hard invariants (post-ADR 0008 + 0009; do not reopen without new evidence — see [`tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md`](tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md)):

- **No JS runtime on the server at runtime.** Node may run during `sveltego build` to prerender SSG routes via `svelte/server` and to transpile compiled-server JS to Go via vendored Acorn. The deployed Go binary plus `static/` is the entire deployable; no JS engine on the request path.
- **Templates are pure Svelte.** `.svelte` files contain only Svelte/JS/TS — runes, JS expressions, lowercase props (`{data.user.name}`). Zero Go syntax in mustaches, blocks, or `<script>`. Svelte LSP and the npm Svelte ecosystem work without a fork.
- **Go owns the server.** Server-side Go files return a typed data shape from `Load(ctx kit.LoadCtx)`; that shape becomes `data` in client `$props()`. JSON tags drive the Go ↔ TypeScript boundary.
- **Codegen, not interpretation.** Static decisions at build time. Codegen emits `.svelte.d.ts` declarations (Go AST → TypeScript), prerendered HTML for SSG routes, and per-route Go `Render()` functions transpiled from `svelte/server` JS output for SSR routes.
- **Svelte 5 only.** Runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`). Skip Svelte 4 legacy reactivity.
- **Performance target:** 20–40k rps for SSG (zero per-request work) and JSON-payload responses; ≥10k rps p50 for transpiled SSR routes (RFC #421 acceptance criterion). If a proposal cannot reach that, surface the gap before writing code.
- **Four render modes per route.** `kit.PageOptions` selects per route; `kit.DefaultPageOptions()` returns `SSR: true`, so **SSR is the default** (matches SvelteKit). The four modes: **SSR** (default; Go `Render()` emits HTML, client hydrates), **SSG** (`Prerender: true`; build-time HTML, static handler), **SPA** (`SSR: false`; app shell + JSON payload, client renders), **Static** (no `_page.server.go`; pure `.svelte`, client renders against `{}`). Layouts cascade per-field; page-level overrides win. Full reference: [`docs/render-modes.md`](./docs/render-modes.md).

For high-level project context, read [`README.md`](./README.md) first, then [`CLAUDE.md`](./CLAUDE.md).

---

## 2. Read order before acting

Read in order before any non-trivial action. Do not invent conventions — consult these:

1. [`README.md`](./README.md) — what the project is.
2. [`CLAUDE.md`](./CLAUDE.md) — full working rules and Claude Code entry point.
3. [`CONTRIBUTING.md`](./CONTRIBUTING.md) — code style, error handling, logging, ctx propagation, naming, testing, forbidden patterns.
4. [`STABILITY.md`](./STABILITY.md) — per-package stability index. Each package ships its own `STABILITY.md` describing tiers per exported symbol.
5. [`tasks/todo.md`](./tasks/todo.md) — current execution plan, milestone scope, phase tracking.
6. [`tasks/lessons.md`](./tasks/lessons.md) — design decisions, append-only journal of why things are the way they are.
7. [`tasks/decisions/*.md`](./tasks/decisions/) — locked ADRs. Never edit an `Accepted` ADR in place; supersede with a new one.
8. **Foundation issues #95–105** on `binsarjr/sveltego` — they define the entire project's conventions. Open via `gh issue view <N> --repo binsarjr/sveltego`.
9. Per-package `CLAUDE.md` for scope-specific patterns when the package exists.

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
| #103 | AGENTS.md + AI doc sync | This file's spec. Single source of truth for AI agent rules |
| #104 | Codegen golden testing | `-update` flag flow, determinism rules, review discipline |
| #105 | Bench regression gate | benchstat thresholds, CI integration, override mechanism |
| #421 | RFC: Go-only SSR runtime for Svelte templates | Option A/B/C trade analysis; selected Option B (build-time JS-to-Go transpile via vendored Acorn) |
| #422 | Tracking: SSR Option B implementation | 9-phase rollout (#423–#431); execution journey + cross-phase carryovers |

---

## 3. Hard rules

These are condensed from `CLAUDE.md` §"Working rules". Read `CLAUDE.md` for the full long-form versions; this section is the enforceable summary.

### 3.1 Plan before build

- Tasks with **3+ steps or architectural impact**: write the plan to `tasks/todo.md` first, get approval, then execute.
- "Plan" and "build" are separate. When asked to plan, output only the plan — no code until "yes / do it / push / lanjut".
- Vague instructions ("add a settings page") warrant a written plan, not immediate code.
- Use spec-based development: interview the user about technical approach, UX, concerns, and tradeoffs before writing code. The spec is the contract.
- If something goes wrong mid-execution: **stop and re-plan**. Do not continue blindly.
- If the user provides a written plan: follow it exactly. Flag real problems and wait — do not improvise around them.
- **One-Word Mode:** when the user says "yes", "do it", "push", "lanjut", "ok": execute. Do not repeat the plan.

### 3.2 Verification before done

A task is **not done** when bytes hit disk. It is done when proven correct.

- Re-read every file you edited. The Edit tool reports success on byte-write, not correctness.
- Run the full local gate before declaring done:
  - `gofumpt -l .` (no output = clean)
  - `goimports -l -local github.com/binsarjr/sveltego .`
  - `golangci-lint run` (when `.golangci.yml` is in scope)
  - `go vet ./...`
  - `go test -race ./...`
  - `go build ./...` across all `go.work` packages
- If tooling is missing for a check, **say so explicitly**. Never claim success when a gate is unverified.
- For codegen work: run golden tests and review the diff line-by-line before approving an `-update`.
- Cross-check artifacts: when issue counts change, README + `tasks/todo.md` + `CLAUDE.md` milestone tables must all match.
- Ask: **"Would a staff engineer approve this in code review?"** If no, fix before declaring done.

### 3.3 Phased execution

- Never refactor across **>5 files** in one response. Break into phases.
- Each phase: complete → verify → wait for "ok lanjut" → next phase.
- For >5 independent files, launch parallel sub-agents (5–8 files per agent). One task per sub-agent.
- **Step 0 before any structural refactor:** delete dead code first (unused exports, imports, debug logs). Commit cleanup separately.

### 3.4 Edit safety

- Re-read files after **10+ messages** before editing — context decay corrupts memory.
- Re-read **before every edit**, re-read **after every edit** to confirm the change applied.
- Never batch >3 edits to the same file without an intervening read.
- When renaming a symbol, grep for: direct calls, type references, string literals, dynamic imports, re-exports, test files, mocks. Assume the first grep missed something.
- One source of truth. If tempted to copy state to fix a display bug, the fix is in the wrong place.

### 3.5 Senior dev override

- Ignore default "minimal change, simplest approach" bias when it produces band-aids.
- If architecture is flawed, state is duplicated, or patterns are inconsistent: propose a structural fix, do not patch around it.
- Ask: **"What would a senior, experienced, perfectionist dev reject in code review?"** Fix all of it.
- For non-trivial changes, pause and ask: "Is there a more elegant way?" If a fix feels hacky, implement the clean solution.
- After 2 failed attempts at the same problem: stop. Re-read the relevant section top-down. Propose something fundamentally different.

### 3.6 Mistake logging

- After **any user correction**, create a new file `tasks/lessons/YYYY-MM-DD-<topic>.md` with:
  - `## YYYY-MM-DD — <topic>` heading.
  - "Insight" — what was wrong, with the underlying pattern named.
  - "Self-rules" — numbered, future-tense, prevent the category.
- Add a bullet to the top of `tasks/lessons.md` index pointing at the new file.
- Never rewrite or delete existing lesson files. Append-only journal.
- After fixing a bug, write an autopsy: why did it happen? What category is it? Add a self-rule.

### 3.7 No over-engineering

- No imaginary scenarios. If nobody asked for the scenario, do not handle it.
- No fallbacks for cases that cannot happen. Trust framework guarantees and internal invariants. Validate at boundaries only (HTTP input, external APIs, file I/O).
- Three similar lines beats a premature abstraction.
- No half-finished implementations. If you cannot complete a feature in this phase, do not stub it; file an issue and stop.

### 3.8 Commits

- **Conventional Commits** per RFC #99: `<type>(<scope>): <subject>`.
- `<scope>` = package name (`sveltego`, `adapter-cloudflare`, `codegen`, `router`, ...) or `repo` for cross-cutting changes.
- Subject is imperative, no trailing period, ≤ 72 characters.
- Breaking changes go in the footer: `BREAKING CHANGE: <description>`.
- Never amend a published commit. Never `--no-verify` unless explicitly asked.

### 3.9 Tone in chat

- Caveman mode is active project-wide via session hook. Drop articles, fillers, pleasantries. Fragments OK.
- **Code blocks and commit messages stay normal English.** No caveman in shipped artifacts.
- End-of-turn summary: 1–2 sentences. What changed, what is next.
- When uncertain, say so. Do not invent file paths, function names, or library APIs.
- Trust raw data (logs, error output, file contents) over memory or theories.

### 3.10 Destructive action safety

- Never `git reset --hard`, `git push --force`, `git branch -D`, `git clean -f`, `rm -rf` without explicit user authorization in the **same conversation**. Authorization once does not stand for next time.
- Never bypass hooks (`--no-verify`, `--no-gpg-sign`) unless asked.
- Investigate before deleting unfamiliar files or branches — they may be in-progress work.

---

## 4. Code conventions

The full spec lives in [`CONTRIBUTING.md`](./CONTRIBUTING.md). Quick non-negotiables:

- **Format:** `gofumpt` (stricter superset of `gofmt`) + `goimports -local github.com/binsarjr/sveltego`. Soft cap 120 chars, hard cap 140.
- **Errors:** wrap with `fmt.Errorf("pkg: op: %w", err)` across package boundaries. Sentinel errors at file top. Inspect with `errors.Is` / `errors.As` — never compare strings, never `switch err.(type)`.
- **Logging:** `log/slog` only in runtime. `fmt.Println`, `log.Printf`, `log.Println` are banned outside `cmd/` startup. Always structured fields. No `Fatal` outside `cmd/`.
- **Context:** `ctx context.Context` is the **first** argument on every public function that does I/O, blocks, or spawns a goroutine. Never store `ctx` in a struct. Check `ctx.Err()` between iterations of long loops.
- **Concurrency:** every `go` statement has a documented exit condition. Pair with stop signal (closed channel, cancelled context, `WaitGroup`). Fire-and-forget is forbidden. `sync.Pool` requires a benchmark.
- **Naming:** `snake_case.go` files. Lowercase single-word packages. PascalCase exports without package stutter (`render.Writer`, not `render.RenderWriter`). Acronyms uppercase always (`HTTPClient`, `UserID`).
- **Docs:** every exported symbol has a one-line godoc comment starting with the symbol name. Multi-file packages ship a `doc.go`.
- **Tests:** table-driven by default. `t.Helper()` in helpers. `t.Cleanup(...)` for teardown. Golden files under `testdata/golden/`, regenerate with `-update` (RFC #104). No `time.Sleep` in tests. Race detector required.

### Forbidden

- `init()` outside `package main` and well-justified plugin registries.
- Global mutable state. Configuration travels through constructors.
- `panic()` outside recovered HTTP middleware boundaries and codegen `must` helpers.
- `interface{}` / `any` in public API surfaces. Reach for generics first.
- `os.Exit` outside `cmd/`.
- `reflect` outside codegen and serialization boundaries.

### Stability tiers

Per RFC #97. Every package ships a `STABILITY.md`.

| Tier | Promise | Allowed change |
|---|---|---|
| `stable` | Won't break in current major. | Additive only. Behavior changes go in CHANGELOG. |
| `experimental` | May break in any minor. Marked `// Experimental:` in godoc. | Anything. Deprecate before promotion. |
| `deprecated` | Will be removed. Marked `// Deprecated: <reason>, use X` in godoc. | Removed in next major. |
| `internal-only` | Not importable even if exported. | Anything. |

Before changing exported symbols, check the package's `STABILITY.md`.

---

## 5. File conventions the framework implements

When designing, codegen, or runtime work touches these names, treat them as **load-bearing**:

```
src/routes/
  _page.svelte           // pure Svelte/JS/TS template
  _page.server.go        // Load(), Actions()           (Go skips _* automatically)
  _layout.svelte         // layout chain
  _layout.server.go      // parent data flow            (Go skips _* automatically)
  _server.go             // REST endpoints (GET, POST)  (Go skips _* automatically)
  _error.svelte          // error boundary
  (group)/               // route group, no URL segment
  _page@.svelte          // layout reset
  [param]/               // route param
  [[optional]]/          // optional segment
  [...rest]/             // catch-all
src/params/<name>/<name>.go  // param matchers              — auto-registered via gen.Matchers()
src/lib/                 // shared modules, $lib alias target
src/service-worker.ts    // service worker convention
hooks.server.go          // Handle, HandleError, HandleFetch, Reroute, Init
```

Generated output lives under `.gen/` (gitignored). Every `.gen/*.go` starts with a provenance header — do not edit generated files directly; edit the `.svelte` source. Files under `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`); Go's default toolchain skips files whose names start with `_`, so no `//go:build sveltego` constraint is required there (RFC #379 phase 1b). Param matchers live in `src/params/<name>/<name>.go` (one matcher per subdirectory; package name equals `<name>`); codegen mirrors them into `.gen/paramssrc/<name>/` and emits `gen.Matchers()` so the runtime sees them automatically — no `//go:build sveltego` constraint required (#511). Codegen parses every user `.go` file via `go/parser` regardless. See ADR 0003 amendment (Phase 0i-fix) and RFC #379.

---

## 6. Issue and PR workflow

- **Issue body contract:** Summary · Background · Goals · Non-Goals · Detailed Design (with code) · Acceptance Criteria · Testing Strategy · Out of Scope · Risks & Open Questions · Dependencies (Blocks / Blocked by) · References.
- **Required labels per issue:** one `area:*`, one `type:*`, one `priority:*` (`priority:p0` blocker / `priority:p1` important / `priority:p2` nice-to-have). Areas in use: `auth`, `cli`, `client`, `codegen`, `design`, `docs`, `forms`, `hooks`, `infra`, `llm`, `perf`, `router`, `runtime`, `tooling`. The `blocked` label flags cross-issue waits.
- Author/edit issues with `gh issue create --body-file` or `gh issue edit --body-file`. **Never** inline `--body` (heredoc avoids quoting traps).
- Definition of Done: see `.github/PULL_REQUEST_TEMPLATE.md` (RFC #102).

### Merging to main

PRs land via the **GitHub merge queue**. To queue a PR for merge:

```bash
gh pr merge <num> --auto --squash --delete-branch
```

Required checks: `lint-and-test (ubuntu-latest, go1.25.x)`, `changes (path-aware fan-out)`, `commit-lint`, `agents-sync (AGENTS.md drift)`. (`isolated-modules` runs on `push`/`merge_group` for extra coverage but is not a required gate.)

Concurrency: PR runs cancel-in-progress; main and merge_group runs always complete. Do not use `--admin` to bypass the queue.

---

## 7. Out of scope (do not propose)

See [ADR 0005](tasks/decisions/0005-non-goals.md) for the canonical list and reasoning. [ADR 0008](tasks/decisions/0008-pure-svelte-pivot.md) is the live decision on template semantics (pure Svelte/JS/TS, no Go in mustaches); [ADR 0009](tasks/decisions/0009-ssr-option-b.md) is the live decision on SSR-at-runtime (build-time JS-to-Go transpile, no JS engine at runtime). [ADR 0007](tasks/decisions/0007-svelte-semantics-revisit.md) is superseded — consult ADR 0008 + 0009 before proposing a JS-runtime-on-server, Go-mustache, or Go-VDOM alternative.

Quick reference:

- Svelte 4 legacy reactivity (`$:`, store autoload).
- Server-side dynamic JS execution.
- A native Go bundler replacing Vite for the client.
- Multi-tenant / RBAC primitives in `kit`.
- Universal (shared client+server) `Load`. Server-only by design.
- Universal `+page.ts` / `+layout.ts` loads.
- `<script context="module">` (deprecated upstream).
- WebSocket / SSE primitives in core (BYO `gorilla/websocket`).
- Vercel / Netlify Functions adapters.
- vitePreprocess / arbitrary preprocessor pipeline in codegen.
- JSDoc-driven type discovery (Go types only).
- Deep dynamic-import code splitting beyond per-route.
- Runtime template interpretation.
- View Transitions API.
- Built-in i18n primitives.
- Built-in form-validation library.

---

## 8. Cross-doc consistency

When you edit ANY of these, check the others against the change in the **same commit**:

- `README.md`
- `CLAUDE.md`
- `AGENTS.md` (this file)
- `tasks/todo.md`
- `tasks/lessons.md`
- Issue counts in milestone tables
- `.cursorrules`, `.github/copilot-instructions.md` — auto-synced from `AGENTS.md` via `scripts/sync-ai-docs.sh`. Run the script after editing this file; the sync is verified in CI.

If counts, file lists, or roadmap stages disagree across these, the doc set is broken. Fix all in the same commit.

---

## 9. When in doubt

Ask via the conversation. Never guess file paths, function names, library APIs, or conventions. The cost of asking once is cheaper than the cost of a hallucinated patch landing in the repo.
