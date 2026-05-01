# CLAUDE.md

Conventions for Claude Code when working in this sveltego project. Read before generating code.

This is the Claude Code-specific entry point. The same canonical rules live in [`AGENTS.md`](./AGENTS.md); cross-tool variants are [`.cursorrules`](./.cursorrules) and [`.github/copilot-instructions.md`](./.github/copilot-instructions.md). When in doubt, [`AGENTS.md`](./AGENTS.md) wins.

---

## Stack

- **sveltego** project. SSR generated at build time from `.svelte` to Go via `sveltego compile`.
- **Server runtime is pure Go.** No JS/Node on the server.
- **Client uses Svelte 5 runes** (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) bundled by Vite for hydration only.
- Generated code under `.gen/` is gitignored. Never edit `.gen/*.go` — edit the `.svelte` source.

---

## Template expressions are Go, not JavaScript

Inside `{...}` mustaches, write **Go**. Field access is **PascalCase**.

| Wrong (JS) | Right (Go) |
|---|---|
| `{user.name}` | `{Data.User.Name}` |
| `{posts.length}` | `{len(Data.Posts)}` |
| `{count + 1}` | `{Count + 1}` |
| `{n.toString()}` | `{strconv.Itoa(N)}` |
| `{user?.name ?? "guest"}` | resolve in `Load()`, expose via `Data` |
| `{users.filter(u => u.active)}` | filter in `Load()`, expose pre-filtered slice |
| `null` | `nil` |

Expressions are validated at codegen via `go/parser.ParseExpr`. Anything that does not parse as a Go expression is a build error.

Imports for any package referenced inside `{...}` (e.g. `strconv`) go in the `<script lang="go">` block of the same component.

---

## File conventions

```
src/routes/
  _page.svelte           SSR template, Go expressions inside {...}
  _page.server.go        Load(), Actions      (Go skips '_*' automatically)
  _layout.svelte         layout chain
  _layout.server.go      layout-level Load    (Go skips '_*' automatically)
  _server.go             REST endpoints       (Go skips '_*' automatically)
  _error.svelte          error boundary
  (group)/               route group
  _page@.svelte          layout reset
  [param]/               route param
  [[optional]]/          optional segment
  [...rest]/             catch-all
src/params/<name>.go     param matchers       — needs //go:build sveltego
src/lib/                 shared modules ($lib alias)
hooks.server.go          Handle, HandleError, HandleFetch, Reroute, Init
```

`_` prefix rules:

- All route files use the `_` prefix: `_page.svelte`, `_layout.svelte`, `_error.svelte`, `_page.server.go`, `_layout.server.go`, `_server.go`.
- The `_` prefix on `.go` files makes Go's default toolchain (build/vet/lint) skip them automatically. Codegen reads them via `go/parser` directly.

User `.go` files under `src/routes/**` are auto-skipped by Go via the `_` prefix; no build constraint required there. `hooks.server.go` (project root) and `src/params/<name>.go` keep the `//go:build sveltego` constraint because their filenames have no `_` prefix.

---

## Patterns

### Load

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (PageData, error) {
    return PageData{
        User:  currentUser(ctx),
        Posts: fetchPosts(ctx),
    }, nil
}

