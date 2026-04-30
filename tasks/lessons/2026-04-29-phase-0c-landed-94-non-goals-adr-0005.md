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

