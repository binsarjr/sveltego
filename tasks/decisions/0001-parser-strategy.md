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

- **Error recovery (Q1):** ~~Phase 1 ships panic+recover — abort on first error, return single `*ParseError`. Phase 2 (post-MVP) upgrades to synchronization points returning `[]*ParseError`.~~ **Superseded 2026-04-29 (Phase 0e).** Public API is `func Parse(src []byte) (*ast.Fragment, ast.Errors)` from day one. Internal control flow uses panic+recover for unwinding inside recursive descent; recovery sync points (next `<` or `{` opener) catch the bailout, append a `ParseError` to the slice, and continue. Same control-flow primitive as the original sub-decision, but the contract surfaces all errors instead of the first one. Rationale: issue #8 acceptance already required ≥2 errors per multi-mistake input, so the deferred upgrade was already due in MVP. Doing it once costs the same as doing it twice.
- **Lexer modes:** `ModeText`, `ModeMustache`, `ModeAttribute`, `ModeScript`, `ModeStyle`, `ModeComment`. Mode pushed/popped on opener/closer tokens.
- **Position tracking:** every AST node carries `Pos token.Pos`. Used by codegen for error attribution and by future LSP for hover/jump.
- **Lexer alphabet:** target ~20 tokens. Final list locked when lexer lands (issue #7).

## Implementation outline

1. `lexer` package emits a token stream from `[]byte` source, mode-aware.
2. `parser` package consumes the stream, produces `ast.File` rooted at `ast.Element` and `ast.ScriptBlock`.
3. Error type: `ast.ParseError{Pos, Message, Hint}` collected into `ast.Errors` (named `[]ParseError`). Stringer for one-line; `Errors.Error()` aggregates one-per-line.

## References

- `text/template/parse`: https://pkg.go.dev/text/template/parse
- `html/template`: https://github.com/golang/go/tree/master/src/html/template
- Crafting Interpreters (recursive descent reference): https://craftinginterpreters.com/parsing-expressions.html
