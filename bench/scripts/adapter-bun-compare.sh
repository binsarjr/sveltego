#!/usr/bin/env bash
# adapter-bun-compare.sh — placeholder for the adapter-bun head-to-head.
#
# The MVP regression gate (see .github/workflows/bench.yml) compares
# sveltego against itself between commits via benchstat. Comparing
# against `@sveltejs/adapter-bun` would require Bun + Node + a built
# SvelteKit app inside CI; that's deferred until Phase 1 hardening.
#
# Run locally once Bun is on PATH and `apps/blog-bun/` exists:
#
#   ./scripts/adapter-bun-compare.sh ./apps/blog-bun http://localhost:3000/posts
#
# The script spawns the Bun server, hits it with `oha`, captures a
# bench.json result, and compares with the matching sveltego scenario.
#
# Until that lands, this script prints a deferral notice and exits 0 so
# CI can surface it as a non-fatal step.

set -euo pipefail

if ! command -v bun >/dev/null 2>&1; then
  echo "adapter-bun-compare: bun not found on PATH"
  echo "  install: https://bun.sh/docs/installation"
  echo "  comparison deferred to Phase 1 (see bench/README.md)"
  exit 0
fi

if ! command -v oha >/dev/null 2>&1; then
  echo "adapter-bun-compare: oha not found on PATH"
  echo "  install: https://github.com/hatoo/oha"
  exit 0
fi

echo "adapter-bun-compare: Phase 0mm placeholder"
echo "  apps/blog-bun is not yet wired; comparison harness is a v1.0 deliverable."
echo "  see bench/README.md 'adapter-bun comparison' section for the plan."
exit 0
