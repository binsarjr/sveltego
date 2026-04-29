package lexer

import (
	"strings"
)

type mode int

const (
	modeText mode = iota
	modeTag
	modeAttrValue
	modeMustache
	modeMustacheClose
	modeScript
	modeStyle
)

// Lexer converts a Svelte 5 template into a stream of tokens.
type Lexer struct {
	src   []byte
	pos   int
	line  int
	col   int
	modes []mode

	// pendingScript / pendingStyle is set when the most recent TagClose
	// belonged to a <script> or <style> opening tag, so the next Next()
	// call must enter the corresponding raw mode.
	pendingScript bool
	pendingStyle  bool
}

// New returns a lexer ready to tokenize src. The slice is retained; do not
// mutate it for the lifetime of the lexer.
func New(src []byte) *Lexer {
	return &Lexer{
		src:   src,
		line:  1,
		col:   1,
		modes: []mode{modeText},
	}
}

func (l *Lexer) currentMode() mode {
	return l.modes[len(l.modes)-1]
}

func (l *Lexer) pushMode(m mode) {
	l.modes = append(l.modes, m)
}

func (l *Lexer) popMode() {
	if len(l.modes) > 1 {
		l.modes = l.modes[:len(l.modes)-1]
	}
}

// Next returns the next token. After TokenEOF, subsequent calls keep
// returning TokenEOF.
func (l *Lexer) Next() Token {
	if l.pendingScript {
		l.pendingScript = false
		l.pushMode(modeScript)
	}
	if l.pendingStyle {
		l.pendingStyle = false
		l.pushMode(modeStyle)
	}

	if l.pos >= len(l.src) {
		return l.makeToken(TokenEOF, l.pos, "")
	}

	switch l.currentMode() {
	case modeText:
		return l.lexText()
	case modeTag:
		return l.lexTag()
	case modeAttrValue:
		return l.lexAttrValue()
	case modeMustache:
		return l.lexMustacheExpr()
	case modeMustacheClose:
		return l.lexMustacheCloseBrace()
	case modeScript:
		return l.lexRawBlock("script", TokenScriptBody, TokenScriptClose)
	case modeStyle:
		return l.lexRawBlock("style", TokenStyleBody, TokenStyleClose)
	default:
		return l.errorToken(l.pos, "internal: unknown lexer mode")
	}
}

func (l *Lexer) lexText() Token {
	if l.pos >= len(l.src) {
		return l.makeToken(TokenEOF, l.pos, "")
	}

	if l.startsWith("<!--") {
		return l.lexComment()
	}
	if l.startsWithFold("<script") && tagBoundaryAt(l.src, l.pos+len("<script")) {
		return l.lexRawOpen("script", TokenScriptOpen, &l.pendingScript)
	}
	if l.startsWithFold("<style") && tagBoundaryAt(l.src, l.pos+len("<style")) {
		return l.lexRawOpen("style", TokenStyleOpen, &l.pendingStyle)
	}
	if l.startsWith("</") {
		startLine, startCol, startPos := l.line, l.col, l.pos
		l.advance(2)
		l.pushMode(modeTag)
		return Token{Kind: TokenTagOpenClose, Value: "</", Offset: startPos, Line: startLine, Col: startCol, Length: 2}
	}
	if l.peek(0) == '<' && l.pos+1 < len(l.src) && isTagNameStart(l.peek(1)) {
		startLine, startCol, startPos := l.line, l.col, l.pos
		l.advance(1)
		l.pushMode(modeTag)
		return Token{Kind: TokenTagOpen, Value: "<", Offset: startPos, Line: startLine, Col: startCol, Length: 1}
	}
	if l.peek(0) == '{' {
		return l.lexMustache()
	}

	startLine, startCol, startPos := l.line, l.col, l.pos
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.peek(0)
		if c == '\\' && l.pos+1 < len(l.src) {
			n := l.peek(1)
			if n == '{' || n == '}' {
				b.WriteByte(n)
				l.advance(2)
				continue
			}
		}
		if c == '<' {
			if l.pos+1 < len(l.src) && (isTagNameStart(l.peek(1)) || l.peek(1) == '/' || l.peek(1) == '!') {
				break
			}
		}
		if c == '{' {
			break
		}
		b.WriteByte(c)
		l.advance(1)
	}
	return Token{Kind: TokenText, Value: b.String(), Offset: startPos, Line: startLine, Col: startCol, Length: l.pos - startPos}
}

