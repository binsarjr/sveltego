package parser

import (
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/internal/lexer"
)

// Parse converts a Svelte 5 template into a Fragment AST. The returned
// Errors slice is empty on success and may contain multiple entries on
// failure; the AST is always non-nil and reflects whatever the parser
// could recover.
func Parse(src []byte) (*ast.Fragment, ast.Errors) {
	p := newParser(src)
	frag := p.parseFragment(stopEOF)
	return frag, p.errs
}

type stopFn func(tok lexer.Token) bool

func stopEOF(tok lexer.Token) bool {
	return tok.Kind == lexer.TokenEOF
}

// bailout unwinds from a deeply nested production back to the nearest
// recovery point. It is recovered only inside parseFragment's child loop;
// any other panic propagates as a programming error.
type bailout struct{}

type parser struct {
	lex  *lexer.Lexer
	buf  []lexer.Token
	errs ast.Errors
}

func newParser(src []byte) *parser {
	return &parser{lex: lexer.New(src)}
}

func (p *parser) peek() lexer.Token {
	if len(p.buf) == 0 {
		p.buf = append(p.buf, p.lex.Next())
	}
	return p.buf[0]
}

func (p *parser) advance() lexer.Token {
	tok := p.peek()
	if tok.Kind == lexer.TokenEOF {
		return tok
	}
	p.buf = p.buf[1:]
	return tok
}

func (p *parser) errorAt(tok lexer.Token, msg string) {
	p.errs = append(p.errs, ast.ParseError{
		Pos:     tokPos(tok),
		Message: msg,
	})
}

func (p *parser) errorWithHint(tok lexer.Token, msg, hint string) {
	p.errs = append(p.errs, ast.ParseError{
		Pos:     tokPos(tok),
		Message: msg,
		Hint:    hint,
	})
}

// expect consumes the next token if it matches kind, otherwise records an
// error and panics bailout to unwind to the nearest synchronization point.
func (p *parser) expect(kind lexer.TokenKind, what string) lexer.Token {
	tok := p.peek()
	if tok.Kind != kind {
		p.errorAt(tok, "expected "+what+", got "+describe(tok))
		panic(bailout{})
	}
	return p.advance()
}

// sync skips tokens until it finds an opener (`<`, `{`, block, at-tag, or
// raw open) or EOF. Used after a bailout so parsing of the next sibling
// can begin from a sane state.
func (p *parser) sync() {
	for {
		tok := p.peek()
		switch tok.Kind {
		case lexer.TokenEOF,
			lexer.TokenTagOpen,
			lexer.TokenTagOpenClose,
			lexer.TokenMustacheOpen,
			lexer.TokenBlockOpen,
			lexer.TokenBlockMid,
			lexer.TokenBlockClose,
			lexer.TokenAtTag,
			lexer.TokenComment,
			lexer.TokenScriptOpen,
			lexer.TokenStyleOpen:
			return
		}
		p.advance()
	}
}

func (p *parser) parseFragment(stop stopFn) *ast.Fragment {
	first := p.peek()
	frag := &ast.Fragment{P: tokPos(first)}
	for {
		tok := p.peek()
		if tok.Kind == lexer.TokenEOF || stop(tok) {
			return frag
		}
		node := p.parseChildSafe(stop)
		if node != nil {
			frag.Children = append(frag.Children, node)
		}
	}
}

// parseChildSafe parses one child node and recovers a bailout into the
// errors slice, advancing the cursor to the next opener so the caller can
// resume.
func (p *parser) parseChildSafe(stop stopFn) (n ast.Node) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(bailout); !ok {
			panic(r)
		}
		n = nil
		p.sync()
	}()
	return p.parseChild(stop)
}

