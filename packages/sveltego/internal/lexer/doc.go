// Package lexer tokenizes Svelte 5 templates for the sveltego compiler.
//
// The lexer is mode-based and hand-rolled for tight control over position
// tracking and error recovery. Mustache content is captured opaquely up to
// the matching brace; expression validation is deferred to codegen via
// go/parser. Errors surface as TokenError tokens and the lexer continues
// to the next safe boundary so the parser can decide whether to abort.
package lexer
