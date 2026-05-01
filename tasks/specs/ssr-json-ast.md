# SSR JSON-AST schema (Phase 2 of SSR Option B)

- **Status:** Locked for v1 (re-evaluate when Svelte minor pin moves).
- **Owner:** `packages/sveltego/internal/codegen/svelterender/sidecar/`.
- **Consumers:** Phase 3 emitter (`internal/codegen/svelte_js2go/`, [#425](https://github.com/binsarjr/sveltego/issues/425)).
- **Related:** [ADR 0009](../decisions/0009-ssr-option-b.md), [issue #424](https://github.com/binsarjr/sveltego/issues/424).

## Purpose

The Node sidecar at `internal/codegen/svelterender/sidecar/` runs at
`sveltego build` time and, in `--mode=ssr`, emits one JSON file per
route describing the ESTree AST of Svelte's compiled server output.
The Go-side pattern-match emitter consumes those files and writes
`Render()` functions per route. No JS runtime ever touches the request
path.

This document fixes the schema so the producer (sidecar) and consumer
(emitter) move independently.

## Pipeline

```
.svelte source
  -> svelte/compiler.compile(source, { generate: 'server', filename, dev: false })
  -> result.js.code   (string of compiled server JS, ESM)
  -> acorn.parse(result.js.code, { sourceType: 'module', ecmaVersion: 'latest' })
  -> ESTree Program node
  -> sorted-key JSON serialization
  -> .gen/svelte_js2go/<route-slug>/ast.json
```

Acorn is pinned to `8.16.0`; Svelte to `5.55.5`. Both pins live in
`packages/sveltego/internal/codegen/svelterender/sidecar/package.json`.
Sub-decision 3 of ADR 0009 governs minor bumps.

## Output path

```
<outDir>/<route-slug>/ast.json
```

`outDir` defaults to `<projectRoot>/.gen/svelte_js2go`. `route-slug` is
derived from the canonical request path:

- `/` → `_root`
- `/about` → `about`
- `/blog/post` → `blog__post`
- `/[slug]` → `[slug]`

The slug uses `__` as a separator to keep one-segment-per-directory
filesystems happy and to leave bracketed parameters intact for
Phase 5/6 lookup.

## File envelope

```jsonc
{
  "schema": "ssr-json-ast/v1",
  "route":  "/hello-world",
  "ast":    { /* ESTree Program node — see below */ }
}
```

- `schema` — fixed string `ssr-json-ast/v1`. Bump on any
  consumer-visible change (envelope shape, sort policy, helper
  identifiers).
- `route` — copied from the input job; lets a single-file consumer
  identify its source without reparsing the path.
- `ast` — Acorn output, untransformed. Keys serialized in sorted order
  (see "Determinism rules").

## ESTree node types the consumer must handle

Svelte 5's `generate: 'server'` lowering produces a flat string-builder
program. The Phase-3 emitter only needs to dispatch on a small set of
ESTree node types in the `ast.body` array (and recursively under
function bodies). The list below is the closed set targeted by v1; new
shapes surface as `unknown shape: <name>` build failures (Sub-decision
2 of ADR 0009).

### Top-level

| ESTree type                  | Origin                                       |
|------------------------------|----------------------------------------------|
| `Program`                    | Root.                                        |
| `ImportDeclaration`          | `import * as $ from 'svelte/internal/server'`. Only one expected per file in v1; record the local namespace identifier. |
| `ExportDefaultDeclaration`   | The render function. Body is a `FunctionDeclaration` or `ArrowFunctionExpression`. |

### Inside the render function

| ESTree type                  | Origin                                       |
|------------------------------|----------------------------------------------|
| `FunctionDeclaration` / `ArrowFunctionExpression` | The exported render function and helpers it nests for control flow. |
| `BlockStatement`             | Function bodies and `if` / `each` arms.      |
| `VariableDeclaration` (`let`/`const`) | `let { data } = $$props`, scratch counters. |
| `ObjectPattern` / `Property` / `Identifier` | Destructuring of `$$props`.                |
| `ExpressionStatement`        | Wraps every emit call.                        |
| `AssignmentExpression` with `+=` | `$$payload.out += '<h1>'`. The dominant emit shape. |
| `MemberExpression`           | Property access: `data.name`, `$$payload.out`, namespaced helpers (`$.escape_html`). |
| `Identifier`                 | Local references (the namespace import, function names). |
| `Literal` (string / number / boolean) | Static template chunks, attribute names, numeric counters. |
| `TemplateLiteral` / `TemplateElement` | Multi-segment HTML strings with interpolated runtime calls. |
| `CallExpression`             | Helper calls: `$.escape_html(x)`, `$.attr(...)`, `$.clsx(...)`, `$.stringify(...)`, `$.spread_attributes(...)`, `$.merge_styles(...)`, `$.head(...)`, `$.html(...)`, `$.each(...)`, control-flow helpers. |
| `IfStatement` / `ConditionalExpression` | Lowered `{#if}` and ternaries in expressions. |
| `ReturnStatement`            | Inside helper closures (mostly slot fragments). |
| `LogicalExpression` / `BinaryExpression` | Coercion guards (`x ?? ''`), index math.    |
| `UnaryExpression`            | `!cond`, `+n`. Rare but produced by Svelte for negation guards. |
| `SpreadElement`              | `{...attrs}` lowered into `$.spread_attributes`. |

### Helper identifiers (closed set, v1)

The sidecar does not transform these. The Phase-3 emitter recognises
them by namespace + member name. The Phase-4 helpers package
([#426](https://github.com/binsarjr/sveltego/issues/426)) implements
each as a Go function with the same semantics.

```
$.escape_html        $.attr               $.clsx
$.stringify          $.spread_attributes  $.merge_styles
$.head               $.html               $.each
$.if                 $.element            $.bind_props
```

A helper not in this list inside a `CallExpression.callee` of shape
`$.<name>` is treated by the Phase-3 emitter as `unknown shape:
helper:<name>` and fails the build.

## Determinism rules

The sidecar guarantees **byte-identical** output for byte-identical
inputs across runs and hosts. Three guard rails:

1. **Sorted keys.** Every object key is emitted in lexicographic
   order. Acorn does not promise key order across versions; pinning a
   minor would not be sufficient on its own.
2. **No timestamps, no absolute paths.** The envelope contains only
   `schema`, `route`, `ast`. The `filename` passed to
   `svelte/compiler` is the manifest-relative source path, never an
   absolute one — so byte-positions inside the AST do not encode the
   developer's home directory.
3. **No `dev: true`.** Svelte's dev mode injects source-map metadata
   and per-build random IDs. The sidecar always compiles with
   `dev: false`.

The Go test suite asserts this by running the sidecar twice into
distinct output directories and `bytes.Equal`-comparing the result
(`TestBuildSSRAST_Determinism`).

## Stability contract

- Schema field changes require bumping `schema` to
  `ssr-json-ast/v2` (or higher) and updating the Phase-3 emitter in
  the same release.
- Helper identifier additions are non-breaking — the emitter must
  treat unknown identifiers as a build error, not a panic, so the
  next sidecar release can ship new helpers in lockstep with new
  emitter dispatch entries.
- Acorn / Svelte minor pins move only via a dedicated PR (Sub-decision
  3 of ADR 0009). The corpus regenerates against the new pin; new
  emit shapes either get a Phase-3 pattern entry or the route opts
  into the sidecar fallback (Phase 8, [#430](https://github.com/binsarjr/sveltego/issues/430)).

## Reference

- Goldens: `packages/sveltego/internal/codegen/svelterender/testdata/ssr-ast/<fixture>/ast.golden.json`
- Sidecar entry: `packages/sveltego/internal/codegen/svelterender/sidecar/index.mjs`
- Go invoker: `BuildSSRAST` in `packages/sveltego/internal/codegen/svelterender/svelterender.go`
- Spec for the Phase-3 emitter pattern matrix: tracked in [#425](https://github.com/binsarjr/sveltego/issues/425).
