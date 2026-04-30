package codegen

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// runeKind tags a top-level rune-bearing statement so the lowering pass
// can route each kind to its emit form.
type runeKind int

const (
	runeNone runeKind = iota
	runeProps
	runeState
	runeDerived
	runeDerivedBy
	runeEffect
	runeBindable
)

// runeProp describes one field extracted from a `let { ... } = $props()`
// destructure. Default is the verbatim Go expression supplied as the
// destructure default (empty when none). Bindable records whether the
// default was wrapped in `$bindable(...)`. Rest marks the `...rest`
// catchall, which lowers to `map[string]any` regardless of annotation.
type runeProp struct {
	Name     string
	Type     string
	Default  string
	Bindable bool
	Rest     bool
}

// runeStmt is a render-body statement lowered from a $state, $derived,
// $derived.by, or $effect call. Effect statements lower to nothing — the
// kind is retained so future passes can emit hydration markers without
// changing the analysis surface.
type runeStmt struct {
	Kind runeKind
	Body string
}

// runeAnalysis is the structured result of scanning one <script> body
// for top-level rune patterns. Decls holds the surviving non-rune top-
// level declarations (functions, types, vars, consts). Imports collects
// imports declared in the same script. Props is the destructured field
// set from a single `let { ... } = $props()` line; nil when absent.
// RestField names the `...rest` capture (PascalCased) when present.
// Stmts holds the render-body lowering of $state / $derived / $effect
// in source order.
type runeAnalysis struct {
	Decls     []string
	Imports   []string
	Props     []runeProp
	RestField string
	Stmts     []runeStmt
	HasProps  bool
}

// analyzeRunes splits the rewritten script body into rune-bearing lines
// and remaining Go declarations. Rune lines are parsed by hand (their
// shape is well-defined); the residue is fed to the existing
// go/parser-based import + decl extractor through the rewritten
// placeholder form.
func analyzeRunes(rewritten string, scriptPos ast.Pos) (runeAnalysis, error) {
	lines := strings.Split(rewritten, "\n")
	residueLines := make([]string, len(lines))
	var ana runeAnalysis

	for i := 0; i < len(lines); i++ {
		stmt, end, err := matchRuneStmt(lines, i, scriptPos)
		if err != nil {
			return runeAnalysis{}, err
		}
		if end == i {
			residueLines[i] = lines[i]
			continue
		}
		if err := applyRuneStmt(stmt, &ana, scriptPos); err != nil {
			return runeAnalysis{}, err
		}
		for j := i; j < end; j++ {
			residueLines[j] = ""
		}
		i = end - 1
	}

	residue := strings.Join(residueLines, "\n")
	if strings.TrimSpace(residue) != "" {
		decls, imports, err := parseScriptResidue(residue, scriptPos)
		if err != nil {
			return runeAnalysis{}, err
		}
		ana.Decls = decls
		ana.Imports = imports
	}

	return ana, nil
}

// matchRuneStmt looks at lines[start] (and possibly continuation lines)
// for a top-level rune-bearing statement. Returns the parsed runeStmtMatch
// and the exclusive end index covering the consumed lines, or
// (zero, start, nil) when no rune statement matches at this position.
func matchRuneStmt(lines []string, start int, scriptPos ast.Pos) (runeStmtMatch, int, error) {
	line := lines[start]
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "//") {
		return runeStmtMatch{}, start, nil
	}

	if strings.HasPrefix(trim, "let ") || strings.HasPrefix(trim, "let{") {
		end, joined := joinUntilBalanced(lines, start)
		if !containsRunePlaceholderTokens(joined, "props", "state", "derived", "bindable") {
			return runeStmtMatch{}, start, nil
		}
		stmt, err := parseLetRune(joined, scriptPos)
		if err != nil {
			return runeStmtMatch{}, start, err
		}
		return stmt, end + 1, nil
	}

	if strings.HasPrefix(trim, runePrefix+"effect") {
		end, joined := joinUntilBalanced(lines, start)
		stmt, err := parseEffectStmt(joined, scriptPos)
		if err != nil {
			return runeStmtMatch{}, start, err
		}
		return stmt, end + 1, nil
	}

	return runeStmtMatch{}, start, nil
}

