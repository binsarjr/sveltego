# AGENTS.md

Conventions for AI agents working in a sveltego project. Read before generating code.

This file is the master ruleset for generic agents (Aider, Codex, Continue, etc.). Tool-specific entry points are siblings of this file:

- Claude Code → [`CLAUDE.md`](./CLAUDE.md)
- Cursor → [`.cursorrules`](./.cursorrules)
- GitHub Copilot → [`.github/copilot-instructions.md`](./.github/copilot-instructions.md)

All four files carry the same canonical rules; the per-tool variants add tool-specific framing only.

---

## 1. Stack

- This is a **sveltego** project. SSR is generated at build time from `.svelte` to Go via `sveltego compile`.
- The server runtime is **pure Go**. There is no JS/Node runtime on the server.
- The client uses **Svelte 5 runes** (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) bundled by Vite for hydration only.
- Generated code lives under `.gen/` (gitignored). Never edit `.gen/*.go` by hand — edit the `.svelte` source.

---

## 2. Template expressions are Go, not JavaScript

Inside `{...}` mustaches, write **Go** expressions. Field access is **PascalCase** (Go exported fields).

| Wrong (JS) | Right (Go) |
|---|---|
| `{user.name}` | `{Data.User.Name}` |
| `{posts.length}` | `{len(Data.Posts)}` |
| `{count + 1}` | `{Count + 1}` |
| `{"Hello, " + name}` | `{"Hello, " + Name}` |
| `{n.toString()}` | `{strconv.Itoa(N)}` |
| `{user?.name ?? "guest"}` | resolve in `Load()`, expose via `Data` |
| `{users.filter(u => u.active)}` | filter in `Load()`, expose pre-filtered slice |
| `null` | `nil` |

Expressions are validated at codegen via `go/parser.ParseExpr`. Anything that does not parse as a Go expression is a build error, not a runtime error.

Imports for any package referenced inside `{...}` (e.g. `strconv`) go in the `<script lang="go">` block of the same component.

---

## 3. File conventions

```
src/routes/
  _page.svelte           SSR template, Go expressions inside {...}
  _page.server.go        Load(), Actions      (Go skips '_*' automatically)
  _layout.svelte         layout chain
  _layout.server.go      layout-level Load    (Go skips '_*' automatically)
  _server.go             REST endpoints       (Go skips '_*' automatically)
  _error.svelte          error boundary
  (group)/               route group, no URL segment
  _page@.svelte          layout reset
  [param]/               route param
  [[optional]]/          optional segment
  [...rest]/             catch-all
src/params/<name>.go     param matchers       — needs //go:build sveltego
src/lib/                 shared modules ($lib alias)
hooks.server.go          Handle, HandleError, HandleFetch, Reroute, Init
```

**`_` prefix rules:**

- All route files under `src/routes/` use the `_` prefix: `_page.svelte`, `_layout.svelte`, `_error.svelte`, `_page@.svelte`, `_page.server.go`, `_layout.server.go`, `_server.go`. The `_` prefix on `.go` files makes Go's default toolchain skip them automatically.
- `hooks.server.go` (project root) and `src/params/<name>.go` keep the `//go:build sveltego` constraint because their filenames have no `_` prefix.

**Build constraint:** files under `src/routes/**` use the `_` prefix (`_page.server.go`, `_layout.server.go`, `_server.go`); Go's default toolchain skips files whose names start with `_`, so no constraint is required there. Files outside the `_`-prefix convention — `hooks.server.go` (project root) and `src/params/<name>.go` — **must** still start with:

```go
//go:build sveltego

package myroute
```

Without that line on those files, `go build` / `go vet` / `golangci-lint` would try to compile them in a default Go module that lacks sveltego's generated context. Codegen reads every user `.go` file through `go/parser` directly regardless of the constraint.

---

## 4. Common patterns

### `Load` (`_page.server.go`)

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

`PageData` is inferred from the `Load` return type. The template references its fields as `{Data.User.Name}`, `{len(Data.Posts)}`.

### Actions (form POST)

