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

## 2026-04-29 — RFC decision flow (main option + sub-questions)

### Insight

- An RFC issue with N alternatives is not enough. Every option drags 2–4 follow-on questions (error recovery strategy, identifier-naming corner cases, builtin allowlist, snippet visibility). Picking only the headline option leaves codegen blocked.
- Decisions split cleanly into **Main option** (the path) + **Sub-decisions** (the corner cases). Both must be locked before code starts.
- Sub-decisions are best surfaced as a numbered list inside the parent RFC. User answers `1 a, 2 b, ...` in one shot. Cheap for both sides.
- Locked decisions live in two places: GitHub issue body (discussion record) + local `tasks/decisions/NNNN-*.md` ADR (offline + grep-able). One source of truth is a myth when contributors work without GitHub access.
- ADR file format borrows from MADR / Sun: Status, Date, linked Issue, Decision, Rationale, Locked sub-decisions, Implementation outline, References.

### Self-rules

1. When proposing alternatives in an RFC, also enumerate the sub-questions that the chosen option will force. Don't ask the user to pick A/B/C without listing the trapdoors under each.
2. After the user answers, write the locked decision to **both** the GitHub issue (prepend a `## Decision (date)` block above existing alternatives) and a local `tasks/decisions/NNNN-*.md` ADR.
3. ADR filename uses zero-padded 4-digit prefix and stable kebab title — never reuse a number, never edit an Accepted ADR in place. Supersede with a new ADR.
4. Sub-decision rationale belongs **with the sub-decision**, not in a separate doc. A future reader hitting one bullet should find the "why" without bouncing files.
5. Code samples in ADRs are signatures + 5-line illustrations, not full implementations. Signatures don't drift; example bodies do.

## 2026-04-29 — Foundation infra landed (#95-103) + commit-msg scope tightened

### Insight

