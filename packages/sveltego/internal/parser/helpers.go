package parser

import (
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/internal/lexer"
)

func tokPos(t lexer.Token) ast.Pos {
	return ast.Pos{Offset: t.Offset, Line: t.Line, Col: t.Col}
}

// describe renders a token in a form suitable for error messages.
func describe(t lexer.Token) string {
	switch t.Kind {
	case lexer.TokenEOF:
		return "end of input"
	case lexer.TokenError:
		return "lexer error: " + t.Value
	case lexer.TokenText:
		return "text"
	case lexer.TokenTagOpen:
		return "`<`"
	case lexer.TokenTagOpenClose:
		return "`</`"
	case lexer.TokenTagSelfClose:
		return "`/>`"
	case lexer.TokenTagClose:
		return "`>`"
	case lexer.TokenIdentifier:
		return "identifier `" + t.Value + "`"
	case lexer.TokenAttrName:
		return "attribute `" + t.Value + "`"
	case lexer.TokenAttrEquals:
		return "`=`"
	case lexer.TokenAttrValue:
		return "attribute value"
	case lexer.TokenMustacheOpen:
		return "`{`"
	case lexer.TokenMustacheClose:
		return "`}`"
	case lexer.TokenBlockOpen, lexer.TokenBlockMid, lexer.TokenBlockClose, lexer.TokenAtTag:
		return "`{" + t.Value + "}`"
	case lexer.TokenComment:
		return "comment"
	case lexer.TokenScriptOpen:
		return "`<script>`"
	case lexer.TokenScriptBody:
		return "script body"
	case lexer.TokenScriptClose:
		return "`</script>`"
	case lexer.TokenStyleOpen:
		return "`<style>`"
	case lexer.TokenStyleBody:
		return "style body"
	case lexer.TokenStyleClose:
		return "`</style>`"
	default:
		return "token"
	}
}

// isComponentName reports whether name is a Svelte component (not an HTML
// element). Component if it starts uppercase or contains a dot, but not if
// it is a `svelte:*` namespaced built-in.
func isComponentName(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, "svelte:") {
		return false
	}
	if strings.Contains(name, ".") {
		return true
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// classifyAttribute returns the AttrKind and modifier (the part after the
// first `:`) for a Svelte attribute name.
func classifyAttribute(name string) (ast.AttrKind, string) {
	prefix, rest, ok := strings.Cut(name, ":")
	if !ok {
		return ast.AttrStatic, ""
	}
	switch prefix {
	case "on":
		return ast.AttrEventHandler, rest
	case "bind":
		return ast.AttrBind, rest
	case "use":
		return ast.AttrUse, rest
	case "class":
		return ast.AttrClassDirective, rest
	case "style":
		return ast.AttrStyleDirective, rest
	default:
		return ast.AttrStatic, ""
	}
}

// splitDirective separates the keyword from the rest of the body. Inputs
// look like "if cond", "each list as item", "else if x", "html expr".
func splitDirective(body string) (string, string) {
	body = strings.TrimLeft(body, " \t")
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == ' ' || c == '\t' {
			return body[:i], body[i:]
		}
	}
	return body, ""
}

func blockArgs(value, prefix string) string {
	rest := strings.TrimPrefix(value, prefix)
	return strings.TrimSpace(rest)
}

// splitEach parses `iter as item` or `iter as item, idx` or with `(key)`
// suffix. Key is captured as the raw expression inside the parentheses.
func splitEach(args string) (iter, item, idx, key string) {
	if i := strings.LastIndex(args, "("); i != -1 && strings.HasSuffix(strings.TrimSpace(args), ")") {
		head := strings.TrimSpace(args[:i])
		tail := strings.TrimSpace(args[i+1:])
		key = strings.TrimSpace(strings.TrimSuffix(tail, ")"))
		args = head
	}
	asIdx := indexWord(args, "as")
	if asIdx < 0 {
		return strings.TrimSpace(args), "", "", key
	}
	iter = strings.TrimSpace(args[:asIdx])
	rest := strings.TrimSpace(args[asIdx+2:])
	if c := strings.IndexByte(rest, ','); c >= 0 {
		item = strings.TrimSpace(rest[:c])
		idx = strings.TrimSpace(rest[c+1:])
		return iter, item, idx, key
	}
	return iter, rest, "", key
}

// splitSnippet parses `name(params)` into name and the inner params text.
func splitSnippet(args string) (name, params string) {
	args = strings.TrimSpace(args)
	open := strings.IndexByte(args, '(')
	if open < 0 {
		return args, ""
	}
	name = strings.TrimSpace(args[:open])
	rest := args[open+1:]
	closeIdx := strings.LastIndexByte(rest, ')')
	if closeIdx < 0 {
		return name, strings.TrimSpace(rest)
	}
	return name, strings.TrimSpace(rest[:closeIdx])
}

// indexWord returns the index of word in s when surrounded by whitespace
// (or string boundaries). Used to find ` as ` without matching identifiers
// that contain "as".
func indexWord(s, word string) int {
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], word)
		if j < 0 {
			return -1
		}
		idx := i + j
		left := idx == 0 || isSpaceByte(s[idx-1])
		right := idx+len(word) == len(s) || isSpaceByte(s[idx+len(word)])
		if left && right {
			return idx
		}
		i = idx + len(word)
	}
	return -1
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// blockBoundary returns a stop function that halts a body parse at the
// matching `{:...}` or `{/name}` token, leaving it unconsumed.
func blockBoundary(name string) stopFn {
	return func(tok lexer.Token) bool {
		switch tok.Kind {
		case lexer.TokenBlockMid:
			return true
		case lexer.TokenBlockClose:
			body := strings.TrimPrefix(tok.Value, "/")
			got, _ := splitDirective(body)
			return got == name
		}
		return false
	}
}

// elementStop halts at a closing tag; the closing-tag matcher then either
// confirms or reports a mismatch.
func elementStop(_ string) stopFn {
	return func(tok lexer.Token) bool {
		return tok.Kind == lexer.TokenTagOpenClose
	}
}

// extractScriptLang reads the `lang` attribute out of a raw `<script ...>`
// open tag value. Returns "" when lang is absent.
func extractScriptLang(openValue string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(openValue, "<script"), ">")
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	const key = "lang"
	idx := strings.Index(inner, key)
	if idx < 0 {
		return ""
	}
	rest := inner[idx+len(key):]
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "=") {
		return ""
	}
	rest = strings.TrimLeft(rest[1:], " \t")
	if rest == "" {
		return ""
	}
	switch rest[0] {
	case '"', '\'':
		quote := rest[0]
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			return ""
		}
		return rest[1 : 1+end]
	}
	end := strings.IndexAny(rest, " \t")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// isVoidElement reports whether name is an HTML void element with no
// closing tag in the source. Components and svelte:* namespaces are never
// void; they may still be self-closing in template syntax.
func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
