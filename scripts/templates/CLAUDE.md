# CLAUDE.md — {{PACKAGE}}

Scope-specific guidance for AI agents and contributors working inside this package. Read the repo-root [`CLAUDE.md`](../../CLAUDE.md) and [`AGENTS.md`](../../AGENTS.md) first; this file only encodes patterns that are not universal.

## What this package does

(One-paragraph purpose; stable across versions.)

## Public API surface

- See [`STABILITY.md`](./STABILITY.md) for the tier of every export.
- Public types/funcs that ship from this package belong in `<file>.go`; helpers in `internal/`.

## Patterns to follow

(Package-specific idioms: how routes are scanned, how codegen is invoked, how tests are organised. Add as the package grows.)

## Patterns to avoid

(Anti-patterns spotted during review or via `tasks/lessons.md` entries that touched this package.)

## Tests

- Golden files under `testdata/golden/` per [RFC #104](https://github.com/binsarjr/sveltego/issues/104).
- Update goldens with `go test ./... -update` (when the codegen package is in scope).

## References

- [RFC #97 — stability tiers](https://github.com/binsarjr/sveltego/issues/97).
- [`CONTRIBUTING.md`](../../CONTRIBUTING.md) for repo-wide style.
