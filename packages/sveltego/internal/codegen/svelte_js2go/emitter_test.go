package sveltejs2go

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sidecarCases are the canonical end-to-end fixtures the Phase 2
// sidecar produced. These test the emitter against bytes that
// `sveltego build` will hand it at request time.
var sidecarCases = []string{
	"hello-world",
	"each-list",
}

func TestTranspile_SidecarFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range sidecarCases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("..", "svelterender", "testdata", "ssr-ast", name, "ast.golden.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got, err := Transpile(data, Options{PackageName: "gen"})
			if err != nil {
				t.Fatalf("Transpile: %v", err)
			}
			if _, ferr := format.Source(got); ferr != nil {
				t.Fatalf("not parseable Go: %v\n%s", ferr, got)
			}
			assertGolden(t, "sidecar/"+name, got)
		})
	}
}

// programmaticCases drive the emitter from synthesized Node trees so
// the tests don't depend on running the Node sidecar. Each case
// captures one of the 30 priority emit shapes.
type programmaticCase struct {
	name string
	root func() *Node
}

func TestTranspile_Programmatic(t *testing.T) {
	t.Parallel()
	cases := allProgrammaticCases()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := TranspileNode(tc.root(), "/test/"+tc.name, Options{PackageName: "gen"})
			if err != nil {
				t.Fatalf("TranspileNode: %v", err)
			}
			if _, ferr := format.Source(got); ferr != nil {
				t.Fatalf("not parseable Go: %v\n%s", ferr, got)
			}
			assertGolden(t, "shapes/"+tc.name, got)
		})
	}
}

// TestTranspile_Extended exercises the Phase 7 (#429) corpus expansion:
// 50+ shape variants on top of the 30 priority cases, pushing coverage
// toward the v1 ~95% target from RFC #421.
func TestTranspile_Extended(t *testing.T) {
	t.Parallel()
	cases := extendedProgrammaticCases()
	if len(cases) < 50 {
		t.Fatalf("extended corpus has %d cases, want ≥50", len(cases))
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := TranspileNode(tc.root(), "/test/"+tc.name, Options{PackageName: "gen"})
			if err != nil {
				t.Fatalf("TranspileNode: %v", err)
			}
			if _, ferr := format.Source(got); ferr != nil {
				t.Fatalf("not parseable Go: %v\n%s", ferr, got)
			}
			assertGolden(t, "shapes_extended/"+tc.name, got)
		})
	}
}

// TestCorpus_Total asserts the combined priority + extended + sidecar
// fixture count meets the ≥80 acceptance bar from issue #429.
func TestCorpus_Total(t *testing.T) {
	t.Parallel()
	priority := allProgrammaticCases()
	extended := extendedProgrammaticCases()
	total := len(priority) + len(extended) + len(sidecarCases)
	if total < 80 {
		t.Fatalf("total corpus = %d, acceptance bar from #429 is ≥80", total)
	}
}

func TestTranspile_HTMLDeferred(t *testing.T) {
	t.Parallel()
	// {@html raw} is intentionally deferred per Phase 4 quirks.
	root := buildProgram(buildBlock(
		callStmt(memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("html")), ident("raw")),
		),
	))
	_, err := TranspileNode(root, "/test/html-deferred", Options{})
	if err == nil {
		t.Fatal("expected unknown shape error for $.html")
	}
	if !strings.Contains(err.Error(), "unknown emit shape") {
		t.Fatalf("want unknown emit shape, got %q", err.Error())
	}
}

func TestTranspile_Determinism(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "svelterender", "testdata", "ssr-ast", "hello-world", "ast.golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	a, err := Transpile(data, Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := Transpile(data, Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic output:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
}

func TestTranspile_UnknownShape(t *testing.T) {
	t.Parallel()
	// LabeledStatement at top level — outside the v1 closed set.
	raw := []byte(`{"schema":"ssr-json-ast/v1","route":"/x","ast":{"type":"Program","sourceType":"module","body":[{"type":"LabeledStatement","label":{"type":"Identifier","name":"x"}}],"start":0,"end":0}}`)
	_, err := Transpile(raw, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown emit shape") {
		t.Fatalf("want 'unknown emit shape' in %q", err.Error())
	}
}

func TestTranspile_SchemaMismatch(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"schema":"ssr-json-ast/v999","route":"/x","ast":{"type":"Program","sourceType":"module","body":[]}}`)
	_, err := Transpile(raw, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("want 'unsupported schema' in %q", err.Error())
	}
}

func TestTranspile_NoExportDefault(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock())
	// Hand-edit: drop the export default so we hit the missing-render
	// error path.
	root.Body = root.Body[:1]
	_, err := TranspileNode(root, "/x", Options{})
	if err == nil {
		t.Fatal("expected error on missing render fn")
	}
}

func TestCompanionFile_Compiles(t *testing.T) {
	t.Parallel()
	src := CompanionFile("gen")
	if _, err := format.Source(src); err != nil {
		t.Fatalf("companion does not parse: %v\n%s", err, src)
	}
}

func TestExprRewriter_Hook(t *testing.T) {
	t.Parallel()
	// Exercise the Phase 5 hook: a rewriter that uppercases the
	// trailing property of MemberExpressions and verifies the
	// emitter calls it.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(
				memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("name")),
			),
		),
	))
	hook := titleCaseRewriter{}
	got, err := TranspileNode(root, "/x", Options{Rewriter: hook})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !bytes.Contains(got, []byte("data.Name")) {
		t.Fatalf("rewriter not applied; got:\n%s", got)
	}
}

type titleCaseRewriter struct{}

func (titleCaseRewriter) Rewrite(_ *Scope, n *Node, def string) string {
	if n.Type == "MemberExpression" && n.Property != nil && n.Property.Type == "Identifier" {
		name := n.Property.Name
		if name == "" {
			return ""
		}
		upper := strings.ToUpper(name[:1]) + name[1:]
		// Re-render manually using the def's prefix up to the dot.
		if dot := strings.LastIndexByte(def, '.'); dot >= 0 {
			return def[:dot+1] + upper
		}
	}
	return ""
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".golden.go")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch %s; run GOLDEN_UPDATE=1\n--- want:\n%s\n--- got:\n%s", path, want, got)
	}
}
