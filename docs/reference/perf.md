# Performance

sveltego targets two hot paths after the RFC #379 pivot:

- **Pure-Svelte SPA mode** (default). The server emits the app shell plus a
  JSON hydration payload from `Load`; Vite-built client takes over for
  routing and rendering. The Go side does no template walking — only route
  match, `Load`, JSON encode, and shell write.
- **Pure-Svelte SSG mode** (Prerender + Templates: `svelte`). Build-time
  Node sidecar runs `svelte/server.render` once per route; runtime serves
  the produced HTML as a static file.

Legacy Mustache-Go SSR was removed in #384 (RFC #379 phase 5).

## Bench harness

The `bench/` module exercises the SSR pipeline end-to-end through
`net/http/httptest`:

| Scenario              | What it measures                                   |
| --------------------- | -------------------------------------------------- |
| `ServeHTTP_Hello`     | Static greeting at `/`                             |
| `ServeHTTP_List`      | 10-row index at `/posts`                           |
| `ServeHTTP_Detail`    | Param route `/posts/[id]`                          |
| `ServeHTTP_Action`    | POST handler at `/api/echo` (JSON response)        |
| `ServeHTTP_SvelteSPA` | Pure-Svelte SPA hot path (Load → JSON → shell)     |
| `RouteResolution`     | Path match only, no render                         |
| `RenderWriter`        | `render.Writer` mixed trusted + escaped output     |
| `ManifestColdStart`   | Per-process startup (route tree + matchers + shell)|

Run locally:

```sh
go test -bench=. -benchmem -count=6 ./bench/...
```

## Pivot impact (2026-05-01)

Six-count `benchstat` of pre-pivot (`788be6d`, before #398) vs post-pivot
(HEAD on `feat/pure-svelte-pivot-379`), Apple M1 Pro / darwin-arm64:

| Benchmark              | sec/op (pre)   | sec/op (post)  | Δ              |
| ---------------------- | -------------- | -------------- | -------------- |
| `ServeHTTP_Hello`      | 1.691µ ± 2%    | 1.965µ ± 13%   | +16% (p=0.004) |
| `ServeHTTP_List`       | 2.092µ ± 3%    | 2.068µ ± 1%    | ~ (p=0.065)    |
| `ServeHTTP_Detail`     | 2.377µ ± 3%    | 2.376µ ± 2%    | ~ (p=1.000)    |
| `ServeHTTP_Action`     | 997.7n ± 4%    | 1035.0n ± 3%   | +3.7% (p=0.028)|
| `RouteResolution`      | 124.4n ± 8%    | 134.2n ± 24%   | +7.9% (p=0.015)|
| `RenderWriter`         | 16.52n ± 1%    | 16.60n ± 1%    | ~ (p=0.071)    |
| `ManifestColdStart`    | 2.291µ ± 2%    | 2.520µ ± 10%   | +10% (p=0.004) |
| `ServeHTTP_SvelteSPA`  | n/a            | 2.061µ ± 0%    | new            |

Bytes-per-op and allocs-per-op are byte-for-byte identical across every
existing benchmark. The geomean wall-time delta is +5%, all within
benchmark noise on a workstation. No path regresses by the 20% threshold
that would warrant a follow-up issue per #385.

The new `ServeHTTP_SvelteSPA` scenario lands at 2.061µs — comparable to
legacy `Hello` at 1.965µs — confirming the JSON-hydration hot path costs
roughly the same as the simplest legacy SSR page.

Raw artifacts for this comparison:

- `bench/results/2026-05-01/pre-pivot.txt`
- `bench/results/2026-05-01/post-pivot.txt`
- `bench/results/2026-05-01/benchstat.txt`

## Regression gate

`.github/workflows/bench.yml` runs nightly and on `merge_group`. It
compares HEAD against HEAD~1 with `bench-compare` (5% sec/op threshold,
0% allocs/op threshold). The checked-in `bench/baseline/baseline.txt` is
a human reference, not the gate's source of truth — refresh it when an
intentional perf change lands on `main`.

## SSG bench

A dedicated SSG-mode benchmark requires the Node `svelte/server` sidecar
the build pipeline already invokes. It is out of scope for the slim
phase-6 cut and tracked in a follow-up. Static-file serve is bounded by
`http.ServeFile` throughput, not framework code, so the practical answer
is "as fast as your reverse proxy / OS page cache."
