# Contributing to sveltego

This document is the single source of truth for code style, error handling,
logging, context propagation, naming, testing, and forbidden patterns. It
codifies [RFC #96](https://github.com/binsarjr/sveltego/issues/96) and
references the surrounding foundation RFCs (#95, #97–#105).

If this file disagrees with a foundation RFC, the RFC wins — open a PR to
fix this file.

## 1. Getting started

```bash
# Clone
git clone https://github.com/binsarjr/sveltego.git
cd sveltego

# Install repo hooks (RFC #99)
bash scripts/install-hooks.sh

# Install dev tools (pin versions in go.mod tools section once it lands)
go install mvdan.cc/gofumpt@latest
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2
```

Verify the install:

```bash
gofumpt -version
goimports -h
golangci-lint version
```

## 2. Workspace layout

The repo is a Go workspace (`go.work`). See [RFC #95](https://github.com/binsarjr/sveltego/issues/95)
for the canonical layout.

| Path | Purpose |
|---|---|
| `packages/` | Versioned, releasable Go modules (core, adapters, tooling). |
| `playgrounds/` | Example apps consumed during development; not released. |
| `benchmarks/` | Bench harness and regression gate (RFC #105). |

Module path naming: `github.com/binsarjr/sveltego/<package>`. New packages
land alongside an issue describing their scope and a fresh `STABILITY.md`
(see §12).

## 3. Formatting

- `gofumpt` (stricter superset of `gofmt`) is the canonical formatter.
- `goimports -local github.com/binsarjr/sveltego` groups repo-local imports
  into their own block, separated from stdlib and third-party.
- Line length: soft cap **120**, hard cap **140**. Lint warns at the cap;
  break long signatures across lines instead of disabling the rule.
- One blank line between methods inside a type. Two blank lines between
  top-level declarations.

```go
import (
    "context"
    "fmt"

    "github.com/google/uuid"

    "github.com/binsarjr/sveltego/packages/sveltego/internal/render"
)
```

## 4. Error handling

- Wrap every error returned across a package boundary with context:

  ```go
  if err := codegen.Emit(ctx, route); err != nil {
      return fmt.Errorf("router: emit %q: %w", route.Path, err)
  }
  ```

- Sentinel errors live at file top, namespaced by package:

  ```go
  var ErrRouteConflict = errors.New("sveltego/router: duplicate route")
  ```

- `return err` with no added context is forbidden. If there is nothing to
  add, the layer is wrong — move the wrap up or down.
- Inspect errors with `errors.Is` / `errors.As`. Never compare error
  strings. Never `switch err.(type)` — use `errors.As`.

```go
var conflict *router.ConflictError
if errors.As(err, &conflict) {
    // handle structured failure
}
```

## 5. Logging

- `log/slog` only in runtime code paths. `fmt.Println`, `log.Printf`, and
  `log.Println` are banned outside `cmd/` startup banners.
- Levels:
  - `Debug` — verbose tracing, off by default.
  - `Info` — lifecycle events (server up, route registered).
  - `Warn` — recoverable anomalies (slow load, retried fetch).
  - `Error` — request-failing or job-failing.
  - `Fatal` is forbidden outside `cmd/`.
- Always use structured fields. No `fmt.Sprintf` into log messages.

```go
slog.InfoContext(ctx, "route registered",
    slog.String("method", route.Method),
    slog.String("path", route.Path),
)
```

- Libraries take an `*slog.Logger` parameter when logging matters. Default
  loggers (`slog.Default()`) are configured once in `cmd/sveltego`.

## 6. Context propagation

- Every public function that performs I/O, blocks, or spawns a goroutine
  takes `ctx context.Context` as the **first** argument.
- `context.Context` is never stored in a struct field. Pass it through.
- Long loops check `ctx.Err()` between iterations and return early on
  cancellation:

  ```go
  for _, item := range items {
      if err := ctx.Err(); err != nil {
          return err
      }
      // ...
  }
  ```

- Request-scoped values use unexported key types so callers cannot collide:

  ```go
  type kitCtxKey int

  const (
      ctxKeyRequestID kitCtxKey = iota
      ctxKeyUser
  )

  func WithRequestID(ctx context.Context, id string) context.Context {
      return context.WithValue(ctx, ctxKeyRequestID, id)
  }
  ```

## 7. Concurrency

- Every `go` statement has a documented exit condition. Comment the
  goroutine's lifetime if it isn't obvious from the surrounding code.
- Pair every goroutine with a stop signal: a closed channel, a cancelled
  context, or a `sync.WaitGroup` the caller waits on. Fire-and-forget is
  forbidden.
- `sync.Pool` is allowed only when paired with a benchmark proving the win
  on realistic inputs. Link the bench in the godoc comment.
- Prefer channels and `errgroup.Group` over hand-rolled mutex orchestration.

## 8. Naming

| Element | Rule | Example |
|---|---|---|
| File | `snake_case.go` | `route_matcher.go` |
| Test file | `xxx_test.go`, colocated | `route_matcher_test.go` |
| Package | lowercase, single word when possible | `codegen`, `manifest` |
| Exported symbol | PascalCase, no package-name repetition | `render.Writer`, **not** `render.RenderWriter` |
| Acronym | uppercase always | `HTTPClient`, `UserID`, `URLBuilder` |
| Unexported symbol | camelCase | `parseExpr`, `routeKey` |
| Receiver | short, consistent across methods | `func (r *Router) ...` |

Avoid stutter: `kit.Kit`, `router.Router`, `codegen.Codegen` are red flags.
Prefer `kit.App`, `router.Mux`, `codegen.Emitter`.

## 9. Documentation

- Every exported symbol carries a one-line godoc comment that **starts with
  the symbol name**:

  ```go
  // Writer streams SSR output for a single response.
  type Writer struct { /* ... */ }

  // Flush forces buffered bytes to the underlying ResponseWriter.
  func (w *Writer) Flush() error { /* ... */ }
  ```

- Multi-paragraph docs allowed when behavior is non-obvious. Keep the first
  sentence self-contained — it appears in `go doc` listings.
- Packages with more than one file ship a `doc.go` containing the package
  comment.
- Public API surfaces ship `example_test.go` files. Examples compile and
  run as part of `go test`, so they cannot rot silently.

## 10. Testing

- Table-driven by default:

  ```go
  func TestMatch(t *testing.T) {
      cases := []struct {
          name string
          in   string
          want bool
      }{
          {"root", "/", true},
          {"miss", "/missing", false},
      }
      for _, tc := range cases {
          t.Run(tc.name, func(t *testing.T) {
              got := router.Match(tc.in)
              if got != tc.want {
                  t.Fatalf("Match(%q) = %v, want %v", tc.in, got, tc.want)
              }
          })
      }
  }
  ```

- `t.Helper()` in every helper that calls `t.Errorf` / `t.Fatalf`.
- Register teardown with `t.Cleanup(...)`. `defer` in test bodies is
  reserved for scopes the helper cannot see.
- Golden files live under `testdata/golden/` and are regenerated with the
  `-update` flag (RFC #104). Review the diff line by line before committing
  an update.
- `time.Sleep` in tests is forbidden. Use channels, `context.WithTimeout`,
  or the `testing/synctest` package.
- Race detector required: `go test -race ./...` runs in CI (RFC #101).

### Testing patterns

- **Golden tests** for deterministic output (codegen, transformers, manifest
  serialization): see
  [documentation/docs/contributing/golden-tests.md](documentation/docs/contributing/golden-tests.md).
  Helper: `github.com/binsarjr/sveltego/packages/sveltego/internal/testutils/golden`. Update flow runs
  the suite with `-args -update`, then humans review the diff before commit.
- **Bench regression gate** runs on every PR touching Go. Threshold rules,
  override format (`bench-regression:` in PR body), and local repro commands
  live in
  [documentation/docs/contributing/bench-gate.md](documentation/docs/contributing/bench-gate.md).

## 11. Forbidden

- `init()` functions outside `package main` and well-justified plugin
  registries. Document the registry in a top-of-file comment.
- Global mutable state. Configuration travels through constructors.
- `panic()` outside recovered HTTP middleware boundaries and codegen `must`
  helpers (compile-time invariants only).
- `interface{}` / `any` in public API surfaces. Reach for generics first;
  if a generic doesn't fit, justify in godoc.
- `os.Exit` outside `cmd/`. Library code returns errors.
- `reflect` outside codegen and serialization boundaries. Document the
  reason inline.

## 12. Stability tiers

Per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Every
package ships a `STABILITY.md` describing the tier of each exported symbol.
The repo-wide index lives at [`STABILITY.md`](./STABILITY.md).

| Tier | Promise | Allowed change |
|---|---|---|
| `stable` | Won't break in the current major. | Additive only. Behavior changes go in CHANGELOG. |
| `experimental` | May break in any minor. Marked `// Experimental:` in godoc. | Anything. Deprecate before promotion. |
| `deprecated` | Will be removed. Marked `// Deprecated: <reason>, use X` in godoc. | Removed in next major. Lint warns on use. |
| `internal-only` | Not importable even if exported. Documented in `STABILITY.md`. | Anything. |

### Deprecation cycle

1. Add `// Deprecated: use <X>, removed in v<N+1>.0.0` to the godoc.
2. Move the entry from `## Stable` to `## Deprecated` in the package
   `STABILITY.md`.
3. `staticcheck SA1019` warns callers via `.golangci.yml` (RFC #98).
4. Keep deprecated for at least one minor cycle.
5. Remove in the next major. Add a CHANGELOG entry under
   `BREAKING CHANGES`.

The adapter-facing API in `packages/sveltego/exports/adapter` becomes
`stable` from `adapter-server v0.1`. Other adapters bump major when that
interface bumps major.

## 13. Commit conventions

Conventional Commits per [RFC #99](https://github.com/binsarjr/sveltego/issues/99):

```
<type>(<scope>): <subject>

<body>

<footer>
```

| Type | Use for |
|---|---|
| `feat` | New user-facing feature. |
| `fix` | Bug fix. |
| `docs` | Docs-only change. |
| `style` | Formatting, no behavior change. |
| `refactor` | Code change that neither fixes a bug nor adds a feature. |
| `perf` | Performance improvement. |
| `test` | Tests only. |
| `build` | Build system, dependencies. |
| `ci` | CI config. |
| `chore` | Tooling, repo housekeeping. |
| `revert` | Revert a prior commit. |
| `rfc` | RFC document or amendment. |

`<scope>` is the package name (`sveltego`, `adapter-cloudflare`, `codegen`,
`router`, ...) or `repo` for cross-cutting changes. For changes that touch
multiple packages, comma-separate the scopes: `feat(codegen,server): ...`.
Subject is imperative, no trailing period, ≤ 72 characters. Breaking
changes go in the footer: `BREAKING CHANGE: <description>`.

`release-please` (RFC #100) reads these commits to compute per-package
version bumps and CHANGELOG entries.

## 14. Local gate (run before push)

The pre-commit hook (RFC #99, `.githooks/pre-commit`) and CI (RFC #101) run
the same gate. Run it locally first:

```bash
gofumpt -l .                                              # no output = clean
goimports -l -local github.com/binsarjr/sveltego .        # no output = clean
golangci-lint run                                         # see .golangci.yml (RFC #98)
go vet ./...
go test -race ./...
go build ./...
```

A task is not done until the gate is green. Bytes hitting disk does not
mean the code compiles — re-read every edited file and re-run the gate.

When opening a PR, GitHub auto-loads
[`.github/PULL_REQUEST_TEMPLATE.md`](./.github/PULL_REQUEST_TEMPLATE.md)
(RFC #102). The template embeds the Definition of Done checklist; tick
each item as it lands. Reviewers reject PRs with unticked boxes that
lack an explanation.

## 15. AI agents

AI agents working in this repo follow [`AGENTS.md`](./AGENTS.md) (master
ruleset, RFC #103) plus per-package `CLAUDE.md` files for scope-specific
patterns. `.cursorrules` and `.github/copilot-instructions.md` are
auto-synced from `AGENTS.md`; do not edit them directly.

If a per-package `CLAUDE.md` disagrees with this file, the package file
wins for that package only. Cross-cutting rules live here.

## 16. Merging to main

PRs land via the **GitHub merge queue**. The queue batches entries, runs CI
on the merged candidate, and lands the commit only when all required checks
pass. This eliminates the cancel-in-progress churn that would otherwise kill
main CI runs every time a PR merged.

### To queue a PR

```bash
gh pr merge <num> --auto --squash --delete-branch
```

`--auto` queues the PR; the merge queue handles the rest. Do not use
`--admin` to bypass the queue — eat your own dogfood.

### Required checks (must all be green before the queue merges)

| Check name | Job |
|---|---|
| `lint-and-test (ubuntu-latest, go1.25.x)` | Lite lint + test + build matrix |
| `changes (path-aware fan-out)` | Path-filter fan-out |
| `commit-lint` | Conventional Commits validation |
| `agents-sync (AGENTS.md drift)` | AI doc sync drift check |

`isolated-modules` runs on `push` and `merge_group` for extra coverage but
is not a required gate (it would block PRs since it doesn't run on
`pull_request`).

### Concurrency rules

- **PR runs**: `cancel-in-progress: true` — stale branch runs are cancelled.
- **main push runs**: `cancel-in-progress: false` — post-merge runs always complete.
- **merge_group runs**: `cancel-in-progress: false` — queue runs always complete.

### Enabling merge queue in GitHub UI (one-time manual step)

The merge queue rule type is not yet available via the REST rulesets API for
this account. Enable it manually:

> Repo Settings → Branches → `main` protection rule → "Require merge queue"
> → Enable → Merge method: Squash → Save changes.
>
> This is required to activate `gh pr merge --auto` queue behavior.

## 17. References

Foundation RFCs on `github.com/binsarjr/sveltego`:

- [#95 — Monorepo workspace layout](https://github.com/binsarjr/sveltego/issues/95)
- [#96 — Code style conventions](https://github.com/binsarjr/sveltego/issues/96)
- [#97 — API stability and versioning](https://github.com/binsarjr/sveltego/issues/97)
- [#98 — golangci-lint config](https://github.com/binsarjr/sveltego/issues/98)
- [#99 — Pre-commit hooks + commit-msg](https://github.com/binsarjr/sveltego/issues/99)
- [#100 — release-please multi-package](https://github.com/binsarjr/sveltego/issues/100)
- [#101 — CI matrix](https://github.com/binsarjr/sveltego/issues/101)
- [#102 — PR template + Definition of Done](https://github.com/binsarjr/sveltego/issues/102)
- [#103 — AGENTS.md + AI doc sync](https://github.com/binsarjr/sveltego/issues/103)
- [#104 — Codegen golden testing](https://github.com/binsarjr/sveltego/issues/104)
- [#105 — Bench regression gate](https://github.com/binsarjr/sveltego/issues/105)

External:

- [Effective Go](https://go.dev/doc/effective_go)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Google Go Style](https://google.github.io/styleguide/go/)
- [`log/slog` design notes](https://go.dev/blog/slog)
- [semver.org](https://semver.org)
- [Go module compatibility](https://go.dev/ref/mod#major-version-suffixes)
- [staticcheck SA1019](https://staticcheck.io/docs/checks/#SA1019)
- [Go 1 compatibility promise](https://go.dev/doc/go1compat)
