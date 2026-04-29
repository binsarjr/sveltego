package lexer

// TokenKind identifies the lexical category of a Token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenError
	TokenText
	TokenTagOpen
	TokenTagOpenClose
	TokenTagSelfClose
	TokenTagClose
	TokenIdentifier
	TokenAttrName
	TokenAttrEquals
	TokenAttrValue
	TokenMustacheOpen
	TokenMustacheClose
	TokenBlockOpen
	TokenBlockMid
	TokenBlockClose
	TokenAtTag
	TokenComment
	TokenScriptOpen
	TokenScriptBody
	TokenScriptClose
	TokenStyleOpen
	TokenStyleBody
	TokenStyleClose
)

// String returns the human-readable name of the token kind. Used by tests
// and golden fixtures.
func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenError:
		return "Error"
	case TokenText:
		return "Text"
	case TokenTagOpen:
		return "TagOpen"
	case TokenTagOpenClose:
		return "TagOpenClose"
	case TokenTagSelfClose:
		return "TagSelfClose"
	case TokenTagClose:
		return "TagClose"
	case TokenIdentifier:
		return "Identifier"
	case TokenAttrName:
		return "AttrName"
	case TokenAttrEquals:
		return "AttrEquals"
	case TokenAttrValue:
		return "AttrValue"
	case TokenMustacheOpen:
		return "MustacheOpen"
	case TokenMustacheClose:
		return "MustacheClose"
	case TokenBlockOpen:
		return "BlockOpen"
	case TokenBlockMid:
		return "BlockMid"
	case TokenBlockClose:
		return "BlockClose"
	case TokenAtTag:
		return "AtTag"
	case TokenComment:
		return "Comment"
	case TokenScriptOpen:
		return "ScriptOpen"
	case TokenScriptBody:
		return "ScriptBody"
	case TokenScriptClose:
		return "ScriptClose"
	case TokenStyleOpen:
		return "StyleOpen"
	case TokenStyleBody:
		return "StyleBody"
	case TokenStyleClose:
		return "StyleClose"
	default:
		return "Unknown"
	}
}

// Token is a single lexical unit produced by the lexer.
type Token struct {
	Kind   TokenKind
	Value  string
	Offset int
	Line   int
	Col    int
	Length int
}
