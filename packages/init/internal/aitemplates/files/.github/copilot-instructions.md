# Cursor rules for sveltego

This is a sveltego project. `.svelte` files are pure Svelte 5; `sveltego compile` reads sibling `_*.server.go` files and emits TypeScript declarations (`.svelte.d.ts`) so the Svelte LSP types `data` end-to-end. The server runtime is pure Go; the client uses Svelte 5 runes (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) via Vite for hydration only. Read this file before generating code.

For the master ruleset, see `AGENTS.md` in the project root. This file is the Cursor-specific shim and stays in sync with `AGENTS.md` and `CLAUDE.md`.

## Templates are pure Svelte / JS / TS

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

```svelte
<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.title}</h1>
```

## File conventions

```
src/routes/
  _page.svelte           SSR template, pure Svelte
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
src/params/<name>/<name>.go  param matchers   — auto-registered via gen.Matchers()
src/lib/                 shared modules ($lib alias)
src/hooks.server.go      Handle, HandleError, HandleFetch, Reroute, Init
```

`_` prefix rules:

- All route files use the `_` prefix: `_page.svelte`, `_layout.svelte`, `_error.svelte`, `_page.server.go`, `_layout.server.go`, `_server.go`.
- The `_` prefix on `.go` files makes Go's default toolchain (build/vet/lint) skip them automatically. Codegen reads them via `go/parser` directly.

`src/hooks.server.go` is the only file outside the `_`-prefix convention that must start with `//go:build sveltego` so the standard Go toolchain skips it. Param matchers in `src/params/<name>/<name>.go` (one matcher per subdirectory; package name equals `<name>`) do **not** need the constraint — codegen mirrors them into `.gen/paramssrc/<name>/` and `gen.Matchers()` registers them on the runtime automatically (#511).

## Common patterns

### Load

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type PageData struct {
    User  User   `json:"user"`
    Posts []Post `json:"posts"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
    return PageData{User: currentUser(ctx), Posts: fetchPosts(ctx)}, nil
}
```

`PageData`'s JSON tags drive the Go ↔ TypeScript boundary; reference fields as `{data.user.name}`, `{data.posts.length}`.

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

The three sealed `ActionResult` constructors are `kit.ActionDataResult`, `kit.ActionFail`, `kit.ActionRedirect`. Do not invent new variants. When a route declares `Actions`, add `Form any \`json:"form"\`` to its `PageData`.

### Redirect / Fail from Load

`kit.Redirect(code, location)` and `kit.Fail(code, data)` return `error` values. Return them from `Load` to short-circuit. The pipeline detects them via `errors.As`.

```go
if user == nil {
    return PageData{}, kit.Redirect(303, "/login")
}
```

### Prerender (SSG)

```go
const (
    Templates = "svelte"
    Prerender = true
)
```

`sveltego build` runs Node once at build time and writes `static/_prerendered/<path>/index.html`.

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

### Hooks (`src/hooks.server.go`)

```go
//go:build sveltego

package hooks

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) { ... }
func HandleError(ev *kit.RequestEvent, err error) (kit.SafeError, error) { ... }
```

`HandleError` returns a sanitized `kit.SafeError` (Code, Message, ID). `_error.svelte` binds `data` to this type directly: `{data.code}`, `{data.message}`.

## Don't

- Go expressions in mustaches (`{Data.User.Name}`, `{len(...)}`, `nil`). Pure Svelte/JS/TS only.
- PascalCase field access in templates. Use camelCase JSON-tag keys.
- Omit JSON tags on `PageData` fields — without them, the boundary breaks.
- Svelte 4 reactivity (`export let`, `$:` blocks, store autoload). Use Svelte 5 runes.
- A JS server runtime at request time. Node only at build time for SSG.
- Editing `.gen/*` directly.
- Universal `Load` (e.g. SvelteKit's `+page.ts`). sveltego is server-only.
- `+` prefix on any route file (e.g. SvelteKit-style `+page.svelte`, `+layout.svelte`, `+page.server.go`). Use `_` prefix instead.
- Omitting `//go:build sveltego` on `src/hooks.server.go`. Route files (`_` prefix auto-skips) and matcher files (`src/params/<name>/<name>.go`) don't need the constraint either (#511).

## Where to find more

- Docs: <https://sveltego.dev> (see `llms.txt` for an LLM-optimized index).
- Source and issue tracker: <https://github.com/binsarjr/sveltego>.
- MCP server (when available): `sveltego mcp` exposes `search_docs`, `lookup_api`, `validate_template`.
- See `AGENTS.md` for the master ruleset and `CLAUDE.md` for the Claude Code-specific entry point.
