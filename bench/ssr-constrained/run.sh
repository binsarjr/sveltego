#!/usr/bin/env bash
# Reproducible SSR capacity bench at a 0.5 vCPU / 1 GiB RAM container cap.
#
# Pipeline:
#   1. sveltego compile + cross-compile linux/arm64 binary on host.
#   2. docker build a distroless runtime image with the binary baked in.
#   3. docker run with --cpus=0.5 --memory=1g.
#   4. Sweep candidate RPS values (binary search by default) driving k6
#      against a single fixed playground route until p99 latency clears
#      100 ms.
#   5. Write a per-step JSON to results/, then summarize the breakpoint
#      into bench/ssr-constrained/last-run.txt for RESULTS.md authoring.
#
# The script is local-only; CI does not run it (see RESULTS.md).
#
# Usage:
#   ./run.sh                   # full sweep
#   RPS_LIST="500 1000 2000" ./run.sh   # explicit RPS list, no search
#   PLAYGROUND_ROUTE=/conditional ./run.sh
#   SUSTAIN_S=60 ./run.sh      # longer sustain window for stable p99
#
# Requirements: docker, k6, Go (for the cross-compile + sveltego compile),
# Node + npm (sidecar deps for sveltego compile).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PLAYGROUND_DIR="$REPO_ROOT/playgrounds/ssr-stress"
SIDECAR_DIR="$REPO_ROOT/packages/sveltego/internal/codegen/svelterender/sidecar"

PLAYGROUND_ROUTE="${PLAYGROUND_ROUTE:-/longlist}"
IMAGE_TAG="${IMAGE_TAG:-sveltego-ssr-constrained:bench}"
CONTAINER_NAME="${CONTAINER_NAME:-sveltego-ssr-constrained-bench}"
HOST_PORT="${HOST_PORT:-13000}"
DOCKER_CPUS="${DOCKER_CPUS:-0.5}"
DOCKER_MEMORY="${DOCKER_MEMORY:-1g}"
DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/arm64}"
TARGET_GOARCH="${TARGET_GOARCH:-arm64}"
TARGET_GOOS="${TARGET_GOOS:-linux}"

WARMUP_S="${WARMUP_S:-5}"
SUSTAIN_S="${SUSTAIN_S:-30}"
COOLDOWN_S="${COOLDOWN_S:-5}"
P99_LIMIT_MS="${P99_LIMIT_MS:-100}"

# Default sweep: coarse first, then narrow around the breakpoint. Override
# with RPS_LIST="..." for explicit values; in that mode the script still
# tags the breakpoint but does not refine.
DEFAULT_RPS_LIST="200 500 1000 1500 2000 3000 5000"
RPS_LIST="${RPS_LIST:-$DEFAULT_RPS_LIST}"

RESULTS_DIR="$SCRIPT_DIR/results/$(date -u +%Y-%m-%dT%H-%M-%SZ)"
mkdir -p "$RESULTS_DIR"
SUMMARY_FILE="$SCRIPT_DIR/last-run.txt"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" >&2; }

cleanup() {
  if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }
}
require docker
require k6
require go
require node
require npm
require jq

log "ensuring sidecar deps installed"
if [ ! -d "$SIDECAR_DIR/node_modules" ]; then
  ( cd "$SIDECAR_DIR" && npm install --no-audit --no-fund )
fi

log "running sveltego compile in $PLAYGROUND_DIR"
( cd "$PLAYGROUND_DIR" && go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile )

