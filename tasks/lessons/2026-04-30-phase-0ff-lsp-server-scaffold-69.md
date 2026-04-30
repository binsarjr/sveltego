## Phase 0ff — LSP server scaffold (#69) (2026-04-30)

### Insight

- **Spec said `cmd/sveltego-lsp/`; repo already had `packages/lsp/` listed in `go.work` and `.github/workflows/ci.yml`'s changes filter.** Following the spec literally would have spawned a parallel `cmd/` tree that CI doesn't lint, doesn't include in changes-output fan-out, and that release-please's per-package versioning doesn't track. Repo conventions outrank prompt-supplied paths when CI is already wired for the existing layout — the binary belongs at `packages/lsp/cmd/sveltego-lsp/`.
- **Zero-dep JSON-RPC framing is ~80 LOC and avoids dragging `go.lsp.dev/protocol`'s transitive surface (jsonrpc2, jsonrpc2_v2, xerrors, ...) into a fresh module.** A scaffold doesn't need the full protocol surface; `Content-Length` framing + a small `Message` struct + a method switch is enough to round-trip `initialize`/`shutdown` and any handler we want to stub later. Adding the dep is a one-line follow-up the day a handler actually needs typed LSP structs.
- **`*RPCError` returned as `error` requires an explicit `Error()` method.** A struct embedding JSON-RPC's three required fields is not automatically an `error` value — Go won't let you return `&RPCError{...}` from a function declared to return `error` without the method. Easy to miss because the struct shape mirrors the wire form, but the wire form is not the language form.
- **`go test ./...` from repo root fails in a multi-module workspace** with "directory prefix does not contain modules listed in go.work". Always per-module test loop (or `go list ./... | xargs go test`) when verifying a multi-module change. The CI matrix knows this; local gates must too.

### Self-rules

1. **Before creating a new top-level directory the prompt names, grep `.github/workflows/` and `go.work` for existing entries.** If the package is already listed, deliver inside the existing path, no matter what the task spec says. CI fan-out is the source of truth.
2. **Roll a minimal JSON-RPC frame loop for LSP scaffolds.** ~80 LOC of `Content-Length` framing + a `Message` struct beats pulling a 10MB protocol library that the scaffold doesn't yet exercise. Add the library when a handler actually needs the typed structs.
3. **Any custom error type that flows through an `error`-typed return must have an `Error() string` method.** Add it the moment the struct is defined, not when the compiler complains. Naming the type `RPCError` does not magically grant the interface.
4. **Multi-module repos: never run `go test ./...` from the root.** Loop over modules (`for d in $(yq '.use[]' go.work)` or hard-code) or use the per-module CI step. The root-level command silently fails to enumerate workspace members.
5. **VS Code extension scaffolds belong in `editor/<editor>/` per the issue spec, not in `packages/lsp/editor/`.** The editor tree is editor-agnostic; the Go package owns the binary. Keep the boundary clean so future Neovim / Helix clients sit alongside as siblings.