// joinUntilBalanced concatenates lines starting at start until the
// running paren+brace count is non-positive. Returns the final inclusive
// index and the joined source. When balance never resolves the join
// stops at the last line so the caller's parser surfaces a proper error.
func joinUntilBalanced(lines []string, start int) (int, string) {
	balance := 0
	end := start
	var b strings.Builder
	for i := start; i < len(lines); i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(lines[i])
		balance += parenBraceDelta(lines[i])
		end = i
		if i > start && balance <= 0 {
			break
		}
		if i == start && balance == 0 {
			break
		}
	}
	return end, b.String()
}

// parenBraceDelta returns the net `{[(` minus `)]}` count, ignoring
// characters inside string and rune literals.
func parenBraceDelta(line string) int {
	delta := 0
	in := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if in != 0 {
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == in {
				in = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			in = c
		case '{', '[', '(':
			delta++
		case '}', ']', ')':
			delta--
		}
	}
	return delta
}

func containsRunePlaceholderTokens(src string, names ...string) bool {
	for _, n := range names {
		if strings.Contains(src, runePrefix+n) {
			return true
		}
	}
	return false
}

// runeStmtMatch is the parsed shape of a single rune-bearing line ready
// for application against runeAnalysis.
type runeStmtMatch struct {
	kind     runeKind
	letProps []runeProp
	letRest  string
	stmtBody string
}

func applyRuneStmt(m runeStmtMatch, ana *runeAnalysis, scriptPos ast.Pos) error {
	switch m.kind {
	case runeProps:
		if ana.HasProps {
			return &CodegenError{Pos: scriptPos, Msg: "<script>: only one $props() destructure allowed"}
		}
		ana.HasProps = true
		ana.Props = append(ana.Props, m.letProps...)
		if m.letRest != "" {
			ana.RestField = m.letRest
		}
		return nil
	case runeState, runeDerived, runeDerivedBy:
		ana.Stmts = append(ana.Stmts, runeStmt{Kind: m.kind, Body: m.stmtBody})
		return nil
	case runeEffect:
		ana.Stmts = append(ana.Stmts, runeStmt{Kind: runeEffect})
		return nil
	}
	return &CodegenError{Pos: scriptPos, Msg: "internal: unknown rune kind"}
}

// parseLetRune parses one `let { ... } = $props()` or
// `let name = $state/$derived(...)` line. The line shape determines the
// rune kind; mismatched shapes return a *CodegenError.
func parseLetRune(src string, scriptPos ast.Pos) (runeStmtMatch, error) {
	rest := strings.TrimPrefix(strings.TrimSpace(src), "let")
	rest = strings.TrimSpace(rest)
	eq := findTopLevelEquals(rest)
	if eq < 0 {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: expected `=` in let statement"}
	}
	lhs := strings.TrimSpace(rest[:eq])
	rhs := strings.TrimSpace(rest[eq+1:])

	rhsExpr, err := parser.ParseExpr(rhs)
	if err != nil {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid let RHS: %v", err)}
	}

	if strings.HasPrefix(lhs, "{") {
		return parseLetDestructure(lhs, rhsExpr, scriptPos)
	}

	name, annotation, err := splitNameAndAnnotation(lhs, scriptPos)
	if err != nil {
		return runeStmtMatch{}, err
	}
	call, ok := rhsExpr.(*goast.CallExpr)
	if !ok {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: let RHS must be a rune call"}
	}
	kind := runeCallKind(call.Fun)
	_ = annotation
	switch kind {
	case runeState:
		if len(call.Args) != 1 {
			return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "$state() requires exactly one initial value"}
		}
		init := mustFormat(call.Args[0])
		return runeStmtMatch{kind: runeState, stmtBody: name + " := " + init}, nil
	case runeDerived:
		if len(call.Args) != 1 {
			return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "$derived requires exactly one argument"}
		}
		expr := mustFormat(call.Args[0])
		return runeStmtMatch{kind: runeDerived, stmtBody: name + " := " + expr}, nil
	case runeDerivedBy:
		if len(call.Args) != 1 {
			return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "$derived.by requires exactly one argument"}
		}
		fn, ok := call.Args[0].(*goast.FuncLit)
		if !ok {
			return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "$derived.by requires a function literal argument"}
		}
		body := mustFormat(fn)
		return runeStmtMatch{kind: runeDerivedBy, stmtBody: name + " := (" + body + ")()"}, nil
	}
	return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: unrecognized rune call in let RHS"}
}