log "cross-compile $TARGET_GOOS/$TARGET_GOARCH binary"
BIN_OUT="$SCRIPT_DIR/build/app"
mkdir -p "$SCRIPT_DIR/build"
( cd "$PLAYGROUND_DIR" && \
  CGO_ENABLED=0 GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" \
  go build -trimpath -ldflags="-s -w" -o "$BIN_OUT" ./cmd/app )
cp "$PLAYGROUND_DIR/app.html" "$SCRIPT_DIR/build/app.html"

log "building docker image $IMAGE_TAG ($DOCKER_PLATFORM)"
docker build \
  --platform "$DOCKER_PLATFORM" \
  --build-context root="$SCRIPT_DIR/build" \
  -f "$SCRIPT_DIR/Dockerfile" \
  -t "$IMAGE_TAG" \
  "$SCRIPT_DIR/build" >"$RESULTS_DIR/docker-build.log" 2>&1

cleanup
log "starting container with --cpus=$DOCKER_CPUS --memory=$DOCKER_MEMORY"
docker run -d --rm \
  --name "$CONTAINER_NAME" \
  --platform "$DOCKER_PLATFORM" \
  --cpus="$DOCKER_CPUS" \
  --memory="$DOCKER_MEMORY" \
  -p "$HOST_PORT:3000" \
  "$IMAGE_TAG" >/dev/null

log "waiting for $PLAYGROUND_ROUTE readiness on :$HOST_PORT"
ready=0
for _ in $(seq 1 50); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$HOST_PORT$PLAYGROUND_ROUTE"; then
    ready=1
    break
  fi
  sleep 0.2
done
if [ "$ready" -ne 1 ]; then
  echo "container did not become ready" >&2
  docker logs "$CONTAINER_NAME" >&2 || true
  exit 1
fi

# Pre-warm: hit the route a few hundred times before any measurement
# step so first-sweep readings aren't dominated by lazy init in the
# render chain. Sequential to avoid load-tool surprises this early.
log "pre-warming $PLAYGROUND_ROUTE (500 sequential reqs)"
for _ in $(seq 1 500); do
  curl -fsS -o /dev/null "http://127.0.0.1:$HOST_PORT$PLAYGROUND_ROUTE" || true
done

# Capture host + Docker context once for RESULTS.md provenance.
{
  printf '## host\n'
  printf 'date_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'host_uname=%s\n' "$(uname -srm)"
  if [ "$(uname -s)" = "Darwin" ]; then
    printf 'macos=%s\n' "$(sw_vers -productVersion 2>/dev/null || echo unknown)"
    printf 'cpu_brand=%s\n' "$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)"
  fi
  printf 'docker_client=%s\n' "$(docker version --format '{{.Client.Version}}' 2>/dev/null || echo unknown)"
  printf 'docker_server=%s\n' "$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)"
  printf 'docker_arch=%s\n' "$(docker info --format '{{.Architecture}}' 2>/dev/null || echo unknown)"
  printf 'docker_os=%s\n' "$(docker info --format '{{.OperatingSystem}}' 2>/dev/null || echo unknown)"
  printf 'k6=%s\n' "$(k6 version 2>&1 | head -n1)"
  printf 'go=%s\n' "$(go version)"
  printf 'image=%s\n' "$IMAGE_TAG"
  printf 'cpus=%s memory=%s\n' "$DOCKER_CPUS" "$DOCKER_MEMORY"
  printf 'route=%s\n' "$PLAYGROUND_ROUTE"
  printf 'profile=warmup=%ss sustain=%ss cooldown=%ss\n' "$WARMUP_S" "$SUSTAIN_S" "$COOLDOWN_S"
} >"$RESULTS_DIR/env.txt"

# Sample container peak RSS via docker stats over the sustain window.
sample_stats() {
  local out_file="$1"
  : >"$out_file"
  # `docker stats --no-stream` is one shot; loop manually for fine-grained
  # samples. 1-second cadence aligns with k6 default summary granularity.
  local end=$(( $(date +%s) + SUSTAIN_S + 2 ))
  while [ "$(date +%s)" -lt "$end" ]; do
    docker stats --no-stream --format \
      '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' \
      "$CONTAINER_NAME" >>"$out_file" 2>/dev/null || true
    sleep 1
  done
}

drive_step() {
  local rps="$1"
  local step_dir="$RESULTS_DIR/rps-$rps"
  mkdir -p "$step_dir"
  log "step rps=$rps (warmup=${WARMUP_S}s sustain=${SUSTAIN_S}s)"

  # Warmup: short low-rps run to JIT the GOMAXPROCS-throttled scheduler
  # and prime the network path. Discard the result.
  local warmup_rps=$(( rps / 4 < 50 ? 50 : rps / 4 ))
  RPS="$warmup_rps" \
  TARGET_URL="http://127.0.0.1:$HOST_PORT$PLAYGROUND_ROUTE" \
  DURATION_S="$WARMUP_S" \
  COOLDOWN_S="1" \
  k6 run \
    --no-summary \
    "$SCRIPT_DIR/load.js" >"$step_dir/warmup.out" 2>&1 || true

  sample_stats "$step_dir/docker-stats.txt" &
  local stats_pid=$!

  RPS="$rps" \
  TARGET_URL="http://127.0.0.1:$HOST_PORT$PLAYGROUND_ROUTE" \
  DURATION_S="$SUSTAIN_S" \
  COOLDOWN_S="$COOLDOWN_S" \
  k6 run \
    --summary-export "$step_dir/summary.json" \
    "$SCRIPT_DIR/load.js" >"$step_dir/k6.out" 2>&1 || true

  wait "$stats_pid" 2>/dev/null || true

  if [ ! -s "$step_dir/summary.json" ]; then
    log "step rps=$rps: no summary.json produced (k6 failed); see k6.out"
    return 1
  fi

  local p50 p95 p99 fail_rate fail_count req_total actual_rps
  p50=$(jq -r '.metrics.http_req_duration["p(50)"] // empty' "$step_dir/summary.json")
  p95=$(jq -r '.metrics.http_req_duration["p(95)"] // empty' "$step_dir/summary.json")
  p99=$(jq -r '.metrics.http_req_duration["p(99)"] // empty' "$step_dir/summary.json")
  # http_req_failed: `passes` = number of failed requests in k6's rate
  # metric vocabulary, `fails` = successes. `value` is the failure rate.
  fail_count=$(jq -r '.metrics.http_req_failed.passes // 0' "$step_dir/summary.json")
  fail_rate=$(jq -r '.metrics.http_req_failed.value // 0' "$step_dir/summary.json")
  req_total=$(jq -r '.metrics.http_reqs.count // 0' "$step_dir/summary.json")
  actual_rps=$(jq -r '.metrics.http_reqs.rate // 0' "$step_dir/summary.json")
  printf '%s rps target -> actual=%.0f/s p50=%.2fms p95=%.2fms p99=%.2fms reqs=%s fails=%s (rate=%.4f)\n' \
    "$rps" "${actual_rps:-0}" "${p50:-0}" "${p95:-0}" "${p99:-0}" "$req_total" "$fail_count" "$fail_rate" \
    >>"$RESULTS_DIR/sweep.tsv"

  # Peak CPU% and peak MEM (MiB) from docker stats samples. `docker stats`
  # prints "<name> <cpu>% <used><unit> / <limit><unit>"; awk converts the
  # used field to MiB.
  if [ -s "$step_dir/docker-stats.txt" ]; then
    awk '
      function to_mib(field,    n, u) {
        n = field + 0
        u = field
        sub(/^[0-9.]+/, "", u)
        if (u == "GiB" || u == "GB") return n * 1024
        if (u == "KiB" || u == "KB") return n / 1024
        return n
      }
      {
        cpu = $2; sub(/%/, "", cpu); cpu = cpu + 0
        if (cpu > peak_cpu) peak_cpu = cpu
        mib = to_mib($3)
        if (mib > peak_mem) peak_mem = mib
      }
      END { printf "peak_cpu_pct=%.1f peak_rss_mib=%.1f\n", peak_cpu, peak_mem }
    ' "$step_dir/docker-stats.txt" >"$step_dir/peaks.txt"
  fi

  # Stash the trio into the sweep table for easy scanning.
  printf '  %s\n' "$(cat "$step_dir/peaks.txt" 2>/dev/null || echo 'no stats')" \
    >>"$RESULTS_DIR/sweep.tsv"

  echo "$p99"
}

# ---- coarse sweep ----
: >"$RESULTS_DIR/sweep.tsv"
breakpoint_rps=""
breakpoint_p99=""
prev_rps=""
prev_p99=""
for rps in $RPS_LIST; do
  if ! p99="$(drive_step "$rps")"; then
    log "step rps=$rps failed"
    continue
  fi
  if [ -z "$p99" ] || [ "$p99" = "null" ]; then
    log "step rps=$rps: no p99 reading"
    continue
  fi
  awk -v p="$p99" -v lim="$P99_LIMIT_MS" 'BEGIN { exit !(p > lim) }' && {
    breakpoint_rps="$rps"
    breakpoint_p99="$p99"
    log "p99=${p99}ms exceeds ${P99_LIMIT_MS}ms at rps=$rps; refining"
    break
  }
  prev_rps="$rps"
  prev_p99="$p99"
done

# ---- refinement: binary search between prev_rps and breakpoint_rps ----
if [ -n "${breakpoint_rps:-}" ] && [ -n "${prev_rps:-}" ] && [ -z "${RPS_LIST_OVERRIDE:-}" ]; then
  lo="$prev_rps"
  hi="$breakpoint_rps"
  for _ in 1 2 3; do
    mid=$(( (lo + hi) / 2 ))
    [ "$mid" -le "$lo" ] && break
    if ! p99="$(drive_step "$mid")"; then continue; fi
    if [ -z "$p99" ] || [ "$p99" = "null" ]; then continue; fi
    if awk -v p="$p99" -v lim="$P99_LIMIT_MS" 'BEGIN { exit !(p > lim) }'; then
      breakpoint_rps="$mid"
      breakpoint_p99="$p99"
      hi="$mid"
    else
      lo="$mid"
      prev_rps="$mid"
      prev_p99="$p99"
    fi
  done
fi

{
  printf 'route=%s\n' "$PLAYGROUND_ROUTE"
  printf 'cpus=%s memory=%s\n' "$DOCKER_CPUS" "$DOCKER_MEMORY"
  printf 'p99_limit_ms=%s\n' "$P99_LIMIT_MS"
  if [ -n "${breakpoint_rps:-}" ]; then
    printf 'breakpoint_rps=%s breakpoint_p99_ms=%s\n' "$breakpoint_rps" "$breakpoint_p99"
  else
    printf 'breakpoint_rps=NOT_REACHED (top RPS in sweep stayed under p99 limit)\n'
  fi
  if [ -n "${prev_rps:-}" ]; then
    printf 'last_clean_rps=%s last_clean_p99_ms=%s\n' "$prev_rps" "$prev_p99"
  fi
  printf 'results_dir=%s\n' "$RESULTS_DIR"
  printf '\n--- sweep ---\n'
  cat "$RESULTS_DIR/sweep.tsv"
  printf '\n--- env ---\n'
  cat "$RESULTS_DIR/env.txt"
} | tee "$SUMMARY_FILE"

log "done. raw artifacts under $RESULTS_DIR"
