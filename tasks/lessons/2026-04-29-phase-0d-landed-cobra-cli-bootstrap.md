## 2026-04-29 — Phase 0d landed (cobra CLI bootstrap)

### Insight

- RFC #5 was authored pre-monorepo with single-module `cmd/`, `internal/`, `pkg/` layout. RFC #95 (Phase 0a) replaced that. Phase 0d delivers the CLI inside the monorepo path `packages/sveltego/cmd/sveltego/` — same files, different parent. The reconciliation belongs in the close comment, not in a re-write of the issue body.
- **Cobra command vars carry state across `Execute()` calls.** Reusing a package-level `var versionCmd = &cobra.Command{...}` across tests means the second test inherits flags parsed by the first. Factory pattern (`newVersionCmd()` returns a fresh tree, `NewRootCmd()` calls all `newXxxCmd()`) eliminates the bleed by construction. Tests should always call `NewRootCmd()`, never reach for the package-level `RootCmd`.
- **`runtime.Version()` already returns `"go1.22.5"` with the prefix.** Spec saying `fmt.Fprintf(..., "go%s", runtime.Version())` produces `gogo1.22.5`. Strip the prefix in a helper. Caught only because tests asserted regex `^sveltego v\S+ \(go\d+\.\d+, ...\)$`.
- **slog has no public level getter.** Tests asserting verbosity flag effect must call `slog.Default().Enabled(ctx, slog.LevelDebug)` rather than introspecting the handler. Equivalent observable signal, no reflection.
- **release-please workflow fails by default on new repos:** `GitHub Actions is not permitted to create or approve pull requests` from `googleapis/release-please-action@v4`. Two fixes: (a) Settings → Actions → General → Workflow permissions → enable "Allow GitHub Actions to create and approve pull requests"; (b) add `permissions: pull-requests: write, contents: write` to the workflow file. Not blocking — CI separate from release-please. Logged for follow-up.
- **LSP cache lag fires misleading diagnostics post-`go mod tidy`.** When agent adds a new dep (cobra) and `go mod tidy` rewrites go.sum, the editor's gopls cache may report `BrokenImport` for ~30s while it re-indexes. Independent `go build` from shell confirms the actual state. Don't trust LSP diagnostics if they contradict a clean shell build.

### Self-rules

1. **Cobra subcommands as factories**, never as package vars referenced from tests. Pattern: `newXxxCmd() *cobra.Command` for each, `NewRootCmd()` composes them. Package var only for the binary entry (`var rootCmd = NewRootCmd()` consumed by `Execute()`).
2. **`runtime.Version()` includes the `"go"` prefix.** Strip it before formatting alongside literal `go` in user-visible strings. Same applies to `runtime.GOOS` / `runtime.GOARCH` (no prefix; safe to concatenate).
3. **slog tests assert observable behavior**, not handler internals. `slog.Default().Enabled(ctx, level)` is the contract.
4. **When LSP and shell disagree, trust the shell.** Re-run `go build` / `go vet` / `go test` from a fresh shell session if diagnostics report imports the disk obviously has. Don't immediately edit code to "fix" a stale-cache symptom.
5. **release-please needs repo settings + workflow permissions.** Either Settings UI or `permissions:` block in the workflow. Document both; pick one per project. Without it, every push to main fails the release-please job silently — easy to miss when CI proper is green.

