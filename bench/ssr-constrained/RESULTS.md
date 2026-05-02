# bench/ssr-constrained — RESULTS

Measurement of sveltego SSR capacity at a fixed resource ceiling of
`--cpus=0.5 --memory=1g`. Closes [#476](https://github.com/binsarjr/sveltego/issues/476).

## Headline

| Metric | Value |
|---|---|
| **Breakpoint rps (p99 first exceeds 100 ms)** | **2917 rps** (p99 = 241.6 ms) |
| **Last clean rps (highest with p99 < 100 ms)** | **2915 rps** (p50 = 0.56 ms, p95 = 2.41 ms, p99 = 18.25 ms) |
| Peak container RSS at the ceiling | 67.7 MiB (out of 1024 MiB granted) |
| Container CPU saturation at the ceiling | ~50% of one logical CPU (matching `cpu.max = 50000 100000`) |
| Failed requests over the entire sweep | 0 |

The ceiling is **CPU-bound, not memory-bound** — RSS never crosses 70
MiB while the cgroup CPU quota is fully spent. Headroom on the memory
side is ~14×.

## What "breakpoint" means here

The transition from clean to overloaded is razor-sharp. At 2915 rps the
service is essentially idle in tail terms (p99 = 18 ms); at 2917 rps —
two requests per second more — p99 jumps to 241 ms. This is the textbook
M/M/1 saturation cliff: as utilization → 1, queue length grows
exponentially and the tail explodes.

The 2917 rps number is therefore the *first* value at which the system
fails the SLO; the *last sustainable* number is 2915 rps. RESULTS.md
reports both — pick whichever framing fits the question.

## Ramp data (warmup 5s, sustain 30s, cooldown 5s)

Three sweeps stitched together; columns are k6's measured numbers from
each sustain window's `summary.json`. Peak CPU% is from `docker stats`
sampled at 1 Hz; peak RSS is the max `MemUsage` value across the same
window.

| Target rps | Actual rps | p50 (ms) | p95 (ms) | p99 (ms) | Peak RSS (MiB) |
|---:|---:|---:|---:|---:|---:|
|  500 |  500 | 0.68 | 1.67 |   7.48 | 11.9 |
| 1000 | 1000 | 0.63 | 2.95 |  19.43 | 20.6 |
| 1500 | 1500 | 0.66 | 3.89 |  43.82 | 29.9 |
| 2000 | 2000 | 0.55 | 2.79 |  17.38 | 38.7 |
| 2500 | 2499 | 0.56 | 2.82 |  30.55 | 47.0 |
| 2750 | 2750 | 0.57 | 3.13 |  16.95 | 65.5 |
| 2875 | 2875 | 0.63 | 6.32 |  34.86 | 56.6 |
| 2900 | 2900 | 0.59 | 2.29 |   7.96 | 67.1 |
| 2910 | 2910 | 0.58 | 2.67 |  46.89 | 69.2 |
| **2915** | **2915** | **0.56** | **2.41** | **18.25** | **67.7** |
| **2917** | **2917** | **0.56** | **5.70** | **241.63** | **69.7** |
| 2920 | 2909 | 0.61 | 64.97 | 474.94 | 66.5 |
| 2937 | 2937 | 0.58 | 5.64 | 147.68 | 64.1 |
| 3000 | 3000 | 0.58 | 4.73 | 177.91 | 60.5 |

Across the full sweep, p50 and p95 stay flat (~0.6 ms / ~3 ms) until
the cliff — confirming the bench is measuring tail behavior, not bulk
throughput collapse. `actual_rps` matches the target within k6's
`constant-arrival-rate` jitter envelope.

## Methodology

- **Route under test:** `/longlist` from
  [`playgrounds/ssr-stress`](../../playgrounds/ssr-stress/src/routes/longlist/).
  Picked because it exercises a 100-item `{#each}` loop over Go-loaded
  data, which is realistic SSR work — not a synthetic minimal handler.
  See `_page.server.go` for the `Load()` shape.
- **Build:** host runs `sveltego compile` (Node sidecar transpiles
  `svelte/server` JS to Go) then `CGO_ENABLED=0 GOOS=linux GOARCH=arm64
  go build -trimpath -ldflags='-s -w'`. The 6.4 MiB stripped binary is
  baked into a `gcr.io/distroless/static-debian12:nonroot` image with
  no shell, no libc, no Node — runtime is the deployable Go binary
  alone.
- **Container:** `docker run --cpus=0.5 --memory=1g
  --platform=linux/arm64`. Verified via `cat /sys/fs/cgroup/cpu.max`
  → `50000 100000` (50 ms quota per 100 ms period) and
  `cat /sys/fs/cgroup/memory.max` → `1073741824` (1 GiB). Go 1.25's
  cgroup-aware `runtime.GOMAXPROCS` automatically picks 1 from the
  fractional `cpu.max`.
- **Load profile:** k6 `constant-arrival-rate` executor.
  - **Warmup:** rps/4 (≥50) for 5 s, summary discarded.
  - **Sustain:** target rps for 30 s; `summary.json` exported.
  - **Cooldown:** 5 s `gracefulStop` window between steps.
  - **maxVUs:** `max(200, RPS × 3)` so request queueing doesn't starve
    the executor under stalls. `preAllocatedVUs` = `max(50, RPS/2)`.
  - **Pre-warm:** 500 sequential `curl` hits before any measurement
    step, so first-sweep readings aren't dominated by the SSR chain's
    lazy init (the unprimed first run consistently shows ~80 ms p99
    even at 500 rps; pre-warm flattens it to ~7 ms).
- **Sweep strategy:** coarse RPS list (default `200 500 1000 1500 2000
  3000 5000`) until p99 first crosses 100 ms, then up to three
  binary-search refinement steps between the last clean rps and the
  overshoot rps. Override with `RPS_LIST="..."` to pin explicit values.
- **Per-step artifacts:** every step writes
  `results/<utc-iso>/rps-<n>/{summary.json,k6.out,docker-stats.txt,peaks.txt}`.
  Sweep summary and host/Docker provenance go to
  `results/<utc-iso>/{sweep.tsv,env.txt}`. Latest sweep also lands
  in `bench/ssr-constrained/last-run.txt` for quick scanning.

## Host + tooling provenance

Numbers above were captured on:

| | |
|---|---|
| Host | Apple M1 Pro, macOS 26.2, darwin/arm64 |
| Docker (client → server) | 29.1.3 → 29.2.1 (Docker Desktop VM, Ubuntu 24.04 aarch64, 4 vCPU / 3.8 GiB) |
| Container platform | linux/arm64 |
| Base image | `gcr.io/distroless/static-debian12:nonroot` |
| Go (build) | 1.26.2 darwin/arm64 (cross-compiled to linux/arm64) |
| k6 | v1.6.1 darwin/arm64 |
| Date (UTC) | 2026-05-02 |
| sveltego sha | `4dff070` (post-#477 LayoutChain retire) |

The container runs natively on Apple silicon (arm64-on-arm64); no
qemu emulation. Docker Desktop's lightweight Linux VM does add some
syscall overhead vs. bare metal Linux, so absolute numbers should be
read as **relative tracking baselines, not SLO promises** for production
hardware. A future re-run on linux/amd64 metal (CI excluded) will
re-anchor the absolute number; until then this RESULTS.md is the
canonical sveltego SSR capacity reference at this resource ceiling.

## What's not in this bench

- **No SvelteKit / Next / Astro comparison.** Per the issue body and
  user direction (2026-05-02), this measures sveltego in isolation. The
  number is for tracking sveltego over time, not pitching against
  another framework.
- **No multi-container scaling.** One container, one SSR route, one
  load generator on the same host. Network overhead is loopback-only.
- **No optimisation work.** The issue explicitly scopes this as
  measurement only; tuning the binary, render path, or scheduler is a
  follow-up if/when this number disappoints.
- **No CI gate.** The whole pipeline (Docker build, container start,
  k6 sweep) takes ~7 minutes per full sweep on this host and is too
  flaky on shared CI runners to make a useful regression signal. Run
  it by hand when SSR-pipeline changes land that could plausibly move
  the number — `internal/codegen/svelte_js2go/`,
  `runtime/svelte/server/`, `server/pipeline.go`, `server/render.go`,
  and the manifest-emit path are the obvious candidates.

## Reproducing

Prerequisites: Docker, `k6`, `go`, `node` + `npm`, `jq`, `curl`. On
macOS: `brew install k6 jq` covers the load-side tooling.

```sh
# default sweep (~7 min on this host); writes results/<utc-iso>/ and
# last-run.txt
bench/ssr-constrained/run.sh

# explicit RPS list, no binary-search refinement
RPS_LIST="500 2000 4000" bench/ssr-constrained/run.sh

# longer sustain window for less noisy tail readings
SUSTAIN_S=60 bench/ssr-constrained/run.sh

# different route from playgrounds/ssr-stress
PLAYGROUND_ROUTE=/conditional bench/ssr-constrained/run.sh
```

Variables surfaced via env: `IMAGE_TAG`, `CONTAINER_NAME`, `HOST_PORT`,
`DOCKER_CPUS`, `DOCKER_MEMORY`, `DOCKER_PLATFORM`, `TARGET_GOOS`,
`TARGET_GOARCH`, `WARMUP_S`, `SUSTAIN_S`, `COOLDOWN_S`, `P99_LIMIT_MS`,
`PLAYGROUND_ROUTE`, `RPS_LIST`. Defaults match the numbers reported
above.

## Notes on tooling choice

- **Why k6 over vegeta / hey / wrk?** k6's `constant-arrival-rate`
  executor enforces target RPS regardless of latency (open-loop) — the
  others are mostly closed-loop or coupled to in-flight VUs. For
  finding a saturation knee the open-loop driver is what we want; once
  the server slows, k6 keeps issuing requests at the target rate
  rather than backing off. Vegeta also offers open-loop, but k6 ships
  full latency histograms in its summary JSON without coordinated
  omission, which simplified the breakpoint detection here.
- **Why distroless over alpine?** No shell, no libc, no extra
  surface — the runtime image is just the Go binary plus `app.html`.
  This matches sveltego's deployment story (single static binary +
  static assets) and keeps the runtime memory floor stable.
- **Why pre-built linux/arm64 binary instead of `go build` inside the
  Docker stage?** The framework's `sveltego compile` step needs the
  Node sidecar (Acorn-driven JS-to-Go transpile of `svelte/server`
  output), which would balloon the build context to the entire
  monorepo. Cross-compiling on the host keeps the Docker context to
  ~6.5 MiB and the runtime image under 9 MiB total.

## Open follow-ups

If sveltego's SSR pipeline regresses past this number, file a new
issue, attach the new `last-run.txt`, and link back here. The numbers
in the table above are the 2026-05-02 anchor on Apple M1 Pro; do not
edit them in place — append a new dated section at the bottom of this
file when re-measuring.
