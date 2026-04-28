# Lessons — sveltego

## 2026-04-29 — Initial R&D

### Insight

- SvelteKit's `Server.respond(Request) → Promise<Response>` is a small contract — Web standards plus optional `AsyncLocalStorage`.
- "Webcontainer mode" was the escape hatch we considered to avoid `AsyncLocalStorage`: serialize requests in runtimes without ALS. It works but caps throughput.
- goja is pure Go but not a drop-in modern JS runtime — partial ESM, no dynamic import, zero Web APIs.
- v8go is the perf king but cross-compile is painful (prebuilt V8 bindings per target).
- subprocess Bun is fastest path to production but is not "true embed" — you ship a 50MB+ runtime alongside the Go binary.

### Self-rules

1. Don't claim "embed" without distinguishing in-process runtime vs ship-binary. Ask the user.
2. Modern SvelteKit bundles use ESM + dynamic import. Runtimes lacking either need a transpile step in the adapter.
3. Web API polyfills in goja are scope creep. Estimate ~70% of total effort.
4. Avoid "production-ready" claims for early PoCs — tier probabilities (PoC vs full vs production).

## 2026-04-29 — Pivot to Go-native rewrite

### Insight

- All JS runtimes bond CPU to a JS engine. Even when the throughput is "OK" (Bun subprocess), the concurrency model is foreign to Go: no goroutines, no `context.Context`, IPC overhead per request.
- Adapters layered on top of SvelteKit-the-JS-server inherit every limitation of the chosen runtime. Going faster than the runtime is impossible.
- The SvelteKit *shape* (file convention, Load/Actions/hooks, layouts) is what users want — not the SvelteKit *implementation*.
- Codegen `.svelte` → Go source is feasible: Svelte 5 templates have a tractable subset, and the `<script>` block can host Go directly when we declare expressions are Go-native.
- Once expressions are Go, we can run `go/parser.ParseExpr` at codegen for validation — type errors surface at build, not runtime.

### Self-rules

1. When the user says "I want X performance," check whether the chosen runtime can ever reach it. If not, propose a different architecture before more polyfill work.
2. Performance ceilings are hard. The runtime defines the max throughput; nothing above it is recoverable via code.
3. Familiar shape (file convention, mental model) is the actual product. Don't conflate it with the upstream implementation.
4. Codegen beats runtime interpretation for SSR every time — static decisions cost nothing per request.

### Decisions captured

- Repo: `binsarjr/sveltego` (private at start).
- Build tool: pure Go. No Node/Bun runtime on the server. Vite stays at build time for the client bundle.
- Expressions: Go-native (PascalCase fields, `nil`, `len()`). No JS-to-Go translator.
- Target: Svelte 5 (runes) only. Skip Svelte 4 legacy syntax.
- Performance target: 20–40k rps for mid-complexity SSR.

## 2026-04-29 — Issue authoring standard

### Insight

- An issue list of ~70 items doesn't speak for itself. Bullet-only checklists without context burn future contributor time looking up "what does this mean."
- Industry-standard issue body is a contract: Summary, Background, Goals, Non-Goals, Detailed Design with code, Acceptance Criteria, Testing Strategy, Out of Scope, Risks & Open Questions, Dependencies (Blocks/Blocked by), References.
- Switching repo language to English mid-project is cheap if done in one pass.

### Self-rules

1. When seeding a roadmap, write each issue as if a stranger will pick it up — context plus contract.
2. Cross-reference dependencies explicitly (Blocks / Blocked by). Don't make readers reconstruct the order.
3. Ship code samples in design sections. Words drift; signatures don't.
4. One language per repo. If switching, batch the migration in a dedicated pass.

## 2026-04-29 — Foundation-first to prevent AI hallucination

### Insight

- An AI agent (or new contributor) joining mid-project hallucinates conventions when the conventions live nowhere central.
- Pre-alpha is the cheapest moment to encode every cross-cutting rule: code style, error handling, logging, ctx propagation, API stability tiers, release process, CI gates, golden testing, bench thresholds.
- Single source of truth per concern. AGENTS.md → auto-sync to .cursorrules + copilot-instructions. Hand-maintaining four copies guarantees drift.
- "Read in this order" list at the top of CLAUDE.md is the cheapest defense against hallucination. The list points at issues #95–105 even before those land as docs, because the issue body is itself the spec.
- A monorepo with N packages needs a per-package STABILITY.md, CHANGELOG.md, and optional CLAUDE.md. Centralized docs help discovery; package-local docs anchor scope-specific patterns.

### Self-rules

1. Encode conventions as foundation issues, not as folklore. If a rule isn't in a referenceable doc or issue, it doesn't exist.
2. AGENTS.md is the master; tool-specific files are generated from it. Never edit `.cursorrules` or `.github/copilot-instructions.md` by hand.
3. CLAUDE.md opens with a numbered "read in this order" list, including issue numbers. Future Claude instances read it before acting.
4. Any new convention adds a checklist item to the PR template Definition of Done. If the DoD doesn't catch it, it's not enforced.
5. Pre-commit + CI form a two-layer gate. Pre-commit gives fast feedback; CI is the enforcement of record.
6. Lint config (.golangci.yml) is the executable form of the style guide. If it can't be linted, write a custom check or accept the drift risk explicitly.
