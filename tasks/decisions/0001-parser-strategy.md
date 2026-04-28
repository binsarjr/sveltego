# ADR 0001 — Parser Strategy

- **Status:** Accepted
- **Date:** 2026-04-29
- **Issue:** [binsarjr/sveltego#1](https://github.com/binsarjr/sveltego/issues/1)

## Decision

**Selected:** Option A — hand-rolled recursive descent parser.

## Rationale

- Svelte 5 templates are context-sensitive (mustache nesting inside attribute values, `<script lang="go">` boundary, `<style>` boundary, comment passthrough). Hand-rolled state machine handles mode switching cleanly.
- Performance budget — codegen runs at build time but must process thousands of templates fast. Hand-rolled emitter avoids allocator pressure from generic combinator output.
- Error messages with file path, line, column, and "expected: X" — control over the parse loop is the only way to ship good DX.
- Reference precedent: `text/template/parse` and `html/template/parse` in the Go stdlib are hand-rolled. Same shape, same constraints.

## Locked sub-decisions

- **Error recovery (Q1):** Phase 1 ships panic+recover — abort on first error, return single `*ParseError`. Phase 2 (post-MVP) upgrades to synchronization points returning `[]*ParseError`. Filed as a follow-up task once base parser is stable.
- **Lexer modes:** `ModeText`, `ModeMustache`, `ModeAttribute`, `ModeScript`, `ModeStyle`, `ModeComment`. Mode pushed/popped on opener/closer tokens.
- **Position tracking:** every AST node carries `Pos token.Pos`. Used by codegen for error attribution and by future LSP for hover/jump.
- **Lexer alphabet:** target ~20 tokens. Final list locked when lexer lands (issue #7).

## Implementation outline

1. `lexer` package emits a token stream from `[]byte` source, mode-aware.
2. `parser` package consumes the stream, produces `ast.File` rooted at `ast.Element` and `ast.ScriptBlock`.
3. Error type: `*ParseError{File, Line, Col, Msg, Expected, Got}`. Stringer for one-line and multi-line forms.

## References

- `text/template/parse`: https://pkg.go.dev/text/template/parse
- `html/template`: https://github.com/golang/go/tree/master/src/html/template
- Crafting Interpreters (recursive descent reference): https://craftinginterpreters.com/parsing-expressions.html
