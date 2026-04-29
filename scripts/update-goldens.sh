#!/usr/bin/env bash
# Rewrite golden fixtures across the workspace.
# Sets GOLDEN_UPDATE=1 and runs every test whose name matches /[Gg]olden/.
# Contributors MUST review the resulting diff (git diff testdata/golden) before
# committing — silent fixture churn is how regressions slip in (RFC #104).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

GOLDEN_UPDATE=1 go test ./... -count=1 -run '.*[Gg]olden.*'

echo
echo "Goldens updated. Review changes:"
echo "  git diff -- '**/testdata/golden/**'"
