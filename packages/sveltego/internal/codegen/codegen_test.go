package codegen

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/testutils/golden"
)

func TestQuoteGo(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", `""`},
		{"hello", "`hello`"},
		{"with \"quote\"", "`with \"quote\"`"},
		{"\ttabbed\n", "`\ttabbed\n`"},
		{"with `back` tick", "\"with `back` tick\""},
		{"`only`", "\"`only`\""},
		{"```", "\"```\""},
		{"only\rcr", "\"only\\rcr\""},
		{"a`b\rc", "\"a`b\\rc\""},
	}
	for _, tc := range cases {
		got := quoteGo(tc.in)
		if got != tc.want {
			t.Errorf("quoteGo(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestMergeAdjacentText(t *testing.T) {
	mk := func(s string) *ast.Text { return &ast.Text{Value: s} }
	el := &ast.Element{Name: "div"}
	mu := &ast.Mustache{Expr: "x"}

	t.Run("empty", func(t *testing.T) {
		got := mergeAdjacentText(nil)
		if got != nil {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("single text untouched", func(t *testing.T) {
		got := mergeAdjacentText([]ast.Node{mk("hi")})
		if len(got) != 1 || got[0].(*ast.Text).Value != "hi" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("merges run", func(t *testing.T) {
		got := mergeAdjacentText([]ast.Node{mk("a"), mk("b"), mk("c")})
		if len(got) != 1 || got[0].(*ast.Text).Value != "abc" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("element splits run", func(t *testing.T) {
		got := mergeAdjacentText([]ast.Node{mk("a"), mk("b"), el, mk("c")})
		if len(got) != 3 {
			t.Fatalf("len=%d, %#v", len(got), got)
		}
		if got[0].(*ast.Text).Value != "ab" {
			t.Fatalf("first: %#v", got[0])
		}
		if got[2].(*ast.Text).Value != "c" {
			t.Fatalf("third: %#v", got[2])
		}
	})

	t.Run("mustache splits run", func(t *testing.T) {
		got := mergeAdjacentText([]ast.Node{mk("a"), mu, mk("b"), mk("c")})
		if len(got) != 3 {
			t.Fatalf("len=%d, %#v", len(got), got)
		}
		if got[2].(*ast.Text).Value != "bc" {
			t.Fatalf("third: %#v", got[2])
		}
	})

	t.Run("does not mutate input", func(t *testing.T) {
		first := mk("a")
		original := first.Value
		_ = mergeAdjacentText([]ast.Node{first, mk("b")})
		if first.Value != original {
			t.Fatalf("input mutated: %q -> %q", original, first.Value)
		}
	})
}

func TestValidateExpr(t *testing.T) {
	pos := ast.Pos{Line: 1, Col: 1}
	good := []string{
		"x",
		"user.Name",
		"len(items)",
		"a + b",
		"[]int{1, 2, 3}",
		"struct{ X int }{X: 1}",
		`"hello"`,
		"f(g(h))",
	}
	for _, src := range good {
		if err := validateExpr(src, pos); err != nil {
			t.Errorf("validateExpr(%q) = %v; want nil", src, err)
		}
	}

	bad := []string{
		"",
		" ",
		"x +",
		"return x",
		"x = 1",
	}
	for _, src := range bad {
		err := validateExpr(src, pos)
		if err == nil {
			t.Errorf("validateExpr(%q) = nil; want error", src)
			continue
		}
		var ce *CodegenError
		if !errors.As(err, &ce) {
			t.Errorf("validateExpr(%q) returned %T, want *CodegenError", src, err)
			continue
		}
		if !strings.Contains(ce.Msg, "invalid Go expression") {
			t.Errorf("validateExpr(%q) msg = %q; want substring %q", src, ce.Msg, "invalid Go expression")
		}
	}
}

func TestValidateStmt(t *testing.T) {
	pos := ast.Pos{Line: 2, Col: 3}
	good := []string{
		"x := 1",
		"x, y := 1, 2",
		"_ = x",
	}
	for _, src := range good {
		if err := validateStmt(src, pos); err != nil {
			t.Errorf("validateStmt(%q) = %v; want nil", src, err)
		}
	}

	bad := []string{
		"",
		"x +",
	}
	for _, src := range bad {
		if err := validateStmt(src, pos); err == nil {
			t.Errorf("validateStmt(%q) = nil; want error", src)
		}
	}
}

func TestGenerate_RejectsRootConst(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Const{P: ast.Pos{Line: 3, Col: 5}, Stmt: "x := 1"},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if ce.Pos.Line != 3 || ce.Pos.Col != 5 {
		t.Errorf("pos = %v, want 3:5", ce.Pos)
	}
	if !strings.Contains(ce.Msg, "{@const} not allowed at template root") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "{@const} not allowed at template root")
	}
}

func TestGenerateLayout_RejectsRootConst(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Const{P: ast.Pos{Line: 1, Col: 1}, Stmt: "x := 1"},
		},
	}
	_, err := GenerateLayout(frag, LayoutOptions{PackageName: "layout"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
}

func TestGenerate_AwaitPendingDiagnostic(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.AwaitBlock{
				P:       ast.Pos{Line: 4, Col: 2},
				Expr:    "fetch()",
				Pending: []ast.Node{&ast.Text{Value: "loading"}},
				Then:    []ast.Node{&ast.Text{Value: "ok"}},
				ThenVar: "v",
			},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if ce.Pos.Line != 4 || ce.Pos.Col != 2 {
		t.Errorf("pos = %v, want 4:2", ce.Pos)
	}
	if !strings.Contains(ce.Msg, "pending branch unsupported") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "pending branch unsupported")
	}
}

func TestGenerate_AwaitWhitespacePendingAccepted(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.AwaitBlock{
				P:       ast.Pos{Line: 1, Col: 1},
				Expr:    "fetch()",
				Pending: []ast.Node{&ast.Text{Value: "  \n\t"}},
				Then:    []ast.Node{&ast.Text{Value: "ok"}},
				ThenVar: "v",
			},
		},
	}
	if _, err := Generate(frag, Options{PackageName: "page"}); err != nil {
		t.Fatalf("whitespace pending should be tolerated, got %v", err)
	}
}

func TestGenerate_AwaitRequiresThenOrCatch(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.AwaitBlock{P: ast.Pos{Line: 1, Col: 1}, Expr: "fetch()"},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) || !strings.Contains(ce.Msg, "requires at least") {
		t.Fatalf("got %v, want CodegenError with 'requires at least'", err)
	}
}

func TestGenerate_KeyCounterIncrements(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.KeyBlock{P: ast.Pos{Line: 1, Col: 1}, Key: "A", Body: []ast.Node{&ast.Text{Value: "x"}}},
			&ast.KeyBlock{P: ast.Pos{Line: 1, Col: 1}, Key: "B", Body: []ast.Node{&ast.Text{Value: "y"}}},
		},
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)
	for _, want := range []string{"sgkey:0:A", "/sgkey:0", "sgkey:1:B", "/sgkey:1"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in:\n%s", want, src)
		}
	}
}