func (l *Lexer) lexComment() Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	l.advance(4)
	for l.pos < len(l.src) {
		if l.startsWith("-->") {
			l.advance(3)
			return Token{
				Kind:   TokenComment,
				Value:  string(l.src[startPos:l.pos]),
				Offset: startPos,
				Line:   startLine,
				Col:    startCol,
				Length: l.pos - startPos,
			}
		}
		l.advance(1)
	}
	return Token{
		Kind:   TokenError,
		Value:  "unterminated comment",
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: l.pos - startPos,
	}
}

func (l *Lexer) lexRawOpen(name string, kind TokenKind, pending *bool) Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	l.advance(1 + len(name))
	for l.pos < len(l.src) {
		c := l.peek(0)
		if c == '>' {
			l.advance(1)
			*pending = true
			return Token{
				Kind:   kind,
				Value:  string(l.src[startPos:l.pos]),
				Offset: startPos,
				Line:   startLine,
				Col:    startCol,
				Length: l.pos - startPos,
			}
		}
		if c == '"' || c == '\'' {
			l.skipQuoted(c)
			continue
		}
		l.advance(1)
	}
	return Token{
		Kind:   TokenError,
		Value:  "unterminated <" + name + "> tag",
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: l.pos - startPos,
	}
}

func (l *Lexer) lexRawBlock(name string, bodyKind, closeKind TokenKind) Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	closer := "</" + name
	for l.pos < len(l.src) {
		if l.startsWithFold(closer) && tagBoundaryAt(l.src, l.pos+len(closer)) {
			if l.pos > startPos {
				return Token{
					Kind:   bodyKind,
					Value:  string(l.src[startPos:l.pos]),
					Offset: startPos,
					Line:   startLine,
					Col:    startCol,
					Length: l.pos - startPos,
				}
			}
			closeLine, closeCol, closePos := l.line, l.col, l.pos
			l.advance(len(closer))
			for l.pos < len(l.src) && l.peek(0) != '>' {
				l.advance(1)
			}
			if l.pos >= len(l.src) {
				l.popMode()
				return Token{
					Kind:   TokenError,
					Value:  "unterminated </" + name + "> tag",
					Offset: closePos,
					Line:   closeLine,
					Col:    closeCol,
					Length: l.pos - closePos,
				}
			}
			l.advance(1)
			l.popMode()
			return Token{
				Kind:   closeKind,
				Value:  string(l.src[closePos:l.pos]),
				Offset: closePos,
				Line:   closeLine,
				Col:    closeCol,
				Length: l.pos - closePos,
			}
		}
		l.advance(1)
	}
	if l.pos > startPos {
		return Token{
			Kind:   TokenError,
			Value:  "unterminated <" + name + "> body",
			Offset: startPos,
			Line:   startLine,
			Col:    startCol,
			Length: l.pos - startPos,
		}
	}
	return l.makeToken(TokenEOF, l.pos, "")
}

func (l *Lexer) lexTag() Token {
	l.skipSpaces()
	if l.pos >= len(l.src) {
		l.popMode()
		return Token{
			Kind:   TokenError,
			Value:  "unterminated tag",
			Offset: l.pos,
			Line:   l.line,
			Col:    l.col,
			Length: 0,
		}
	}
	c := l.peek(0)
	if c == '>' {
		startLine, startCol, startPos := l.line, l.col, l.pos
		l.advance(1)
		l.popMode()
		return Token{Kind: TokenTagClose, Value: ">", Offset: startPos, Line: startLine, Col: startCol, Length: 1}
	}
	if c == '/' && l.pos+1 < len(l.src) && l.peek(1) == '>' {
		startLine, startCol, startPos := l.line, l.col, l.pos
		l.advance(2)
		l.popMode()
		return Token{Kind: TokenTagSelfClose, Value: "/>", Offset: startPos, Line: startLine, Col: startCol, Length: 2}
	}
	if c == '=' {
		startLine, startCol, startPos := l.line, l.col, l.pos
		l.advance(1)
		l.pushMode(modeAttrValue)
		return Token{Kind: TokenAttrEquals, Value: "=", Offset: startPos, Line: startLine, Col: startCol, Length: 1}
	}
	if isTagNameStart(c) {
		return l.lexTagNameOrAttr()
	}
	return l.recoverTagError(c)
}

func (l *Lexer) lexTagNameOrAttr() Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	for l.pos < len(l.src) && isTagNameCont(l.peek(0)) {
		l.advance(1)
	}
	value := string(l.src[startPos:l.pos])
	kind := TokenIdentifier
	if startPos > 0 && !isTagOpenBoundary(l.src, startPos) {
		kind = TokenAttrName
	}
	return Token{Kind: kind, Value: value, Offset: startPos, Line: startLine, Col: startCol, Length: l.pos - startPos}
}

