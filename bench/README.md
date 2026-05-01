# bench

Standalone benchmark module for sveltego. Closes [#60](https://github.com/binsarjr/sveltego/issues/60) — the MVP slice of the suite that backs the "Go-level performance" claim and gates regressions on `main`.

The module is a workspace member alongside the existing `benchmarks/` package. `benchmarks/` covers per-package guards from RFC #105; `bench/` covers full-pipeline SSR scenarios from #60. Two suites, two lifecycles, gated separately in CI.

## Layout

```
bench/
  bench_test.go              # Go testing.B benchmarks (CI gate input)
  go.mod                     # standalone module, not in go.work
  scenarios/
    scenarios.go             # hello, list, detail, action, svelte-spa builders
    log.go                   # quiet slog used in fixtures
  cmd/sveltego-bench/
    main.go                  # rps + p50/p99 driver (no external load tools)
  baseline/
    README.md                # how to refresh checked-in numbers
    baseline.txt             # `go test -bench=.` reference output
  results/
    YYYY-MM-DD/              # dated raw runs (e.g. pivot before/after)
  scripts/
    adapter-bun-compare.sh   # deferred adapter-bun harness (placeholder)
```

## Run

```sh
go test -bench=. -benchmem -count=6 -run='^$' ./bench/...
```

Single scenario via the driver:

```sh
go run ./bench/cmd/sveltego-bench -scenario hello -duration 5s
```

## Scenarios

| Name       | Pattern           | Notes                                                    |
| ---------- | ----------------- | -------------------------------------------------------- |
| hello      | `GET /`           | static greeting — measures pipeline floor                |
| list       | `GET /posts`      | 10-row index — measures iterative writer + escape        |
| detail     | `GET /posts/[id]` | param resolution + small body                            |
| action     | `POST /api/echo`  | _server.go path — bypasses page render, exercises mux    |
| svelte-spa | `GET /spa`        | pure-Svelte SPA hot path: Load → JSON payload + shell    |

Beyond the four HTTP scenarios:

- `BenchmarkRouteResolution` — isolated `tree.Match` cost
- `BenchmarkRenderWriter` — isolated `render.Writer` hot loop
- `BenchmarkManifestColdStart` — per-process scenario build (router tree + matchers + shell parse)

## Regression gate

[`.github/workflows/bench.yml`](../.github/workflows/bench.yml) runs the suite on every push to `main`, on the nightly cron, and on manual dispatch. The job:

1. Checks out `HEAD~1`, runs `go test -bench=. -benchmem -count=3 -run='^$' ./bench/...` → `/tmp/base.txt`.
2. Checks out `HEAD`, runs the same → `/tmp/head.txt`.
3. Pipes both files through `bench-compare` (which wraps `benchstat`).
4. Fails the job when any benchmark regresses past the threshold (default `5%` for `sec/op`, see `benchmarks/cmd/bench-compare/main.go`).

## Threshold tuning

`bench-compare` accepts `-threshold-pct` (default 5 — issue #60 calls for 10; we keep the stricter floor since per-package benches already use 5). Override per-job by patching `.github/workflows/bench.yml` if a wave of intentional regressions lands.

## adapter-bun comparison (deferred)

Issue #60's headline goal is sveltego vs `@sveltejs/adapter-bun` on identical sample apps. Implementing that head-to-head needs:

- A Bun runtime in CI (image bump or setup step).
- A SvelteKit-source-of-truth app per scenario (`apps/blog-bun/`).
- `oha` or `wrk` on PATH for the load step.
- A baseline bun result file checked in, plus a refresh procedure.

None of that lands in Phase 0mm. The MVP gate ships sveltego-vs-sveltego regression detection; the cross-runtime comparison is filed for v1.0 hardening alongside the streaming/SSG/CSP work tracked in milestone v1.0. See [`scripts/adapter-bun-compare.sh`](scripts/adapter-bun-compare.sh) for the placeholder harness — it prints a deferral notice and exits 0 today.

## Performance reference

Apple M1 Pro, darwin/arm64, `count=6`, 2026-05-01 (post RFC #379 pivot):

| Bench               | ns/op | B/op | allocs/op |
| ------------------- | ----: | ---: | --------: |
| ServeHTTP_Hello     |  ~1965 |  2726 |        31 |
| ServeHTTP_List      |  ~2068 |  3199 |        32 |
| ServeHTTP_Detail    |  ~2376 |  3969 |        38 |
| ServeHTTP_Action    |  ~1035 |  1728 |        23 |
| ServeHTTP_SvelteSPA |  ~2061 |  3256 |        37 |
| RouteResolution     |   ~134 |   336 |         2 |
| RenderWriter        |    ~17 |     0 |         0 |
| ManifestColdStart   |  ~2520 |  7731 |        43 |

Single-thread per-request floor for the hello scenario still translates to >500k rps; sveltego's 20–40k rps mid-complexity SSR target (CLAUDE.md) is comfortably exceeded under no contention. The pure-Svelte SPA hot path lands in the same band as the legacy hello scenario, confirming the JSON-payload pivot does not regress the hot path.

Pivot impact details (pre vs post #398–#407): see [`docs/reference/perf.md`](../docs/reference/perf.md) and the dated artifacts under [`bench/results/`](results/).

CI's runner numbers will differ; treat this table as a sanity reference, not a contract.
