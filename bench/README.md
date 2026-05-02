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
    baseline.txt             # `go test -bench=.` reference output (all modes)
    baseline-ssr.txt         # SSR-mode subset (regression gate, #105)
    baseline-ssg.txt         # SSG-mode subset (regression gate, #105)
    baseline-spa.txt         # SPA-mode subset (regression gate, #105)
    baseline-static.txt      # static-no-Load subset (regression gate, #105)
  results/
    YYYY-MM-DD/              # dated raw runs (e.g. pivot before/after)
  scripts/
    adapter-bun-compare.sh   # deferred adapter-bun harness (placeholder)
  ssr-constrained/           # local-only constrained-resource SSR bench (#476)
    Dockerfile               # distroless runtime, --cpus=0.5 --memory=1g
    load.js                  # k6 constant-arrival-rate profile
    run.sh                   # build + image + sweep + p99 breakpoint
    RESULTS.md               # measured rps@p99=100ms ceiling
```

## Run

```sh
go test -bench=. -benchmem -count=6 -run='^$' ./bench/...
```

Single scenario via the driver:

```sh
go run ./bench/cmd/sveltego-bench -scenario hello -duration 5s
```

Run a single mode subset:

```sh
go run ./bench/cmd/sveltego-bench -mode ssg -duration 5s
go run ./bench/cmd/sveltego-bench -mode spa -duration 5s
go run ./bench/cmd/sveltego-bench -mode static -duration 5s
```

`-mode` accepts `ssr`, `ssg`, `spa`, `static`, or `all` (default).
`-scenario` selects one scenario by name and overrides `-mode`.

### Constrained-resource bench (local-only, #476)

```sh
bench/ssr-constrained/run.sh
```

Sweeps RPS against a Docker container locked to `--cpus=0.5 --memory=1g`
using k6 + the `playgrounds/ssr-stress` `/longlist` route, then writes
the rps@p99=100 ms breakpoint to `bench/ssr-constrained/last-run.txt`.
See [`bench/ssr-constrained/RESULTS.md`](ssr-constrained/RESULTS.md) for
the latest measurement and host/Docker provenance. CI does not run this
suite (Docker-in-Docker + load-tool flake on shared runners).

## Scenarios

Scenarios are tagged with a `Mode` (`ssr` / `ssg` / `spa` / `static`) so a CI gate or local run can target one mode at a time via the driver's `--mode` flag.

| Name           | Mode   | Pattern              | Notes                                                                |
| -------------- | ------ | -------------------- | -------------------------------------------------------------------- |
| hello          | ssr    | `GET /`              | static greeting — measures pipeline floor                            |
| list           | ssr    | `GET /posts`         | 10-row index — measures iterative writer + escape                    |
| detail         | ssr    | `GET /posts/[id]`    | param resolution + small body                                        |
| action         | ssr    | `POST /api/echo`     | _server.go path — bypasses page render, exercises mux                |
| svelte-spa     | ssr    | `GET /spa`           | pure-Svelte hot path with SSR=true: Load → JSON payload + shell      |
| ssr-hello      | ssr    | `GET /`              | emitted Render simplest case: Push + EscapeHTML pair                 |
| ssr-typical    | ssr    | `GET /page`          | mid-complexity SSR: header + conditional + 10-item list + footer     |
| ssr-heavy      | ssr    | `GET /heavy`         | 100-item each-loop, stresses hot loop + per-iter EscapeHTML          |
| ssg-serve      | ssg    | `GET /`              | full server short-circuit via `servePrerendered` — real SSG path     |
| spa-shell      | spa    | `GET /spa-shell`     | SSR=false short-circuit to renderEmptyShell + JSON payload (#448)    |
| static-no-load | static | `GET /static`        | Templates="svelte", no Load — empty payload pipeline cost (#448)     |

Beyond the HTTP scenarios:

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

Apple M1 Pro, darwin/arm64, `count=6`, 2026-05-02 (post #448 multi-mode bench):

| Bench                     | ns/op | B/op  | allocs/op | rps p50 |
| ------------------------- | ----: | ----: | --------: | ------: |
| ServeHTTP_Hello           |  ~1926 |  2772 |        31 |  519k   |
| ServeHTTP_List            |  ~2250 |  3239 |        32 |  444k   |
| ServeHTTP_Detail          |  ~2693 |  4023 |        38 |  371k   |
| ServeHTTP_Action          |  ~1067 |  1728 |        23 |  937k   |
| ServeHTTP_SvelteSPA       |  ~2194 |  3273 |        37 |  456k   |
| ServeHTTP_SSRHello        |  ~1996 |  2864 |        32 |  501k   |
| ServeHTTP_SSRTypical      |  ~2551 |  3776 |        49 |  392k   |
| ServeHTTP_SSRHeavy        |  ~5893 | 11655 |       143 |  170k   |
| ServeHTTP_SSGServe        | ~17560 |  2411 |        18 |   57k   |
| ServeHTTP_SPAShell        |  ~1273 |  2352 |        28 |  786k   |
| ServeHTTP_StaticNoLoad    |  ~1772 |  2896 |        31 |  564k   |
| RouteResolution           |   ~134 |   336 |         2 | 7.5M    |
| RenderWriter              |    ~17 |     0 |         0 |   60M   |
| ManifestColdStart         |  ~2470 |  7795 |        43 |  405k   |

### Per-mode budget vs measured

Numbers are single-thread, in-process, `httptest`. The "v1.0 budget" column is the rps target from CLAUDE.md and RFC #421; "headroom" is `(measured - budget) / budget`.

| Mode    | Bench                  | v1.0 budget    | measured rps | headroom |
| ------- | ---------------------- | -------------- | -----------: | -------: |
| ssr     | ServeHTTP_SSRTypical   | 10–40k rps     | 392k         | ~10×     |
| ssg     | ServeHTTP_SSGServe     | 20–40k rps     |  57k         | ~1.4×    |
| spa     | ServeHTTP_SPAShell     | JSON-payload   | 786k         | ~80×     |
| static  | ServeHTTP_StaticNoLoad | static-payload | 564k         | ~14×     |

The SSG scenario drives the real `Server.ServeHTTP` → `servePrerendered`
short-circuit (map lookup + `os.ReadFile`) — the production hot path.
Build-time prerender output goes through `Server.Prerender` once at
scenario construction, so the hot loop measures only the request-time
work: lookup, file read, header write. The cold-read budget (no OS page
cache) sets the floor; production traffic with warm-cache repeats will
exceed it.

Single-thread per-request floor for the hello scenario still translates to >500k rps; sveltego's 20–40k rps mid-complexity SSR target (CLAUDE.md) is comfortably exceeded under no contention. The pure-Svelte SPA hot path lands in the same band as the legacy hello scenario, confirming the JSON-payload pivot does not regress the hot path.

Pivot impact details (pre vs post #398–#407): see [`docs/reference/perf.md`](../docs/reference/perf.md) and the dated artifacts under [`bench/results/`](results/).

CI's runner numbers will differ; treat this table as a sanity reference, not a contract.
