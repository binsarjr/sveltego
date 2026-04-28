#!/usr/bin/env bash
# Sync .cursorrules and .github/copilot-instructions.md from AGENTS.md.
# AGENTS.md is the single source of truth for AI agent rules (RFC #103).
# Run after editing AGENTS.md; CI verifies these files stay in sync.
set -euo pipefail
# Bash bridge until cmd/sveltego lands; ports to scripts/sync-ai-docs.go in Phase 0c.

ROOT="$(git rev-parse --show-toplevel)"
SRC="$ROOT/AGENTS.md"
HEADER='<!-- AUTO-GENERATED from AGENTS.md by scripts/sync-ai-docs.sh — DO NOT EDIT -->'

if [[ ! -f "$SRC" ]]; then
  echo "AGENTS.md missing at $SRC" >&2
  exit 1
fi

write_target() {
  local dest="$1"
  mkdir -p "$(dirname "$dest")"
  { printf '%s\n\n' "$HEADER"; cat "$SRC"; } > "$dest"
  echo "wrote $dest"
}

write_target "$ROOT/.cursorrules"
write_target "$ROOT/.github/copilot-instructions.md"
