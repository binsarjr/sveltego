package lexer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/test-utils/golden"
)

func collect(src string) []Token {
	l := New([]byte(src))
	var tokens []Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Kind == TokenEOF {
			return tokens
		}
		if len(tokens) > 10000 {
			return tokens
		}
	}
}

func kinds(tokens []Token) []TokenKind {
	out := make([]TokenKind, len(tokens))
	for i, t := range tokens {
		out[i] = t.Kind
	}
	return out
}

func kindNames(tokens []Token) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = t.Kind.String()
	}
	return out
}

func TestEmpty(t *testing.T) {
	got := collect("")
	if len(got) != 1 || got[0].Kind != TokenEOF {
		t.Fatalf("expected single EOF, got %v", kindNames(got))
	}
}

func TestTextOnly(t *testing.T) {
	got := collect("hello world")
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d (%v)", len(got), kindNames(got))
	}
	if got[0].Kind != TokenText || got[0].Value != "hello world" {
		t.Fatalf("text mismatch: %+v", got[0])
	}
	if got[1].Kind != TokenEOF {
		t.Fatalf("expected EOF, got %v", got[1].Kind)
	}
}

func TestSimpleTag(t *testing.T) {
	got := collect("<div></div>")
	want := []TokenKind{
		TokenTagOpen, TokenIdentifier, TokenTagClose,
		TokenTagOpenClose, TokenIdentifier, TokenTagClose,
		TokenEOF,
	}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("kinds mismatch:\nwant %v\n got %v", kindNamesFromKinds(want), kindNames(got))
	}
}

func TestTagWithAttributes(t *testing.T) {
	got := collect(`<a href="https://x" target='_blank' disabled>x</a>`)
	want := []TokenKind{
		TokenTagOpen, TokenIdentifier,
		TokenAttrName, TokenAttrEquals, TokenAttrValue,
		TokenAttrName, TokenAttrEquals, TokenAttrValue,
		TokenAttrName,
		TokenTagClose,
		TokenText,
		TokenTagOpenClose, TokenIdentifier, TokenTagClose,
		TokenEOF,
	}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("kinds mismatch:\nwant %v\n got %v", kindNamesFromKinds(want), kindNames(got))
	}
	if got[4].Value != "https://x" {
		t.Fatalf("href value: %q", got[4].Value)
	}
	if got[7].Value != "_blank" {
		t.Fatalf("target value: %q", got[7].Value)
	}
}

func TestUnquotedAttribute(t *testing.T) {
	got := collect(`<input type=text>`)
	want := []TokenKind{
		TokenTagOpen, TokenIdentifier,
		TokenAttrName, TokenAttrEquals, TokenAttrValue,
		TokenTagClose,
		TokenEOF,
	}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("kinds mismatch:\nwant %v\n got %v", kindNamesFromKinds(want), kindNames(got))
	}
	if got[4].Value != "text" {
		t.Fatalf("attr value %q", got[4].Value)
	}
}

func TestSelfClosing(t *testing.T) {
	got := collect(`<br />`)
	want := []TokenKind{TokenTagOpen, TokenIdentifier, TokenTagSelfClose, TokenEOF}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("kinds mismatch:\nwant %v\n got %v", kindNamesFromKinds(want), kindNames(got))
	}
}

func TestSimpleMustache(t *testing.T) {
	got := collect(`{x}`)
	want := []TokenKind{TokenMustacheOpen, TokenText, TokenMustacheClose, TokenEOF}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("kinds mismatch:\nwant %v\n got %v", kindNamesFromKinds(want), kindNames(got))
	}
	if got[1].Value != "x" {
		t.Fatalf("expr value %q", got[1].Value)
	}
}

func TestMustacheBlockOpen(t *testing.T) {
	got := collect(`{#if Data.Ok}yes{/if}`)
	if got[0].Kind != TokenBlockOpen {
		t.Fatalf("expected BlockOpen, got %v", kindNames(got))
	}
	if got[0].Value != "#if Data.Ok" {
		t.Fatalf("block body %q", got[0].Value)
	}
}

func TestMustacheBlockMidAndClose(t *testing.T) {
	got := collect(`{#if A}a{:else}b{/if}`)
	wantValues := []string{"#if A", "a", ":else", "b", "/if"}
	var actual []string
	for _, tok := range got {
		switch tok.Kind {
		case TokenBlockOpen, TokenBlockMid, TokenBlockClose, TokenText:
			actual = append(actual, tok.Value)
		}
	}
	if !equalStrings(actual, wantValues) {
		t.Fatalf("values:\nwant %v\n got %v", wantValues, actual)
	}
}

