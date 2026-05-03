package sveltejs2go

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
)

// fakeImageVariants returns a [images.Result] map covering the fixture
// images the tests reference. Mirrors the build-time pipeline output
// shape so the lowering walks the same code path it would in
// production.
func fakeImageVariants() map[string]images.Result {
	return map[string]images.Result{
		"hero.png": {
			Source:          "hero.png",
			IntrinsicWidth:  1920,
			IntrinsicHeight: 1080,
			Variants: []images.Variant{
				{Width: 320, Height: 180, URL: "/_app/immutable/assets/hero.abc12345.320.png"},
				{Width: 640, Height: 360, URL: "/_app/immutable/assets/hero.abc12345.640.png"},
				{Width: 1280, Height: 720, URL: "/_app/immutable/assets/hero.abc12345.1280.png"},
				{Width: 1920, Height: 1080, URL: "/_app/immutable/assets/hero.abc12345.1920.png"},
			},
		},
		"single.png": {
			Source:          "single.png",
			IntrinsicWidth:  200,
			IntrinsicHeight: 100,
			Variants: []images.Variant{
				{Width: 200, Height: 100, URL: "/_app/immutable/assets/single.deadbeef.200.png"},
			},
		},
		"needsescape.png": {
			Source:          "needsescape.png",
			IntrinsicWidth:  100,
			IntrinsicHeight: 50,
			Variants: []images.Variant{
				{Width: 100, Height: 50, URL: "/_app/immutable/assets/needsescape&special.cafef00d.100.png"},
			},
		},
	}
}

// imageImportNode builds the `import { Image } from "@sveltego/enhanced-img"`
// declaration used by the test fixtures.
func imageImportNode(local string) *Node {
	return &Node{
		Type: "ImportDeclaration",
		Source: &Node{
			Type:    "Literal",
			LitKind: litString,
			LitStr:  imageImportSource,
		},
		Specifiers: []*Node{
			{
				Type:     "ImportSpecifier",
				Imported: ident("Image"),
				Local:    ident(local),
			},
		},
	}
}

// imageCall synthesises the Acorn shape svelte/server emits for a
// `<Image …>` invocation: `Image($$renderer, { …props })`.
func imageCall(local string, props ...[2]*Node) *Node {
	pairs := make([]*Node, 0, len(props))
	for _, p := range props {
		pairs = append(pairs, &Node{Type: "Property", Key: p[0], Value: p[1], Kind: "init"})
	}
	return callStmt(
		ident(local),
		ident("$$renderer"),
		&Node{Type: "ObjectExpression", Properties: pairs},
	)
}

func TestInjectImage_BasicSrcset(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/hero.png")},
				[2]*Node{ident("alt"), strLit("Hero")},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)

	wants := []string{
		`payload.Push("<img src=\"/_app/immutable/assets/hero.abc12345.1920.png\"`,
		`srcset=\"/_app/immutable/assets/hero.abc12345.320.png 320w, /_app/immutable/assets/hero.abc12345.640.png 640w, /_app/immutable/assets/hero.abc12345.1280.png 1280w, /_app/immutable/assets/hero.abc12345.1920.png 1920w\"`,
		`width=\"1920\"`,
		`height=\"1080\"`,
		`alt=\"Hero\"`,
		`loading=\"lazy\" decoding=\"async\"`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q\n--- got:\n%s", w, got)
		}
	}
}

func TestInjectImage_ExplicitWidthHeight(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/hero.png")},
				[2]*Node{ident("alt"), strLit("Hero")},
				[2]*Node{ident("width"), numLit(800)},
				[2]*Node{ident("height"), numLit(450)},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `width=\"800\"`) {
		t.Errorf("expected explicit width=800, got:\n%s", got)
	}
	if !strings.Contains(got, `height=\"450\"`) {
		t.Errorf("expected explicit height=450, got:\n%s", got)
	}
}

func TestInjectImage_SingleVariantNoSrcset(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/single.png")},
				[2]*Node{ident("alt"), strLit("Solo")},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "srcset=") {
		t.Errorf("single-variant image must NOT emit srcset, got:\n%s", got)
	}
	if !strings.Contains(got, "/single.deadbeef.200.png") {
		t.Errorf("expected fallback src, got:\n%s", got)
	}
}

func TestInjectImage_PriorityFlipsLoading(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/hero.png")},
				[2]*Node{ident("alt"), strLit("Hero")},
				[2]*Node{ident("priority"), boolLit(true)},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `loading=\"eager\"`) {
		t.Errorf("priority must flip loading to eager, got:\n%s", got)
	}
}

func TestInjectImage_AliasedImport(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Img",
				[2]*Node{ident("src"), strLit("/single.png")},
				[2]*Node{ident("alt"), strLit("Solo")},
			),
		),
		imageImportNode("Img"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "/single.deadbeef.200.png") {
		t.Errorf("aliased import must still substitute, got:\n%s", got)
	}
}

func TestInjectImage_AttrEscaping(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/needsescape.png")},
				[2]*Node{ident("alt"), strLit(`"quote" & <tag>`)},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `&amp;special`) {
		t.Errorf("URL & must be HTML-escaped, got:\n%s", got)
	}
	if !strings.Contains(got, `&quot;quote&quot; &amp; &lt;tag&gt;`) {
		t.Errorf("alt special chars must be HTML-escaped, got:\n%s", got)
	}
}

