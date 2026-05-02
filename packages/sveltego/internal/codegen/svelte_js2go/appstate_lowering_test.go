package sveltejs2go

import (
	"bytes"
	"strings"
	"testing"
)

// transpileAppState runs Transpile against root with EmitPageStateParam
// flipped on. Lowering errors short-circuit the test — the expectation
// is byte-identical Go output for the lowered chains, not a partial
// emit that gofmt fails on.
func transpileAppState(t *testing.T, root *Node) []byte {
	t.Helper()
	lo := NewLowerer(nil, LowererOptions{Route: "/test/appstate", Strict: false})
	got, err := TranspileNode(root, "/test/appstate", Options{
		PackageName:        "gen",
		Rewriter:           lo,
		EmitPageStateParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	return got
}

// pageStateImport is a one-liner shorthand for the `$app/state` import
// shape Svelte's compiled-server output emits when a route reads any
// of the runes.
func pageStateImport(names ...string) *Node {
	return importFrom("$app/state", names...)
}

func TestLowerer_PageURLPathname(t *testing.T) {
	t.Parallel()
	// Source: `<p>{page.url.pathname}</p>` → emits
	// `payload.Push(server.Stringify(server.EscapeHTML(pageState.URL.Path)))`.
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(memExpr(ident("page"), ident("url")), ident("pathname"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.URL.Path")) {
		t.Fatalf("want pageState.URL.Path in output:\n%s", got)
	}
}

func TestLowerer_PageParamsLookup(t *testing.T) {
	t.Parallel()
	// Source: `{page.params.id}` → `pageState.Params["id"]`
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(memExpr(ident("page"), ident("params")), ident("id"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte(`pageState.Params["id"]`)) {
		t.Fatalf("want pageState.Params[\"id\"] in output:\n%s", got)
	}
}

func TestLowerer_PageRouteID(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(memExpr(ident("page"), ident("route")), ident("id"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Route.ID")) {
		t.Fatalf("want pageState.Route.ID in output:\n%s", got)
	}
}

func TestLowerer_PageStatus(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("page"), ident("status"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Status")) {
		t.Fatalf("want pageState.Status in output:\n%s", got)
	}
}

func TestLowerer_PageErrorMessage(t *testing.T) {
	t.Parallel()
	// Source: `{page.error.message}` (non-optional; error guarded
	// upstream by `{#if page.error}`).
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(memExpr(ident("page"), ident("error")), ident("message"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Error.Message")) {
		t.Fatalf("want pageState.Error.Message in output:\n%s", got)
	}
}

func TestLowerer_PageErrorTruthy(t *testing.T) {
	t.Parallel()
	// `{#if page.error}` lowers to `if pageState.Error != nil` via the
	// truthy check (LogicalExpression-wrap is server.Truthy). The
	// dominant compiled-output shape uses an IfStatement; we verify the
	// lowered base expression is `pageState.Error`.
	root := buildProgramWithImports(
		buildBlock(
			ifStmt(
				memExpr(ident("page"), ident("error")),
				buildBlock(pushString("err")),
				nil,
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Error")) {
		t.Fatalf("want pageState.Error in output:\n%s", got)
	}
}

func TestLowerer_PageData(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("page"), ident("data"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Data")) {
		t.Fatalf("want pageState.Data in output:\n%s", got)
	}
}

func TestLowerer_PageForm(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("page"), ident("form"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Form")) {
		t.Fatalf("want pageState.Form in output:\n%s", got)
	}
}

func TestLowerer_PageState(t *testing.T) {
	t.Parallel()
	// `page.state` itself (the map) lowers to pageState.State; sub-key
	// access lowers to map indexing.
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("page"), ident("state"))),
			),
		),
		pageStateImport("page"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.State")) {
		t.Fatalf("want pageState.State in output:\n%s", got)
	}
}

func TestLowerer_NavigatingCurrent(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("navigating"), ident("current"))),
			),
		),
		pageStateImport("navigating"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Navigating")) {
		t.Fatalf("want pageState.Navigating in output:\n%s", got)
	}
}

func TestLowerer_NavigatingCurrentType(t *testing.T) {
	t.Parallel()
	// `navigating.current.type` → pageState.Navigating.Type. Reads only
	// fire when Navigating is non-nil at runtime; lowerer doesn't guard.
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(memExpr(ident("navigating"), ident("current")), ident("type"))),
			),
		),
		pageStateImport("navigating"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Navigating.Type")) {
		t.Fatalf("want pageState.Navigating.Type in output:\n%s", got)
	}
}

func TestLowerer_UpdatedCurrent(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			callStmt(
				memExpr(ident("$$renderer"), ident("push")),
				escapeOf(memExpr(ident("updated"), ident("current"))),
			),
		),
		pageStateImport("updated"),
	)
	got := transpileAppState(t, root)
	if !bytes.Contains(got, []byte("pageState.Updated")) {
		t.Fatalf("want pageState.Updated in output:\n%s", got)
	}
}

func TestRecordImport_AppStateAllNames(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(pushString("hello")),
		pageStateImport("page", "navigating", "updated"),
	)
	if _, err := TranspileNode(root, "/test/appstate", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
	}); err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
}

func TestRecordImport_AppStateUnknownName(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(pushString("hello")),
		pageStateImport("notARune"),
	)
	_, err := TranspileNode(root, "/test/appstate", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
	})
	if err == nil {
		t.Fatalf("want unknown-name error, got nil")
	}
	if !strings.Contains(err.Error(), "import:$app/state:notARune") {
		t.Fatalf("error should name the offending import: %v", err)
	}
}

func TestRecordImport_AppNavigation(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(pushString("hello")),
		importFrom("$app/navigation", "goto", "invalidate"),
	)
	if _, err := TranspileNode(root, "/test/nav", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
	}); err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
}

func TestRenderSignature_PageStateParam(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(pushString("hello")),
		pageStateImport("page"),
	)
	got, err := TranspileNode(root, "/test/sig", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !bytes.Contains(got, []byte("pageState server.PageState")) {
		t.Fatalf("want pageState param in signature:\n%s", got)
	}
}

func TestRenderSignature_PageStateWithChildrenAndTypedData(t *testing.T) {
	t.Parallel()
	// All three knobs flipped: the signature must be
	//   func Render(payload *server.Payload, data MyData,
	//               children func(*server.Payload),
	//               pageState server.PageState)
	root := buildProgramWithImports(
		buildBlock(propsDestructure("data")),
		pageStateImport("page"),
	)
	got, err := TranspileNode(root, "/test/sig", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		EmitChildrenParam:  true,
		TypedDataParam:     "MyData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	wantSubs := []string{
		"data MyData",
		"children func(*server.Payload)",
		"pageState server.PageState",
	}
	for _, s := range wantSubs {
		if !bytes.Contains(got, []byte(s)) {
			t.Fatalf("missing %q in signature:\n%s", s, got)
		}
	}
}

func TestRenderSignature_PageStateOff(t *testing.T) {
	t.Parallel()
	// Default emit (EmitPageStateParam = false) keeps the legacy shape
	// so the 30+ priority and 50+ extended goldens stay byte-identical.
	root := buildProgram(buildBlock(pushString("hello")))
	got, err := TranspileNode(root, "/test/sig", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if bytes.Contains(got, []byte("pageState")) {
		t.Fatalf("default emit must not mention pageState:\n%s", got)
	}
}