func (l *Lexer) lexAttrValue() Token {
	if l.pos >= len(l.src) {
		l.popMode()
		return Token{
			Kind:   TokenError,
			Value:  "unterminated attribute value",
			Offset: l.pos,
			Line:   l.line,
			Col:    l.col,
			Length: 0,
		}
	}
	c := l.peek(0)
	if c == '{' {
		l.popMode()
		return l.lexMustache()
	}
	startLine, startCol, startPos := l.line, l.col, l.pos
	if c == '"' || c == '\'' {
		quote := c
		l.advance(1)
		var b strings.Builder
		for l.pos < len(l.src) && l.peek(0) != quote {
			b.WriteByte(l.peek(0))
			l.advance(1)
		}
		if l.pos >= len(l.src) {
			l.popMode()
			return Token{
				Kind:   TokenError,
				Value:  "unterminated attribute value",
				Offset: startPos,
				Line:   startLine,
				Col:    startCol,
				Length: l.pos - startPos,
			}
		}
		l.advance(1)
		l.popMode()
		return Token{Kind: TokenAttrValue, Value: b.String(), Offset: startPos, Line: startLine, Col: startCol, Length: l.pos - startPos}
	}
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.peek(0)
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
			break
		}
		b.WriteByte(c)
		l.advance(1)
	}
	l.popMode()
	if b.Len() == 0 {
		return Token{
			Kind:   TokenError,
			Value:  "expected attribute value",
			Offset: startPos,
			Line:   startLine,
			Col:    startCol,
			Length: 0,
		}
	}
	return Token{Kind: TokenAttrValue, Value: b.String(), Offset: startPos, Line: startLine, Col: startCol, Length: l.pos - startPos}
}

func (l *Lexer) lexMustache() Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	l.advance(1)
	savePos, saveLine, saveCol := l.pos, l.line, l.col
	l.skipSpaces()
	if l.pos < len(l.src) {
		switch l.peek(0) {
		case '#':
			return l.lexBlockTag(startPos, startLine, startCol, TokenBlockOpen)
		case ':':
			return l.lexBlockTag(startPos, startLine, startCol, TokenBlockMid)
		case '/':
			return l.lexBlockTag(startPos, startLine, startCol, TokenBlockClose)
		case '@':
			return l.lexBlockTag(startPos, startLine, startCol, TokenAtTag)
		}
	}
	l.pos, l.line, l.col = savePos, saveLine, saveCol
	l.pushMode(modeMustache)
	return Token{Kind: TokenMustacheOpen, Value: "{", Offset: startPos, Line: startLine, Col: startCol, Length: 1}
}

func (l *Lexer) lexMustacheExpr() Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	body, ok := l.scanMustacheBody(false)
	if !ok {
		l.popMode()
		return Token{
			Kind:   TokenError,
			Value:  "unterminated mustache",
			Offset: startPos,
			Line:   startLine,
			Col:    startCol,
			Length: l.pos - startPos,
		}
	}
	l.popMode()
	l.pushMode(modeMustacheClose)
	return Token{
		Kind:   TokenText,
		Value:  body,
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: l.pos - startPos,
	}
}

func (l *Lexer) lexMustacheCloseBrace() Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	l.advance(1)
	l.popMode()
	return Token{
		Kind:   TokenMustacheClose,
		Value:  "}",
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: 1,
	}
}

func (l *Lexer) lexBlockTag(startPos, startLine, startCol int, kind TokenKind) Token {
	body, ok := l.scanMustacheBody(true)
	if !ok {
		return Token{
			Kind:   TokenError,
			Value:  "unterminated mustache block",
			Offset: startPos,
			Line:   startLine,
			Col:    startCol,
			Length: l.pos - startPos,
		}
	}
	return Token{
		Kind:   kind,
		Value:  body,
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: l.pos - startPos,
	}
}

