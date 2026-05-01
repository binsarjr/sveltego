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
  +page.svelte           SSR template, Go expressions inside {...}
  page.server.go         Load(), Actions      — needs //go:build sveltego
  +layout.svelte         layout chain
  layout.server.go       layout-level Load    — needs //go:build sveltego
  server.go              REST endpoints       — needs //go:build sveltego
  +error.svelte          error boundary
  (group)/               route group, no URL segment
  +page@.svelte          layout reset
  [param]/               route param
  [[optional]]/          optional segment
  [...rest]/             catch-all
src/params/<name>.go     param matchers       — needs //go:build sveltego
src/lib/                 shared modules ($lib alias)
hooks.server.go          Handle, HandleError, HandleFetch, Reroute, Init
```

**`+` prefix rules (do not invert):**

- `.svelte` files keep the `+` prefix: `+page.svelte`, `+layout.svelte`, `+error.svelte`, `+page@.svelte`.
- User `.go` files **drop** the `+` prefix: `page.server.go`, `layout.server.go`, `server.go`, `hooks.server.go`. SvelteKit-style `+page.server.go` is rejected by the scanner.

**Build constraint:** every user `.go` file under `src/` (and `hooks.server.go` at project root) **must** start with:

```go
//go:build sveltego

package myroute
```

Without that line, `go build` / `go vet` / `golangci-lint` would try to compile the file in a default Go module that lacks sveltego's generated context. The constraint makes the standard toolchain skip these files; codegen reads them through `go/parser` directly.

---

## 4. Common patterns

### `Load`

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

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

### REST endpoints (`server.go`)

```go
//go:build sveltego

package api

func GET(ev *kit.RequestEvent) (*kit.Response, error) { ... }
func POST(ev *kit.RequestEvent) (*kit.Response, error) { ... }
```

One verb per Go function; the dispatcher routes by HTTP method.

### Hooks (`hooks.server.go`)

```go
//go:build sveltego

package hooks

import "github.com/binsarjr/sveltego/exports/kit"

var Handle kit.HandleFn = func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) { ... }
var HandleError kit.HandleErrorFn = func(ev *kit.RequestEvent, err error) kit.SafeError { ... }
```

`HandleError` returns a sanitized `kit.SafeError` (Code, Message, ID). The `+error.svelte` template binds `data` to this type directly: `{data.Code}`, `{data.Message}`.

---

## 5. Anti-patterns

Reject these at code review. They will fail at codegen or produce wrong runtime behavior.

- **JS expressions in mustaches.** No `?.`, no `??`, no template literals, no `.map`/`.filter`/`.length`. Compute in `Load()` and expose via `Data`.
- **Svelte 4 reactivity.** No `export let`, no `$:` blocks, no store autoload. Use `$props()`, `$state()`, `$derived()`, `$effect()`.
- **`null` instead of `nil`.** Templates evaluate as Go.
- **camelCase field access in templates.** `{data.user.name}` will not compile against a `PageData` whose field is `User`.
- **Adding a JS server runtime.** No Node, no Bun, no Deno on the server. The point of sveltego is Go-only SSR.
- **Editing `.gen/*.go`.** Generated files are overwritten on every `sveltego compile`.
- **Universal `Load` (`+page.ts`).** sveltego is server-only by design (ADR 0005). All `Load` runs on the server in Go.
- **`+page.server.go` filename.** Drop the `+` prefix on user `.go` files.
- **Missing `//go:build sveltego`.** Codegen will warn; the standard Go toolchain will then try to compile a file that references undeclared symbols.

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
