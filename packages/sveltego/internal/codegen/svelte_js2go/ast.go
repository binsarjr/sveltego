package svelte_js2go

import (
	"encoding/json"
	"fmt"
)

// Envelope is the top-level shape the sidecar emits per route. The
// schema field is fixed at "ssr-json-ast/v1" by ADR 0009.
type Envelope struct {
	Schema string          `json:"schema"`
	Route  string          `json:"route"`
	AST    json.RawMessage `json:"ast"`
}

// Node is the polymorphic ESTree wrapper. Each variant decodes a small
// closed set of fields — anything outside the set is irrelevant to the
// emitter and skipped. Node values are immutable after Decode; the
// walker peels variants by inspecting Type and reaching into the typed
// pointer fields.
type Node struct {
	Type  string
	Start int
	End   int

	// Top-level / structural ----------------------------------------
	Body       []*Node // Program.body, BlockStatement.body, FunctionBody.body
	SourceType string  // Program.sourceType

	Specifiers []*Node // ImportDeclaration.specifiers
	Source     *Node   // ImportDeclaration.source (Literal)
	Local      *Node   // ImportNamespaceSpecifier.local

	Declaration  *Node   // ExportDefaultDeclaration.declaration
	Declarations []*Node // VariableDeclaration.declarations
	Kind         string  // VariableDeclaration.kind
	ID           *Node   // VariableDeclarator.id, FunctionDeclaration.id
	Init         *Node   // VariableDeclarator.init, ForStatement.init

	Async     bool // FunctionDeclaration / ArrowFunctionExpression
	Generator bool // FunctionDeclaration / ArrowFunctionExpression
	Params    []*Node
	FuncBody  *Node // FunctionDeclaration.body / ArrowFunctionExpression.body

	// Patterns ------------------------------------------------------
	Properties []*Node // ObjectPattern.properties / ObjectExpression.properties
	Key        *Node   // Property.key
	Value      *Node   // Property.value, Literal.value via raw, AssignmentPattern.right
	Shorthand  bool
	Computed   bool
	MethodProp bool

	// Expressions ---------------------------------------------------
	Expression  *Node // ExpressionStatement.expression, ConditionalExpression test fold helpers
	Arguments   []*Node
	Callee      *Node
	Object      *Node
	Property    *Node
	Optional    bool
	Operator    string
	Left        *Node
	Right       *Node
	Argument    *Node
	Prefix      bool
	Test        *Node
	Consequent  *Node
	Alternate   *Node
	Update      *Node // ForStatement.update
	Quasis      []*Node
	Expressions []*Node
	Tail        bool
	Cooked      string
	RawValue    string

	// Identifiers / Literals ---------------------------------------
	Name    string // Identifier.name
	Raw     string // Literal.raw
	LitKind litKind
	LitStr  string
	LitNum  float64
	LitBool bool
}

type litKind uint8

const (
	litUnknown litKind = iota
	litNull
	litBool
	litNumber
	litString
)

// Decode parses an Envelope's AST into a tree of Node pointers.
func Decode(raw []byte) (*Node, error) {
	var n Node
	if err := n.UnmarshalJSON(raw); err != nil {
		return nil, err
	}
	return &n, nil
}