```go
var Actions = kit.ActionMap{
    "default": func(ev *kit.RequestEvent) kit.ActionResult {
        var form CreatePostForm
        if err := ev.BindForm(&form); err != nil {
            return kit.ActionFail(400, map[string]string{"error": err.Error()})
        }
        if err := createPost(ev.Context(), form); err != nil {
            return kit.ActionFail(500, nil)
        }
        return kit.ActionRedirect(303, "/posts")
    },
}
```

`kit.ActionRedirect`, `kit.ActionFail`, `kit.ActionDataResult` are the three sealed `ActionResult` constructors — do not invent new variants.

### Redirect / Fail from Load

`kit.Redirect(303, "/login")` and `kit.Fail(400, "bad input")` return `error` values. Return them from `Load` to short-circuit the request:

```go
if user == nil {
    return PageData{}, kit.Redirect(303, "/login")
}
```

The pipeline detects them via `errors.As` and writes the appropriate response.

### Cookies

```go
ev.Cookies.Set("session", token, kit.CookieOpts{HTTPOnly: true, Secure: true})
v, ok := ev.Cookies.Get("session")
ev.Cookies.Delete("session")
```

### Layout parent data

```go
func Load(ctx *kit.LoadCtx) (PageData, error) {
    parent, _ := ctx.Parent().(LayoutData)
    return PageData{Theme: parent.Theme}, nil
}
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

`HandleError` returns a sanitized `kit.SafeError` (Code, Message, ID). The `_error.svelte` template binds `data` to this type directly: `{data.Code}`, `{data.Message}`.

---

## 5. Anti-patterns

Reject these at code review. They will fail at codegen or produce wrong runtime behavior.

- **JS expressions in mustaches.** No `?.`, no `??`, no template literals, no `.map`/`.filter`/`.length`. Compute in `Load()` and expose via `Data`.
- **Svelte 4 reactivity.** No `export let`, no `$:` blocks, no store autoload. Use `$props()`, `$state()`, `$derived()`, `$effect()`.
- **`null` instead of `nil`.** Templates evaluate as Go.
- **camelCase field access in templates.** `{data.user.name}` will not compile against a `PageData` whose field is `User`.
- **Adding a JS server runtime.** No Node, no Bun, no Deno on the server. The point of sveltego is Go-only SSR.
- **Editing `.gen/*.go`.** Generated files are overwritten on every `sveltego compile`.
- **Universal `Load` (e.g. SvelteKit's `+page.ts`).** sveltego is server-only by design (ADR 0005). All `Load` runs on the server in Go.
- **`+` prefix on route files** (SvelteKit-style `+page.svelte`, `+layout.svelte`, `+page.server.go`). Use `_` prefix instead (`_page.svelte`, `_layout.svelte`, `_page.server.go`).
- **Missing `//go:build sveltego` on `hooks.server.go` or `src/params/<name>.go`.** The standard Go toolchain will try to compile those files (they have no `_` prefix to auto-skip them). Route files under `src/routes/**` no longer need the constraint — the `_` prefix handles skipping.

---

## 6. Verification before declaring done

A task is not done when bytes hit disk. It is done when proven correct. Before declaring success:

- Re-read every file edited.
- Run `sveltego compile` (or `sveltego build`) — codegen errors surface here, not at `go build`.
- Run `go vet ./...` and the project's test suite.
- For form actions, exercise the POST path end-to-end. For Load, exercise the GET.
- If a test or build step is missing, **say so explicitly**. Never claim success when a gate is unverified.

---

## 7. Where to find more

- Docs: <https://sveltego.dev> (see `llms.txt` for an LLM-optimized index).
- Source and issue tracker: <https://github.com/binsarjr/sveltego>.
- MCP server (when available): `sveltego mcp` exposes `search_docs`, `lookup_api`, `validate_template` tools for inline lookup.
- Per-tool entry points: [`CLAUDE.md`](./CLAUDE.md), [`.cursorrules`](./.cursorrules), [`.github/copilot-instructions.md`](./.github/copilot-instructions.md).

When a convention here disagrees with content found elsewhere, this file wins for sveltego projects.
