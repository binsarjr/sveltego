## Phase 0p — CI budget split (PR lite vs main full) (2026-04-30)

### Insight

- **Pre-Phase-0p PR cost was ~11 jobs:** 3 OS × 2 Go = 6 lint-and-test, plus changes, isolated-modules, agents-sync, commit-lint, playground-smoke, bench. That spend is a tax on every push during iteration. Most of it (full matrix, isolated modules, playground smoke, bench) only catches regressions at merge boundaries — running them on every PR push is double-payment for the same coverage.
- **Splitting by `github.event_name` is cleaner than a conditional matrix.** Two sibling jobs (`lint-and-test-pr` with 1×1 matrix, `lint-and-test-main` with 3×2 matrix), each gated by `if: github.event_name == 'pull_request'` / `'push'`, reads top-down. A single job with `fromJSON`-driven matrix dispatch is shorter but harder to debug — when a Windows-only failure trips, the YAML reader has to mentally evaluate the `fromJSON` to know which leg ran.
- **Concurrency cancel-in-progress is a free 30% on multi-push PRs.** Without it, force-pushes during iteration queue extra runs that the next push will obsolete. With `cancel-in-progress: true`, only the latest commit's run survives; the in-flight stale runs are killed before they consume their budget.
- **`paths-ignore` short-circuits the workflow before any job spins up.** Docs-only commits (`**.md`, `tasks/**`, `tasks/decisions/**`, `docs/**`) trigger zero jobs. The gate runs at the workflow level, not the job level — no runner is provisioned.
- **bench.yml has no good reason to run on PRs.** PR bench compares base vs head, posts a sticky comment, and gates merge — but the bench-compare gate is advisory in our setup (`bench-regression:` PR-body opt-out exists), and the data lands as a comment that nobody reads on most PRs. Moving bench to push-to-main + nightly schedule + workflow_dispatch keeps the regression gate at merge boundary (where it actually blocks bad merges) and lets nightly drift detection run unattended. Manual dispatch covers the rare PR that needs explicit perf signoff.
- **Removing the bench PR trigger required dropping `github.base_ref` and `github.event.pull_request.head.sha` references** because those expressions are undefined on `push` / `schedule` / `workflow_dispatch` events. The replacement compares `HEAD~1` vs `HEAD`, which is what main-merge regression detection wants anyway.

### Self-rules

1. **CI matrices split by trigger, not collapsed.** Lite PR matrix (1 OS × 1 Go) and full main matrix (N OS × M Go) live in separate jobs gated by `if: github.event_name == ...`. The duplication is annoying once but pays back every PR. A conditional matrix via `fromJSON` is acceptable only when the matrix axes are identical and only the values differ.
2. **Every workflow gets `concurrency: cancel-in-progress: true`.** Free spend reduction on iterative pushes. The group key is `<workflow>-${{ github.ref }}` so different branches don't collide.
3. **Docs-only paths go in `paths-ignore`.** `**.md`, `tasks/**`, `tasks/decisions/**`, `docs/**`, `.github/ISSUE_TEMPLATE/**` should never trigger CI. The list lives at the workflow `on:` level so the gate runs before any job dispatch.
4. **Bench gates on push-to-main, not PR.** PR bench is advisory at best; main-push bench is the merge-time regression check. Add `schedule` (nightly cron `0 3 * * *`) for unattended drift detection and `workflow_dispatch` for manual reruns. PRs that genuinely need bench data run it locally or via dispatch.
5. **Workflow YAML changes ship in a single-scope commit (`ci(workflows):` or `ci:`).** The pre-commit `commit-msg` regex enforces lowercase scope; mixing CI YAML with non-CI changes splits the diff and trips the validator. One PR per workflow restructure keeps the diff reviewable.

