package ast

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestPosString(t *testing.T) {
	cases := []struct {
		name string
		in   Pos
		want string
	}{
		{"origin", Pos{Offset: 0, Line: 1, Col: 1}, "1:1"},
		{"deep", Pos{Offset: 42, Line: 7, Col: 13}, "7:13"},
		{"zero", Pos{}, "0:0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestPosIsValid(t *testing.T) {
	cases := []struct {
		name string
		in   Pos
		want bool
	}{
		{"origin", Pos{Line: 1, Col: 1}, true},
		{"zero", Pos{}, false},
		{"missing col", Pos{Line: 1}, false},
		{"missing line", Pos{Col: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.IsValid(); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestParseErrorFormat(t *testing.T) {
	cases := []struct {
		name string
		in   ParseError
		want string
	}{
		{
			name: "no hint",
			in:   ParseError{Pos: Pos{Line: 3, Col: 5}, Message: "unexpected token"},
			want: "3:5: unexpected token",
		},
		{
			name: "with hint",
			in:   ParseError{Pos: Pos{Line: 1, Col: 1}, Message: "missing closing brace", Hint: "did you forget }?"},
			want: "1:1: missing closing brace (hint: did you forget }?)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Error(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestErrorsAggregate(t *testing.T) {
	empty := Errors{}
	if got := empty.Error(); got != "" {
		t.Fatalf("empty Errors should stringify empty, got %q", got)
	}
	if err := empty.ErrorOrNil(); err != nil {
		t.Fatalf("empty Errors.ErrorOrNil should be nil, got %v", err)
	}

	one := Errors{{Pos: Pos{Line: 2, Col: 1}, Message: "boom"}}
	if got, want := one.Error(), "2:1: boom"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	multi := Errors{
		{Pos: Pos{Line: 1, Col: 1}, Message: "first"},
		{Pos: Pos{Line: 2, Col: 4}, Message: "second", Hint: "fix it"},
	}
	want := "1:1: first\n2:4: second (hint: fix it)"
	if got := multi.Error(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	err := multi.ErrorOrNil()
	if err == nil {
		t.Fatal("non-empty Errors.ErrorOrNil should not be nil")
	}
	var es Errors
	if !errors.As(err, &es) {
		t.Fatalf("ErrorOrNil should preserve Errors via errors.As, got %T", err)
	}
}

// syntheticTree returns a Fragment touching every concrete Node type so a
// single Walk invocation can prove visitor coverage.
func syntheticTree() *Fragment {
	pos := Pos{Offset: 0, Line: 1, Col: 1}
	interp := &InterpolatedValue{P: pos, Parts: []Node{
		&Text{P: pos, Value: "card-"},
		&Mustache{P: pos, Expr: "Theme"},
	}}
	elem := &Element{
		P:    pos,
		Name: "div",
		Attributes: []Attribute{
			{P: pos, Name: "id", Kind: AttrStatic, Value: &StaticValue{P: pos, Value: "root"}},
			{P: pos, Name: "data-theme", Kind: AttrDynamic, Value: &DynamicValue{P: pos, Expr: "Theme"}},
			{P: pos, Name: "class", Kind: AttrStatic, Value: interp},
			{P: pos, Name: "click", Kind: AttrEventHandler, Modifier: "click"},
		},
		Children: []Node{
			&Comment{P: pos, Value: "hello"},
			&Text{P: pos, Value: "x"},
			&IfBlock{
				P:    pos,
				Cond: "Open",
				Then: []Node{&Mustache{P: pos, Expr: "Title"}},
				Elifs: []ElifBranch{
					{P: pos, Cond: "Loading", Body: []Node{&RawHTML{P: pos, Expr: "Spinner"}}},
				},
				Else: []Node{&Const{P: pos, Stmt: "n := len(Items)"}},
			},
			&EachBlock{
				P:    pos,
				Iter: "Items",
				Item: "item",
				Body: []Node{&Render{P: pos, Expr: "row(item)"}},
				Else: []Node{&Text{P: pos, Value: "empty"}},
			},
			&AwaitBlock{
				P:        pos,
				Expr:     "Fetch()",
				Pending:  []Node{&Text{P: pos, Value: "..."}},
				Then:     []Node{&Text{P: pos, Value: "ok"}},
				ThenVar:  "v",
				Catch:    []Node{&Text{P: pos, Value: "err"}},
				CatchVar: "e",
			},
			&KeyBlock{P: pos, Key: "ID", Body: []Node{&Text{P: pos, Value: "k"}}},
			&SnippetBlock{P: pos, Name: "row", Params: "item Item", Body: []Node{&Text{P: pos, Value: "r"}}},
		},
	}
	return &Fragment{P: pos, Children: []Node{
		elem,
		&Script{P: pos, Lang: "go", Body: "var x int"},
		&Style{P: pos, Body: ".x{}"},
	}}
}

type recordingVisitor struct {
	seen map[string]int
}

func (r *recordingVisitor) Visit(n Node) Visitor {
	if n == nil {
		return r
	}
	r.seen[reflect.TypeOf(n).String()]++
	return r
}

func TestWalkVisitsEveryNodeType(t *testing.T) {
	tree := syntheticTree()
	rv := &recordingVisitor{seen: map[string]int{}}
	Walk(rv, tree)

	want := []string{
		"*ast.Fragment",
		"*ast.Element",
		"*ast.StaticValue",
		"*ast.DynamicValue",
		"*ast.InterpolatedValue",
		"*ast.Text",
		"*ast.Mustache",
		"*ast.Comment",
		"*ast.IfBlock",
		"*ast.RawHTML",
		"*ast.Const",
		"*ast.EachBlock",
		"*ast.Render",
		"*ast.AwaitBlock",
		"*ast.KeyBlock",
		"*ast.SnippetBlock",
		"*ast.Script",
		"*ast.Style",
	}
	for _, name := range want {
		if rv.seen[name] == 0 {
			t.Errorf("Walk did not visit %s", name)
		}
	}
}

type pruner struct{ visited int }

func (p *pruner) Visit(n Node) Visitor {
	if n == nil {
		return p
	}
	p.visited++
	if _, ok := n.(*Element); ok {
		return nil
	}
	return p
}

func TestWalkPrunesOnNilVisitor(t *testing.T) {
	tree := syntheticTree()
	p := &pruner{}
	Walk(p, tree)
	// Pruning Element skips its subtree but Script and Style siblings still
	// get visited: Fragment, Element, Script, Style.
	if p.visited != 4 {
		t.Fatalf("expected pruner to count 4 top-level nodes, got %d", p.visited)
	}
}

func TestJSONRoundTripFragment(t *testing.T) {
	original := &Fragment{
		P: Pos{Offset: 0, Line: 1, Col: 1},
		Children: []Node{
			&Text{P: Pos{Line: 1, Col: 1}, Value: "hello "},
			&Mustache{P: Pos{Line: 1, Col: 7}, Expr: "Name"},
		},
	}

	first, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}

	// Children are interfaces, so unmarshaling without type info loses
	// concrete types. The contract from issue #8 is "stable across runs":
	// re-marshaling the original must yield byte-identical output, and a
	// shallow round-trip into a parallel struct preserves field values.
	second, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("non-deterministic Marshal:\nfirst:  %s\nsecond: %s", first, second)
	}

	type textShape struct {
		P     Pos
		Value string
	}
	type fragShape struct {
		P        Pos
		Children []json.RawMessage
	}
	var decoded fragShape
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.P != original.P {
		t.Fatalf("Pos mismatch: got %+v want %+v", decoded.P, original.P)
	}
	if len(decoded.Children) != len(original.Children) {
		t.Fatalf("children len: got %d want %d", len(decoded.Children), len(original.Children))
	}

	var gotText textShape
	if err := json.Unmarshal(decoded.Children[0], &gotText); err != nil {
		t.Fatalf("unmarshal text child: %v", err)
	}
	wantText := textShape{P: Pos{Line: 1, Col: 1}, Value: "hello "}
	if !reflect.DeepEqual(gotText, wantText) {
		t.Fatalf("text child: got %+v want %+v", gotText, wantText)
	}
}

func TestAttrKindString(t *testing.T) {
	cases := []struct {
		in   AttrKind
		want string
	}{
		{AttrStatic, "Static"},
		{AttrDynamic, "Dynamic"},
		{AttrEventHandler, "EventHandler"},
		{AttrBind, "Bind"},
		{AttrUse, "Use"},
		{AttrClassDirective, "ClassDirective"},
		{AttrStyleDirective, "StyleDirective"},
		{AttrKind(99), "Unknown"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("AttrKind(%d): got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestAttributePos(t *testing.T) {
	a := Attribute{P: Pos{Line: 4, Col: 2}, Name: "id"}
	if got := a.Pos(); got != a.P {
		t.Fatalf("Attribute.Pos mismatch: got %+v want %+v", got, a.P)
	}
	e := ElifBranch{P: Pos{Line: 5, Col: 3}, Cond: "x"}
	if got := e.Pos(); got != e.P {
		t.Fatalf("ElifBranch.Pos mismatch: got %+v want %+v", got, e.P)
	}
}
