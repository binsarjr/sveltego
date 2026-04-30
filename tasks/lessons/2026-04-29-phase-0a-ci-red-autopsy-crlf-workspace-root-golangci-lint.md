## 2026-04-29 — Phase 0a CI red autopsy (CRLF + workspace-root golangci-lint)

### Insight

- First Phase 0a push went red on Windows runners with `gofumpt would reformat the following files: benchmarks\doc.go ...` (14 stub files). Root cause: Windows git defaults to `core.autocrlf=true`, rewriting `\n` → `\r\n` on checkout. gofumpt rejects CRLF. Fix: `.gitattributes` with `* text=auto eol=lf` plus per-extension overrides for `.go .mod .sum .sh .yml .yaml .json .md .bash`. `.bat .cmd` keep CRLF; binaries marked `binary`.
- Same push went red on Ubuntu with `golangci/golangci-lint-action@v6` exit 7: `pattern ./...: directory prefix . does not contain modules listed in go.work`. The action runs `golangci-lint run ./...` from `$GITHUB_WORKSPACE`, which is `go.work` root with no `go.mod`. workspace-root invocation is unsupported.
- Replaced action with manual install + per-module loop iterating `go list -m -f '{{.Dir}}'`, sharing the same path-aware skip pattern as vet/test/build steps. Action's caching benefits lost — acceptable until upstream fixes workspace handling.
- First manual install attempt failed silently: `golangci/golangci-lint info found version: 1.62.2 ...` then exit 1 with no diagnostic. install.sh writes to `$bindir` and exits non-zero if dir absent. `$(go env GOPATH)/bin` doesn't exist on fresh setup-go runners. Fix: `mkdir -p` before `sh install.sh`, plus `set -euxo pipefail` and a post-install `--version` probe to surface real causes next time.

### Self-rules

1. **Repos with Windows CI legs ship `.gitattributes` from day one.** Without it, any text file fails Linux/macOS-authored format checks the moment Windows touches the working tree. Set `* text=auto eol=lf` baseline plus extension overrides; mark images binary; keep `.bat .cmd` CRLF.
2. **Don't trust workspace-aware GitHub Actions for `go.work` repos.** Multi-module workspaces are a minority case and most tool actions assume single-module repos. Default to manual install + per-module loop; add the action back only after verifying it handles `go list -m`.
3. **`install.sh`-style scripts get `set -euxo pipefail` and an explicit verification probe.** Silent install failures waste a CI cycle. The probe (`<binary> --version`) makes the next failure mode obvious in logs.
4. **`$(go env GOPATH)/bin` is not guaranteed to exist** on fresh runners until something `go install`s into it. Always `mkdir -p` before piping a downloader at it.
5. **CI red is normal during foundation phase.** First push green is the exception, not the rule. Budget 1–2 fix cycles for any new workflow before declaring Phase 0a complete.

