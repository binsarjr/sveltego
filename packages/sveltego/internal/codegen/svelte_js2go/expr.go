package sveltejs2go

import (
	"fmt"
	"strconv"
	"strings"
)

// formatExpression renders an ESTree expression as a Go expression.
// Phase 5 layers JSON-tag lowering by intercepting the Identifier and
// MemberExpression branches via opts.Rewriter.
func (e *emitter) formatExpression(n *Node) (string, error) {
	if n == nil {
		return "nil", nil
	}
	switch n.Type {
	case "Identifier":
		return e.formatIdentifier(n)
	case "Literal":
		return formatLiteral(n)
	case "TemplateLiteral":
		return e.formatTemplateLiteralExpr(n)
	case "MemberExpression":
		return e.formatMember(n)
	case "ChainExpression":
		return e.formatExpression(n.Expression)
	case "CallExpression":
		return e.formatCall(n)
	case "BinaryExpression":
		return e.formatBinary(n)
	case "LogicalExpression":
		return e.formatLogical(n)
	case "UnaryExpression":
		return e.formatUnary(n)
	case "UpdateExpression":
		return e.formatUpdate(n)
	case "ConditionalExpression":
		return e.formatConditional(n)
	case "ObjectExpression":
		return e.formatObject(n)
	case "ArrayExpression":
		return e.formatArray(n)
	case "ArrowFunctionExpression", "FunctionExpression":
		return e.formatFunctionExpr(n)
	case "AssignmentExpression":
		return e.formatAssignment(n)
	case "SpreadElement":
		inner, err := e.formatExpression(n.Argument)
		if err != nil {
			return "", err
		}
		return inner, nil
	}
	return "", unknownShape(n, "expr:"+n.Type)
}

func (e *emitter) formatIdentifier(n *Node) (string, error) {
	def := mangleIdent(n.Name)
	if e.opts.Rewriter != nil {
		if rep := e.opts.Rewriter.Rewrite(e.scope, n, def); rep != "" {
			return rep, nil
		}
	}
	return def, nil
}

func (e *emitter) formatMember(n *Node) (string, error) {
	obj, err := e.formatExpression(n.Object)
	if err != nil {
		return "", err
	}
	if n.Computed {
		idx, err := e.formatExpression(n.Property)
		if err != nil {
			return "", err
		}
		def := fmt.Sprintf("%s[%s]", obj, idx)
		// Phase 5 (#427) sees computed access too — strict-mode
		// lowerers reject it because there's no JSON tag to look up.
		if e.opts.Rewriter != nil {
			if rep := e.opts.Rewriter.Rewrite(e.scope, n, def); rep != "" {
				return rep, nil
			}
		}
		return def, nil
	}
	if n.Property == nil || n.Property.Type != "Identifier" {
		return "", unknownShape(n, "member-prop")
	}
	def := obj + "." + mangleIdent(n.Property.Name)
	if e.opts.Rewriter != nil {
		if rep := e.opts.Rewriter.Rewrite(e.scope, n, def); rep != "" {
			return rep, nil
		}
	}
	return def, nil
}

func formatLiteral(n *Node) (string, error) {
	switch n.LitKind {
	case litString:
		return strconv.Quote(n.LitStr), nil
	case litBool:
		if n.LitBool {
			return "true", nil
		}
		return "false", nil
	case litNumber:
		f := n.LitNum
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10), nil
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case litNull:
		return "nil", nil
	}
	if n.Raw != "" {
		return n.Raw, nil
	}
	return "", unknownShape(n, "literal:unknown-kind")
}

func (e *emitter) formatTemplateLiteralExpr(n *Node) (string, error) {
	pieces, err := e.formatTemplateLiteralPieces(n)
	if err != nil {
		return "", err
	}
	return strings.Join(pieces, " + "), nil
}

