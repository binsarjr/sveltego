# Bench regression gate

Every PR that touches Go is benchmarked against `main`. If a perf-critical
path slows down by more than 5% with statistical significance, the gate
fails the PR. Spec: [#105](https://github.com/binsarjr/sveltego/issues/105).

## TL;DR

The gate runs on every PR. It:

- Runs `go test -bench=. -benchmem -count=6 -run=^$ ./benchmarks/...` on
  the base branch and on the PR head.
- Feeds both result files through `benchstat`.
- Fails the PR if any benchmark regresses by **>5% with p<0.05**, or if
  `allocs/op` goes up on a hot path.
- Posts a sticky comment with the full benchstat table.

You override the gate by adding a `bench-regression:` line to the PR
description (see below). Reviewers must accept the justification before
merge.

## When you'd add a benchmark

Add a benchmark when the code runs **per request** or **per template**.
The current perf-critical list:

| Path | Frequency | Benchmark |
|---|---|---|
| `core/codegen.Compile` | per template | `BenchmarkCodegen{Small,Medium,Large}` |
| `runtime/render.Writer` | per request | `BenchmarkRenderWrite` |
| `router.Match` | per request | `BenchmarkRouterMatch` |
| `runtime/server.Handle` | per request | `BenchmarkHTTPCycle` |

A path is "perf-critical" if any of these hold:

- Runs on every HTTP request.
- Runs on every template compile (codegen).
- Allocates in a tight loop.
- Was called out as perf-sensitive in its design issue (label
  [`area:perf`](https://github.com/binsarjr/sveltego/labels/area%3Aperf)).

If you add a path to this list, file a PR that:

1. Adds the benchmark in `benchmarks/`.
2. Updates the table above.
3. Tags the PR with `area:perf`.

## Layout

```
benchmarks/
├── go.mod
├── codegen_bench_test.go
├── render_bench_test.go
├── router_bench_test.go
├── load_pipeline_bench_test.go
└── testdata/
    ├── small.svelte
    ├── medium.svelte
    └── large.svelte
```

`benchmarks/` is a separate Go module so its dependencies do not pollute the
core packages. Bench targets follow the standard `testing.B` shape:

```go
func BenchmarkRouterMatch(b *testing.B) {
    r := buildRouter(routeManifestFromTestdata())
    paths := []string{"/", "/about", "/posts/123", "/api/users/42/posts"}

    b.ReportAllocs()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        r.Match(paths[i%len(paths)])
    }
}
```

Rules:

- Always call `b.ReportAllocs()`. Memory regressions are gated.
- Always call `b.ResetTimer()` after setup. Setup time pollutes the result.
- Avoid hidden allocations in the loop — sample inputs from a pre-built
  slice, do not build them per iteration.
- Use realistic input sizes. A bench that runs in 1ns is measuring nothing
  useful.

## CI flow

```
PR opened
   ├── checkout base, run benchmarks → base.txt
   ├── checkout head, run benchmarks → head.txt
   ├── benchstat base.txt head.txt    → benchstat.txt
   ├── scripts/bench-gate.sh benchstat.txt
   │     ├── parse delta + p-value per row
   │     ├── exit 0 on green, 1 on regression
   │     └── honor `bench-regression:` override in PR body
   └── post benchstat.txt as sticky PR comment
```

The sticky comment is updated in place on every push, so the PR always
shows the latest table. Baselines from `main` push events persist via
`benchmark-action/github-action-benchmark` for trend tracking.

## Threshold semantics

Benchstat reports a delta and a p-value per benchmark row. The gate uses
both:

| Condition | Outcome |
|---|---|
| `p ≥ 0.05` | Noise. Ignored. |
| `delta` ≤ 5% slower, `p < 0.05` | Warn in PR comment. Does not fail. |
| `delta` > 5% slower, `p < 0.05` | **Fail** unless override present. |
| `allocs/op` increases (any amount) on a hot path | **Fail** unless override (configurable per path via `-allocs-pct`). |
| `B/op` increases > 10% on a hot path | **Fail** unless override. |

Improvements (negative delta) are reported but never fail the gate.

The 5% number is a deliberate compromise between noise tolerance on shared
runners and catching real regressions early. It tightens once we move to a
self-hosted runner.

## Override

Some PRs intentionally trade perf for correctness, security, or clarity.
Override by adding a single line to the PR description:

```
bench-regression: <reason>
```

Format:

- Free-form prose after the colon.
- One sentence minimum, must explain *why* the regression is acceptable.
- Reviewer must explicitly approve the justification before merge.

Example:

```
bench-regression: correctness fix in router.Match; +6% acceptable since security trumps perf
```

The gate parses this line out of the PR body and skips the failing rows.
A missing or empty justification still fails the gate.

## Local repro

Reproduce the CI gate before opening the PR:

```bash
# 1. Bench the head (current working tree).
go test -bench=. -benchmem -count=6 -run=^$ ./benchmarks/... > head.txt

# 2. Stash, switch to main, bench again, switch back.
git stash
git checkout main
go test -bench=. -benchmem -count=6 -run=^$ ./benchmarks/... > base.txt
git checkout -
git stash pop

# 3. Compare.
go install golang.org/x/perf/cmd/benchstat@latest
benchstat base.txt head.txt
```

`-count=6` gives benchstat enough samples to compute a stable p-value. Lower
counts produce noisy deltas that flip on rerun.

For a faster inner loop while iterating on a single benchmark:

```bash
go test -bench=BenchmarkRouterMatch -benchmem -count=6 -run=^$ ./benchmarks/...
```

## Hardware variance

Shared GitHub Actions runners are noisy. Two CPU-bound runs on the same
commit can differ by a few percent. We mitigate three ways:

1. `-count=6` per side (12 total samples per benchmark) raises p-value
   confidence.
2. The 5% threshold is wider than typical noise on shared runners.
3. Long-term: dedicated self-hosted runner labeled `[self-hosted, perf]`.
   When that lands, this page documents the runner spec (CPU model, kernel,
   isolation settings) and the threshold tightens.

If a benchmark is consistently flaky on shared runners, that is a signal —
either the bench is too small to measure, or it has a hidden source of
variance. Fix the bench, do not raise the threshold.

## References

- Spec: [#105 Bench regression gate](https://github.com/binsarjr/sveltego/issues/105)
- [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) — delta
  and p-value computation
- [`benchmark-action/github-action-benchmark`](https://github.com/benchmark-action/github-action-benchmark) — baseline persistence
- [`testing.B`](https://pkg.go.dev/testing#B) — bench API