// UnmarshalJSON pulls only the fields the emitter cares about. Unknown
// fields are dropped so AST size doesn't bloat memory and so future
// Acorn additions don't break us until they introduce a new node type.
func (n *Node) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("svelte_js2go: parse ast: %w", err)
	}
	if v, ok := raw["type"]; ok {
		_ = json.Unmarshal(v, &n.Type)
	}
	if v, ok := raw["start"]; ok {
		_ = json.Unmarshal(v, &n.Start)
	}
	if v, ok := raw["end"]; ok {
		_ = json.Unmarshal(v, &n.End)
	}

	get := func(k string) json.RawMessage {
		return raw[k]
	}

	decodeNode := func(k string) *Node {
		v := get(k)
		if len(v) == 0 || string(v) == "null" {
			return nil
		}
		var sub Node
		if err := sub.UnmarshalJSON(v); err != nil {
			return nil
		}
		return &sub
	}
	decodeNodeList := func(k string) []*Node {
		v := get(k)
		if len(v) == 0 || string(v) == "null" {
			return nil
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(v, &arr); err != nil {
			return nil
		}
		out := make([]*Node, 0, len(arr))
		for _, item := range arr {
			if len(item) == 0 || string(item) == "null" {
				continue
			}
			var sub Node
			if err := sub.UnmarshalJSON(item); err != nil {
				continue
			}
			out = append(out, &sub)
		}
		return out
	}
	decodeStr := func(k string) string {
		v := get(k)
		if len(v) == 0 {
			return ""
		}
		var s string
		_ = json.Unmarshal(v, &s)
		return s
	}
	decodeBool := func(k string) bool {
		v := get(k)
		if len(v) == 0 {
			return false
		}
		var x bool
		_ = json.Unmarshal(v, &x)
		return x
	}

	switch n.Type {
	case "Program":
		n.Body = decodeNodeList("body")
		n.SourceType = decodeStr("sourceType")

	case "ImportDeclaration":
		n.Specifiers = decodeNodeList("specifiers")
		n.Source = decodeNode("source")

	case "ImportNamespaceSpecifier", "ImportDefaultSpecifier", "ImportSpecifier":
		n.Local = decodeNode("local")

	case "ExportDefaultDeclaration", "ExportNamedDeclaration":
		n.Declaration = decodeNode("declaration")

	case "FunctionDeclaration", "FunctionExpression", "ArrowFunctionExpression":
		n.Async = decodeBool("async")
		n.Generator = decodeBool("generator")
		n.ID = decodeNode("id")
		n.Params = decodeNodeList("params")
		n.FuncBody = decodeNode("body")

	case "BlockStatement":
		n.Body = decodeNodeList("body")

	case "VariableDeclaration":
		n.Kind = decodeStr("kind")
		n.Declarations = decodeNodeList("declarations")

	case "VariableDeclarator":
		n.ID = decodeNode("id")
		n.Init = decodeNode("init")

	case "ObjectPattern", "ObjectExpression":
		n.Properties = decodeNodeList("properties")

	case "ArrayPattern", "ArrayExpression":
		n.Properties = decodeNodeList("elements")

	case "Property":
		n.Key = decodeNode("key")
		n.Value = decodeNode("value")
		n.Kind = decodeStr("kind")
		n.Shorthand = decodeBool("shorthand")
		n.Computed = decodeBool("computed")
		n.MethodProp = decodeBool("method")

	case "ExpressionStatement":
		n.Expression = decodeNode("expression")

	case "AssignmentExpression", "BinaryExpression", "LogicalExpression":
		n.Operator = decodeStr("operator")
		n.Left = decodeNode("left")
		n.Right = decodeNode("right")

	case "MemberExpression":
		n.Object = decodeNode("object")
		n.Property = decodeNode("property")
		n.Computed = decodeBool("computed")
		n.Optional = decodeBool("optional")

	case "CallExpression", "NewExpression":
		n.Callee = decodeNode("callee")
		n.Arguments = decodeNodeList("arguments")
		n.Optional = decodeBool("optional")

	case "Identifier":
		n.Name = decodeStr("name")

	case "Literal":
		n.Raw = decodeStr("raw")
		decodeLiteralValue(n, get("value"))

	case "TemplateLiteral":
		n.Quasis = decodeNodeList("quasis")
		n.Expressions = decodeNodeList("expressions")

	case "TemplateElement":
		n.Tail = decodeBool("tail")
		decodeTemplateValue(n, get("value"))

	case "IfStatement":
		n.Test = decodeNode("test")
		n.Consequent = decodeNode("consequent")
		n.Alternate = decodeNode("alternate")

	case "ConditionalExpression":
		n.Test = decodeNode("test")
		n.Consequent = decodeNode("consequent")
		n.Alternate = decodeNode("alternate")

	case "ReturnStatement":
		n.Argument = decodeNode("argument")

	case "UnaryExpression", "UpdateExpression":
		n.Operator = decodeStr("operator")
		n.Argument = decodeNode("argument")
		n.Prefix = decodeBool("prefix")

	case "ForStatement":
		n.Init = decodeNode("init")
		n.Test = decodeNode("test")
		n.Update = decodeNode("update")
		n.FuncBody = decodeNode("body")

	case "ForOfStatement", "ForInStatement":
		n.Left = decodeNode("left")
		n.Right = decodeNode("right")
		n.FuncBody = decodeNode("body")

	case "WhileStatement", "DoWhileStatement":
		n.Test = decodeNode("test")
		n.FuncBody = decodeNode("body")

	case "SpreadElement", "RestElement":
		n.Argument = decodeNode("argument")

	case "ChainExpression":
		n.Expression = decodeNode("expression")

	case "AssignmentPattern":
		n.Left = decodeNode("left")
		n.Right = decodeNode("right")
	}

	return nil
}

func decodeLiteralValue(n *Node, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		n.LitKind = litNull
		return
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			n.LitKind = litString
			n.LitStr = s
			n.Value = nil
		}
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			n.LitKind = litBool
			n.LitBool = b
		}
	default:
		var f float64
		if err := json.Unmarshal(raw, &f); err == nil {
			n.LitKind = litNumber
			n.LitNum = f
		}
	}
}

func decodeTemplateValue(n *Node, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var v struct {
		Cooked string `json:"cooked"`
		Raw    string `json:"raw"`
	}
	if err := json.Unmarshal(raw, &v); err == nil {
		n.Cooked = v.Cooked
		n.RawValue = v.Raw
	}
}