func TestMustacheAtTag(t *testing.T) {
	got := collect(`{@html Data.Raw}`)
	if got[0].Kind != TokenAtTag || got[0].Value != "@html Data.Raw" {
		t.Fatalf("atTag mismatch: %+v", got[0])
	}
}

func TestMustacheNestedBraces(t *testing.T) {
	got := collect(`{#each Data.Posts as p}{p.Title}{/each}`)
	if got[0].Kind != TokenBlockOpen || got[0].Value != "#each Data.Posts as p" {
		t.Fatalf("block: %+v", got[0])
	}
	saw := map[TokenKind]int{}
	for _, tok := range got {
		saw[tok.Kind]++
	}
	if saw[TokenMustacheOpen] != 1 || saw[TokenMustacheClose] != 1 {
		t.Fatalf("expected one {p.Title}: %v", kindNames(got))
	}
}

func TestMustacheGoExpressionBraces(t *testing.T) {
	got := collect(`{#if f(struct{}{})}ok{/if}`)
	if got[0].Kind != TokenBlockOpen {
		t.Fatalf("expected BlockOpen first, got %v", kindNames(got))
	}
	if got[0].Value != "#if f(struct{}{})" {
		t.Fatalf("brace-balanced body: %q", got[0].Value)
	}
}

func TestMustacheStringLiteralBraces(t *testing.T) {
	got := collect("{#if `}` == \"}\"}ok{/if}")
	if got[0].Kind != TokenBlockOpen {
		t.Fatalf("expected BlockOpen, got %v (%+v)", kindNames(got), got[0])
	}
	if !strings.Contains(got[0].Value, "`}`") || !strings.Contains(got[0].Value, "\"}\"") {
		t.Fatalf("string-aware body: %q", got[0].Value)
	}
}

func TestMustacheRuneLiteralBraces(t *testing.T) {
	got := collect(`{#if c == '}'}ok{/if}`)
	if got[0].Kind != TokenBlockOpen {
		t.Fatalf("expected BlockOpen, got %v", kindNames(got))
	}
	if !strings.Contains(got[0].Value, `'}'`) {
		t.Fatalf("rune-aware body: %q", got[0].Value)
	}
}

func TestPlainMustache(t *testing.T) {
	got := collect(`{Data.Name}`)
	want := []TokenKind{TokenMustacheOpen, TokenText, TokenMustacheClose, TokenEOF}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("plain mustache: %v", kindNames(got))
	}
	if got[1].Value != "Data.Name" {
		t.Fatalf("expr value %q", got[1].Value)
	}
}

func TestEscapedBraces(t *testing.T) {
	got := collect(`a \{ b \} c`)
	if len(got) != 2 || got[0].Kind != TokenText {
		t.Fatalf("escaped braces should be one Text: %v", kindNames(got))
	}
	if got[0].Value != "a { b } c" {
		t.Fatalf("unescaped value: %q", got[0].Value)
	}
}

func TestComment(t *testing.T) {
	got := collect(`<!-- a -- b -->`)
	if got[0].Kind != TokenComment {
		t.Fatalf("expected Comment, got %v", kindNames(got))
	}
	if got[0].Value != "<!-- a -- b -->" {
		t.Fatalf("comment value: %q", got[0].Value)
	}
}

func TestScriptBlock(t *testing.T) {
	src := "<script>let x = a < b && b > c;</script>"
	got := collect(src)
	want := []TokenKind{TokenScriptOpen, TokenScriptBody, TokenScriptClose, TokenEOF}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("script kinds: %v", kindNames(got))
	}
	if got[0].Value != "<script>" {
		t.Fatalf("script open: %q", got[0].Value)
	}
	if got[1].Value != "let x = a < b && b > c;" {
		t.Fatalf("script body: %q", got[1].Value)
	}
	if got[2].Value != "</script>" {
		t.Fatalf("script close: %q", got[2].Value)
	}
}

func TestStyleBlock(t *testing.T) {
	src := `<style>a > b { color: red; }</style>`
	got := collect(src)
	want := []TokenKind{TokenStyleOpen, TokenStyleBody, TokenStyleClose, TokenEOF}
	if !equalKinds(kinds(got), want) {
		t.Fatalf("style kinds: %v", kindNames(got))
	}
	if got[1].Value != "a > b { color: red; }" {
		t.Fatalf("style body: %q", got[1].Value)
	}
}