// parseLetDestructure parses `let { ... }(: T)? = $props()` and returns
// the prop set + bindable / rest metadata. The RHS must be exactly the
// `$props()` placeholder call; bindable defaults are detected inside
// destructure values.
func parseLetDestructure(lhs string, rhsExpr goast.Expr, scriptPos ast.Pos) (runeStmtMatch, error) {
	call, ok := rhsExpr.(*goast.CallExpr)
	if !ok || runeCallKind(call.Fun) != runeProps {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "let-destructure must be assigned from $props()"}
	}
	closeIdx := matchClosingBrace(lhs)
	if closeIdx < 0 {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: malformed destructure pattern"}
	}
	inner := lhs[1:closeIdx]
	tail := strings.TrimSpace(lhs[closeIdx+1:])
	annotation := ""
	if strings.HasPrefix(tail, ":") {
		annotation = strings.TrimSpace(tail[1:])
	}
	annotMap, err := parseAnnotationFields(annotation, scriptPos)
	if err != nil {
		return runeStmtMatch{}, err
	}

	entries := splitDestructureEntries(inner)

	var props []runeProp
	rest := ""
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "...") {
			name := pascalCase(strings.TrimSpace(strings.TrimPrefix(entry, "...")))
			props = append(props, runeProp{Name: name, Type: "map[string]any", Rest: true})
			rest = name
			continue
		}
		prop, err := parseDestructureEntry(entry, annotMap, scriptPos)
		if err != nil {
			return runeStmtMatch{}, err
		}
		props = append(props, prop)
	}
	return runeStmtMatch{kind: runeProps, letProps: props, letRest: rest}, nil
}

// parseDestructureEntry parses one destructure entry: `Name`, `Name = expr`,
// or `Name = $bindable(expr)`. Type is taken from the annotation map when
// the user supplied one; otherwise inferred from the default expression's
// literal kind, falling back to `string`.
func parseDestructureEntry(entry string, annotated map[string]string, scriptPos ast.Pos) (runeProp, error) {
	eq := findTopLevelEquals(entry)
	if eq < 0 {
		name := strings.TrimSpace(entry)
		if !isIdent(name) {
			return runeProp{}, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid prop name %q", name)}
		}
		typ := annotated[name]
		if typ == "" {
			typ = "string"
		}
		return runeProp{Name: pascalCase(name), Type: typ}, nil
	}
	name := strings.TrimSpace(entry[:eq])
	defExprSrc := strings.TrimSpace(entry[eq+1:])
	expr, err := parser.ParseExpr(defExprSrc)
	if err != nil {
		return runeProp{}, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid prop default %q: %v", defExprSrc, err)}
	}
	bindable := false
	valueExpr := expr
	if call, ok := expr.(*goast.CallExpr); ok && runeCallKind(call.Fun) == runeBindable {
		bindable = true
		switch len(call.Args) {
		case 0:
			valueExpr = nil
		case 1:
			valueExpr = call.Args[0]
		default:
			return runeProp{}, &CodegenError{Pos: scriptPos, Msg: "$bindable accepts at most one default argument"}
		}
	}
	typ, defaultStr := inferPropTypeAndDefault(valueExpr, annotated[name])
	return runeProp{
		Name:     pascalCase(name),
		Type:     typ,
		Default:  defaultStr,
		Bindable: bindable,
	}, nil
}

