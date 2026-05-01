# CLAUDE.md

Conventions for Claude Code when working in this sveltego project. Read before generating code.

This is the Claude Code-specific entry point. The same canonical rules live in [`AGENTS.md`](./AGENTS.md); cross-tool variants are [`.cursorrules`](./.cursorrules) and [`.github/copilot-instructions.md`](./.github/copilot-instructions.md). When in doubt, [`AGENTS.md`](./AGENTS.md) wins.

---

## Stack

- **sveltego** project. `.svelte` files are pure Svelte 5; `sveltego compile` reads sibling `_*.server.go` files and emits TypeScript declarations (`.svelte.d.ts`) so editors and the Svelte LSP get strong types for `data`.
- **Server runtime is pure Go.** No JS/Node on the server at request time. Node runs **only at build time** to prerender SSG routes through `svelte/server`.
- **Client uses Svelte 5 runes** (`$props`, `$state`, `$derived`, `$effect`, `$bindable`) bundled by Vite for hydration only.
- Generated code under `.gen/` is gitignored. Never edit `.gen/*` — edit the `.svelte` source or its `_*.server.go` sibling.

---

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

---

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
src/params/<name>.go     param matchers       — needs //go:build sveltego
src/lib/                 shared modules ($lib alias)
src/hooks.server.go      Handle, HandleError, HandleFetch, Reroute, Init
```

`_` prefix rules:

- All route files use the `_` prefix: `_page.svelte`, `_layout.svelte`, `_error.svelte`, `_page.server.go`, `_layout.server.go`, `_server.go`.
- The `_` prefix on `.go` files makes Go's default toolchain (build/vet/lint) skip them automatically. Codegen reads them via `go/parser` directly.

User `.go` files under `src/routes/**` are auto-skipped by Go via the `_` prefix; no build constraint required there. `src/hooks.server.go` and `src/params/<name>.go` keep the `//go:build sveltego` constraint because their filenames have no `_` prefix.

---

## Patterns

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
    return PageData{
        User:  currentUser(ctx),
        Posts: fetchPosts(ctx),
    }, nil
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

The three sealed `ActionResult` constructors are `kit.ActionDataResult`, `kit.ActionFail`, `kit.ActionRedirect`. Do not invent new variants.

When a route declares `Actions`, add `Form any \`json:"form"\`` to its `PageData` so the action result is reachable in the template (e.g. `{#if data.form}` ...).

### Redirect / Fail from Load

`kit.Redirect(code, location)` and `kit.Fail(code, data)` return `error` values. Return them from `Load` to short-circuit:

```go
if user == nil {
    return PageData{}, kit.Redirect(303, "/login")
}
```

The pipeline detects them via `errors.As`.

### Prerender (SSG)

For build-time-static routes:

```go
const (
    Templates = "svelte"
    Prerender = true
)
```

`sveltego build` invokes Node once to render the component to `static/_prerendered/<path>/index.html`. The deployed Go binary serves it as a static file.

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

---

## Don't

- Go expressions in mustaches (`{Data.User.Name}`, `{len(...)}`, `nil`). Pure Svelte/JS/TS only.
- PascalCase field access in templates. Use camelCase JSON-tag keys.
- Omit JSON tags on `PageData` fields — without them, the boundary breaks.
- Svelte 4 reactivity (`export let`, `$:` blocks, store autoload). Use Svelte 5 runes.
- A JS server runtime at request time. Node only at build time for SSG.
- Editing `.gen/*` directly.
- Universal `Load` (e.g. SvelteKit's `+page.ts`). sveltego is server-only.
- `+` prefix on any route file (e.g. SvelteKit-style `+page.svelte`, `+layout.svelte`, `+page.server.go`). Use `_` prefix instead.
- Omitting `//go:build sveltego` on `src/hooks.server.go` or `src/params/<name>.go` (route files no longer need it).

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
