package sveltejs2go

import (
	"bytes"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// shape returns a typegen Shape with one root struct and any nested
// types the test wants. Pure helper — keeps the lowering test cases
// readable without touching the typegen package's parser path.
func shape(root string, types ...typegen.ShapeType) *typegen.Shape {
	m := map[string]typegen.ShapeType{}
	for _, t := range types {
		m[t.Name] = t
	}
	if _, ok := m[root]; !ok {
		// Caller forgot the root entry; default to empty fields so
		// the test-side mistake surfaces as "not present in tag map"
		// instead of a nil-pointer panic.
		m[root] = typegen.ShapeType{Name: root}
	}
	return &typegen.Shape{RootType: root, Types: m}
}

// pageDataNested is the fixture-equivalent shape for a route with
// PageData → User struct → primitive fields. Mirrors
// typegen/testdata/nested.
func pageDataNested() *typegen.Shape {
	return shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "user", GoName: "User", GoType: "User", NamedType: "User"},
			{Name: "posts", GoName: "Posts", GoType: "[]Post", NamedType: "Post", Slice: true},
		}},
		typegen.ShapeType{Name: "User", Fields: []typegen.Field{
			{Name: "id", GoName: "ID", GoType: "string"},
			{Name: "name", GoName: "Name", GoType: "string"},
			{Name: "email", GoName: "Email", GoType: "string"},
		}},
		typegen.ShapeType{Name: "Post", Fields: []typegen.Field{
			{Name: "id", GoName: "ID", GoType: "string"},
			{Name: "title", GoName: "Title", GoType: "string"},
		}},
	)
}

// snakeCaseShape covers the `email_address → EmailAddress` round-trip
// the issue's acceptance criteria call out.
func snakeCaseShape() *typegen.Shape {
	return shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "email_address", GoName: "EmailAddress", GoType: "string"},
			{Name: "display_name", GoName: "Renamed", GoType: "string"},
		}},
	)
}

// optionalShape models a route whose root type has an optional
// pointer field that lowering must guard against nil.
func optionalShape() *typegen.Shape {
	return shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "user", GoName: "User", GoType: "*User", NamedType: "User", Pointer: true},
		}},
		typegen.ShapeType{Name: "User", Fields: []typegen.Field{
			{Name: "profile", GoName: "Profile", GoType: "*Profile", NamedType: "Profile", Pointer: true},
		}},
		typegen.ShapeType{Name: "Profile", Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
		}},
	)
}

// transpileWith runs Transpile against root with a Lowerer wired into
// Options. Returns the rendered Go source plus the accumulated
// lowering error, both for the assertion site to inspect.
func transpileWith(t *testing.T, root *Node, lo *Lowerer) ([]byte, error) {
	t.Helper()
	got, err := TranspileNode(root, "/test/lower", Options{
		PackageName: "gen",
		Rewriter:    lo,
	})
	return got, err
}

func TestLowerer_DataNameToData_Name(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("name")),
			),
		),
	))
	lo := NewLowerer(shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
		}},
	), LowererOptions{Route: "/test/lower", Strict: true})

	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	if !bytes.Contains(got, []byte("data.Name")) {
		t.Fatalf("want data.Name in output:\n%s", got)
	}
	if bytes.Contains(got, []byte("data.name")) {
		t.Fatalf("output still contains JS-style data.name:\n%s", got)
	}
}

