#!/usr/bin/env bash
# Single source of truth for playgrounds the per-module CI loops skip.
#
# These playgrounds import their own gitignored .gen/ packages (produced
# by `sveltego compile`). Generic per-module loops in ci.yml don't run
# codegen, so SSA-based analyzers (golangci-lint, go vet, go build, go
# test) fail to load export data. The dedicated `playground-smoke` job
# runs codegen first and exercises the basic playground end-to-end.
#
# Adding a new playground = one line below.
#
# Usage in a ci.yml `run:` block (shell: bash):
#   . scripts/ci/skip-playgrounds.sh
#   for d in $dirs; do
#     should_skip_playground "$d" && continue
#     ...
#   done
#
# `should_skip_playground` accepts both relative paths (e.g.
# `playgrounds/basic`, used by isolated-modules where $d comes from
# `dirname go.mod`) and absolute module paths from `go list -m`
# (e.g. `/home/runner/work/sveltego/sveltego/playgrounds/basic` or, on
# Windows runners, `D:\a\sveltego\sveltego\playgrounds\basic`).

SKIP_PLAYGROUNDS=(
  playgrounds/basic
  playgrounds/blog
  playgrounds/dashboard
)

should_skip_playground() {
  local module="$1"
  local sp
  for sp in "${SKIP_PLAYGROUNDS[@]}"; do
    # Normalize backslashes to forward slashes for the comparison so a
    # single glob covers both Unix and Windows paths from `go list -m`.
    local norm="${module//\\//}"
    case "$norm" in
      "$sp"|*/"$sp")
        return 0
        ;;
    esac
  done
  return 1
}