func (p *parser) parseChild(stop stopFn) ast.Node {
	tok := p.peek()
	if stop(tok) {
		return nil
	}
	switch tok.Kind {
	case lexer.TokenError:
		p.errorAt(tok, tok.Value)
		p.advance()
		p.sync()
		return nil
	case lexer.TokenText:
		return p.parseText()
	case lexer.TokenComment:
		return p.parseComment()
	case lexer.TokenTagOpen:
		return p.parseElement()
	case lexer.TokenTagOpenClose:
		p.errorAt(tok, "unexpected closing tag")
		p.advance()
		p.skipUntilTagClose()
		return nil
	case lexer.TokenScriptOpen:
		return p.parseScript()
	case lexer.TokenStyleOpen:
		return p.parseStyle()
	case lexer.TokenMustacheOpen:
		return p.parseMustache()
	case lexer.TokenBlockOpen:
		return p.parseBlock()
	case lexer.TokenAtTag:
		return p.parseAtTag()
	case lexer.TokenBlockMid, lexer.TokenBlockClose:
		p.errorAt(tok, "unexpected "+describe(tok))
		p.advance()
		return nil
	default:
		p.errorAt(tok, "unexpected "+describe(tok))
		p.advance()
		return nil
	}
}

func (p *parser) parseText() ast.Node {
	tok := p.advance()
	return &ast.Text{P: tokPos(tok), Value: tok.Value}
}

func (p *parser) parseComment() ast.Node {
	tok := p.advance()
	return &ast.Comment{P: tokPos(tok), Value: tok.Value}
}

func (p *parser) parseElement() ast.Node {
	open := p.advance()
	nameTok := p.expect(lexer.TokenIdentifier, "tag name")
	el := &ast.Element{P: tokPos(open), Name: nameTok.Value}
	el.Component = isComponentName(nameTok.Value)
	el.Attributes = p.parseAttributes()

	switch p.peek().Kind {
	case lexer.TokenTagSelfClose:
		p.advance()
		el.SelfClosing = true
		return el
	case lexer.TokenTagClose:
		p.advance()
	default:
		tok := p.peek()
		p.errorAt(tok, "expected `>` or `/>` to close <"+nameTok.Value+">, got "+describe(tok))
		panic(bailout{})
	}

	if isVoidElement(nameTok.Value) {
		return el
	}

	el.Children = p.parseChildren(elementStop(nameTok.Value))
	p.consumeClosingTag(nameTok.Value)
	return el
}

func (p *parser) parseAttributes() []ast.Attribute {
	var attrs []ast.Attribute
	for {
		tok := p.peek()
		if tok.Kind != lexer.TokenAttrName {
			return attrs
		}
		attrs = append(attrs, p.parseAttribute())
	}
}

func (p *parser) parseAttribute() ast.Attribute {
	nameTok := p.advance()
	attr := ast.Attribute{P: tokPos(nameTok), Name: nameTok.Value}
	attr.Kind, attr.Modifier = classifyAttribute(nameTok.Value)

	if p.peek().Kind != lexer.TokenAttrEquals {
		return attr
	}
	p.advance()

	switch p.peek().Kind {
	case lexer.TokenAttrValue:
		v := p.advance()
		attr.Value = &ast.StaticValue{P: tokPos(v), Value: v.Value}
	case lexer.TokenMustacheOpen:
		open := p.advance()
		exprTok := p.expect(lexer.TokenText, "expression")
		p.expect(lexer.TokenMustacheClose, "`}`")
		attr.Value = &ast.DynamicValue{P: tokPos(open), Expr: strings.TrimSpace(exprTok.Value)}
	default:
		tok := p.peek()
		p.errorAt(tok, "expected attribute value, got "+describe(tok))
		panic(bailout{})
	}
	return attr
}

func (p *parser) parseMustache() ast.Node {
	open := p.advance()
	exprTok := p.expect(lexer.TokenText, "expression")
	p.expect(lexer.TokenMustacheClose, "`}`")
	return &ast.Mustache{P: tokPos(open), Expr: strings.TrimSpace(exprTok.Value)}
}

func (p *parser) parseAtTag() ast.Node {
	tok := p.advance()
	pos := tokPos(tok)
	body := strings.TrimPrefix(tok.Value, "@")
	name, rest := splitDirective(body)
	rest = strings.TrimSpace(rest)
	switch name {
	case "html":
		return &ast.RawHTML{P: pos, Expr: rest}
	case "const":
		return &ast.Const{P: pos, Stmt: rest}
	case "render":
		return &ast.Render{P: pos, Expr: rest}
	default:
		p.errorAt(tok, "unknown @-tag `@"+name+"`")
		return nil
	}
}