func TestLowerer_NestedChain(t *testing.T) {
	t.Parallel()
	// data.user.name → data.User.Name.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(memExpr(ident("data"), ident("user")), ident("name")),
			),
		),
	))
	lo := NewLowerer(pageDataNested(), LowererOptions{Route: "/p", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !bytes.Contains(got, []byte("data.User.Name")) {
		t.Fatalf("want data.User.Name in output:\n%s", got)
	}
}

func TestLowerer_SnakeCaseTagToCamelGo(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("email_address")),
			),
		),
	))
	lo := NewLowerer(snakeCaseShape(), LowererOptions{Route: "/p", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !bytes.Contains(got, []byte("data.EmailAddress")) {
		t.Fatalf("want data.EmailAddress:\n%s", got)
	}
}

func TestLowerer_RenamedTagToGoFieldName(t *testing.T) {
	t.Parallel()
	// JSON tag display_name → Go field Renamed (per testdata/jsontag).
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("display_name")),
			),
		),
	))
	lo := NewLowerer(snakeCaseShape(), LowererOptions{Route: "/p", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !bytes.Contains(got, []byte("data.Renamed")) {
		t.Fatalf("want data.Renamed (JSON tag → Go field):\n%s", got)
	}
}

func TestLowerer_MissingFieldHardErrors(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(memExpr(ident("data"), ident("user")), ident("phone_number")),
			),
		),
	))
	lo := NewLowerer(pageDataNested(), LowererOptions{Route: "/r", Strict: true})
	_, err := transpileWith(t, root, lo)
	if err != nil {
		// TranspileNode itself succeeds — Lowerer surfaces missing-field
		// errors via Err(); the emitter only fails on shape unknowns.
		t.Fatalf("unexpected emit error: %v", err)
	}
	got := lo.Err()
	if got == nil {
		t.Fatal("expected lowering error for missing field")
	}
	msg := got.Error()
	if !strings.Contains(msg, "phone_number") {
		t.Errorf("error missing JSON tag name: %s", msg)
	}
	if !strings.Contains(msg, "User JSON tag map") {
		t.Errorf("error missing parent type: %s", msg)
	}
	if !strings.Contains(msg, "/r:byte=") {
		t.Errorf("error missing route + byte position: %s", msg)
	}
	if !strings.Contains(msg, "// sveltego:ssr-fallback") {
		t.Errorf("error missing fallback hint: %s", msg)
	}
}

func TestLowerer_UnknownRootHardErrors(t *testing.T) {
	t.Parallel()
	// `mystery.thing` — root not in scope, not the data root.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("mystery"), ident("thing")),
			),
		),
	))
	lo := NewLowerer(pageDataNested(), LowererOptions{Route: "/r", Strict: true})
	_, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("unexpected emit error: %v", err)
	}
	got := lo.Err()
	if got == nil {
		t.Fatal("expected lowering error for unknown root")
	}
	msg := got.Error()
	if !strings.Contains(msg, "mystery") {
		t.Errorf("error missing root name: %s", msg)
	}
	if !strings.Contains(msg, "neither in scope nor a recognised data root") {
		t.Errorf("error missing diagnosis: %s", msg)
	}
}

func TestLowerer_ComputedAccessHardErrors(t *testing.T) {
	t.Parallel()
	// `data["name"]` — computed access.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				computedMember(ident("data"), strLit("name")),
			),
		),
	))
	lo := NewLowerer(shape("PageData", typegen.ShapeType{
		Name: "PageData",
		Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
		},
	}), LowererOptions{Route: "/r", Strict: true})
	_, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("unexpected emit error: %v", err)
	}
	got := lo.Err()
	if got == nil {
		t.Fatal("expected hard error for computed access")
	}
	msg := got.Error()
	if !strings.Contains(msg, "computed access not supported") {
		t.Errorf("missing computed-access reason: %s", msg)
	}
	if !strings.Contains(msg, "// sveltego:ssr-fallback") {
		t.Errorf("missing fallback hint: %s", msg)
	}
}

func TestLowerer_OptionalChainSingleLink(t *testing.T) {
	t.Parallel()
	// data.user?.profile → guard data.User.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				chain(optionalMember(
					memExpr(ident("data"), ident("user")),
					ident("profile"),
				)),
			),
		),
	))
	lo := NewLowerer(optionalShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	src := string(got)
	if !strings.Contains(src, "if data.User == nil") {
		t.Errorf("missing guard for data.User:\n%s", src)
	}
	if !strings.Contains(src, "return data.User.Profile") {
		t.Errorf("missing return for guarded chain:\n%s", src)
	}
}

