# ADR 0002 — Template Expression Syntax

- **Status:** Accepted
- **Date:** 2026-04-29
- **Issue:** [binsarjr/sveltego#2](https://github.com/binsarjr/sveltego/issues/2)

## Decision

**Selected:** Option A — full Go expression accepted by `go/parser.ParseExpr`, with two post-parse rejections.

## Rationale

- `ParseExpr` already rejects statement-only forms (`go fn()`, `defer fn()`, `select`, `for`, `if`, `return`, assignments). What it accepts maps cleanly to "things you can put on the right-hand side of `=` in Go" — exactly the shape expected inside a mustache.
- Validation comes for free: `go/parser` is part of the Go toolchain and produces the same AST the codegen emits into.
- User mental model: "anything you'd write between `=` and `;` in Go is legal here, minus the side-effecting bits."

## Locked sub-decisions

- **Channel ops (Q2):** **rejected** at codegen via post-parse AST validator. Reject `*ast.UnaryExpr{Op: token.ARROW}` (receive) and chan send. Error: `channel ops not allowed in template expressions; do channel work in Load() and pass values via Data`.
- **Built-ins (Q3):** allow read-only and append-style only — `len`, `cap`, `append`, `copy`. **Reject** `delete`, `make`, `new`. Validator checks `*ast.CallExpr` whose `Fun` is one of the rejected built-in identifiers.
- **Type assertions:** allowed (`v.(T)`). Common need for unwrapping `interface{}` from generic stores.
- **Function literals (`FuncLit`):** allowed but discouraged. Future lint warns when complexity exceeds N statements (post-MVP).
- **Composite literals:** allowed. Inline struct/map/slice constants in templates are useful (`{User{Name: "x"}}` for component prop defaults).

## What `ParseExpr` accepts (kept, for reference)

`BasicLit`, `Ident`, `SelectorExpr`, `CallExpr`, `IndexExpr`, `SliceExpr`, `BinaryExpr`, `UnaryExpr`, `TypeAssertExpr`, `CompositeLit`, `FuncLit`, `StarExpr`, `ParenExpr`, `KeyValueExpr`, `ChanType`. Validator rejects `UnaryExpr{Op:ARROW}`, `BinaryExpr{Op:ARROW}`, and `CallExpr{Fun:make|new|delete}`.

## Implementation outline

1. Codegen receives mustache source string.
2. `go/parser.ParseExpr` produces `ast.Expr`.
3. Validator walks the expression, applies the rejection rules above.
4. Validator outputs the original expression unchanged into emitted Go (with type-aware escape wrapper for HTML output).

## References

- `go/parser.ParseExpr`: https://pkg.go.dev/go/parser#ParseExpr
- Go AST node types: https://pkg.go.dev/go/ast