func (e *emitter) formatCall(n *Node) (string, error) {
	if n.Callee == nil {
		return "", unknownShape(n, "call:no-callee")
	}
	// Helper namespace dispatch: `$.<name>(args)`.
	if n.Callee.Type == "MemberExpression" {
		obj := n.Callee.Object
		prop := n.Callee.Property
		if obj != nil && obj.Type == "Identifier" && obj.Name == e.helperNS &&
			prop != nil && prop.Type == "Identifier" {
			return e.formatHelperCall(prop.Name, n.Arguments, n)
		}
	}
	// Generic call — used by snippet invocation and user method calls
	// that survived compilation. Render as Go function call.
	callee, err := e.formatExpression(n.Callee)
	if err != nil {
		return "", err
	}
	args := make([]string, 0, len(n.Arguments))
	for _, a := range n.Arguments {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		args = append(args, s)
	}
	return fmt.Sprintf("%s(%s)", callee, strings.Join(args, ", ")), nil
}

func (e *emitter) formatBinary(n *Node) (string, error) {
	left, err := e.formatExpression(n.Left)
	if err != nil {
		return "", err
	}
	right, err := e.formatExpression(n.Right)
	if err != nil {
		return "", err
	}
	op := n.Operator
	switch op {
	case "===":
		op = "=="
	case "!==":
		op = "!="
	case "==", "!=", "<", ">", "<=", ">=", "+", "-", "*", "/", "%":
		// pass through
	default:
		return "", unknownShape(n, "binary:"+op)
	}
	return fmt.Sprintf("(%s %s %s)", left, op, right), nil
}

func (e *emitter) formatLogical(n *Node) (string, error) {
	left, err := e.formatExpression(n.Left)
	if err != nil {
		return "", err
	}
	right, err := e.formatExpression(n.Right)
	if err != nil {
		return "", err
	}
	switch n.Operator {
	case "&&":
		return fmt.Sprintf("(%s && %s)", left, right), nil
	case "||":
		return fmt.Sprintf("(%s || %s)", left, right), nil
	case "??":
		// JS nullish-coalescing — Go has no operator; lower to a
		// runtime helper. server.Fallback returns left when non-nil.
		return fmt.Sprintf("%s.Fallback(%s, %s)", e.opts.HelperAlias, left, right), nil
	}
	return "", unknownShape(n, "logical:"+n.Operator)
}

func (e *emitter) formatUnary(n *Node) (string, error) {
	arg, err := e.formatExpression(n.Argument)
	if err != nil {
		return "", err
	}
	switch n.Operator {
	case "!":
		return fmt.Sprintf("!(%s)", arg), nil
	case "-":
		return fmt.Sprintf("-(%s)", arg), nil
	case "+":
		return fmt.Sprintf("+(%s)", arg), nil
	}
	return "", unknownShape(n, "unary:"+n.Operator)
}

func (e *emitter) formatUpdate(n *Node) (string, error) {
	arg, err := e.formatExpression(n.Argument)
	if err != nil {
		return "", err
	}
	switch n.Operator {
	case "++":
		return arg + "++", nil
	case "--":
		return arg + "--", nil
	}
	return "", unknownShape(n, "update:"+n.Operator)
}

func (e *emitter) formatConditional(n *Node) (string, error) {
	test, err := e.formatExpression(n.Test)
	if err != nil {
		return "", err
	}
	cons, err := e.formatExpression(n.Consequent)
	if err != nil {
		return "", err
	}
	alt, err := e.formatExpression(n.Alternate)
	if err != nil {
		return "", err
	}
	// Go has no ternary. Use a helper-free inline closure.
	return fmt.Sprintf("func() any { if %s { return %s }; return %s }()", test, cons, alt), nil
}

