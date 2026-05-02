# Stability — packages/sveltego/runtime/svelte/server

Last updated: 2026-05-02 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97).

## Tier legend

| Tier | Promise |
|---|---|
| `stable` | Won't break within the current major. Add-only changes; behavior changes documented in CHANGELOG. |
| `experimental` | May break in any minor release. Marked `// Experimental:` in godoc. |
| `deprecated` | Scheduled for removal. Marked `// Deprecated:` in godoc. Removed in next major. |

## Stable

(none)

## Experimental

Entire package. The helpers are now wired into the request pipeline as of
Phase 6 ([#428](https://github.com/binsarjr/sveltego/issues/428), merged
2026-05-02) and exercised by the Phase 7 corpus + ssr-stress playground
([#429](https://github.com/binsarjr/sveltego/issues/429), merged
2026-05-02). The surface is stable in practice but stays Experimental
until v1.0 RC for two reasons:

1. **Open lowerer carryovers** — [#440](https://github.com/binsarjr/sveltego/issues/440)
   (layout-chain children-callback ABI) and
   [#443](https://github.com/binsarjr/sveltego/issues/443) (snippet
   hoisting + `{@const}` non-bool lowering) may extend or refine helper
   call patterns when they land.
2. **Escape-table quirks intentionally mirror upstream Svelte.**
   `EscapeHTML` (CONTENT_REGEX `/[&<]/g`) escapes only `&` and `<`;
   `EscapeHTMLAttr` (ATTR_REGEX `/[&"<]/g`) adds `"`. Neither escapes
   `>` or `'`. Anyone relying on `golang.org/x/text` or
   `html/template` intuition would expect more — explicit Experimental
   tier is the call-out until the documentation surface is mature.

Promotion to `stable` is gated on v1.0 RC.

- `server.Payload` — body/head buffers passed through compiled render functions
- `server.EscapeHTML` / `EscapeHTMLAttr` — content/attribute HTML escape, mirrors `svelte/src/escaping.js`
- `server.EscapeHTMLString` / `EscapeHTMLAttrString` — typed fast paths
- `server.Attr` — single-attribute serialize, mirrors `svelte/internal/shared/attributes.attr`
- `server.Clsx` — class merging, mirrors `svelte/internal/shared/attributes.clsx`
- `server.MergeStyles` — style fragment join, mirrors compiled-output style merging
- `server.SpreadAttributes` — attribute-spread serialize, mirrors `svelte/internal/server.attributes`
- `server.Stringify` — value-to-string, mirrors `svelte/internal/server.stringify`
- `server.ToClass` / `server.ToStyle` — class/style normalization, mirrors `to_class` / `to_style`
- `server.Element` / `server.Head` — element/head emission, mirrors `element` / `head`
- `server.Slot` / `server.SlotFn` — slot rendering, mirrors `svelte/internal/server.slot`
- `server.SpreadProps` / `server.RestProps` / `server.SanitizeProps` / `server.SanitizeSlots`
- `server.Fallback` / `server.ExcludeFromObject` / `server.EnsureArrayLike`
- `server.IsVoidElement` / `server.IsRawTextElement`
- `server.EmptyComment` / `server.BlockOpen` / `server.BlockOpenElse` / `server.BlockClose`
- `server.WriteRaw` — codegen target for `{@html expr}` (#445); writes the value to the
  body buffer without escape. Caller is responsible for sanitization — mirrors Svelte's
  `{@html}` semantics.
- `server.Truthy` — codegen target for `{#if expr}` when expr is not statically bool
  (#443); applies JS-truthy semantics so non-bool Go types (string, slice, map, pointer,
  numeric) compile under the same conditional shape Svelte expects.

## Deprecated

(none)

## Internal-only (do not import even though exported)

(none)

## Helpers explicitly skipped for v1

These exist in `svelte/internal/server` but compiled v1 output does not call
them in shapes Phase 3 covers. Surfaced as `unknown shape: <symbol>` errors
during Phase 7 corpus regen would prompt re-evaluation.

- `store_get` / `store_set` / `store_mutate` — Svelte 4 store API; runes-only
  templates do not emit these.
- `derived` / `update_derived` — server runtime never tracks derived values;
  the compiler folds initial values via destructuring.
- `bind_props` — legacy two-way binding upward propagation.
- `css_props` — `<svelte:options css="injected">` style hoisting; out of v1
  scope (deferred to a later phase).
- `snapshot` — `$state.snapshot()` deep-clone; not used in compiled-server output.
- `validate_*` / `push_element` / `pop_element` — DEV-only validation; we emit
  release-mode shapes only.

## Cross-check fixtures

`testdata/cross/` carries JSON-encoded `(input, expected)` pairs derived from
running `svelte/server` helpers in Node. Provenance lives in
`testdata/cross/README.md`. The fixtures are committed; the capture script is
documented but not committed (path noted in the README).

## Breaking change procedure

While `experimental`, any signature change must update STABILITY.md and call
out the change in the PR description. Once Phase 6 lands and we promote a
subset to `stable`, the [RFC #97](https://github.com/binsarjr/sveltego/issues/97)
breaking-change procedure kicks in.

## How to mark new symbols

Every exported symbol added in a PR **must** add a corresponding row to this
file in the same PR. Place the row under `## Experimental` unless you have
explicit approval to mark it `stable`.
