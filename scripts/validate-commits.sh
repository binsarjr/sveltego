#!/usr/bin/env bash
set -euo pipefail

range="${1:-origin/main..HEAD}"

pattern='^(Merge .*|Revert .*|fixup!.*|squash!.*|(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|rfc)(\([a-z0-9/_-]+\))?!?: .{1,72})$'

# -E (POSIX ERE) is portable across BSD grep (macOS) and GNU grep (Linux CI).
# The pattern uses no PCRE-only features.
bad=$(git log --format=%s "$range" | grep -Ev "$pattern" || true)
if [[ -n "$bad" ]]; then
  echo "Invalid Conventional Commits in range $range:" >&2
  echo "$bad" >&2
  exit 1
fi
echo "All commits in $range pass Conventional Commits."