func inferPropTypeAndDefault(valueExpr goast.Expr, annotated string) (string, string) {
	if valueExpr == nil {
		if annotated != "" {
			return annotated, ""
		}
		return "string", ""
	}
	defaultStr := mustFormat(valueExpr)
	if annotated != "" {
		return annotated, defaultStr
	}
	return inferTypeFromExpr(valueExpr), defaultStr
}

func inferTypeFromExpr(e goast.Expr) string {
	switch v := e.(type) {
	case *goast.BasicLit:
		switch v.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "rune"
		}
	case *goast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return "bool"
		}
	case *goast.UnaryExpr:
		return inferTypeFromExpr(v.X)
	case *goast.CompositeLit:
		if v.Type != nil {
			return mustFormat(v.Type)
		}
	}
	return "any"
}

// parseEffectStmt accepts `$effect(fn)`, `$effect.pre(fn)`, and
// `$effect.root(fn)` calls and returns a runeEffect placeholder. Server
// emits nothing for these — they're client-only side-effects.
func parseEffectStmt(src string, scriptPos ast.Pos) (runeStmtMatch, error) {
	expr, err := parser.ParseExpr(strings.TrimSpace(src))
	if err != nil {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid $effect call: %v", err)}
	}
	call, ok := expr.(*goast.CallExpr)
	if !ok {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: $effect must be called"}
	}
	if runeCallKind(call.Fun) != runeEffect {
		return runeStmtMatch{}, &CodegenError{Pos: scriptPos, Msg: "<script>: unrecognized rune call"}
	}
	return runeStmtMatch{kind: runeEffect}, nil
}

// runeCallKind classifies a CallExpr.Fun against the rewritten rune
// placeholder names. SelectorExpr handles `$derived.by`, `$effect.pre`,
// and `$effect.root` shapes.
func runeCallKind(fun goast.Expr) runeKind {
	switch f := fun.(type) {
	case *goast.Ident:
		return runeNameKind(f.Name)
	case *goast.SelectorExpr:
		base, ok := f.X.(*goast.Ident)
		if !ok {
			return runeNone
		}
		baseKind := runeNameKind(base.Name)
		if baseKind == runeDerived && f.Sel != nil && f.Sel.Name == "by" {
			return runeDerivedBy
		}
		if baseKind == runeEffect && f.Sel != nil {
			return runeEffect
		}
	}
	return runeNone
}

func runeNameKind(name string) runeKind {
	if !strings.HasPrefix(name, runePrefix) {
		return runeNone
	}
	switch strings.TrimPrefix(name, runePrefix) {
	case "props":
		return runeProps
	case "state":
		return runeState
	case "derived":
		return runeDerived
	case "effect":
		return runeEffect
	case "bindable":
		return runeBindable
	}
	return runeNone
}