func (p *parser) parseBlock() ast.Node {
	tok := p.peek()
	body := strings.TrimPrefix(tok.Value, "#")
	name, _ := splitDirective(body)
	switch name {
	case "if":
		return p.parseIfBlock()
	case "each":
		return p.parseEachBlock()
	case "await":
		return p.parseAwaitBlock()
	case "key":
		return p.parseKeyBlock()
	case "snippet":
		return p.parseSnippetBlock()
	default:
		p.errorAt(tok, "unknown block `#"+name+"`")
		p.advance()
		return nil
	}
}

func (p *parser) parseIfBlock() ast.Node {
	open := p.advance()
	cond := blockArgs(open.Value, "#if")
	if cond == "" {
		p.errorAt(open, "`{#if}` requires a condition")
	}
	node := &ast.IfBlock{P: tokPos(open), Cond: cond}
	node.Then = p.parseChildren(blockBoundary("if"))

	for p.peek().Kind == lexer.TokenBlockMid {
		mid := p.peek()
		midBody := strings.TrimPrefix(mid.Value, ":")
		midName, args := splitDirective(midBody)
		switch midName {
		case "else":
			args = strings.TrimSpace(args)
			if rest := strings.TrimPrefix(args, "if"); rest != args && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
				p.advance()
				cond := strings.TrimSpace(rest)
				if cond == "" {
					p.errorAt(mid, "`{:else if}` requires a condition")
				}
				branch := ast.ElifBranch{P: tokPos(mid), Cond: cond}
				branch.Body = p.parseChildren(blockBoundary("if"))
				node.Elifs = append(node.Elifs, branch)
				continue
			}
			p.advance()
			node.Else = p.parseChildren(blockBoundary("if"))
			goto closeIf
		default:
			p.errorAt(mid, "unexpected `{:"+midName+"}` inside `{#if}`")
			p.advance()
		}
	}
closeIf:
	p.expectBlockClose("if")
	return node
}

func (p *parser) parseEachBlock() ast.Node {
	open := p.advance()
	args := blockArgs(open.Value, "#each")
	iter, item, idx, key := splitEach(args)
	node := &ast.EachBlock{P: tokPos(open), Iter: iter, Item: item, Index: idx, Key: key}
	if iter == "" || item == "" {
		p.errorAt(open, "`{#each}` requires `iter as item`")
	}
	node.Body = p.parseChildren(blockBoundary("each"))

	if p.peek().Kind == lexer.TokenBlockMid {
		mid := p.peek()
		midBody := strings.TrimPrefix(mid.Value, ":")
		midName, _ := splitDirective(midBody)
		if midName == "else" {
			p.advance()
			node.Else = p.parseChildren(blockBoundary("each"))
		} else {
			p.errorAt(mid, "unexpected `{:"+midName+"}` inside `{#each}`")
			p.advance()
		}
	}
	p.expectBlockClose("each")
	return node
}

func (p *parser) parseAwaitBlock() ast.Node {
	open := p.advance()
	expr := blockArgs(open.Value, "#await")
	if expr == "" {
		p.errorAt(open, "`{#await}` requires an expression")
	}
	node := &ast.AwaitBlock{P: tokPos(open), Expr: expr}
	node.Pending = p.parseChildren(blockBoundary("await"))

	for p.peek().Kind == lexer.TokenBlockMid {
		mid := p.peek()
		midBody := strings.TrimPrefix(mid.Value, ":")
		midName, args := splitDirective(midBody)
		args = strings.TrimSpace(args)
		switch midName {
		case "then":
			p.advance()
			node.ThenVar = args
			node.Then = p.parseChildren(blockBoundary("await"))
		case "catch":
			p.advance()
			node.CatchVar = args
			node.Catch = p.parseChildren(blockBoundary("await"))
		default:
			p.errorAt(mid, "unexpected `{:"+midName+"}` inside `{#await}`")
			p.advance()
		}
	}
	p.expectBlockClose("await")
	return node
}

func (p *parser) parseKeyBlock() ast.Node {
	open := p.advance()
	key := blockArgs(open.Value, "#key")
	if key == "" {
		p.errorAt(open, "`{#key}` requires an expression")
	}
	node := &ast.KeyBlock{P: tokPos(open), Key: key}
	node.Body = p.parseChildren(blockBoundary("key"))
	p.expectBlockClose("key")
	return node
}

