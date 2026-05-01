# svelterender/sidecar — Node-side build companion

The Node sidecar is the single piece of the framework that runs JS at
build time (and, for opted-in routes, at request time). It exists
because Svelte's compiler is the source of truth for both the client
bundle and the server output; running it once at build time is
cheaper and safer than re-implementing what it already does.

For the project-level invariants this package keeps: read
[ADR 0008](../../../../../../tasks/decisions/0008-pure-svelte-pivot.md)
and [ADR 0009](../../../../../../tasks/decisions/0009-ssr-option-b.md)
first. The "no JS runtime on the server **at runtime**" line ([ADR
0005](../../../../../../tasks/decisions/0005-non-goals.md)) is honoured
by the sidecar by construction — the Go binary never spawns Node
unless a route opted in via `<!-- sveltego:ssr-fallback -->`.

## Three modes

The entry point `index.mjs` dispatches on `--mode`:

### `--mode=ssg`

Prerender pure-Svelte routes to static HTML at build time. Routes
opting into `kit.PageOptions{Prerender: true}` go through this path;
the output drops into `static/` and the Go binary serves it as a
plain file. ADR 0008 defines the contract.

### `--mode=ssr`

For each route, compile its `.svelte` via
`svelte/compiler.compile(source, { generate: 'server', dev: false })`,
then `acorn.parse` the resulting JS and write the ESTree JSON AST to
`.gen/svelte_js2go/<route-slug>/ast.json`. The Go-side
`internal/codegen/svelte_js2go/` package consumes those JSON files and
emits per-route `Render()` Go functions. ADR 0009 + RFC #421 define
the contract; the JSON envelope is locked in
[`tasks/specs/ssr-json-ast.md`](../../../../../../tasks/specs/ssr-json-ast.md).

This mode is build-time-only. It never runs at request time.

### `--mode=ssr-serve`

Long-running HTTP server that renders Svelte routes on demand.
**Only used by `runtime/svelte/fallback/`** for routes that opted out
of the build-time transpiler via the
`<!-- sveltego:ssr-fallback -->` HTML comment. Phase 8 of the SSR
Option B track ([#430](https://github.com/binsarjr/sveltego/issues/430))
introduced this mode.

The Go-side supervisor in `runtime/svelte/fallback/sidecar.go` boots
this mode **only** when at least one route is annotated; otherwise
the Node process never starts. The supervisor restarts the sidecar
up to 3× with exponential backoff (1s → 30s) before giving up.

## Listen-port protocol (`ssr-serve`)

The Go side cannot dial a fixed port (collisions, multi-tenant test
hosts), so the sidecar binds an ephemeral port and announces it on
stderr:

```
SVELTEGO_SSR_FALLBACK_LISTEN port=NNN
```

The supervisor (`fallback.Start`) reads stderr line-by-line, parses
`port=NNN` after the prefix, and dials `127.0.0.1:NNN`. The handshake
times out after 30 seconds (`sidecarReadyTimeout` in the Go side); a
sidecar that doesn't print the line in time is treated as broken.

Why HTTP on localhost rather than Unix sockets:
- Browsable / debuggable with curl during dev.
- No socket-path length limits (macOS sun_path is 104 chars; nested
  worktree paths blow that quickly).
- Parity with the build-time `--mode=ssr` transport, which is also
  HTTP-shaped via stdin/stdout JSON envelopes.

## Pinned versions

`package.json` pins:
- `acorn` to `8.16.0`
- `svelte` to `5.55.5`

ADR 0009 sub-decision 3 (lock-one-minor, support N-1, no chase-HEAD)
governs bumps. Bumping is a deliberate maintainer action: bump the
pin in a dedicated PR, re-run the corpus, surface new emit shapes as
`unknown shape: …` build errors, add Phase-3 pattern entries, land
the bump.

## Determinism rules

- AST output is byte-identical for byte-identical inputs across runs
  and hosts: sorted keys, no timestamps, no absolute paths, never
  `dev: true`. `TestBuildSSRAST_Determinism` enforces this.
- The ssr-serve mode caches by `(route, hash(load_result))`; the Go
  client supplies the hash, the sidecar is stateless beyond the
  cache.

## Side fixes worth knowing

When `ssr_serve.mjs` writes the response, it must JSON-encode the
payload **before** calling `res.writeHead`, and the catch-block
`writeHead` is gated on `!res.headersSent`. Phase 8 introduced this
guard to fix `ERR_HTTP_HEADERS_SENT` on the render-error path. Don't
regress the order.

## Files

```
index.mjs       — argv parser + mode dispatch
ssg.mjs         — --mode=ssg implementation
ssr.mjs         — --mode=ssr implementation (Acorn JSON AST emit)
ssr_serve.mjs   — --mode=ssr-serve implementation (long-running HTTP)
package.json    — pinned acorn + svelte
```

## Cross-references

- [ADR 0009](../../../../../../tasks/decisions/0009-ssr-option-b.md) —
  decision codifying build-time JS-to-Go transpile.
- [`tasks/specs/ssr-json-ast.md`](../../../../../../tasks/specs/ssr-json-ast.md) —
  JSON envelope schema and reality deltas.
- `runtime/svelte/fallback/STABILITY.md` — Go-side fallback runtime
  consuming `--mode=ssr-serve`.
- `internal/codegen/svelte_js2go/CLAUDE.md` — Go-side consumer of
  `--mode=ssr` output.
