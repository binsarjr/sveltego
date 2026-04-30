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