type PageData struct {
    User  User
    Posts []Post
}
```

`PageData` is inferred from the `Load` return type; reference fields as `{Data.User.Name}`, `{len(Data.Posts)}`.

### Actions

```go
var Actions = kit.ActionMap{
    "default": func(ev *kit.RequestEvent) kit.ActionResult {
        var form CreatePostForm
        if err := ev.BindForm(&form); err != nil {
            return kit.ActionFail(400, map[string]string{"error": err.Error()})
        }
        return kit.ActionRedirect(303, "/posts")
    },
}
```

The three sealed `ActionResult` constructors are `kit.ActionDataResult`, `kit.ActionFail`, `kit.ActionRedirect`. Do not invent new variants.

### Redirect / Fail from Load

`kit.Redirect(code, location)` and `kit.Fail(code, data)` return `error` values. Return them from `Load` to short-circuit:

```go
if user == nil {
    return PageData{}, kit.Redirect(303, "/login")
}
```

The pipeline detects them via `errors.As`.

### Cookies

```go
ev.Cookies.Set("session", token, kit.CookieOpts{HTTPOnly: true, Secure: true})
v, ok := ev.Cookies.Get("session")
ev.Cookies.Delete("session")
```

### Layout parent data

```go
parent, _ := ctx.Parent().(LayoutData)
```

`LoadCtx.Parent()` returns `any`. Type-assert to the immediate parent's data type.

### REST endpoints (`_server.go`)

```go
package api

func GET(ev *kit.RequestEvent) (*kit.Response, error) { ... }
func POST(ev *kit.RequestEvent) (*kit.Response, error) { ... }
```

One verb per Go function; the dispatcher routes by HTTP method.

### Hooks (`hooks.server.go`)

```go
//go:build sveltego

package hooks

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

var Handle kit.HandleFn = func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) { ... }
var HandleError kit.HandleErrorFn = func(ev *kit.RequestEvent, err error) kit.SafeError { ... }
```

`HandleError` returns a sanitized `kit.SafeError` (Code, Message, ID). `_error.svelte` binds `data` to this type directly: `{data.Code}`, `{data.Message}`.

---

## Don't

- JS expressions in mustaches (`?.`, `??`, template literals, `.map`/`.filter`/`.length`). Compute in `Load()` and expose via `Data`.
- Svelte 4 reactivity (`export let`, `$:` blocks, store autoload). Use Svelte 5 runes.
- `null` — write `nil`.
- camelCase field access in templates.
- A JS server runtime. No Node / Bun / Deno on the server.
- Editing `.gen/*.go` directly.
- Universal `Load` (e.g. SvelteKit's `+page.ts`). sveltego is server-only.
- `+` prefix on any route file (e.g. SvelteKit-style `+page.svelte`, `+layout.svelte`, `+page.server.go`). Use `_` prefix instead.
- Omitting `//go:build sveltego` on `hooks.server.go` or `src/params/<name>.go` (route files no longer need it).

---

## Working rules

These match the project-level Claude Code working rules. Adhere to them when generating code.

1. **Plan before build.** For any task with 3+ steps or architectural impact, write the plan first, get approval, then execute. "Plan" and "build" are separate steps.
2. **Verification before done.** A task is not done when bytes hit disk. Re-read every file edited. Run `sveltego compile`, `go vet ./...`, and the test suite. If a gate is missing tooling, **say so explicitly**. Never claim success when a gate is unverified.
3. **Edit safety.** Re-read files before editing after long conversations — context decay corrupts memory. Re-read after every edit to confirm the change applied. The Edit tool reports success on byte-write, not correctness.
4. **No over-engineering.** No imaginary scenarios. No fallbacks for cases that cannot happen. Validate at boundaries only (HTTP input, external APIs, file I/O). Three similar lines beats a premature abstraction.
5. **Comments.** Default: write none. Code with named identifiers explains itself. Add a comment only when the WHY is non-obvious. Godoc on exported symbols: one sentence starting with the symbol name.
6. **Destructive action safety.** Never `git reset --hard`, `git push --force`, `rm -rf` without explicit user authorization in the same conversation.

---

## Where to find more

- Docs: <https://sveltego.dev> (see `llms.txt` for the LLM-optimized index).
- Source and issue tracker: <https://github.com/binsarjr/sveltego>.
- MCP server (when available): `sveltego mcp` exposes `search_docs`, `lookup_api`, `validate_template`.
- See [`AGENTS.md`](./AGENTS.md) for the master ruleset; this file is the Claude-specific entry point with extra working-rule guidance.
