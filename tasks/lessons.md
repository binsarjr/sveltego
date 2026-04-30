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

## 2026-04-29 — Phase 0d landed (cobra CLI bootstrap)

### Insight

- RFC #5 was authored pre-monorepo with single-module `cmd/`, `internal/`, `pkg/` layout. RFC #95 (Phase 0a) replaced that. Phase 0d delivers the CLI inside the monorepo path `packages/sveltego/cmd/sveltego/` — same files, different parent. The reconciliation belongs in the close comment, not in a re-write of the issue body.
- **Cobra command vars carry state across `Execute()` calls.** Reusing a package-level `var versionCmd = &cobra.Command{...}` across tests means the second test inherits flags parsed by the first. Factory pattern (`newVersionCmd()` returns a fresh tree, `NewRootCmd()` calls all `newXxxCmd()`) eliminates the bleed by construction. Tests should always call `NewRootCmd()`, never reach for the package-level `RootCmd`.
- **`runtime.Version()` already returns `"go1.22.5"` with the prefix.** Spec saying `fmt.Fprintf(..., "go%s", runtime.Version())` produces `gogo1.22.5`. Strip the prefix in a helper. Caught only because tests asserted regex `^sveltego v\S+ \(go\d+\.\d+, ...\)$`.
- **slog has no public level getter.** Tests asserting verbosity flag effect must call `slog.Default().Enabled(ctx, slog.LevelDebug)` rather than introspecting the handler. Equivalent observable signal, no reflection.
- **release-please workflow fails by default on new repos:** `GitHub Actions is not permitted to create or approve pull requests` from `googleapis/release-please-action@v4`. Two fixes: (a) Settings → Actions → General → Workflow permissions → enable "Allow GitHub Actions to create and approve pull requests"; (b) add `permissions: pull-requests: write, contents: write` to the workflow file. Not blocking — CI separate from release-please. Logged for follow-up.
- **LSP cache lag fires misleading diagnostics post-`go mod tidy`.** When agent adds a new dep (cobra) and `go mod tidy` rewrites go.sum, the editor's gopls cache may report `BrokenImport` for ~30s while it re-indexes. Independent `go build` from shell confirms the actual state. Don't trust LSP diagnostics if they contradict a clean shell build.

### Self-rules

1. **Cobra subcommands as factories**, never as package vars referenced from tests. Pattern: `newXxxCmd() *cobra.Command` for each, `NewRootCmd()` composes them. Package var only for the binary entry (`var rootCmd = NewRootCmd()` consumed by `Execute()`).
2. **`runtime.Version()` includes the `"go"` prefix.** Strip it before formatting alongside literal `go` in user-visible strings. Same applies to `runtime.GOOS` / `runtime.GOARCH` (no prefix; safe to concatenate).
3. **slog tests assert observable behavior**, not handler internals. `slog.Default().Enabled(ctx, level)` is the contract.
4. **When LSP and shell disagree, trust the shell.** Re-run `go build` / `go vet` / `go test` from a fresh shell session if diagnostics report imports the disk obviously has. Don't immediately edit code to "fix" a stale-cache symptom.
5. **release-please needs repo settings + workflow permissions.** Either Settings UI or `permissions:` block in the workflow. Document both; pick one per project. Without it, every push to main fails the release-please job silently — easy to miss when CI proper is green.

## 2026-04-29 — Phase 0e landed (parser foundation)

### Insight

- **Issue specs lock package paths the user may override.** Issues #7 and #8 both said `internal/parser/` for token/lexer/AST/parser. User's locked layout decision was three-package split (`internal/lexer/`, `internal/ast/`, `internal/parser/`). The agents need an explicit "override the issue" instruction in the brief — agents will otherwise default to the issue body and produce a single fat package.
- **Field/method name collisions when a struct field shares a getter name.** Issue #8 design showed both `Pos Position` field and `Pos() Position` method on every node. That collides in Go. Phase 0e-B agent renamed the field to `P` and kept `Pos()` as the accessor. Mechanical resolution but it ripples — every parser construction site uses `&ast.Mustache{P: ast.Pos{...}}` not `&ast.Mustache{Pos: ast.Pos{...}}`. Brief downstream agents about it explicitly; they will otherwise transcribe the issue spec verbatim and fail to compile.
- **ADR sub-decisions can defer too aggressively.** ADR 0001 deferred multi-error model to "post-MVP Phase 2". Issue #8 acceptance criteria required ≥2 errors per multi-mistake input — i.e., the deferred work was already MVP-blocking. Reading the dependent issue's acceptance list before sealing the ADR would have caught it. Caught at Phase 0e launch instead.
- **LSP DuplicateDecl false positives after agent file splits.** When an agent moves helper functions from `parser.go` into a new `helpers.go`, gopls reports DuplicateDecl on both files for ~30s while re-indexing. Independent `go build` + `go test` from shell shows clean. Same class as the cobra `BrokenImport` lag from Phase 0d. Reinforces the "trust shell over LSP" rule.
- **Multi-error parser test as forcing function.** A single fixture with two distinct mistakes is the only way to prove recovery actually re-syncs and continues. Without that fixture, a parser that silently aborts on the first error passes every other test. Make the multi-error test a Phase 0e acceptance gate, not a Phase 1 nice-to-have.
- **`text/dump` over JSON for AST goldens.** JSON serialization of an AST with interface fields can't fully round-trip without custom unmarshalers — the discriminator is lost. Indented text dump (`Element "div" @1:5\n  Attribute "class" static="foo" @1:10`) sidesteps the unmarshal problem and makes diffs readable for human review of `-update` flows. JSON acceptance criterion in issue #8 was an over-spec for the actual goal (deterministic diffable output).
- **Two parallel agents on touching layers (lexer + ast) work because each defines its own Pos.** Agent A's Token has flat `Offset/Line/Col` fields. Agent B's `ast.Pos` is a struct. Parser bridges by reading lexer fields and constructing `ast.Pos`. No cross-import, no cycle. The cost is field-naming friction at the bridge, not at the package boundary. Worth it for parallelism.