// matchClosingBrace returns the index of the `}` that closes the
// opening `{` at lhs[0], or -1 when the input is unbalanced. String,
// rune, and backtick-string literals are skipped.
func matchClosingBrace(lhs string) int {
	if len(lhs) == 0 || lhs[0] != '{' {
		return -1
	}
	depth := 0
	in := byte(0)
	for i := 0; i < len(lhs); i++ {
		c := lhs[i]
		if in != 0 {
			if c == '\\' && i+1 < len(lhs) {
				i++
				continue
			}
			if c == in {
				in = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			in = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitDestructureEntries splits the inside of `{ a, b = 1, ...rest }`
// at top-level commas, respecting paren / brace nesting and string
// literals.
func splitDestructureEntries(inner string) []string {
	var out []string
	depth := 0
	in := byte(0)
	last := 0
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if in != 0 {
			if c == '\\' && i+1 < len(inner) {
				i++
				continue
			}
			if c == in {
				in = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			in = c
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, inner[last:i])
				last = i + 1
			}
		}
	}
	out = append(out, inner[last:])
	return out
}

// splitNameAndAnnotation splits `name` or `name: T` LHS forms.
func splitNameAndAnnotation(lhs string, scriptPos ast.Pos) (string, string, error) {
	colon := strings.IndexByte(lhs, ':')
	if colon < 0 {
		if !isIdent(lhs) {
			return "", "", &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid let LHS %q", lhs)}
		}
		return lhs, "", nil
	}
	name := strings.TrimSpace(lhs[:colon])
	typ := strings.TrimSpace(lhs[colon+1:])
	if !isIdent(name) {
		return "", "", &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid let LHS %q", lhs)}
	}
	return name, typ, nil
}

// parseAnnotationFields parses `{ A T1; B T2 }` style type annotations
// into a map keyed by field name.
func parseAnnotationFields(annotation string, scriptPos ast.Pos) (map[string]string, error) {
	annotation = strings.TrimSpace(annotation)
	if annotation == "" {
		return nil, nil
	}
	expr, err := parser.ParseExpr("struct" + annotation)
	if err != nil {
		expr, err = parser.ParseExpr(annotation)
		if err != nil {
			return nil, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: invalid prop type annotation %q: %v", annotation, err)}
		}
	}
	st, ok := expr.(*goast.StructType)
	if !ok {
		return nil, &CodegenError{Pos: scriptPos, Msg: fmt.Sprintf("<script>: prop type annotation must be a struct shape, got %q", annotation)}
	}
	out := map[string]string{}
	for _, fd := range st.Fields.List {
		typeStr := mustFormat(fd.Type)
		for _, n := range fd.Names {
			out[n.Name] = typeStr
		}
	}
	return out, nil
}

// findTopLevelEquals returns the index of the first `=` that sits
// outside any string literal or matched paren/brace pair, ignoring `==`,
// `!=`, `:=`, `<=`, `>=`. Returns -1 when not found.
func findTopLevelEquals(s string) int {
	depth := 0
	in := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if in != 0 {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == in {
				in = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			in = c
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case '=':
			if depth != 0 {
				continue
			}
			if i+1 < len(s) && s[i+1] == '=' {
				i++
				continue
			}
			if i > 0 {
				prev := s[i-1]
				if prev == '!' || prev == ':' || prev == '<' || prev == '>' || prev == '=' {
					continue
				}
			}
			return i
		}
	}
	return -1
}

// parseScriptResidue runs the original go/parser-based extraction over
// the residue (script body with rune lines blanked out). It returns the
// non-rune declarations and imports.
func parseScriptResidue(residue string, scriptPos ast.Pos) ([]string, []string, error) {
	fileSrc := "package _x\n" + residue + "\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", fileSrc, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "expected declaration") {
			return nil, nil, &CodegenError{
				Pos: scriptPos,
				Msg: "<script> body must contain only imports, top-level declarations, or rune statements",
			}
		}
		return nil, nil, &CodegenError{
			Pos: scriptPos,
			Msg: fmt.Sprintf("invalid Go in <script>: %v", err),
		}
	}

	var decls []string
	var imports []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*goast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			for _, spec := range gen.Specs {
				is, ok := spec.(*goast.ImportSpec)
				if !ok {
					continue
				}
				rendered, err := formatImportSpec(fset, is)
				if err != nil {
					return nil, nil, &CodegenError{Pos: scriptPos, Msg: err.Error()}
				}
				imports = append(imports, rendered)
			}
			continue
		}
		rendered, err := formatNode(fset, decl)
		if err != nil {
			return nil, nil, &CodegenError{Pos: scriptPos, Msg: err.Error()}
		}
		decls = append(decls, rendered)
	}
	return decls, imports, nil
}

// pascalCase upper-cases the first ASCII letter of name. Identifiers
// already PascalCase pass through unchanged.
func pascalCase(name string) string {
	if name == "" {
		return name
	}
	first := name[0]
	if first >= 'a' && first <= 'z' {
		return string(first-32) + name[1:]
	}
	return name
}

// mustFormat renders n with go/printer; on error the empty string is
// returned. Errors only occur for malformed AST nodes that the parser
// already accepted, so the loss is informational at worst.
func mustFormat(n goast.Node) string {
	out, err := formatNode(token.NewFileSet(), n)
	if err != nil {
		return ""
	}
	return out
}
