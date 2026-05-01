// Package sveltejs2go transpiles the ESTree JSON AST emitted by the
// Phase 2 sidecar (svelterender, ADR 0009) into Go render functions
// that mirror Svelte's compiled server output.
//
// The transpiler consumes one ast.json envelope per route and writes a
// Render(payload, props) Go function that performs the same string
// concatenation Svelte's generate:'server' would have at runtime — but
// without any JS engine on the request path.
//
// Pattern matching is intentionally narrow: the closed set of ESTree
// shapes documented in tasks/specs/ssr-json-ast.md is what v1
// recognises. Any unknown shape becomes a hard build error
// (`unknown emit shape at <pos>: <snippet>`) rather than a silent
// fallback. ADR 0009 sub-decision 2 codifies that policy.
//
// Property-access lowering (data.name -> data.Name via JSON tags) is
// out of scope here; Phase 5 (#427) layers it on top of the expression
// walker via the rewriter hook exposed in emitter.go.
package sveltejs2go