func (p *parser) parseSnippetBlock() ast.Node {
	open := p.advance()
	args := blockArgs(open.Value, "#snippet")
	name, params := splitSnippet(args)
	if name == "" {
		p.errorAt(open, "`{#snippet}` requires a name")
	}
	node := &ast.SnippetBlock{P: tokPos(open), Name: name, Params: params}
	node.Body = p.parseChildren(blockBoundary("snippet"))
	p.expectBlockClose("snippet")
	return node
}

func (p *parser) expectBlockClose(name string) {
	tok := p.peek()
	if tok.Kind != lexer.TokenBlockClose {
		p.errorAt(tok, "expected `{/"+name+"}`, got "+describe(tok))
		panic(bailout{})
	}
	body := strings.TrimPrefix(tok.Value, "/")
	got, _ := splitDirective(body)
	if got != name {
		p.errorAt(tok, "expected `{/"+name+"}`, got `{/"+got+"}`")
		panic(bailout{})
	}
	p.advance()
}

func (p *parser) parseScript() ast.Node {
	open := p.advance()
	lang := extractScriptLang(open.Value)
	module := extractScriptModule(open.Value)
	body := ""
	bodyTok := p.peek()
	if bodyTok.Kind == lexer.TokenScriptBody {
		body = bodyTok.Value
		p.advance()
	}
	close := p.peek()
	if close.Kind != lexer.TokenScriptClose {
		p.errorAt(close, "expected `</script>`, got "+describe(close))
		return &ast.Script{P: tokPos(open), Lang: lang, Module: module, Body: body}
	}
	p.advance()
	switch {
	case module && lang != "" && lang != "ts":
		p.errorWithHint(open,
			"unsupported `<script module>` lang `"+lang+"`",
			"<script module> compiles to JS via Vite; drop `lang` or use lang=\"ts\"")
	case !module && lang != "" && lang != "go":
		p.errorWithHint(open,
			"unsupported script lang `"+lang+"`",
			"sveltego compiles Go inside <script>; remove `lang` or use lang=\"go\"")
	}
	return &ast.Script{P: tokPos(open), Lang: lang, Module: module, Body: body}
}

func (p *parser) parseStyle() ast.Node {
	open := p.advance()
	body := ""
	bodyTok := p.peek()
	if bodyTok.Kind == lexer.TokenStyleBody {
		body = bodyTok.Value
		p.advance()
	}
	close := p.peek()
	if close.Kind != lexer.TokenStyleClose {
		p.errorAt(close, "expected `</style>`, got "+describe(close))
		return &ast.Style{P: tokPos(open), Body: body}
	}
	p.advance()
	return &ast.Style{P: tokPos(open), Body: body}
}

func (p *parser) parseChildren(stop stopFn) []ast.Node {
	var out []ast.Node
	for {
		tok := p.peek()
		if tok.Kind == lexer.TokenEOF || stop(tok) {
			return out
		}
		node := p.parseChildSafe(stop)
		if node != nil {
			out = append(out, node)
		}
	}
}

func (p *parser) consumeClosingTag(name string) {
	tok := p.peek()
	if tok.Kind != lexer.TokenTagOpenClose {
		p.errorAt(tok, "expected `</"+name+">` to close <"+name+">, got "+describe(tok))
		return
	}
	p.advance()
	nameTok := p.peek()
	if nameTok.Kind != lexer.TokenIdentifier {
		p.errorAt(nameTok, "expected tag name in closing tag, got "+describe(nameTok))
		p.skipUntilTagClose()
		return
	}
	if nameTok.Value != name {
		p.errorAt(nameTok, "mismatched closing tag `</"+nameTok.Value+">` for `<"+name+">`")
	}
	p.advance()
	end := p.peek()
	if end.Kind == lexer.TokenTagClose {
		p.advance()
		return
	}
	p.errorAt(end, "expected `>` to close `</"+name+">`, got "+describe(end))
	p.skipUntilTagClose()
}

func (p *parser) skipUntilTagClose() {
	for {
		tok := p.peek()
		switch tok.Kind {
		case lexer.TokenEOF:
			return
		case lexer.TokenTagClose, lexer.TokenTagSelfClose:
			p.advance()
			return
		}
		p.advance()
	}
}
