// Package parser turns a Svelte 5 template token stream into a typed AST.
//
// The parser is hand-rolled recursive descent over tokens emitted by
// internal/lexer. It collects every problem it finds into ast.Errors and
// keeps going from the next safe boundary (an opening `<` or `{`) so a
// single run reports as many distinct mistakes as possible. Mustache
// expression text is captured opaquely; Go-syntax validation is deferred
// to codegen.
package parser
