## 2026-04-30 — CI playground skip lists are pre-existing footguns

### Insight

- `.github/workflows/ci.yml` lints/vets/tests/builds each `go.work` module in a loop, with a hard-coded skip for `playgrounds/basic`. Two newer playgrounds (`playgrounds/blog` #149, `playgrounds/dashboard` #150) shipped without extending the skip list, and main went red the moment they merged: each playground imports its own `.gen` package which is gitignored, so SSA-based golangci-lint analyzers fail with `could not load export data`, exit 3.
- The skip pattern is duplicated nine times in one workflow file (4 jobs × 2 matrices, plus `isolated-modules`). Any future playground will hit the same trap unless the lister is replaced with a config-driven loop or the skip is moved to a single shell function. Per-playground per-job copy-paste guarantees regressions.
- Issues #151 and #152 reported the symptom from two different angles (lint failure, build failure) but the root cause was a single workflow gap. PR #156 (`fix(ci): skip playground modules with .gen deps in lint loop`) closed both.

### Self-rules

1. **When adding a playground module, also extend every CI skip list in the same PR.** Search `.github/workflows/` for the existing playground name (`grep -rn "playgrounds/basic" .github/workflows/`) and mirror every match. If you can't add the new playground to all of them in one commit, do not merge the playground.
2. **Duplicated skip clauses across loops are a pre-existing footgun. Flag in PR review whenever a workflow contains the same skip case repeated >2 times.** The structural fix is a single shell function `should_skip "$d"` sourced once per job; until that lands, every new playground costs N edits with N-1 chances to forget one.
3. **CI-time codegen (`sveltego compile` before `go test`) is the proper fix for `.gen` import errors. Until that's wired, every playground module must be skipped from the per-module workspace loop, even if it slows feature parity.**
4. **A red main blocks every other agent. Treat "main red" as p0 — drop in-flight feature work, branch from main, fix CI, merge, then resume. Confirm with `gh run list --limit 3 --branch main` before declaring done.**
