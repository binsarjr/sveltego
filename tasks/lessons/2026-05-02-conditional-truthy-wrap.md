# 2026-05-02 — formatConditional wrap with Truthy is required for non-bool tests

## Insight

The `formatConditional` function in `internal/codegen/svelte_js2go/expr.go`
emitted JS ternaries as Go `func() any { if <test> { return <a> }; return <b> }()`
using `formatExpression(test)` directly. That works when the test is a bool
(e.g. `data.active bool`) but fails to compile when the test is a pointer
or any non-bool typed expression — Go's `if` requires a bool.

Issue #466 (`page.error ? page.error.message : 'null'`) hit this: `page.error`
lowers to `pageState.Error` (`*server.PageError`), which is not bool-typed.
Generated Go failed compilation with `non-boolean condition in if statement`.

Truthy-wrapping was already implemented for `{#if}` blocks via
`formatTruthy` (introduced in #443) but `ConditionalExpression`
(ternary) was never plumbed through it. The fix is a one-liner: route
the test expression through `formatTruthy` instead of `formatExpression`.

This shifts three pre-existing priority/extended goldens
(`ternary-in-template`, `ternary-nested`, `escape-of-conditional`) by
wrapping their bare-bool member access in `server.Truthy(...)`. The
wrapping is semantically a no-op for already-bool values but the byte
shift counts as a regression on the byte-identical golden contract.
Accepted as a correctness fix because without it #466 cannot compile.

## Self-rules

1. When emitting any Go `if <test>` from a JS source, route the test
   through `formatTruthy` not `formatExpression`. Anywhere except
   already-known-bool sites (e.g. compiler-emitted comparison shapes
   inside hand-rolled `for` headers).

2. New JS ternary or short-circuit shapes that lower to Go `if` blocks
   must be tested with at least one non-bool typed test (e.g. nullable
   pointer or struct-typed map). The 3 priority goldens covered
   bool-typed cases only and masked the gap for ~2 months.

3. When extending byte-identical golden contracts, an emerging
   correctness fix that shifts the existing bytes is acceptable IF
   (a) the new bytes still pass the existing semantics, (b) the bytes
   *fix* a previously latent bug, (c) the change is logged here so the
   golden shift isn't surprising in PR review.

4. For interface-wrapped typed nil values (`*T(nil)` widened to `any`),
   a plain `v == nil` check returns false. Truthy must use
   `reflect.Value.IsNil()` for Ptr/Map/Slice/Chan/Func/Interface kinds.
   Already extended in `runtime/svelte/server/raw.go:Truthy`.
