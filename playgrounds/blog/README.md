# blog

Non-trivial playground that exercises the SvelteKit shape via sveltego:
markdown-backed posts on disk, paginated index, post detail with a
comment form, and an in-memory comment store. Demonstrates the breadth
of the framework in one familiar app.

## Status

End-to-end smoke is **partial**:

- `/` (index, paginated) renders correctly via `sveltego build` plus the
  `cmd/app` binary.
- `/<slug>` (post detail) returns HTTP 500 because the route declares
  both `Load()` and `var Actions`. The codegen-emitted `PageData`
  always carries an injected `Form any` field while the user's Load
  return cannot include `Form` without a duplicate-declaration compile
  error. See follow-up
  [#143](https://github.com/binsarjr/sveltego/issues/143).

The Action source ships as spec'd so the gap is reproducible and the
fix can be validated against this app.

## Features demonstrated

| File | Framework features |
|---|---|
| `src/routes/_layout.svelte` | Layout chain (#24): root layout wraps every page in shared chrome (header / footer / nav). |
| `src/routes/_page.svelte` | Pure Svelte template (`{data.posts.length}`, conditional `{#if}`/`{:else}`, `{#each}`). |
| `src/routes/_page.server.go` | `kit.LoadCtx` + `ctx.URL.Query()` for `?page=N` pagination, anonymous-struct PageData inference (ADR 0004), `kit.Error` short-circuit. |
| `src/routes/[slug]/_page.svelte` | Dynamic `[slug]` param, `{@html}` for sanitized markdown, conditional rendering of comments and form-error state. |
| `src/routes/[slug]/_page.server.go` | `kit.LoadCtx.Params` for `[slug]`, `goldmark` + `bluemonday` for safe markdown, `kit.ActionMap` with default action, `ev.BindForm` for form binding, `kit.ActionFail` / `kit.ActionRedirect`, `kit.Cookies.Set` for the last-author cookie. |
| `content/posts/*.md` | Frontmatter-driven content; index reads metadata, detail renders the body. |
| `cmd/app/main.go` | `server.Config{Routes, Matchers, Shell, Hooks}` boot — same shape every sveltego app uses. |

That covers more than five framework features end-to-end:

1. Layout chain (#24) wrapping pages.
2. Pagination via `kit.LoadCtx.URL.Query()`.
3. Dynamic param routing (`[slug]`).
4. Form `Actions` map with `BindForm` + `ActionFail` + `ActionRedirect`.
5. Cookie write through `kit.Cookies.Set`.
6. Markdown rendering with `{@html}` and a sanitizer.
7. PageData inference from inline anonymous struct returns.

## Layout

```
playgrounds/blog/
├── go.mod                      # require + replace points at packages/sveltego
├── app.html                    # shell with %sveltego.head% / %sveltego.body%
├── src/routes/
│   ├── _layout.svelte          # root chrome
│   ├── _page.svelte            # paginated post list
│   ├── _page.server.go         # `_` prefix — list Load
│   └── [slug]/
│       ├── _page.svelte        # post detail + comment form
│       └── _page.server.go     # `_` prefix — detail Load + comment Action
├── content/posts/*.md          # markdown sources with `---` frontmatter
└── cmd/app/main.go             # boots server.New on :8080
```

## Conventions

- User `.go` files under `src/routes/**` use the `_` prefix (`_page.server.go`,
  `_layout.server.go`, `_server.go`) so the default Go toolchain skips them;
  the codegen pipeline reads them through `go/parser` and mirrors them into
  `.gen/usersrc/<encoded>/`. See [ADR 0003 amendment](../../tasks/decisions/0003-file-convention.md)
  and RFC #379 phase 1b.
- `Load()` returns an inline anonymous struct literal so PageData
  inference (ADR 0004) extracts its fields verbatim. Named-type returns
  are out of scope until a future RFC.
- The `[slug]` directory mirrors to `_slug_` in the gen tree because Go
  rejects `[` in package paths.

## Run

```bash
cd playgrounds/blog
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app                                    # listens on :8080
curl -fsS http://localhost:8080/ | head -40    # paginated index, 200 OK
curl -fsS "http://localhost:8080/?page=2"      # second page, 200 OK
# Post detail returns 500 today; tracked by #143:
curl -sS -o /dev/null -w "%{http_code}\n" http://localhost:8080/welcome-to-sveltego
```

The index route exercises layout + Load + pagination + each + if. Once
[#143](https://github.com/binsarjr/sveltego/issues/143) lands, the post
detail and comment Action become reachable through the same binary.

## References

- [#62](https://github.com/binsarjr/sveltego/issues/62) — original blog
  example spec.
- [#143](https://github.com/binsarjr/sveltego/issues/143) — open gap
  blocking the post-detail smoke.
- [ADR 0003](../../tasks/decisions/0003-file-convention.md) — file
  convention + Phase 0i-fix amendment.
- [ADR 0004](../../tasks/decisions/0004-codegen-shape.md) — codegen
  shape + PageData inference.