func TestFixtures(t *testing.T) {
	root := "testdata/codegen"
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "layout" || d.Name() == "errorpage" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".svelte") {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(matches) < 60 {
		t.Fatalf("expected >= 60 fixtures, found %d", len(matches))
	}
	sort.Strings(matches)
	for _, path := range matches {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".svelte")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			errGoldenPath := filepath.Join("testdata/golden/codegen", name+".error.golden")
			_, hasErrGolden := os.Stat(errGoldenPath)
			frag, errs := parser.Parse(src)
			if len(errs) > 0 {
				if hasErrGolden == nil {
					assertErrorGolden(t, errGoldenPath, errs)
					return
				}
				t.Fatalf("parse errors: %v", errs)
			}
			opts := Options{PackageName: "page"}
			serverPath := strings.TrimSuffix(path, ".svelte") + ".server.go"
			if _, statErr := os.Stat(serverPath); statErr == nil {
				opts.ServerFilePath = serverPath
			}
			out, genErr := Generate(frag, opts)

			if hasErrGolden == nil {
				assertErrorGolden(t, errGoldenPath, genErr)
				return
			}
			if genErr != nil {
				t.Fatalf("generate: %v", genErr)
			}
			golden.EqualString(t, "codegen/"+name+".gen.go", string(out))
		})
	}
}

// assertErrorGolden compares the codegen error against a stored prefix in
// testdata/golden/codegen/<name>.error.golden. The comparison is a
// substring match because go/parser's exact wording is not stable across
// Go releases; the prefix locks down the line:col + the framework-owned
// message ("invalid Go expression").
func assertErrorGolden(t *testing.T, path string, got error) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected codegen error, got nil")
	}
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		want := got.Error()
		if i := strings.Index(want, ": invalid Go expression"); i >= 0 {
			want = want[:i+len(": invalid Go expression")]
		} else if i := strings.Index(want, ": invalid Go statement"); i >= 0 {
			want = want[:i+len(": invalid Go statement")]
		}
		if err := os.WriteFile(path, []byte(want+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	want := strings.TrimRight(string(raw), "\n")
	if !strings.Contains(got.Error(), want) {
		t.Fatalf("error mismatch\n want substring: %q\n got: %q", want, got.Error())
	}
}