- Phase 0a landed all foundation infra in one commit: monorepo layout (#95), code style + stability docs (#96, #97), `.golangci.yml` (#98), pre-commit + commit-msg hooks (#99), release-please configs (#100), CI workflows (#101), PR template + DoD (#102), AGENTS.md master + auto-sync to `.cursorrules` and copilot-instructions (#103).
- `.githooks/commit-msg` regex enforces lowercase scope `[a-z0-9/_-]+`. Pre-Phase-0a commits used uppercase scopes like `docs(CLAUDE): ...` because no hook was installed. Those commits already on `main` cannot be retroactively fixed; CI invokes `validate-commits.sh` only on `origin/main..HEAD` (PR delta), so pre-existing history is excluded. Going forward every commit is gated.
- macOS BSD `grep` does not support `-P` (PCRE). `validate-commits.sh` uses `-Ev` (POSIX ERE) instead — the Conventional Commits regex needs no PCRE-only constructs, so semantics are preserved. CI Ubuntu runs are byte-equivalent.
- AGENTS.md sync drift guard works in two directions: editing `AGENTS.md` triggers regeneration + auto-restage; editing only `.cursorrules` or `.github/copilot-instructions.md` is rejected with a "do not edit generated AI rule files" message. Pre-commit hook + CI `agents-sync` job keep both copies aligned.
- RFC #103 specified a Go program (`scripts/sync-ai-docs.go`); we shipped bash (`scripts/sync-ai-docs.sh`) as a bridge until `cmd/sveltego` lands. Documented in the script header.

### Self-rules

1. **Hooks land in the foundation commit**, not later. Any contributor cloning post-Phase-0a runs `bash scripts/install-hooks.sh` before their first commit; CONTRIBUTING.md instructs this. If a contributor bypasses the hook, CI re-validates on PR.
2. **Pre-existing commit history is immutable.** Validate-commits scoping must use `origin/<base>..HEAD`, never the full repo history, to avoid blocking PRs on legacy bad commits.
3. **Cross-platform shell scripts target POSIX ERE**, not PCRE. macOS BSD grep + Linux GNU grep both support `-E`; only Linux supports `-P`. Same lesson applies to `sed -i` (BSD requires `-i ''`, GNU does not) — file each spelling difference as an inline comment when shipping shared shell.
4. **Auto-generated files have a header that says so.** `.cursorrules` and `.github/copilot-instructions.md` start with `<!-- AUTO-GENERATED from AGENTS.md by scripts/sync-ai-docs.sh — DO NOT EDIT -->`. Pre-commit reverse-guard rejects edits to either file when AGENTS.md is unchanged.
5. **Foundation issues close in the same commit that lands their infrastructure.** Issue close happens via `gh issue close` after `git push`, not before, so the close comment can cite the commit SHA.

## 2026-04-29 — Phase 0a CI red autopsy (CRLF + workspace-root golangci-lint)

### Insight

- First Phase 0a push went red on Windows runners with `gofumpt would reformat the following files: benchmarks\doc.go ...` (14 stub files). Root cause: Windows git defaults to `core.autocrlf=true`, rewriting `\n` → `\r\n` on checkout. gofumpt rejects CRLF. Fix: `.gitattributes` with `* text=auto eol=lf` plus per-extension overrides for `.go .mod .sum .sh .yml .yaml .json .md .bash`. `.bat .cmd` keep CRLF; binaries marked `binary`.
- Same push went red on Ubuntu with `golangci/golangci-lint-action@v6` exit 7: `pattern ./...: directory prefix . does not contain modules listed in go.work`. The action runs `golangci-lint run ./...` from `$GITHUB_WORKSPACE`, which is `go.work` root with no `go.mod`. workspace-root invocation is unsupported.
- Replaced action with manual install + per-module loop iterating `go list -m -f '{{.Dir}}'`, sharing the same path-aware skip pattern as vet/test/build steps. Action's caching benefits lost — acceptable until upstream fixes workspace handling.
- First manual install attempt failed silently: `golangci/golangci-lint info found version: 1.62.2 ...` then exit 1 with no diagnostic. install.sh writes to `$bindir` and exits non-zero if dir absent. `$(go env GOPATH)/bin` doesn't exist on fresh setup-go runners. Fix: `mkdir -p` before `sh install.sh`, plus `set -euxo pipefail` and a post-install `--version` probe to surface real causes next time.

### Self-rules

1. **Repos with Windows CI legs ship `.gitattributes` from day one.** Without it, any text file fails Linux/macOS-authored format checks the moment Windows touches the working tree. Set `* text=auto eol=lf` baseline plus extension overrides; mark images binary; keep `.bat .cmd` CRLF.
2. **Don't trust workspace-aware GitHub Actions for `go.work` repos.** Multi-module workspaces are a minority case and most tool actions assume single-module repos. Default to manual install + per-module loop; add the action back only after verifying it handles `go list -m`.
3. **`install.sh`-style scripts get `set -euxo pipefail` and an explicit verification probe.** Silent install failures waste a CI cycle. The probe (`<binary> --version`) makes the next failure mode obvious in logs.
4. **`$(go env GOPATH)/bin` is not guaranteed to exist** on fresh runners until something `go install`s into it. Always `mkdir -p` before piping a downloader at it.
5. **CI red is normal during foundation phase.** First push green is the exception, not the rule. Budget 1–2 fix cycles for any new workflow before declaring Phase 0a complete.

## 2026-04-29 — Phase 0b landed (#104 golden + #105 bench gate)

### Insight

- Golden harness lives in `test-utils/golden` as a generic `golden.Equal(t, name, got)` helper, NOT inside any codegen package. RFC #104 sketched the harness adjacent to `codegen_test.go`, but codegen doesn't exist yet — building the harness as a reusable library lets every package consume it without circular deps.
- Two-mode update toggle (`-args -update` flag AND `GOLDEN_UPDATE=1` env) covers both ergonomic local use and CI scripts. `init()`-time flag registration with a sync.Once guard avoids "flag redefined" panics when multiple packages import the harness in one test binary.
- Bench gate gets its own CLI (`benchmarks/cmd/bench-compare`) instead of the bash `scripts/bench-gate.sh` from RFC #105. Reasons: bash parsing of benchstat CSV is fragile (header rows, blank lines, multi-section format); Go gives table-driven tests for the gate itself; one extra binary on CI is acceptable.
- Trivial `BenchmarkNoop` keeps the workflow exercising end-to-end. Without it, `go test -bench=.` returns "no benchmarks" and benchstat produces empty CSV — gate would never catch a real regression at integration time.
- `bench.yml` triggers on both `pull_request` (gate) and `push: branches: [main]` (deferred baseline persistence). On main push the job is `if: github.event_name == 'pull_request'` so it shows as `skipped`. Acceptable noise vs revisiting the workflow when baseline storage lands.
- `benchstat -format=csv` requires Go 1.22+ in install path; `go install golang.org/x/perf/cmd/benchstat@latest` works against `setup-go@v5`.

### Self-rules

1. **Reusable test helpers live in their own package**, not adjacent to the consumer. `test-utils/<helper>` keeps zero coupling and lets every package import without back-references.
2. **Update toggles need both flag + env.** Local dev wants `go test -args -update`; CI scripts want `GOLDEN_UPDATE=1 go test`. Support both. Guard flag registration with `sync.Once` so multi-package test binaries don't double-register.
3. **Parsers for tool output go in Go, not bash.** When a workflow's logic depends on parsing third-party output (benchstat csv, jq results, etc.), write the parser in Go with table-driven tests. Bash wrapper scripts are fine for orchestration; never for parsing.
4. **Workflows for not-yet-built features need a smoke trigger.** A `BenchmarkNoop` (or `TestNoop`) keeps the pipeline alive end-to-end. Without it, the workflow never proves itself before real tests land — first real bench would also be the first time anyone validates the gate.
5. **Skipped jobs on triggers we don't gate yet are acceptable noise.** If `bench.yml` runs on `push: main` only to keep future baseline persistence one-line away, keep the trigger and `if:`-skip the job. Cheaper than rewriting the workflow later.

## 2026-04-29 — Phase 0c landed (#94 non-goals + ADR 0005)

### Insight

- ADR 0005 mirrors GitHub issue #94 with the locked decision block prepended above the existing draft. Issue body keeps full reasoning; ADR keeps Implementation outline pointing at where each non-goal is enforced (codegen rejects `+page.ts`, no `kit.I18n` package, etc.). Both are canonical; they don't compete because the issue is the discussion log and the ADR is the offline grep-able record.
- Auditing #94's draft caught a stale risk note: "Cloudflare adapter may flip later" — but `packages/adapter-cloudflare` already exists in the workspace. The non-goals doc is allergic to drift; what's listed as "out of scope" must match what's actually missing from the codebase. Fixed inline as part of the lock.
- AskUserQuestion with 4 focused sub-decisions (View Transitions, i18n + forms, Cloudflare risk note, re-eval cadence) was right-sized — answer in <30 seconds, every option mutually exclusive. Earlier RFC locks (e.g., #1-4) used larger interviews; #94 just needed gap-fill on a substantially-drafted body.
- Three orthogonal docs (`tasks/todo.md` "Out of scope", `CLAUDE.md` "Out of scope (do not propose)", `README.md` "What it is not") all carry copies of the non-goal list. Drift between them was real — `README.md` was sparser than `CLAUDE.md`, both lagged the new ADR. Single-pass cross-doc edit kept them aligned. Cross-doc consistency rule (CLAUDE.md §12) earned its keep.

### Self-rules

1. **When locking an RFC that already has a substantial draft, AskUserQuestion only on the gaps.** Don't re-interview categories the user already wrote out. Burden of proof is on the new sub-decision (View Transitions, i18n, etc.), not on what's already drafted.
2. **Audit "may flip later" notes against current codebase before locking.** A non-goals doc that contradicts shipped packages is worse than no doc — readers stop trusting it. Run `ls packages/` and check workspace before locking.
3. **Issue + ADR together; never one without the other.** Issue body holds discussion record (above-the-fold Decision block + original sub-options as history). ADR holds offline reference + Implementation outline. The Implementation outline is the unique value of the ADR — names the codegen rejection point, the missing package, the reading direction.
4. **Cross-doc copies of canonical lists need a single-pass sync rule.** When `tasks/todo.md`, `CLAUDE.md`, `README.md` all carry their own copy of "out of scope", reduce to: ADR is canonical, others get a one-line cross-ref + short bulleted summary. Don't maintain three full copies.
5. **`gh issue edit --body-file` via `gh issue view --json body --jq .body`** is the safe round-trip for editing a long issue body. `--body` inline blows up on quoting; `--body-file -` from stdin works but loses the round-trip safety of editing on disk first.