### Self-rules

1. **Always state package-layout overrides explicitly in agent briefs.** "Issue #N puts files at path X; override to path Y, do not file files at X" — verbatim. Otherwise agents follow the issue body and the orchestrator pays a fix-up round.
2. **Field-vs-method name collisions on AST nodes default to renaming the field, never the method.** Method is the interface contract (`Node.Pos()`); field is internal storage. Brief downstream agents on the field name once; cite the file (`packages/sveltego/internal/ast/nodes.go`) rather than re-declaring the convention each time.
3. **Read every dependent issue's acceptance criteria before locking an ADR sub-decision.** "Phase 1 / Phase 2" deferrals are valid only if the dependent MVP issues don't already require Phase 2 behavior. Otherwise the deferral is fictional and the ADR misleads.
4. **Trust shell over LSP** (reinforced from Phase 0d). When LSP reports DuplicateDecl, BrokenImport, or any errors that contradict `go build`, run the shell and move on. Never edit code in response to a diagnostic without a confirming shell signal.
5. **Multi-error parser tests are mandatory, not optional.** A single fixture with ≥2 distinct mistakes proves recovery sync. Add it to the test plan up front, not as a follow-up.
6. **AST goldens use indented text dump, not JSON.** JSON loses interface discriminators. Text dumps diff cleanly. Reserve JSON for serialization to the wire, not for in-repo goldens.
7. **Cross-module imports need BOTH go.work entries AND go.mod require+replace.** Workspace mode (`go.work`) is the dev-time cross-module resolver. Isolated mode (`GOWORK=off` — used by the `isolated-modules` CI job per RFC #95) requires the consuming module's `go.mod` to carry `require <module> v0.0.0-00010101000000-000000000000` plus `replace <module> => <relative-path>`. First cross-module consumer pays the wiring cost; subsequent consumers reuse it. An agent that says "no go.mod change needed" without verifying GOWORK=off is wrong by default — always run `GOWORK=off go build ./... && GOWORK=off go test -race -short ./...` from the consuming module before declaring any cross-module wiring complete.

## Phase 0f — render + kit + codegen pipeline (2026-04-29)

### Insight

- **Render API drift between ADR draft and shipped code.** ADR 0004 listed `WriteAttr(name, val string)` as part of the runtime surface. The shipped `render` package never grew that method — codegen composes attributes inline as `WriteString(\` name="\`) + WriteEscapeAttr(val) + WriteString(\`"\`)`. Root cause: the ADR was written before the reductive realization that quoting context belongs in codegen, not runtime. The doc-as-spec lagged the design. Phase 0f wrap caught the drift only because the smoke test forced a re-read of the actual render surface.
- **`restoreRunesBytes` round-trip placeholder is a smell, not a feature.** Phase 0f-E ships codegen that rewrites `$rune` → `__sveltegoRune__` to keep `go/parser` happy, then `restoreRunesBytes` un-substitutes after `go/format`. The output does not compile while the rune reference is intact. That is rune-lowering scope creep leaking into the script-extraction phase. The issue body for #14 said "stash for downstream", and we stashed plus transformed plus restored. Should have stashed only.
- **Struct-literal-only PageData was a deliberate user pivot off the issue body.** Issue #15 discussed inferring from explicit `type PageData struct{...}` declarations in `+page.server.go`. The user picked struct-literal-only on `Load()` return as the MVP rule. The ADR amendment captures both the chosen path AND the rejected one (explicit-type) so a future reader does not re-litigate the deferral.
- **LSP false-positive on `restoreRunesBytes`** (UndeclaredName at `codegen.go:82` despite the func existing in `script.go`) reinforced the existing rule (Phases 0d/0e): trust shell `go build` output, not LSP cache, for cross-file symbol lookups. Same class of stale-index bug we have now seen three phases running.

### Self-rules

1. **When an ADR lists API methods, regenerate the list from `grep '^func' <package>/*.go` at codegen time, not from the ADR draft.** Doc-as-spec is fine for capture; doc-as-source-of-truth is wrong once code lands. Every ADR with a code-shaped block gets a "regenerate from disk" step before declaring it locked.
2. **When an issue body says "stash for downstream", stash only — do not transform or round-trip.** No source rewriting in the phase that defers the feature. The placeholder pass for runes belongs deleted once #43–#47 land; file the followup now so the cleanup is tracked, not folklore.
3. **When the user picks a sub-decision that contradicts an issue body, the ADR amendment must capture both paths.** Chosen path goes into the locked sub-decision bullet; rejected path goes in the same bullet as a one-clause "out of scope until <future RFC>" note. Future Claude reads one bullet and gets the full lattice without bouncing files.
4. **Smoke tests for codegen-style pipelines pick fixtures by what compiles, not by what is named first.** Fixtures referencing unbound identifiers (`Data.Ok`, `Items`) need either companion declarations or a `+page.server.go` to be valid. State the binding strategy in the test up front; never assume "it parses, so it builds".
5. **LSP cross-file symbol lookups stay on the bench until shell agrees.** When LSP reports `UndeclaredName` for a symbol declared in a sibling file in the same package, run `go build` and ignore the diagnostic. Never edit functioning code to silence a stale-index warning.

## Phase 0g — router foundation (2026-04-30)

### Insight

- **Sequential dispatch (A → B → C) was the right call vs forced parallelism.** Phase 0g-A shipped the runtime radix tree + matcher API; B layered the route scanner on A's exported `Segment` / `SegmentKind`; C bolted the manifest emitter and `kit/params` built-ins on B's `ScanResult`. Forcing parallel runs would have required a phantom "types-only" sub-phase whose only output was a frozen interface header — cheap to write, but the orchestration cost (extra agent context, extra rendezvous) exceeds the wall-clock saving on a job this small. Sequential beats parallel when each layer's deliverable is narrow and the next layer's first move is to import the previous one.
- **LSP false-positive class continues across phases.** Cross-file undefined-symbol claims (`encodePackageName`, `builtinMatchers`, `discoverMatchers`) appeared during Phase 0g-B/C any time the LSP cache was stale right after a multi-file commit. Phase 0d (cobra), 0e (parser), 0f (codegen) all hit the same pattern. The shell `go build` + `go test -race` are authoritative; the LSP cache is advisory at best for the 30-second window after a batch lands.
- **Pre-commit `golangci-lint --new-from-rev=HEAD ./...` failed at workspace root** because the linter cannot process modules with mixed go.work / GOWORK=off state from a single invocation. Fix in the hook: scope per-module via the same `go list -m -f '{{.Dir}}'` loop that CI uses. This caught me twice now (Phase 0a CI fix + Phase 0g-C local hook fix); the rule belongs in the hook code, not in lessons.
- **`strconv.Atoi("+1")` accepts the leading sign.** The `int` matcher's first cut accepted `+1`, `-0`, and other sign-prefixed inputs — fine by the function name, surprising for the spec's intent of "positive integer-looking string." We kept Atoi semantics for MVP and documented the surprise; a stricter `posint` matcher can ship later. The general lesson is that built-in matchers need a published truth table (positive examples + negative examples + edge cases) at the moment they ship, not in a follow-up PR.
- **Manifest emitter's `Page{}.Render` invocation pattern is load-bearing on ADR 0004.** The codegen-emitted package shape (struct `Page` with method `Render`) is what the manifest cross-imports. ADR 0004's PageData inference therefore has a forward-reference into ADR 0003's manifest emit; ADR 0003's amendment for `GenerateManifest` has a backward-reference into ADR 0004's `Page{}.Render` shape. Both ADRs must point at each other so a future reader hitting one finds the other in one hop.

### Self-rules

1. **Choose sequential dispatch when each phase's first action is to import the previous phase's exports.** The dependency arrow is the orchestration constraint; a "types-only" sub-phase is cheaper to skip than to add. Save parallelism for phases whose interfaces are already locked from a prior milestone.
2. **When LSP reports `unusedfunc`, `UndeclaredName`, or `DuplicateDecl` on a symbol declared in a freshly-landed batch, run `grep -rn '<symbol>' <package>/` before reaching for an editor.** Three phases of false positives establishes the pattern. Do not modify code to silence an LSP diagnostic that contradicts a clean shell build.
3. **Pre-commit hook gates that work in CI must be tested at workspace root with multi-module GOWORK=off before merging.** The pattern is "for each `go list -m -f '{{.Dir}}'` entry, run the gate inside that directory." Any hook that runs lint/vet/test from `$(git rev-parse --show-toplevel)` against a `go.work` parent fails the moment a contributor lands an isolated-module change. Fix the hook, don't fix the workflow.
4. **Every kit/params built-in matcher ships with a truth table.** Truth table covers `+`, `-`, leading zeros, empty string, whitespace, mixed case, the canonical form, and at least one Unicode boundary case. Truth table goes in the matcher file as a comment block adjacent to the package var; tests exercise every row. Not optional — matchers are user-visible API and surprise here is expensive.
5. **When an ADR amendment lands, grep for cross-references in other ADRs and add forward references both ways.** ADR 0003 amend ⇄ ADR 0004 amend is the working example: 0003's manifest emit references 0004's `Page{}.Render`; 0004's PageData inference is consumed by 0003's emitter. One-way pointers rot the moment the consumer is the side that gets reread.

## Phase 0h — HTTP server pipeline (2026-04-30)

### Insight

- **Issue body drift is the norm, not the exception.** Issue #20 was authored before Phase 0f/0g locked the actual runtime API. The body referenced `gen.Manifest`, `pkg/server`, `route.PageHead`, `render.Pool.Get().(*render.Writer)`, and `kit.NewLoadCtx(r, w, params)` — none of those are the shipped surface. Actual surface is `gen.Routes() []router.Route`, `packages/sveltego/server/`, no `PageHead` (deferred to v0.4 `<svelte:head>`), `render.Acquire/Release`, and `kit.NewLoadCtx(r, params)` (no writer). The brief calling out drift verbatim with file paths is what kept the implementation honest. Lesson: every issue body older than the most recent foundation phase should be re-read against the actual disk before coding.
- **`sloglint` no-raw-keys + kv-only is a real constraint, not a style preference.** First pass used `"method"`/`"path"`/`"err"` raw string keys. Linter rejected. The fix is named string constants (`logKeyMethod`, `logKeyPath`, `logKeyError`, `logKeyStatus`) at package scope; values stay alongside via kv-pairs. Snake-case enforced. Bonus: grep for `logKeyError` finds every callsite that emits an error attribute.
- **`tparallel` flags `defer ts.Close()` in a parent test that has parallel subtests.** The parent's `defer` may fire while subtests are still running, causing flaky races. Fix: `t.Cleanup(ts.Close)` runs after all subtests (including parallel ones) finish. Same root cause as the `t.Parallel` requirement on subtests in a parent that calls `t.Parallel`.
- **`gosec` G112 (slowloris) on `http.Server{Addr, Handler}` literal needs `ReadHeaderTimeout`.** A 10-second default bounds the attack surface and is invisible to well-behaved clients. Users wanting custom timeouts can construct their own `http.Server` around `Server.Handler()`.
- **In-process ServeHTTP bench is microsecond-class** (~163 ns/op, 144 B/op, 4 allocs/op on Apple M1 Pro). Issue #20's "10k req/s on 4-core M-series" target is for the full server with TCP + OS network stack; the in-process number is informational only and should not be confused with end-to-end throughput.
- **`render.Acquire/Release` cycle is opaque to the caller** (the pool is package-internal). The pool-reuse test asserts the observable contract: `Acquire()` always returns a Writer with `Len() == 0`. Direct introspection of the pool would break encapsulation.

### Self-rules

1. **Re-read every API the brief references before writing the first line of code.** When an issue body says "use `X`", verify `X` is the current shipped name. Phase 0g landed three rename rounds; Phase 0h's brief explicitly listed five API drifts because the orchestrator did this re-read. Skip the re-read and you write against ghost APIs.
2. **`sloglint` named-key constants are cheap; raw string keys are linter debt.** Define `logKey*` at package scope on the first slog call. Any package logging more than two attrs gets the constant block.
3. **Parent tests with parallel subtests must use `t.Cleanup` for setup teardown, not `defer`.** The `defer` fires when the parent function returns, which is before parallel subtests run.
4. **`http.Server` literals always set `ReadHeaderTimeout`.** `gosec` G112 is non-negotiable in this repo. 10 seconds is a sane default; document overrides.
5. **Bench files for Phase 0 work include the actual measured number in the file header comment.** Future readers should see the p50 without re-running. State the platform (Apple M1 Pro) and the date.
6. **In-pool resource cycle tests assert the observable contract, not the pool internals.** `Acquire().Len() == 0` is a contract; the sync.Pool itself is unobservable.

## Phase 0i — CLI build orchestrator + $lib alias (2026-04-30)

### Insight

- **Issue body drift, again.** Issue #21 referenced `pkg/server`, `pkg/kit`, `gen.Manifest`, `internal/codegen.Run`. None of those names landed. The current shape is `server`, `exports/kit`, `gen.Routes()` factory, and the driver itself was the deliverable for this phase. Issue #83's body assumed the user's go.mod module is literally `app`; the real implementation reads whatever path the user declared and substitutes it. Both drifts caught only because the Phase 0i brief listed them verbatim with file paths. Pattern repeats from 0g/0h: **read every API the brief references against disk before line one of code.**
- **Stdlib-only `go.mod` parsing was the right call for MVP.** `golang.org/x/mod/modfile` is robust but adds a dep, and Phase 0i does not need replace-directive awareness or version parsing. A `bufio.Scanner` looking for `module ` on the first non-blank, non-comment line covers every well-formed go.mod and degrades gracefully on malformed input. Adding `x/mod` later is one line; removing a needless dep is harder.
- **`$lib` rewrite at the source-text level beats AST manipulation.** The `<script lang="go">` body still has to round-trip through `go/parser` after rewriting; replacing the import literal with `regexp.ReplaceAllStringFunc` against `"\$lib(/[^"]*)?"` is one regexp and a string substitution. The trade-off is the rewriter only catches double-quoted import paths — back-tick literals and computed paths are out of scope and documented in the helper's godoc. Cheaper and more obviously correct than running an AST pass for what is fundamentally a textual rename.
- **`cobra.Command.Flags().GetCount("verbose")` reaches the persistent flag from a subcommand.** Reusing the root's `-v` count for build-level verbose avoided redefining the same flag on the leaf and avoided a flag-name collision on `-V`. The subcommand only needs to know "is verbose at all," not the count; `count > 0` is the contract.
- **Test for "no go.mod ancestry" needs an isolated TempDir.** macOS `t.TempDir()` lives under `/var/folders/.../T/...` which has no go.mod up the chain — safe. Linux CI runners may have a `$RUNNER_TEMP` that lives under a workspace; defensive check would be to construct an explicit ancestor without go.mod. For now we accept the platform assumption; if CI flakes we add a guard.
- **Phase split (page emit + manifest emit + embed emit) keeps `Build` readable.** The first cut put everything inside one 200-line `Build`. Refactoring to per-step helpers (`emitPage`, `emitServerStub`, `emitEmbedStub`) made the diagnostic split, `$lib` accumulation, and verbose logging trivial to reason about. Pattern: when an orchestrator grows past ~80 LOC, the next reader will thank you for extracting the per-step helpers up front.

### Self-rules

1. **For every issue body older than the most recent foundation phase, re-read the API surface against disk before writing code.** Phase 0g/0h/0i all had the same drift pattern; the cost of one `grep -rn` against the package is trivial compared to a fix-up round.
2. **Choose stdlib over `golang.org/x/mod` until you need replace/exclude/version awareness.** `bufio.Scanner` for `module ` is sufficient and the dep boundary stays clean.
3. **Source-level rewrites for alias substitution beat AST manipulation when the body parses through `go/parser` immediately after.** Regexp scope is documented; computed paths and back-tick literals are explicitly out of scope.
4. **Subcommand verbose flags reuse the root's persistent count, not a new bool.** `cmd.Flags().GetCount("verbose")` returns the inherited value; collision avoided.
5. **When an orchestrator function passes 80 LOC, extract per-step helpers before adding the next branch.** `Build` is the canonical example: page emit + server stub + manifest + embed split keeps each helper under 30 LOC.
6. **Generated files always use `0o600`.** gosec G306 rejects 0o644 in non-test code. The `genFileMode` constant documents the choice once.
7. **Fixture projects under `cmd/sveltego/testdata/example/` use `.template` suffix for files that need substitution at copy time.** The test harness reads `__SVELTEGO__` and rewrites to the absolute sveltego module path so isolated-mode `go build` resolves imports without requiring `go.work`.
8. **Integration tests that subprocess `go build` get the `integration` build tag.** Default `go test` stays under one second per package; CI runs the tagged set explicitly. Document the tag in the test file header.

## Phase 0i-fix — three framework bugs at MVP boundary (2026-04-30)

### Insight

- **Empirical-validation rule for spec claims.** ADR 0003 locked the SvelteKit-style `+page.server.go` filename and the `[slug]/` directory shape on the assumption Go would tolerate both. Five minutes with `go list ./...` against a real fixture would have rejected both at the time the ADR landed. Both bugs sat latent for months and surfaced only at the Phase 0j hello-world smoke. Rule: every "we just need X" assumption in an ADR earns a five-minute toolchain probe before the ADR seals.
- **Mirror tree pattern for "user code Go cannot load directly".** The `[slug]/` directory is invalid as a Go import path (the `[` character is illegal in module paths). Renaming the directory breaks SvelteKit parity. The fix is decoupling: user owns the source path; codegen owns a Go-loadable mirror at `.gen/usersrc/<encoded>/` whose directory and filenames are deterministic underscores. The wire glue imports the mirror, never the user tree. Authoring DX and compiler constraints stop fighting once each side has its own path.
- **`any`-widening adapters are the right shape when a strongly-typed user method must satisfy a `func(any)` interface field.** Manifest emit cannot assign `Page{}.Render` (typed `data PageData`) to `router.PageHandler` (typed `data any`). Renaming the runtime field destroys the type-erasure contract; widening the user method destroys the user's type-safety. The middle ground is a per-route adapter function emitted by codegen that does the assertion and forwards. The user keeps PageData; the runtime keeps `any`; the adapter is the only place the cast happens, and a mismatch fails fast with a descriptive `fmt.Errorf`.
- **Build constraint `//go:build sveltego` is the cheapest possible exclusion mechanism that keeps gopls / golangci-lint quiet on non-loadable trees.** Cheaper than rename-and-rewrite, cheaper than sub-module schemes, cheaper than `_`-prefix file naming (which still gets caught by gopls's "underscore_prefixed file" walk in some scenarios). The only surface area is one comment line at the top of every user `.go` file plus a scanner diagnostic when it's missing.
- **Go validates filenames before honoring build tags.** A file named `+page.server.go` is rejected with "invalid input file name" regardless of any build constraint inside the file — the package list step happens before file content is read. Renaming is the only fix; tagging cannot rescue a file Go won't even open. By contrast, a file named `page.server.go` carrying `//go:build sveltego` *inside* a `[slug]/` directory works: Go opens the file, reads the constraint, decides to exclude it, and the directory ends up with zero compilable Go files — at which point Go silently skips the directory instead of validating its name.
- **Manifest references symbols in the gen package; the gen package must declare them.** ADR 0003 sketched a `wire.gen.go` re-export but no codegen step actually wrote one — manifest emitted `<gen>.Load` / `<gen>.Actions` against an empty package. Two-side contracts where one side is generated and the other is not need an explicit emitter step. "It's mentioned in the ADR" is not a substitute for running the codegen and reading the diff.
- **`Actions()` stubs are cheaper than conditional manifest emission.** When the user's `page.server.go` declares Load but no Actions, the wire could either emit nothing for Actions (forcing the manifest to know which routes have Actions) or emit a `func Actions() any { return nil }` stub. Stubbing keeps the manifest emitter ignorant of per-route detail and removes a coordination point that would have been a follow-up bug.

### Self-rules

1. **Probe toolchain assumptions before sealing an ADR.** When an ADR locks a filename or directory shape, run `go list ./...` against a fixture matching the proposal before declaring it accepted. The probe takes five minutes; missing it costs a Phase boundary's worth of fix-up.
2. **Mirror trees for user source whose path Go cannot load.** When a user-facing path convention conflicts with Go import path rules, decouple via a deterministic mirror at `.gen/usersrc/<encoded>/`. User owns the source path; codegen owns the loadable copy. Never rename the user tree to please the toolchain.
3. **`any`-widening adapters belong in codegen, not in the runtime contract.** When a generated typed method must satisfy a `func(any)` interface field, emit a per-route adapter function that does the type assertion and forwards. Do not weaken the user method; do not widen the runtime field; emit the adapter.
4. **Every user `.go` file under sveltego conventions starts with `//go:build sveltego`.** Scanner emits a warning diagnostic when the line is absent; codegen drives `go/parser` directly so the missing constraint does not break codegen, only `go build` from outside.
5. **When a generated file references symbols, an emitter step writes those symbols.** "Mentioned in the ADR" is not "implemented in the pipeline." Every cross-package symbol reference in a generated file traces back to a generator function with a name and a test; no exceptions.
6. **Stub before conditional emission when the cost is one no-op function.** A `func Actions() any { return nil }` stub keeps the manifest emitter ignorant of per-route detail. Conditional emission is a coordination point waiting to misfire.
7. **Filename validation in Go runs before build tags.** A file named with an invalid character (`+`, `:`, etc.) cannot be rescued by `//go:build ignore` because Go rejects the filename before opening the file. Plan filename schemes against this rule, not against the build-tag escape hatch.

## Phase 0j (re-attempt) — hello-world playground partial; #109 surfaced (2026-04-30)

### Insight

- **Inline-struct-literal Load + named-PageData adapter is a runtime type-mismatch by construction.** Phase 0i-fix's pipeline emits `type PageData struct{...}` in the gen page package and a per-route adapter that does `data.(<gen>.PageData)`. The user's mirror Load returns an anonymous struct literal; once boxed into `any` by the wire wrapper, type assertion against the *named* `PageData` always fails (anon-to-named is a value conversion, not an identity). The build test suite passed because it asserts on emitted file contents, not runtime behavior. Running the playground binary surfaced the bug immediately on the first curl. Filed as #109.
- **PageData inference test fixtures need a runtime gate, not just a file-presence gate.** `TestBuild_HappyPath` validated that `wire.gen.go` and the manifest adapter emit the right *strings*, but the strings encode a runtime trap. The hello-world smoke was the first end-to-end consumer; without it the latent mismatch could have lived through several more phases. Lesson: every codegen feature with a runtime contract gets at least one black-box `go run` smoke before declaring it landed.
- **Parser today emits StaticValue or DynamicValue, never InterpolatedValue.** Codegen's `element.go` has full `*ast.InterpolatedValue` handling, but the parser never constructs one — `<a href="/post/{p.ID}">` parses as a single static literal `"/post/{p.ID}"` with the mustache embedded as plain text. Workaround: write `href={"/post/" + p.ID}` so the parser's mustache branch fires. The gap is real but not a Phase 0j blocker; it's a parser feature for a future phase, separate from #109.
- **Skip-list in the per-module CI loop is unavoidable for any module that imports a generated package not in git.** `playgrounds/basic/cmd/app/main.go` imports `<module>/.gen` which only exists after `sveltego compile` runs. Workspace-mode `go vet/test/build` and `GOWORK=off` isolated runs both fail without it. The dedicated `playground-smoke` job runs codegen first, so it is the canonical home for playground checks; the generic loop must skip the playground or it will go red on every push that touches `packages/sveltego`. The earlier "new convention obviates the skip" assumption was wrong — `//go:build sveltego` only hides the user `.go` files, not the gen-import in the main package.
- **Pre-commit hook does not understand the convention either.** The hook iterates staged `.go` files and runs `go test -short -race <dir>` per parent dir. For user files: `src/routes/[id]/page.server.go` triggers `go test ./.../src/routes/post/[id]` which Go rejects ("invalid char `[`"); `src/routes/page.server.go` triggers `go test` against an all-tagged directory which fails with "build constraints exclude all Go files". Filed as #110. Two framework gaps in this phase (#109 runtime adapter, #110 hook gap) both must close before MVP scope is fully sealed.
- **Compile-then-build subcommand split was the right shape.** `sveltego compile` validates codegen end-to-end before `go build` ever runs. When #109 finally fires, the failure surfaces at the binary's first request, not at compile time. The CLI split keeps codegen errors and Go build errors in separate diagnostic frames, which made the bug easy to localize.

### Self-rules

1. **Every codegen feature with a runtime contract gets a black-box smoke before declaring it landed.** File-content assertions catch emit-time bugs; only `go run` catches type-assertion mismatches, ABI drift, and adapter wiring bugs. The smoke fixture lives in a playground or `-tags=integration` test; a unit test that compares emitted strings is necessary but not sufficient.
2. **Modules whose `cmd/*` imports generated packages get an explicit skip in the per-module CI loop, paired with a dedicated codegen-aware job.** The skip is a structural requirement, not a temporary workaround. Document the reason inline so a future maintainer does not "clean it up" without realizing why it exists.
3. **When user-facing convention requires a particular Load shape (inline struct literal here), test that the shape compiles AND runs end-to-end before sealing the convention.** ADR 0004's struct-literal-only PageData rule looked clean on paper; only the runtime smoke proved that the rule plus the adapter emitter cannot interoperate. Convention reviews must include a runtime hop, not just a parse-and-emit hop.
4. **Phase boundary discipline: when a smoke fails because of a framework bug, file the issue, document the partial state, and stop.** Do not silently fix the framework from inside the playground phase — the fix touches a different package and deserves its own commit, its own test, and its own ADR amendment if needed. Filing #109 plus updating `playgrounds/basic/README.md` with the blocker is the correct partial-completion state.

## Phase 0j-fix — MVP closure (2026-04-30)

### Insight

- **Type alias vs new named type for inferred PageData is a one-character bug with runtime-only consequences.** Phase 0i-fix emitted `type PageData struct{...}`; this looks identical to `type PageData = struct{...}` until `data.(PageData)` runs against an anonymous struct literal. New named types break type identity (anon-to-named is a value conversion, not assertion); aliases preserve identity. The build-test suite passed because it asserts on emitted strings, not runtime semantics. Hello-world smoke surfaced it on the first request, just like #109's filing predicted. Two-character fix (`type X struct` → `type X = struct`); 60+ goldens regenerated; manifest adapter and wire emitter unchanged.
- **Pre-commit hook skip pattern for non-loadable user code splits cleanly into universal-formatting vs lint+test.** Universal formatting (gofumpt, goimports) cares about source bytes only — it works on any `.go` file Go's parser accepts, regardless of build tags or directory shape. Lint and test require a loadable package, which fails on bracketed paths and all-tagged dirs. The fix is a single filter applied between the formatting step (over `staged_go`) and the lint+test step (over `testable_go`). Filter has two layers: path-based (catches `src/{routes,params}/` and `hooks.server.go`) plus build-tag-based (catches any `.go` with `//go:build ... sveltego ...` first line, defense-in-depth for files outside the convention paths).
- **macOS bash 3.2 rejects `case` statements inside `while IFS= read` inside `$(...)`** with "syntax error near unexpected token `;;`" even when the syntax is valid bash 4+. Workaround is to use chained `grep -vE` filters for path-shape skips (`grep` is portable across bash versions) and reserve the `while read` loop for the build-tag content check that genuinely needs file I/O. Two `grep -vE` invocations plus one `while read` loop is uglier than a single pipeline but works on every shell the repo claims to support.
- **MVP-end retrospective: Phase 0j surfaced 5 framework bugs over 3 fix passes (#106–108 in 0i-fix, #109–110 in 0j-fix).** All five were latent from earlier phases; none were caught by unit tests. The forcing function in every case was end-to-end smoke (`go run ./cmd/sveltego compile && go build && curl`). File-content assertions in `internal/codegen` test packages caught zero of the five. The pattern is unambiguous: codegen features that compose into a runtime contract need a black-box smoke before declaring landed, not after MVP close.

### Self-rules

1. **When generating a struct type to receive an inline anonymous-struct literal, prefer the type alias form (`type X = struct{...}`) over a new named type.** The alias preserves runtime type identity for the assertion path; the new type forces a value conversion that the wire layer must emit explicitly. Default to the alias unless the codegen has a concrete reason for a distinct nominal type (e.g., method receivers).
2. **Pre-commit hook filters that target a specific file convention belong as path-based `grep -vE` (portable, bash 3.2-safe) plus a `while read` build-tag check (only when path filter is insufficient).** Universal-formatting steps stay outside the filter — gofumpt and goimports work on any parseable `.go` file. Lint and test steps consume the filtered list.
3. **End-to-end smoke is the forcing function for codegen-runtime contracts; schedule it once per MVP-class feature, not once at MVP close.** Five latent bugs across two fix passes is direct evidence the unit tests don't cover the runtime contract. Add a `go run` or HTTP-curl smoke as soon as a codegen step claims to produce a loadable package; do not wait for the integration milestone.
4. **When a fix changes a hot-cache emit shape (here, every `page.gen.go`), regenerate goldens with `GOLDEN_UPDATE=1` and inspect the diff line-by-line before committing.** 60 golden files flipped from `type PageData struct` to `type PageData = struct`; the diff is large but trivially uniform. Visual inspection is cheap insurance against an emit drift that the test runner might silently accept.

## Phase 0k-A — layout chain rendering (2026-04-30)

### Insight

- **Go's build system silently excludes filenames starting with `_`.** A first emit of `_layout.gen.go` compiled stand-alone via `go build ./.gen/routes/` because Go was already in the directory and didn't apply the underscore filter, but `go list ./.gen/routes` returned `[page.gen.go wire.gen.go]` — the file was simply ignored as if missing. The manifest in the parent package then failed with "undefined: page_routes.Layout" even though the symbol clearly existed in the underscore-prefixed file. Lesson: any `_`-prefixed name is invisible to the toolchain regardless of build tags. The fix was renaming to `layout.gen.go`.
- **Breaking changes to runtime types (`router.Route.LayoutChain` from `[]*Route` to `[]LayoutHandler`) are cheap pre-stable but demand grep across the entire repo.** The old field was unused, so the change was source-compatible with every consumer — but only because no one had wired it. Future changes to stable runtime types need full reference enumeration before flipping. The `[]*Route` shape was a Phase 0h placeholder; documenting the eventual handler shape from day one would have prevented this churn.
- **Slot lowering depends on whether the enclosing emitter is a Page or a Layout.** A naïve `<slot />` → `children(w)` lowering fails to compile inside a Page (no `children` parameter). The minimum-cost solution is a Builder flag (`hasChildren`) that the slot emitter consults; layouts set it true, pages leave it false and emit a TODO comment. Thread emitter context through the Builder, not through every emit function signature.
- **Layout adapter emission and route adapter emission share the alias pool.** A layout dir that also owns a +page.svelte resolves to the same gen package; both adapters import it via the same alias. Tracking aliases per-package (rather than per-route or per-layout) yields a single import line. Emitting two distinct aliases for the same package would compile but produce duplicate import lines and break gofumpt's import grouping.
- **Layout pipeline composition reverses the chain: outer wraps inner, inner is innermost.** Iteration runs `for i := len(chain)-1; i >= 0; i--` to build closures from inside out. The resulting `inner` closure, when called, runs the outermost layout, which calls its `children`, which is the next-out layout, and so on until the page. Order of iteration matches the chain encoding (ancestor → self) but inverts during composition.

### Self-rules

1. **Generated filenames must not start with `_`.** Go's build tooling ignores them silently — no warning, no error, just a missing symbol downstream. Use `layout.gen.go`, `wire.gen.go`, etc. The same rule applies to `.`-prefixed names. Prefer descriptive prefixes (`layout_`, `page_`) over leading separators.
2. **Verify generator output via `go list -f '{{.GoFiles}}'` on the emitted package.** A successful `go build ./<pkg>/` is not proof of a complete file set; the toolchain may have silently filtered files. `go list` reports the canonical accepted set and surfaces invisible files immediately.
3. **Every emit step that produces a file gets a `go list` smoke in its build test.** Cheap insurance against filename rules (`_`, `.`, `testdata/`), build-tag exclusions, and casing mismatches that produce a "missing symbol" error far away from the real cause.
4. **When a Builder needs context-dependent behavior (e.g., slot lowering varies by Page vs Layout), thread the flag through the Builder struct, not through every emit function signature.** The flag is set once at the top of the generator; emit code paths read it without modification.
5. **Pre-stable runtime type changes must enumerate every reference site before flipping.** The change is cheap source-wise but a missed consumer is a silent runtime bug. `grep -rn '<TypeName>' packages/` plus a clean compile across all `go.work` modules is the minimum bar.


## Phase 0m-X2 — kit sentinel helpers (2026-04-30)

### Insight

- **Issue #33 specced `panic(RedirectErr{...})`; we shipped error returns instead.** SvelteKit's `throw redirect()` is JS-flavored; Go's idiom is `return val, err`. Panic-as-control-flow is a known anti-pattern outside of unrecoverable bugs — it interferes with stack-trace analysis, complicates `defer/recover` chains in middleware, and forces every Load callsite to live behind the pipeline's recover guard. Error returns let `errors.As` route the sentinel cleanly, keep the Load contract `(any, error)`, and stay grep-able. The pipeline already had an `httpStatuser` interface lookup; sentinels slot in as additional `errors.As` checks before the generic interface fallback.
- **Branch order in `handleLoadError` matters because `RedirectErr` also implements `httpStatuser`.** The old `httpStatuser` fallback would have grabbed the redirect status (303) and called `writePlain` — losing the Location header entirely. Sentinel-typed checks must run before the generic interface lookup; otherwise a sentinel that happens to satisfy a broader contract gets misrouted. Same shape applies to any future sentinel: its specific branch goes first, generic interfaces fall through last.
- **sloglint's `kv-only: true` plus `no-raw-keys: true` rejects ad-hoc `slog.String("location", ...)` calls.** The repo enforces a closed set of `logKey*` constants used as kv-pair keys. Mixing `slog.Int(...)` with kv-style args also trips `no-mixed-args`. Adding new log attributes requires (1) new constant in the per-package log key block, (2) kv-pair usage at the callsite. Dropping a `slog.X(...)` builder call into otherwise kv-style code fails lint silently from a unit-test perspective — only `golangci-lint run` catches it.
- **`http.Redirect` writes the Location header AND the status code.** No need to call `w.Header().Set("Location", ...)` separately. The function also handles the response body (writes a small HTML stub when method permits). For 303/307/308 with no body content needed, `http.Redirect` is the one-call API.

### Self-rules

1. **Prefer error returns over panics for control-flow sentinels in Go.** When porting a JS framework that uses `throw` for short-circuits, translate to typed errors plus `errors.As` at the boundary. Document the departure from upstream spec inline (godoc on the helper) and in lessons. The only exception is genuinely unrecoverable bugs where the program cannot continue.
2. **Sentinel-type checks in error-handling chains run before generic-interface checks.** When a sentinel implements both a specific identity and a broad interface, the broader interface lookup must come last or the sentinel's specific behavior (e.g. Location header for redirect) gets lost. Order: specific types via `errors.As(&specific)`, then interfaces via `errors.As(&iface)`, then default fallback.
3. **Adding a log attribute requires a `logKey*` constant before the callsite.** sloglint's `no-raw-keys` plus `kv-only` is strict; raw string keys and `slog.X(...)` builder calls both trip it. New attributes get a constant in the package's log-key block and kv-pair usage at every callsite.
4. **Run `golangci-lint run` after any change that touches log calls or imports.** Local gates (`gofumpt`, `go vet`, `go test`) accept code that lint rejects; the lint gate is the only one that catches sloglint violations and mixed-args bugs.
