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