func (e *emitter) formatObject(n *Node) (string, error) {
	parts := make([]string, 0, len(n.Properties))
	for _, p := range n.Properties {
		if p.Type == "SpreadElement" {
			inner, err := e.formatExpression(p.Argument)
			if err != nil {
				return "", err
			}
			// Phase 5 (#427) sees spread via the optional
			// SpreadRewriter interface — the lowerer either expands it
			// using the typegen Shape or records a hard error. When no
			// rewriter is configured we fall back to the legacy
			// placeholder so Phase 3 goldens keep parsing under
			// go/format.
			if sr, ok := e.opts.Rewriter.(SpreadRewriter); ok {
				rewritten, expanded := sr.RewriteObjectSpread(e.scope, p, inner)
				if expanded {
					parts = append(parts, rewritten)
					continue
				}
			}
			parts = append(parts, "/* spread */ "+inner)
			continue
		}
		if p.Type != "Property" {
			return "", unknownShape(p, "object-prop:"+p.Type)
		}
		key, err := e.formatObjectKey(p)
		if err != nil {
			return "", err
		}
		val, err := e.formatExpression(p.Value)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s: %s", key, val))
	}
	return fmt.Sprintf("map[string]any{%s}", strings.Join(parts, ", ")), nil
}

func (e *emitter) formatObjectKey(p *Node) (string, error) {
	if p.Computed {
		k, err := e.formatExpression(p.Key)
		if err != nil {
			return "", err
		}
		return k, nil
	}
	if p.Key.Type == "Identifier" {
		return strconv.Quote(p.Key.Name), nil
	}
	if p.Key.Type == "Literal" && p.Key.LitKind == litString {
		return strconv.Quote(p.Key.LitStr), nil
	}
	return "", unknownShape(p.Key, "object-key:"+p.Key.Type)
}

func (e *emitter) formatArray(n *Node) (string, error) {
	parts := make([]string, 0, len(n.Properties))
	for _, el := range n.Properties {
		s, err := e.formatExpression(el)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return fmt.Sprintf("[]any{%s}", strings.Join(parts, ", ")), nil
}

// formatFunctionExpr lowers an arrow function or function expression
// into a Go closure. Used by component invocation, slot fragments,
// and {#snippet} bodies. The closure captures the outer payload and
// inherits scope; introduced parameters get declared as locals.
func (e *emitter) formatFunctionExpr(n *Node) (string, error) {
	if n.FuncBody == nil || n.FuncBody.Type != "BlockStatement" {
		return "", unknownShape(n, "fn-expr-body")
	}
	parent := e.scope
	defer func() { e.scope = parent }()
	e.scope = newScope(parent)

	params := make([]string, 0, len(n.Params))
	for _, p := range n.Params {
		switch p.Type {
		case "Identifier":
			name := mangleIdent(p.Name)
			e.scope.declare(name, LocalUnknown)
			params = append(params, name+" any")
		case "ObjectPattern":
			// Snippet parameters often arrive as object patterns.
			// Bind them to a synthetic name; the body's destructuring
			// usually unpacks via member access. Phase 5 expands.
			synthetic := fmt.Sprintf("ssvar_arg%d", len(params))
			params = append(params, synthetic+" any")
		default:
			return "", unknownShape(p, "fn-param:"+p.Type)
		}
	}

	body := &Buf{indent: 0}
	if err := e.emitBlock(body, n.FuncBody, false); err != nil {
		return "", err
	}
	indented := indent(body.String(), 1)
	return fmt.Sprintf("func(%s) {\n%s}", strings.Join(params, ", "), indented), nil
}

func (e *emitter) formatAssignment(n *Node) (string, error) {
	left, err := e.formatExpression(n.Left)
	if err != nil {
		return "", err
	}
	right, err := e.formatExpression(n.Right)
	if err != nil {
		return "", err
	}
	switch n.Operator {
	case "=":
		return fmt.Sprintf("%s = %s", left, right), nil
	case "+=", "-=", "*=", "/=":
		return fmt.Sprintf("%s %s %s", left, n.Operator, right), nil
	}
	return "", unknownShape(n, "assign:"+n.Operator)
}

func indent(s string, n int) string {
	if s == "" {
		return ""
	}
	pad := strings.Repeat("\t", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n") + "\n"
}
