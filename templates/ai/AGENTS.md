# AGENTS.md

Conventions for AI agents working in a sveltego project. Read before generating code.

This file is the master ruleset for generic agents (Aider, Codex, Continue, etc.). Tool-specific entry points are siblings of this file:

- Claude Code → [`CLAUDE.md`](./CLAUDE.md)
- Cursor → [`.cursorrules`](./.cursorrules)
- GitHub Copilot → [`.github/copilot-instructions.md`](./.github/copilot-instructions.md)

All four files carry the same canonical rules; the per-tool variants add tool-specific framing only.

---

## 1. Stack

- This is a **sveltego** project. `.svelte` files are pure Svelte 5; `sveltego compile` reads sibling `_*.server.go` files and emits TypeScript declarations (`.svelte.d.ts`) so editors and the Svelte LSP get strong types for `data`, plus the Go runtime that hosts SSG output and SPA payloads.
- The server runtime is **pure Go**. There is no JS/Node runtime on the server at request time. Node may run **at build time only** to prerender SSG routes through `svelte/server`.
- The client uses **Svelte 5 runes** (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) bundled by Vite for hydration only.
- Generated code lives under `.gen/` (gitignored). Never edit `.gen/*` by hand — edit the `.svelte` source or its `_*.server.go` sibling.

---

## 2. Templates are pure Svelte / JS / TS

Inside `.svelte` files, write **Svelte/JS/TS only**. Field access uses **camelCase** keys derived from JSON tags on the Go-side `PageData` struct.

| Wrong (Go in mustaches) | Right (pure Svelte) |
|---|---|
| `{Data.User.Name}` | `{data.user.name}` |
| `{len(Data.Posts)}` | `{data.posts.length}` |
| `{Count + 1}` | `{count + 1}` |
| `{strconv.Itoa(N)}` | `{n.toString()}` |
| `nil` | `null` |
| `{#if Data.User != nil}` | `{#if data.user != null}` |

Server-side data shaping (filtering, formatting, computing derived fields) belongs in `Load()`. Templates only render.

Imports inside `<script lang="ts">` are normal TypeScript. Reach for `PageData` from the sibling `.svelte.d.ts` codegen emits:

```svelte
<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.title}</h1>
```

---

## 3. File conventions

```
src/routes/
  _page.svelte           SSR template, pure Svelte
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
src/hooks.server.go      Handle, HandleError, HandleFetch, Reroute, Init
```

**`_` prefix rules:**

- All route files under `src/routes/` use the `_` prefix: `_page.svelte`, `_layout.svelte`, `_error.svelte`, `_page@.svelte`, `_page.server.go`, `_layout.server.go`, `_server.go`. The `_` prefix on `.go` files makes Go's default toolchain skip them automatically.
- `src/hooks.server.go` and `src/params/<name>.go` keep the `//go:build sveltego` constraint because their filenames have no `_` prefix.

Without that constraint on the non-`_` files, `go build` / `go vet` / `golangci-lint` would try to compile them in a default Go module that lacks sveltego's generated context. Codegen reads every user `.go` file through `go/parser` directly regardless of the constraint.

---

## 4. Common patterns

### `Load` (`_page.server.go`)

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type Post struct {
    ID    string `json:"id"`
    Title string `json:"title"`
}

type PageData struct {
    User  User   `json:"user"`
    Posts []Post `json:"posts"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
    return PageData{
        User:  currentUser(ctx),
        Posts: fetchPosts(ctx),
    }, nil
}
```

The struct's JSON tags drive the Go ↔ TypeScript boundary. The template references its fields as `{data.user.name}`, `{data.posts.length}`. Codegen generates a sibling `.svelte.d.ts` declaration so the Svelte LSP type-checks `data` end-to-end.

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

When a route declares `Actions`, codegen widens the page payload with a `form` field. Add `Form any \`json:"form"\`` to your `PageData` struct so the template can render the action result via `{data.form?.error}` or similar.

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

### Prerender (SSG)

For routes that are content-stable at build time, opt into static prerender:

```go
const (
    Templates = "svelte"
    Prerender = true
)
```

`sveltego build` runs Node once at build time and writes `static/_prerendered/<path>/index.html`. The deployed binary serves it as a static file — no Node, no Go template work per request.

### REST endpoints (`_server.go`)

```go
package api

func GET(ev *kit.RequestEvent) (*kit.Response, error) { ... }
func POST(ev *kit.RequestEvent) (*kit.Response, error) { ... }
```

One verb per Go function; the dispatcher routes by HTTP method.

### Hooks (`src/hooks.server.go`)

```go
//go:build sveltego

package hooks

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) { ... }
func HandleError(ev *kit.RequestEvent, err error) (kit.SafeError, error) { ... }
```

`HandleError` returns a sanitized `kit.SafeError` (Code, Message, ID). The `_error.svelte` template binds `data` to this type directly: `{data.code}`, `{data.message}`.

---

## 5. Anti-patterns

Reject these at code review. They will fail at codegen or produce wrong runtime behavior.

- **Go expressions in mustaches.** No `{Data.User.Name}`, no `{len(...)}`, no `nil`. Pure Svelte/JS/TS only — `{data.user.name}`, `{data.posts.length}`, `null`. The legacy `Templates: "go-mustache"` mode is removed.
- **PascalCase field access in templates.** Templates read JSON-tagged fields, so use camelCase: `{data.user.name}`, not `{data.User.Name}`.
- **Missing JSON tags on `PageData` fields.** Without explicit `\`json:"..."\`` tags, Go marshals fields with their PascalCase Go names and the template `{data.foo}` won't match.
- **Svelte 4 reactivity.** No `export let`, no `$:` blocks, no store autoload. Use `$props()`, `$state()`, `$derived()`, `$effect()`.
- **Adding a JS server runtime at request time.** No Node, no Bun, no Deno on the request path. Node may run at build time for SSG only.
- **Editing `.gen/*` files.** Generated files are overwritten on every `sveltego compile`.
- **Universal `Load` (e.g. SvelteKit's `+page.ts`).** sveltego is server-only by design (ADR 0005). All `Load` runs on the server in Go.
- **`+` prefix on route files** (SvelteKit-style `+page.svelte`, `+layout.svelte`, `+page.server.go`). Use `_` prefix instead (`_page.svelte`, `_layout.svelte`, `_page.server.go`).
- **Missing `//go:build sveltego` on `src/hooks.server.go` or `src/params/<name>.go`.** The standard Go toolchain will try to compile those files (they have no `_` prefix to auto-skip them). Route files under `src/routes/**` no longer need the constraint — the `_` prefix handles skipping.

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
