# 2026-05-03 — `//go:build sveltego` on hooks + config was defensive, never load-bearing (#527)

## Insight

The scaffolded `src/hooks.server.go` and `sveltego.config.go` carried `//go:build sveltego` since the early ADR 0003 amendment (Phase 0i-fix). The original justification: "without the tag, Go's default toolchain (build, vet, lint) would try to compile [these non-`_`-prefix] files." After RFC #379 (route files moved to `_` prefix) and #511 (param matchers gained codegen mirror + auto-register), the tag remained on these last two scaffolded files purely by inertia.

Investigation for #527 surfaced the actual binding facts:

1. **Codegen reads via `go/parser`**, which ignores `//go:build` tags. The tag never gated codegen behavior — `scanHooksServer` strips constraints (via `stripBuildConstraint`) before parsing anyway.
2. **The user file is never linked into the binary.** `cmd/app/main.go` imports `gen.Hooks()` from `<module>/.gen`, which references the codegen mirror at `.gen/hookssrc/hooks_server.go` — package-rewritten and tag-stripped by `mirrorUserSource`. The user's `src/hooks.server.go` (package `hooks`) is a sibling that nothing imports.
3. **`sveltego.config.go` is read by no consumer at all today.** It's a placeholder for future `Config()` wiring; no scanner exists. So its tag was 100% decoration.
4. **Side-benefit of dropping it:** `go vet` and `golangci-lint` finally see the user's `Handle` / `HandleError` / `Reroute` / `Init` implementations. Previously invisible — bugs in these functions only surfaced at `sveltego build` codegen time, not at lint time.

Back-compat verified end-to-end: a project that still carries `//go:build sveltego` on either file builds fine — the tag is a harmless no-op now, since sveltego never passes `-tags=sveltego` to `go build` (and never did for these files; the package-isolation argument from point 2 means `go build ./...` would skip the standalone `hooks` / `config` packages regardless of tag presence, because nothing imports them).

## Self-rules

1. **Audit "defensive" tags before propagating them.** When a scaffold emits a build constraint to "be safe", trace whether any compiler/linter/codegen path actually depends on it. Constraints written by the original author for a reason that no longer holds become permanent ceremony if nobody questions them. The historical justification for the tag was "Go's default toolchain would try to compile this" — which is true but irrelevant when nothing imports the file's package.
2. **`go/parser` ignores build constraints.** When designing codegen that reads user `.go` files, never assume a build tag gates visibility. If you need to filter, do it explicitly (e.g. by filename, by package clause, or by an opt-out comment) — not by build tag.
3. **A scaffold-emitted file that no `main.go` import path reaches is dead-from-the-binary's-POV regardless of tags.** Use this property when deciding whether a file needs ceremony: if it package-isolates and nothing imports the package, the default toolchain will not link it. The build-tag dance is unnecessary.
4. **When dropping a long-standing convention, make it backwards-compatible.** Existing user projects that still carry the tag must keep working. The "tag is now a no-op" framing in the migration notes lets users adopt at their own pace; no flag day, no migration script.
5. **Sweep the doc set in the same PR as the convention change.** README, CLAUDE.md, AGENTS.md (synced .cursorrules + Copilot), `docs/guide/*.md`, `docs/reference/*.md`, `docs/render-modes.md`, AI templates (canonical `templates/ai/` + embedded mirror at `packages/init/internal/aitemplates/files/`). Otherwise the doc set carries stale instructions that LLM agents and human contributors will keep regenerating.
