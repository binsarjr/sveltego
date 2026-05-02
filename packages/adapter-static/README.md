# adapter-static

Static-site (SSG) deploy adapter. Drives sveltego's existing
`Server.Prerender` engine and packages the rendered HTML into a flat
deploy tree ready for any static host (S3, GitHub Pages, Cloudflare
Pages, Netlify static, your own nginx).

## Usage

Via the standalone driver:

```bash
sveltego-adapter build --target=static --root . --out ./dist
```

Or programmatically:

```go
err := adapterstatic.Build(ctx, adapterstatic.BuildContext{
    ProjectRoot:   "/abs/path/to/project",
    OutputDir:     "/abs/path/to/dist",
    MainPackage:   "./cmd/app",  // optional, defaults to "./cmd/app"
    FailOnDynamic: false,        // set true to fail when any non-prerender route exists
})
```

The adapter:

1. Compiles the user main with `go build` into a scratch dir.
2. Runs the binary with `SVELTEGO_PRERENDER=1` so the in-binary
   `MaybePrerenderFromEnv` hook drives `Server.Prerender` against
   that scratch dir.
3. Copies the rendered `<route>/index.html` files to `OutputDir`.
4. Mirrors the project's `static/` directory to `OutputDir/static/`
   (skipping the `_prerendered` runtime artifact).
5. Writes a deterministic `_prerender_manifest.json` at the root of
   `OutputDir` summarizing the tree.

Two consecutive `Build` calls with identical inputs produce a
byte-identical tree, so downstream `rsync` / `aws s3 sync` commands do
not thrash on no-op rebuilds.

## Output layout

```
dist/
  index/index.html        # "/" route
  about/index.html        # "/about" route
  blog/hello/index.html   # "/blog/[slug]" entry
  static/                 # mirrored from project's static/
    favicon.ico
    img/logo.png
  _prerender_manifest.json
```

## Limitations

- The default subprocess runner cannot enumerate **all** routes in the
  user binary, so `FailOnDynamic` only triggers when a custom Runner
  reports dynamic routes via `RunInfo.DynamicRoutes`. CLI users who
  need strict dynamic-route detection should consult the route
  manifest at `.gen/manifest.gen.go` until a richer Runner ships.
- Parameterized routes (`/post/[slug]`) need an `Entries()` supplier
  in the user `_page.server.go` (see #65). The adapter consumes
  whatever the prerender engine emits.

## Status

Pre-alpha. See [STABILITY.md](./STABILITY.md). Tracks #65 (prerender
engine) and #447 (adapter wiring).
