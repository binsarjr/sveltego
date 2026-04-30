# ADR 0004 — Codegen Output Shape

- **Status:** Accepted
- **Date:** 2026-04-29
- **Issue:** [binsarjr/sveltego#4](https://github.com/binsarjr/sveltego/issues/4)

## Decision

**Selected:** Option B — method on `Page` struct.

## Rationale

- Combines naturally with ADR 0003 (mirror source structure): each route directory is a Go package, each package has `type Page struct{}` plus methods. The package path itself is the namespace.
- Free functions (option A) flatten to identifier soup once snippets, slots, and component nesting enter the picture. Methods organize them on the receiver.
- Lifecycle (option C) is overkill for SSR — server only needs `Render`. Mount/Destroy belong in the client bundle, not generated Go.

## Locked sub-decisions

- **Component nesting (Q7):** **file-based default + import override.** Codegen walks `src/lib/` and `src/components/` (configurable in `sveltego.toml`) and produces a generated component per file. User can also explicitly import via `<script lang="go">import comp "myapp/src/lib/MyComponent"</script>` — the import wins over file-based discovery. Match SvelteKit DX where common case is automatic and explicit override is one line.
- **`<script lang="go">` content (Q8):** **hoist to package-level.** User imports become package imports of the generated file. User funcs become package-level funcs in the same package as `Page`. Visibility default unexported; user can export with PascalCase. Mirrors Svelte upstream where `<script>` is module scope.
- **Snippet method naming (Q9):** **lowercase + `snippet_` prefix on Page receiver.** `{#snippet item(post)}...{/snippet}` becomes `func (p Page) snippet_item(w *render.Writer, ctx *kit.RenderCtx, post Post) error`. Lowercase signals internal-only (Go visibility); prefix avoids future clash with Page methods that the framework may add.
- **`{@html ...}`:** emits `w.WriteRaw(...)` instead of `w.WriteEscape(...)`. CONTRIBUTING.md documents the XSS risk and recommends `bluemonday` for user-supplied HTML.
- **PageData inference (struct-literal-only):** Codegen reads sibling `+page.server.go`, locates `Load()`, and if the first return expression is a composite struct literal extracts its fields into `type PageData struct{...}`. Named-type returns and missing files fall back to `type PageData struct{}`. Explicit `type PageData struct{...}` declarations in `+page.server.go` are out of scope until a future RFC. Tracked in `internal/codegen/pagedata.go`.

## Generated shape

```go
package _slug_

import (
    "myapp/packages/sveltego/render"
    "myapp/packages/sveltego/exports/kit"
    "myapp/.gen/lib"
    // user's <script lang="go"> imports hoisted here
)

// user's <script lang="go"> helper funcs hoisted here

type Page struct{}

type PageData struct {
    Post Post  // inferred from +page.server.go Load() return type
}

func (p Page) Render(w *render.Writer, ctx *kit.RenderCtx, data PageData) error {
    w.WriteString("<h1>")
    w.WriteEscape(data.Post.Title)
    w.WriteString("</h1>")
    if len(data.Post.Comments) > 0 {
        for _, c := range data.Post.Comments {
            p.snippet_item(w, ctx, c)
        }
    }
    return nil
}

func (p Page) snippet_item(w *render.Writer, ctx *kit.RenderCtx, c Comment) error {
    w.WriteString("<li>")
    w.WriteEscape(c.Author)
    w.WriteString(": ")
    w.WriteEscape(c.Text)
    w.WriteString("</li>")
    return nil
}
```

## Render writer interface

```go
package render

type Writer struct { /* unexported buffer; sync.Pool friendly */ }

func New() *Writer
func Acquire() *Writer
func Release(w *Writer)

func (w *Writer) WriteString(s string)         // raw, pre-trusted (literals from template)
func (w *Writer) WriteRaw(s string)            // {@html ...} explicit unsafe
func (w *Writer) WriteEscape(v any)            // text-context HTML escape
func (w *Writer) WriteEscapeAttr(v any)        // attribute-context HTML escape
func (w *Writer) WriteJSON(v any) error        // hydration payload
func (w *Writer) Bytes() []byte
func (w *Writer) Len() int
func (w *Writer) Reset()
```

`WriteAttr(name, val string)` was reserved in the original draft but rejected during Phase 0f: attribute serialization is composed inline in codegen as

```go
w.WriteString(` name="`)
w.WriteEscapeAttr(val)
w.WriteString(`"`)
```

The composition pattern keeps the runtime surface narrow and lets codegen handle quoting context (boolean attrs, class directives, style directives) without needing a per-shape helper.

## `kit.RenderCtx`

```go
type RenderCtx struct {
    Locals  map[string]any
    URL     *url.URL
    Params  map[string]string
    Cookies *Cookies
    Request *http.Request
    Writer  http.ResponseWriter  // exposed for streaming + headers
}
```

## Load and Actions stay user-written

`Page.Load()` is **not** a method on the generated `Page` struct. Reason: signature varies per route, generics don't fit cleanly, and it lets users keep server logic in `+page.server.go` separate from generated code.

User writes:

```go
// src/routes/posts/[slug]/+page.server.go
package posts_slug

func Load(ctx *kit.LoadCtx) (PageData, error) {
    p, err := db.GetPost(ctx.Params["slug"])
    if err != nil { return PageData{}, kit.Error(404, "not found") }
    return PageData{Post: p}, nil
}
```

Codegen produces `.gen/routes/posts/_slug_/wire.gen.go`:

```go
package _slug_

import server "myapp/src/routes/posts/_slug_"

var _ = server.Load  // compile-time check signature matches expected shape

func init() {
    Routes.Register("/posts/:slug", Page{}, server.Load, server.Actions)
}
```

User code stays in user space, generated wire glue is replaced atomically on rebuild.

## Implementation outline

1. Codegen walks parsed AST, emits a single `.gen/.../page.gen.go` per `+page.svelte`.
2. Render method body is a sequence of `WriteString` / `WriteEscape` calls interleaved with control flow lowered from `{#if}`, `{#each}`, etc. `{@html}` lowers to `WriteRaw`.
3. `wire.gen.go` companion file holds the `init()` registration and the Load/Actions glue.
4. Adapter codegen produces `manifest.gen.go` aggregating all routes.

## References

- SvelteKit page server: https://svelte.dev/docs/kit/page-options
- `text/template` Render shape (precedent): https://pkg.go.dev/text/template

### Amendments

- **2026-04-29 (Phase 0f wrap):** Render writer surface locked to as-shipped methods. `WriteAttr` rejected. PageData inference rule pinned to struct-literal-only.
- **2026-04-30 (Phase 0i-fix):** User `.go` filename convention amended (see ADR 0003 amendment): `+page.server.go` → `page.server.go`, `+layout.server.go` → `layout.server.go`, `+server.go` → `server.go`. All sveltego user `.go` files start with `//go:build sveltego`. The above examples in this ADR predate the rename; treat them as historical. Manifest now emits per-route `render__<alias>` adapters wrapping `Page{}.Render(data PageData)` to satisfy `router.PageHandler(data any)`. Wire `wire.gen.go` re-exports user Load via a user-source mirror tree at `.gen/usersrc/<encoded>/` (codegen-emitted, never imports the user `src/` tree directly because directory names like `[slug]/` are not valid Go import paths).
