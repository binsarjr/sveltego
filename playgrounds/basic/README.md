# basic

Hello-world playground exercising the MVP pipeline end-to-end: parser →
codegen → mirror tree → wire glue → manifest adapters → router → server.

## Status

**Smoke blocked by two framework gaps:**

- [#109](https://github.com/binsarjr/sveltego/issues/109) — runtime
  PageData type-assertion mismatch. Codegen produces the expected `.gen/`
  tree and `sveltego build` produces a binary, but every page route
  returns HTTP 500 with `PageData type mismatch` because the manifest
  adapter cannot type-assert an anonymous struct value against the gen
  package's named `PageData` type.
- [#110](https://github.com/binsarjr/sveltego/issues/110) — pre-commit
  hook rejects user `.go` files. The hook does `go test` per staged-file
  directory, which fails for `src/routes/[id]/` (Go rejects `[` in import
  paths) and `src/routes/` (all `.go` files carry `//go:build sveltego`,
  Go reports "build constraints exclude all Go files"). User `.go` files
  for this playground are deferred to Phase 0j-fix.

This commit lands the non-user-go portion of the playground (svelte
templates, app.html, cmd/app, CI workflow). The user `.go` files
(`page.server.go` under `src/routes/` and `[id]/`) land alongside the
fix that closes #109 + #110.

## Layout

```
playgrounds/basic/
├── go.mod                    # require + replace points at packages/sveltego
├── app.html                  # shell with %sveltego.head% / %sveltego.body%
├── src/routes/
│   ├── +layout.svelte        # inert in MVP — layout chain is v0.2 (#24)
│   ├── +page.svelte          # uses {data.Greeting} + {#each Posts}
│   ├── page.server.go        # //go:build sveltego; inline-struct-literal Load
│   └── post/
│       └── [id]/
│           ├── +page.svelte
│           └── page.server.go
└── cmd/app/main.go           # boots server with gen.Routes()
```

## Conventions

- User `.go` files under `src/routes/**` carry `//go:build sveltego` as
  the first line so the default Go toolchain skips them. The codegen
  pipeline reads them through `go/parser` directly and mirrors them into
  `.gen/usersrc/<encoded>/` with the constraint stripped and the package
  clause rewritten to the encoded directory name. See ADR 0003 amendment
  (Phase 0i-fix).
- `+page.svelte` files keep the `+` prefix; only user `.go` files dropped
  it in Phase 0i-fix.
- `Load()` returns an inline anonymous struct literal so PageData
  inference (ADR 0004) extracts its fields into `type PageData struct{...}`
  in the generated page package. Named-type returns are out of scope
  until a future RFC.
- The `+layout.svelte` is inert in the MVP. Layout chain rendering lands
  in v0.2 (#24).

## Run (once #109 ships)

```bash
cd playgrounds/basic
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app           # listens on :3000
curl http://localhost:3000/
curl http://localhost:3000/post/123
```

## References

- [#23](https://github.com/binsarjr/sveltego/issues/23) — original
  hello-world spec.
- [#109](https://github.com/binsarjr/sveltego/issues/109) — blocking
  PageData type-assertion bug.
- [ADR 0003](../../tasks/decisions/0003-file-convention.md) — file
  convention + Phase 0i-fix amendment.
- [ADR 0004](../../tasks/decisions/0004-codegen-shape.md) — codegen
  shape + PageData inference.