func TestLowerer_OptionalChainDeep(t *testing.T) {
	t.Parallel()
	// data.user?.profile?.name → cascading guards.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				chain(optionalMember(
					optionalMember(
						memExpr(ident("data"), ident("user")),
						ident("profile"),
					),
					ident("name"),
				)),
			),
		),
	))
	lo := NewLowerer(optionalShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	src := string(got)
	if !strings.Contains(src, "if data.User == nil") {
		t.Errorf("missing first guard:\n%s", src)
	}
	if !strings.Contains(src, "if data.User.Profile == nil") {
		t.Errorf("missing second guard:\n%s", src)
	}
	if !strings.Contains(src, "return data.User.Profile.Name") {
		t.Errorf("missing return:\n%s", src)
	}
}

func TestLowerer_NoShape_NonStrict_PassThrough(t *testing.T) {
	t.Parallel()
	// Without a shape, the lowerer must be a no-op so existing Phase 3
	// goldens keep their pre-Phase-5 rendering.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("name")),
			),
		),
	))
	lo := NewLowerer(nil, LowererOptions{Route: "/r", Strict: true})
	got, err := transpileWith(t, root, lo)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected errors when shape=nil: %v", errs)
	}
	if !bytes.Contains(got, []byte("data.name")) {
		t.Fatalf("expected pass-through when shape is nil:\n%s", got)
	}
}

func TestLowerer_DeterministicOutput(t *testing.T) {
	t.Parallel()
	root := func() *Node {
		return buildProgram(buildBlock(
			propsDestructure("data"),
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				callExpr(memExpr(ident("$"), ident("escape")),
					memExpr(memExpr(ident("data"), ident("user")), ident("name")),
				),
			),
		))
	}
	for i := 0; i < 5; i++ {
		// Build a fresh lowerer + tree each iteration so any hidden
		// shared state would surface as a diff.
		lo := NewLowerer(pageDataNested(), LowererOptions{Route: "/p", Strict: true})
		a, err := transpileWith(t, root(), lo)
		if err != nil {
			t.Fatalf("iter %d: emit: %v", i, err)
		}
		lo2 := NewLowerer(pageDataNested(), LowererOptions{Route: "/p", Strict: true})
		b, err := transpileWith(t, root(), lo2)
		if err != nil {
			t.Fatalf("iter %d: emit2: %v", i, err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("non-deterministic output\n--- a:\n%s\n--- b:\n%s", a, b)
		}
	}
}

func TestLowerer_GoldenLowered(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		root  func() *Node
		shape *typegen.Shape
	}{
		{
			name: "data-name-flat",
			root: func() *Node {
				return buildProgram(buildBlock(
					propsDestructure("data"),
					callStmt(
						memExpr(ident("$$renderer"), ident("push")),
						callExpr(memExpr(ident("$"), ident("escape")),
							memExpr(ident("data"), ident("name")),
						),
					),
				))
			},
			shape: shape("PageData",
				typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
					{Name: "name", GoName: "Name", GoType: "string"},
				}},
			),
		},
		{
			name: "data-user-name-nested",
			root: func() *Node {
				return buildProgram(buildBlock(
					propsDestructure("data"),
					callStmt(
						memExpr(ident("$$renderer"), ident("push")),
						callExpr(memExpr(ident("$"), ident("escape")),
							memExpr(memExpr(ident("data"), ident("user")), ident("name")),
						),
					),
				))
			},
			shape: pageDataNested(),
		},
		{
			name: "data-snake-case-tag",
			root: func() *Node {
				return buildProgram(buildBlock(
					propsDestructure("data"),
					callStmt(
						memExpr(ident("$$renderer"), ident("push")),
						callExpr(memExpr(ident("$"), ident("escape")),
							memExpr(ident("data"), ident("email_address")),
						),
					),
				))
			},
			shape: snakeCaseShape(),
		},
		{
			name: "optional-chain-shallow",
			root: func() *Node {
				return buildProgram(buildBlock(
					propsDestructure("data"),
					callStmt(
						memExpr(ident("$$renderer"), ident("push")),
						callExpr(memExpr(ident("$"), ident("escape")),
							chain(optionalMember(
								memExpr(ident("data"), ident("user")),
								ident("profile"),
							)),
						),
					),
				))
			},
			shape: optionalShape(),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lo := NewLowerer(tc.shape, LowererOptions{Route: "/test/lowered/" + tc.name, Strict: true})
			got, err := TranspileNode(tc.root(), "/test/lowered/"+tc.name, Options{
				PackageName: "gen",
				Rewriter:    lo,
			})
			if err != nil {
				t.Fatalf("TranspileNode: %v", err)
			}
			if errs := lo.Err(); errs != nil {
				t.Fatalf("lowering errors: %v", errs)
			}
			assertGolden(t, "lowered/"+tc.name, got)
		})
	}
}