// scanMustacheBody walks forward from the byte just after the opening
// brace and stops at the matching closing brace. Brace depth honors Go
// string literals so braces inside backticked strings, double-quoted
// strings, or single-quoted runes do not affect depth. The closing brace
// is consumed only if consumeClose is true; callers that emit the close
// as a separate token leave it in place. Returns false if EOF is hit
// before the matching brace.
func (l *Lexer) scanMustacheBody(consumeClose bool) (string, bool) {
	bodyStart := l.pos
	depth := 1
	for l.pos < len(l.src) {
		c := l.peek(0)
		switch c {
		case '`':
			l.advance(1)
			for l.pos < len(l.src) && l.peek(0) != '`' {
				l.advance(1)
			}
			if l.pos < len(l.src) {
				l.advance(1)
			}
			continue
		case '"':
			l.advance(1)
			for l.pos < len(l.src) {
				ch := l.peek(0)
				if ch == '\\' && l.pos+1 < len(l.src) {
					l.advance(2)
					continue
				}
				if ch == '"' || ch == '\n' {
					break
				}
				l.advance(1)
			}
			if l.pos < len(l.src) && l.peek(0) == '"' {
				l.advance(1)
			}
			continue
		case '\'':
			l.advance(1)
			for l.pos < len(l.src) {
				ch := l.peek(0)
				if ch == '\\' && l.pos+1 < len(l.src) {
					l.advance(2)
					continue
				}
				if ch == '\'' || ch == '\n' {
					break
				}
				l.advance(1)
			}
			if l.pos < len(l.src) && l.peek(0) == '\'' {
				l.advance(1)
			}
			continue
		case '{':
			depth++
			l.advance(1)
		case '}':
			depth--
			if depth == 0 {
				body := string(l.src[bodyStart:l.pos])
				if consumeClose {
					l.advance(1)
				}
				return body, true
			}
			l.advance(1)
		default:
			l.advance(1)
		}
	}
	return "", false
}

func (l *Lexer) recoverTagError(c byte) Token {
	startLine, startCol, startPos := l.line, l.col, l.pos
	for l.pos < len(l.src) && l.peek(0) != '>' && l.peek(0) != '\n' {
		l.advance(1)
	}
	if l.pos < len(l.src) && l.peek(0) == '>' {
		l.advance(1)
	}
	l.popMode()
	return Token{
		Kind:   TokenError,
		Value:  "unexpected character " + quoteByte(c) + " in tag",
		Offset: startPos,
		Line:   startLine,
		Col:    startCol,
		Length: l.pos - startPos,
	}
}

func (l *Lexer) makeToken(kind TokenKind, offset int, value string) Token {
	return Token{Kind: kind, Value: value, Offset: offset, Line: l.line, Col: l.col, Length: len(value)}
}

func (l *Lexer) errorToken(offset int, msg string) Token {
	return Token{Kind: TokenError, Value: msg, Offset: offset, Line: l.line, Col: l.col, Length: 0}
}

func (l *Lexer) peek(n int) byte {
	if l.pos+n >= len(l.src) {
		return 0
	}
	return l.src[l.pos+n]
}

func (l *Lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.src); i++ {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *Lexer) startsWith(s string) bool {
	if l.pos+len(s) > len(l.src) {
		return false
	}
	return string(l.src[l.pos:l.pos+len(s)]) == s
}

func (l *Lexer) startsWithFold(s string) bool {
	if l.pos+len(s) > len(l.src) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if lower(l.src[l.pos+i]) != lower(s[i]) {
			return false
		}
	}
	return true
}

func (l *Lexer) skipSpaces() {
	for l.pos < len(l.src) {
		c := l.peek(0)
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			l.advance(1)
			continue
		}
		break
	}
}

func (l *Lexer) skipQuoted(quote byte) {
	l.advance(1)
	for l.pos < len(l.src) && l.peek(0) != quote {
		l.advance(1)
	}
	if l.pos < len(l.src) {
		l.advance(1)
	}
}

func isTagNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isTagNameCont(c byte) bool {
	return isTagNameStart(c) || (c >= '0' && c <= '9') || c == '-' || c == ':' || c == '.'
}

// isTagOpenBoundary reports whether the byte just before pos is `<` or
// `</`, marking the position as a tag-name slot rather than an attribute.
func isTagOpenBoundary(src []byte, pos int) bool {
	if pos == 0 {
		return false
	}
	if src[pos-1] == '<' {
		return true
	}
	if pos >= 2 && src[pos-2] == '<' && src[pos-1] == '/' {
		return true
	}
	return false
}

func tagBoundaryAt(src []byte, pos int) bool {
	if pos >= len(src) {
		return true
	}
	c := src[pos]
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func quoteByte(c byte) string {
	if c >= 0x20 && c < 0x7f {
		return "'" + string(c) + "'"
	}
	const hex = "0123456789abcdef"
	return "0x" + string([]byte{hex[c>>4], hex[c&0x0f]})
}