func TestInjectImage_ClassAndSizes(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/hero.png")},
				[2]*Node{ident("alt"), strLit("Hero")},
				[2]*Node{ident("class"), strLit("rounded shadow-md")},
				[2]*Node{ident("sizes"), strLit("(min-width: 768px) 50vw, 100vw")},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `class=\"rounded shadow-md\"`) {
		t.Errorf("expected class attribute, got:\n%s", got)
	}
	if !strings.Contains(got, `sizes=\"(min-width: 768px) 50vw, 100vw\"`) {
		t.Errorf("expected sizes attribute, got:\n%s", got)
	}
}

func TestInjectImage_MissingVariantHardError(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/missing.png")},
				[2]*Node{ident("alt"), strLit("oops")},
			),
		),
		imageImportNode("Image"),
	)
	_, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err == nil {
		t.Fatalf("expected hard error for missing variant, got nil")
	}
	if !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("error should name the missing source: %v", err)
	}
}

func TestInjectImage_DynamicSrcHardError(t *testing.T) {
	t.Parallel()
	// `<Image src={data.hero}>` — src compiles to a MemberExpression,
	// not a static literal. Reject so the user sees a clear opt-in
	// path instead of a silent passthrough that omits the variant set.
	root := buildProgramWithImports(
		buildBlock(
			propsDestructure("data"),
			imageCall("Image",
				[2]*Node{ident("src"), memExpr(ident("data"), ident("hero"))},
				[2]*Node{ident("alt"), strLit("hero")},
			),
		),
		imageImportNode("Image"),
	)
	_, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err == nil {
		t.Fatalf("expected hard error for dynamic src, got nil")
	}
	if !strings.Contains(err.Error(), "src") {
		t.Errorf("error should name the dynamic prop: %v", err)
	}
}

func TestInjectImage_MissingSrcHardError(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("alt"), strLit("oops")},
			),
		),
		imageImportNode("Image"),
	)
	_, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err == nil {
		t.Fatalf("expected hard error for missing src, got nil")
	}
	if !strings.Contains(err.Error(), "src") {
		t.Errorf("error should call out missing src: %v", err)
	}
}

func TestInjectImage_NoVariantsMapNoOp(t *testing.T) {
	t.Parallel()
	// When ImageVariants is nil the pre-pass should not run and the
	// import should still pass through to the unknownShape branch in
	// recordImport (the existing behaviour for non-recognised imports).
	// This test asserts the no-op shape, not the unknownShape outcome —
	// the lowering is opt-in via Options.
	root := buildProgramWithImports(
		buildBlock(pushString("<p>no images here</p>")),
	)
	out, err := TranspileNode(root, "/test", Options{})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if !strings.Contains(string(out), `payload.Push("<p>no images here</p>")`) {
		t.Errorf("expected baseline output preserved, got:\n%s", out)
	}
}

func TestInjectImage_DropsImportFromAST(t *testing.T) {
	t.Parallel()
	// The pre-pass rewrites the @sveltego/enhanced-img import to an
	// empty ExportNamedDeclaration so emitProgram's switch silently
	// skips it instead of hitting unknownShape("import:..."). Without
	// this the transpile would fail before lowering even runs.
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/single.png")},
				[2]*Node{ident("alt"), strLit("Solo")},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if !strings.Contains(string(out), "/single.deadbeef.200.png") {
		t.Errorf("expected lowered <img>, got:\n%s", out)
	}
}

func TestInjectImage_InsideEachLoopAndIf(t *testing.T) {
	t.Parallel()
	// Image calls nested inside {#if} or {#each} bodies must still be
	// rewritten — the walker covers every ESTree branch the emitter
	// visits.
	body := buildBlock(
		ifStmt(
			boolLit(true),
			buildBlock(
				imageCall("Image",
					[2]*Node{ident("src"), strLit("/hero.png")},
					[2]*Node{ident("alt"), strLit("Hero")},
				),
			),
			nil,
		),
	)
	root := buildProgramWithImports(body, imageImportNode("Image"))
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if !strings.Contains(string(out), "/_app/immutable/assets/hero.abc12345.1920.png") {
		t.Errorf("expected nested Image to lower, got:\n%s", out)
	}
}

func TestInjectImage_UnknownAttrHardError(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/single.png")},
				[2]*Node{ident("alt"), strLit("Solo")},
				[2]*Node{ident("draggable"), boolLit(false)},
			),
		),
		imageImportNode("Image"),
	)
	_, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err == nil {
		t.Fatalf("expected hard error for unsupported attr, got nil")
	}
	if !strings.Contains(err.Error(), "draggable") {
		t.Errorf("error should name the unsupported attr: %v", err)
	}
}

func TestInjectImage_NoAltDefaultsEmpty(t *testing.T) {
	t.Parallel()
	root := buildProgramWithImports(
		buildBlock(
			imageCall("Image",
				[2]*Node{ident("src"), strLit("/single.png")},
			),
		),
		imageImportNode("Image"),
	)
	out, err := TranspileNode(root, "/test", Options{
		ImageVariants: fakeImageVariants(),
	})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if !strings.Contains(string(out), `alt=\"\"`) {
		t.Errorf("expected empty alt default, got:\n%s", out)
	}
}