func TestLowerer_PriorityShapesLowered(t *testing.T) {
	t.Parallel()
	// Shape covers every JSON tag the 30 priority shapes reference on
	// the `data` root so strict-mode lowering succeeds across the
	// fixtures the Phase 3 emitter validates.
	sh := shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
			{Name: "body", GoName: "Body", GoType: "string"},
			{Name: "loggedIn", GoName: "LoggedIn", GoType: "bool"},
			{Name: "status", GoName: "Status", GoType: "string"},
			{Name: "items", GoName: "Items", GoType: "[]Item", NamedType: "Item", Slice: true},
			{Name: "groups", GoName: "Groups", GoType: "[]Group", NamedType: "Group", Slice: true},
			{Name: "active", GoName: "Active", GoType: "bool"},
			{Name: "ready", GoName: "Ready", GoType: "bool"},
			{Name: "visible", GoName: "Visible", GoType: "bool"},
			{Name: "user", GoName: "User", GoType: "*User", NamedType: "User", Pointer: true},
			{Name: "value", GoName: "Value", GoType: "string"},
			{Name: "classes", GoName: "Classes", GoType: "string"},
			{Name: "style", GoName: "Style", GoType: "string"},
			{Name: "attrs", GoName: "Attrs", GoType: "map[string]any"},
			{Name: "tag", GoName: "Tag", GoType: "string"},
			{Name: "show", GoName: "Show", GoType: "bool"},
			{Name: "a", GoName: "A", GoType: "int"},
			{Name: "b", GoName: "B", GoType: "int"},
			{Name: "item", GoName: "Item", GoType: "Item", NamedType: "Item"},
			{Name: "token", GoName: "Token", GoType: "string"},
		}},
		typegen.ShapeType{Name: "Item", Fields: []typegen.Field{
			{Name: "title", GoName: "Title", GoType: "string"},
			{Name: "name", GoName: "Name", GoType: "string"},
			{Name: "count", GoName: "Count", GoType: "int"},
		}},
		typegen.ShapeType{Name: "Group", Fields: []typegen.Field{
			{Name: "visible", GoName: "Visible", GoType: "bool"},
			{Name: "items", GoName: "Items", GoType: "[]Item", NamedType: "Item", Slice: true},
		}},
		typegen.ShapeType{Name: "User", Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
		}},
	)
	for _, tc := range allProgrammaticCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lo := NewLowerer(sh, LowererOptions{Route: "/test/" + tc.name, Strict: true})
			_, err := TranspileNode(tc.root(), "/test/"+tc.name, Options{
				PackageName: "gen",
				Rewriter:    lo,
			})
			if err != nil {
				t.Fatalf("TranspileNode: %v", err)
			}
			if errs := lo.Err(); errs != nil {
				t.Fatalf("lowering errors on priority shape %s: %v", tc.name, errs)
			}
		})
	}
}
