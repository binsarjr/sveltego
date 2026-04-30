## 2026-04-29 — Foundation-first to prevent AI hallucination

### Insight

- An AI agent (or new contributor) joining mid-project hallucinates conventions when the conventions live nowhere central.
- Pre-alpha is the cheapest moment to encode every cross-cutting rule: code style, error handling, logging, ctx propagation, API stability tiers, release process, CI gates, golden testing, bench thresholds.
- Single source of truth per concern. AGENTS.md → auto-sync to .cursorrules + copilot-instructions. Hand-maintaining four copies guarantees drift.
- "Read in this order" list at the top of CLAUDE.md is the cheapest defense against hallucination. The list points at issues #95–105 even before those land as docs, because the issue body is itself the spec.
- A monorepo with N packages needs a per-package STABILITY.md, CHANGELOG.md, and optional CLAUDE.md. Centralized docs help discovery; package-local docs anchor scope-specific patterns.

### Self-rules

1. Encode conventions as foundation issues, not as folklore. If a rule isn't in a referenceable doc or issue, it doesn't exist.
2. AGENTS.md is the master; tool-specific files are generated from it. Never edit `.cursorrules` or `.github/copilot-instructions.md` by hand.
3. CLAUDE.md opens with a numbered "read in this order" list, including issue numbers. Future Claude instances read it before acting.
4. Any new convention adds a checklist item to the PR template Definition of Done. If the DoD doesn't catch it, it's not enforced.
5. Pre-commit + CI form a two-layer gate. Pre-commit gives fast feedback; CI is the enforcement of record.
6. Lint config (.golangci.yml) is the executable form of the style guide. If it can't be linted, write a custom check or accept the drift risk explicitly.