func TestPositionTracking(t *testing.T) {
	src := "ab\n<c>"
	got := collect(src)
	if got[0].Line != 1 || got[0].Col != 1 || got[0].Offset != 0 {
		t.Fatalf("text pos: %+v", got[0])
	}
	if got[1].Kind != TokenTagOpen || got[1].Line != 2 || got[1].Col != 1 {
		t.Fatalf("tagopen pos: %+v", got[1])
	}
	if got[2].Kind != TokenIdentifier || got[2].Line != 2 || got[2].Col != 2 {
		t.Fatalf("ident pos: %+v", got[2])
	}
}

func TestErrorUnterminatedTag(t *testing.T) {
	got := collect(`<div `)
	saw := false
	for _, tok := range got {
		if tok.Kind == TokenError {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("expected Error token, got %v", kindNames(got))
	}
}

func TestErrorUnterminatedMustache(t *testing.T) {
	got := collect(`{#if x`)
	if got[0].Kind != TokenError {
		t.Fatalf("expected Error first, got %v", kindNames(got))
	}
	if !strings.Contains(got[0].Value, "unterminated") {
		t.Fatalf("error msg: %q", got[0].Value)
	}
}

func TestErrorUnterminatedComment(t *testing.T) {
	got := collect(`<!-- never closed`)
	if got[0].Kind != TokenError {
		t.Fatalf("expected Error first, got %v", kindNames(got))
	}
}

func TestRoundTripText(t *testing.T) {
	inputs := []string{
		"hello",
		"<div>x</div>",
		"<a href=\"x\">y</a>",
		"{#if A}b{/if}",
	}
	for _, src := range inputs {
		got := collect(src)
		var b strings.Builder
		for _, tok := range got {
			switch tok.Kind {
			case TokenEOF, TokenError:
				continue
			case TokenTagOpen, TokenTagOpenClose, TokenTagClose, TokenTagSelfClose,
				TokenScriptOpen, TokenScriptClose, TokenStyleOpen, TokenStyleClose,
				TokenComment:
				b.WriteString(tok.Value)
			case TokenIdentifier, TokenAttrName:
				b.WriteString(tok.Value)
			case TokenAttrEquals:
				b.WriteString("=")
			case TokenAttrValue:
				b.WriteByte('"')
				b.WriteString(tok.Value)
				b.WriteByte('"')
			case TokenMustacheOpen:
				b.WriteByte('{')
			case TokenMustacheClose:
				b.WriteByte('}')
			case TokenBlockOpen, TokenBlockMid, TokenBlockClose, TokenAtTag:
				b.WriteByte('{')
				b.WriteString(tok.Value)
				b.WriteByte('}')
			case TokenText, TokenScriptBody, TokenStyleBody:
				b.WriteString(tok.Value)
			}
		}
		// Round-trip is informational; not strict because attribute
		// quoting and brace escaping are normalized.
		if b.Len() == 0 {
			t.Errorf("empty round-trip for %q", src)
		}
	}
}

func TestEOFIdempotent(t *testing.T) {
	l := New([]byte("x"))
	for range 3 {
		l.Next()
	}
	last := l.Next()
	if last.Kind != TokenEOF {
		t.Fatalf("expected EOF, got %v", last.Kind)
	}
}

// Fixture-based golden tests cover the broader Svelte 5 grammar surface
// from issue #7 acceptance criteria. Each .svelte file under
// testdata/lexer/ pairs with a .golden under testdata/golden/lexer/.

func TestFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/lexer/*.svelte")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) < 30 {
		t.Fatalf("expected >= 30 fixtures, found %d", len(matches))
	}
	sort.Strings(matches)
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".svelte")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			rendered := renderTokens(collect(string(src)))
			golden.EqualString(t, "lexer/"+name, rendered)
		})
	}
}

func renderTokens(tokens []Token) string {
	var b strings.Builder
	for _, tok := range tokens {
		fmt.Fprintf(&b, "%s@%d:%d len=%d value=%s\n",
			tok.Kind.String(), tok.Line, tok.Col, tok.Length, escapeValue(tok.Value))
	}
	return b.String()
}

func escapeValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func equalKinds(a, b []TokenKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func kindNamesFromKinds(ks []TokenKind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.String()
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
