package typegen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestMapType_Primitives covers the row-by-row mapping table from
// RFC #379. Each case parses a tiny Go expression as a type and runs
// the mapper directly. Anything that touches a referenced type decl
// is covered by the walker-level golden tests.
func TestMapType_Primitives(t *testing.T) {
	t.Parallel()
	cases := []struct {
		goExpr string
		wantTS string
	}{
		{"string", "string"},
		{"int", "number"},
		{"int32", "number"},
		{"int64", "number"},
		{"uint", "number"},
		{"uint8", "number"},
		{"float32", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"any", "unknown"},
		{"[]string", "string[]"},
		{"[]int", "number[]"},
		{"[][]bool", "boolean[][]"},
		{"map[string]int", "Record<string, number>"},
		{"map[string]string", "Record<string, string>"},
		{"*string", "string | null"},
		{"*int", "number | null"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.goExpr, func(t *testing.T) {
			t.Parallel()
			expr, err := parser.ParseExpr(tc.goExpr)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			r := newSyntheticResolver()
			got, _ := r.mapType(expr)
			if got != tc.wantTS {
				t.Fatalf("%s: got %q want %q", tc.goExpr, got, tc.wantTS)
			}
		})
	}
}

// TestMapType_TimeTime asserts the time.Time selector maps to string.
// Using a hand-built AST avoids depending on a parsed file context.
func TestMapType_TimeTime(t *testing.T) {
	t.Parallel()
	expr := &ast.SelectorExpr{X: &ast.Ident{Name: "time"}, Sel: &ast.Ident{Name: "Time"}}
	r := newSyntheticResolver()
	got, _ := r.mapType(expr)
	if got != "string" {
		t.Fatalf("got %q want %q", got, "string")
	}
}

// TestMapType_Streamed asserts kit.Streamed[T] maps to Promise<T[]>.
// The generic instantiation goes through *ast.IndexExpr.
func TestMapType_Streamed(t *testing.T) {
	t.Parallel()
	expr := &ast.IndexExpr{
		X: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "kit"},
			Sel: &ast.Ident{Name: "Streamed"},
		},
		Index: &ast.Ident{Name: "string"},
	}
	r := newSyntheticResolver()
	got, _ := r.mapType(expr)
	if got != "Promise<string[]>" {
		t.Fatalf("got %q want %q", got, "Promise<string[]>")
	}
}

// TestQuoteIfNeeded covers the property-name quoting in the emitter.
// Bare identifiers stay unquoted; anything else gets wrapped.
func TestQuoteIfNeeded(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"name":      "name",
		"camelCase": "camelCase",
		"_under":    "_under",
		"$dollar":   "$dollar",
		"with-dash": `"with-dash"`,
		"with.dot":  `"with.dot"`,
		"":          `""`,
		"123start":  `"123start"`,
	}
	for in, want := range cases {
		if got := quoteIfNeeded(in); got != want {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", in, got, want)
		}
	}
}

// newSyntheticResolver builds a resolver against an empty file so
// mapper-level tests can exercise the type table without a real
// fixture. recordNamedType degrades to a no-op because findStructDecl
// returns nil for any name in this empty file.
func newSyntheticResolver() *structResolver {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "synthetic.go", "package x\n", parser.SkipObjectResolution)
	return &structResolver{file: f, filePath: "synthetic.go", named: map[string]NamedType{}}
}
