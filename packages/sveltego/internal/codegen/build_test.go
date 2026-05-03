package codegen

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/svelterender"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// requireSSRSidecar skips the test when Node or the svelterender
// sidecar's node_modules are unavailable. CI workflows that run the
// SSR pipeline install both before the gate; the lint-and-test job
// installs nothing JS-side, so codegen tests that drive a live
// _layout.svelte / _page.svelte through the SSR transpile must skip
// rather than fail. Mirrors svelterender.requireSidecarReady.
func requireSSRSidecar(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skipping SSR-transpile codegen test")
	}
	dir, err := svelterender.SidecarRoot()
	if err != nil {
		t.Skipf("sidecar tree not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "acorn")); err != nil {
		t.Skipf("sidecar node_modules missing; run `npm install` in %s", dir)
	}
}

// scaffoldProject builds a synthetic project tree under root with one
// root _page.svelte, one [id]/_page.svelte + _page.server.go, and a
// lib/db/posts.go file. The project go.mod declares module path module.
func scaffoldProject(t *testing.T, root, module string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+module+"\n\ngo 1.22\n")

	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		`<script lang="go">
import (
	"context"
	"$lib/db"
)
</script>
<h1>{db.Title()}</h1>
`)

	writeFile(t, filepath.Join(root, "src", "routes", "[id]", "_page.svelte"),
		"<h2>id page</h2>\n")
	writeFile(t, filepath.Join(root, "src", "routes", "[id]", "_page.server.go"),
		`//go:build sveltego

package _id_

import (
	"context"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func Load(ctx *kit.LoadCtx) (PageData, error) {
	return struct{ ID string }{ID: "x"}, nil
}
`)

	writeFile(t, filepath.Join(root, "lib", "db", "posts.go"),
		"package db\n\nfunc Title() string { return \"hi\" }\n")
}

func TestBuild_MissingGoMod(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"), "<h1>x</h1>\n")
	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on missing go.mod")
	}
}

func TestBuild_MissingRoutesDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n\ngo 1.22\n")
	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on missing src/routes/")
	}
}

func TestBuild_RelativeProjectRoot(t *testing.T) {
	t.Parallel()
	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: "relative/path"}); err == nil {
		t.Fatal("expected error on relative ProjectRoot")
	}
}

func TestBuild_ConflictAborts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "_page.svelte"), "<h1>p</h1>\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "_server.go"),
		"//go:build sveltego\n\npackage api\n\nimport \"net/http\"\n\nvar Handlers = map[string]http.HandlerFunc{}\n")
	_, err := Build(context.Background(), BuildOptions{ProjectRoot: root})
	if err == nil {
		t.Fatal("expected fatal diagnostic on conflicting page+server")
	}
	if !strings.Contains(err.Error(), "fatal scanner diagnostics") {
		t.Errorf("expected fatal diagnostics wrapper, got: %v", err)
	}
}

func TestBuild_Determinism(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	first := snapshotGen(t, root)

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("second Build: %v", err)
	}
	second := snapshotGen(t, root)

	if len(first) != len(second) {
		t.Fatalf("file count differs: first=%d second=%d", len(first), len(second))
	}
	for path, a := range first {
		b, ok := second[path]
		if !ok {
			t.Errorf("file %s missing on second build", path)
			continue
		}
		if !bytes.Equal(a, b) {
			t.Errorf("non-deterministic output in %s", path)
		}
	}
}

// TestBuild_EmitsLayoutServer covers Phase 0k-B: a _layout.server.go
// adjacent to _layout.svelte produces a layoutsrc mirror, a sibling
// wire_layout.gen.go in the gen package, a typed LayoutData alias from
// the inferred Load() return, and a manifest LayoutLoaders entry.
//
// Post-#478: layouts SSR-transpile via Option B, so the legacy
// `.gen/routes/layout.gen.go` Mustache-Go body is no longer emitted.
// The data-flow contract — typed LayoutData, server-source mirror,
// wire wrapper, manifest LayoutLoaders entry — is unchanged.
func TestBuild_EmitsLayoutServer(t *testing.T) {
	t.Parallel()
	requireSSRSidecar(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_layout.svelte"),
		"<header>root</header>{@render children()}\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_layout.server.go"),
		`//go:build sveltego

package routes

import (
	"context"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func Load(ctx *kit.LoadCtx) (LayoutData, error) {
	return struct{ User string }{User: "alice"}, nil
}
`)
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "_layout.svelte"),
		"<nav>dash</nav>{@render children()}\n")
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "_page.svelte"),
		`<h1>Dash</h1>`+"\n")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	mirror := filepath.Join(root, ".gen", "layoutsrc", "routes", "layout_server.go")
	wire := filepath.Join(root, ".gen", "routes", "wire_layout.gen.go")
	manifest := filepath.Join(root, ".gen", "manifest.gen.go")
	for _, p := range []string{mirror, wire, manifest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	// The Mustache-Go layout.gen.go is no longer emitted for
	// SSR-transpiled layouts (#478). Verify it stays out of .gen.
	if _, err := os.Stat(filepath.Join(root, ".gen", "routes", "layout.gen.go")); err == nil {
		t.Errorf("legacy layout.gen.go must not be emitted alongside SSR-transpiled layouts")
	}

	mirrorBytes, err := os.ReadFile(mirror)
	if err != nil {
		t.Fatalf("read layout mirror: %v", err)
	}
	if bytes.Contains(mirrorBytes, []byte("//go:build")) {
		t.Errorf("layout mirror retained build constraint:\n%s", mirrorBytes)
	}
	if !bytes.Contains(mirrorBytes, []byte("package routes")) {
		t.Errorf("layout mirror package clause not rewritten:\n%s", mirrorBytes)
	}

	wireBytes, err := os.ReadFile(wire)
	if err != nil {
		t.Fatalf("read layout wire: %v", err)
	}
	if !bytes.Contains(wireBytes, []byte(`usersrc "example.com/app/.gen/layoutsrc/routes"`)) {
		t.Errorf("layout wire missing mirror import:\n%s", wireBytes)
	}
	if !bytes.Contains(wireBytes, []byte("func LayoutLoad(ctx *kit.LoadCtx)")) {
		t.Errorf("layout wire missing LayoutLoad wrapper:\n%s", wireBytes)
	}

	manifestBytes, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, want := range []string{
		`func loadLayout__page_routes(`,
		`LayoutLoaders: []router.LayoutLoadHandler{`,
		`loadLayout__page_routes,`,
	} {
		if !bytes.Contains(manifestBytes, []byte(want)) {
			t.Errorf("manifest missing %q:\n%s", want, manifestBytes)
		}
	}

	for _, p := range []string{mirror, wire, manifest} {
		assertParsesAsGo(t, p)
	}
}

// TestBuild_EmitsSvelteHead covers #51 end-to-end: a _page.svelte and
// _layout.svelte that each declare <svelte:head> produce Head methods
// in their gen packages and matching head adapters + Head/LayoutHeads
// fields in the manifest.
// TestBuild_EmitsSPARouterModule asserts that the codegen pass writes
// the shared SPA router module at .gen/client/__router/router.ts and
// that every per-route entry.ts imports it (#37).
func TestBuild_EmitsSPARouterModule(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	routerPath := filepath.Join(root, ".gen", "client", "__router", "router.ts")
	routerBytes, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatalf("router.ts not emitted: %v", err)
	}
	for _, want := range []string{
		"export function startRouter",
		"export function shouldNotIntercept",
		`"/": () => import(`,
		`"/[id]": () => import(`,
	} {
		if !bytes.Contains(routerBytes, []byte(want)) {
			t.Errorf("router.ts missing %q:\n%s", want, routerBytes)
		}
	}

	rootEntry := filepath.Join(root, ".gen", "client", "routes", "_page", "entry.ts")
	rootEntryBytes, err := os.ReadFile(rootEntry)
	if err != nil {
		t.Fatalf("root entry.ts not emitted: %v", err)
	}
	if !bytes.Contains(rootEntryBytes, []byte("startRouter")) {
		t.Errorf("root entry.ts missing startRouter import:\n%s", rootEntryBytes)
	}
	if !bytes.Contains(rootEntryBytes, []byte(`from "../../__router/router"`)) {
		t.Errorf("root entry.ts router import path wrong:\n%s", rootEntryBytes)
	}

	idEntry := filepath.Join(root, ".gen", "client", "routes", "[id]", "_page", "entry.ts")
	idEntryBytes, err := os.ReadFile(idEntry)
	if err != nil {
		t.Fatalf("id entry.ts not emitted: %v", err)
	}
	if !bytes.Contains(idEntryBytes, []byte(`from "../../../__router/router"`)) {
		t.Errorf("id entry.ts router import path wrong:\n%s", idEntryBytes)
	}

	navPath := filepath.Join(root, ".gen", "client", "__router", "navigation.ts")
	navBytes, err := os.ReadFile(navPath)
	if err != nil {
		t.Fatalf("navigation.ts not emitted: %v", err)
	}
	for _, want := range []string{
		"goto,",
		"invalidate,",
		"preloadData,",
		"from './router'",
	} {
		if !bytes.Contains(navBytes, []byte(want)) {
			t.Errorf("navigation.ts missing %q:\n%s", want, navBytes)
		}
	}
}

func TestBuild_EmitsSnapshotWiring(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/snap\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		`<script module>
export const snapshot = {
  capture: () => window.scrollY,
  restore: (y) => window.scrollTo(0, y),
};
</script>
<h1>hi</h1>
`)
	writeFile(t, filepath.Join(root, "src", "routes", "plain", "_page.svelte"),
		"<p>plain</p>\n")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	routerPath := filepath.Join(root, ".gen", "client", "__router", "router.ts")
	routerBytes, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatalf("router.ts not emitted: %v", err)
	}
	if !bytes.Contains(routerBytes, []byte(`"/": true,`)) {
		t.Errorf("snapshot route /  missing from snapshotRoutes table:\n%s", routerBytes)
	}
	if bytes.Contains(routerBytes, []byte(`"/plain": true`)) {
		t.Errorf("snapshot-free route /plain should not appear in snapshotRoutes:\n%s", routerBytes)
	}
	if !bytes.Contains(routerBytes, []byte("function captureSnapshot()")) {
		t.Errorf("router missing captureSnapshot helper:\n%s", routerBytes)
	}

	rootEntry := filepath.Join(root, ".gen", "client", "routes", "_page", "entry.ts")
	rootEntryBytes, err := os.ReadFile(rootEntry)
	if err != nil {
		t.Fatalf("root entry.ts not emitted: %v", err)
	}
	if !bytes.Contains(rootEntryBytes, []byte(`import Root, { snapshot } from`)) {
		t.Errorf("snapshot route entry.ts missing snapshot import:\n%s", rootEntryBytes)
	}
	if !bytes.Contains(rootEntryBytes, []byte(`startRouter({ component, payload, target, snapshot, chainKey: `)) {
		t.Errorf("snapshot route entry.ts must hand snapshot + chainKey to startRouter:\n%s", rootEntryBytes)
	}

	plainEntry := filepath.Join(root, ".gen", "client", "routes", "plain", "_page", "entry.ts")
	plainEntryBytes, err := os.ReadFile(plainEntry)
	if err != nil {
		t.Fatalf("plain entry.ts not emitted: %v", err)
	}
	if bytes.Contains(plainEntryBytes, []byte("snapshot")) {
		t.Errorf("plain route should not import snapshot:\n%s", plainEntryBytes)
	}
}

// TestBuild_EmitsLayoutWrapper covers issue #508: a route with a
// _layout.svelte chain must get a per-route wrapper.svelte that nests
// the layout(s) around the page, the entry.ts must mount the wrapper
// (not the bare page), startRouter must receive a non-empty chainKey,
// and the SPA router must carry a chainKeys table plus the
// wrapper-store import. Routes without a layout chain stay on the
// pre-#508 page-only mount path.
func TestBuild_EmitsLayoutWrapper(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/lw\n\ngo 1.22\n")
	// SSR=false opts the page (and its layout chain) out of the
	// build-time SSR transpile so the test does not depend on a Node
	// sidecar being installed at $PATH.
	writeFile(t, filepath.Join(root, "src", "routes", "_page.server.go"),
		"//go:build sveltego\n\npackage routes\n\nconst SSR = false\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_layout.svelte"),
		"<script>let { data, children } = $props();</script><header>{data?.user ?? ''}</header>{@render children()}")
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		"<h1>home</h1>\n")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Wrapper file must be present alongside entry.ts for the root route.
	wrapperPath := filepath.Join(root, ".gen", "client", "routes", "_page", "wrapper.svelte")
	wrapperBytes, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("wrapper.svelte not emitted at %s: %v", wrapperPath, err)
	}
	for _, want := range []string{
		`import L0 from `,
		`_layout.svelte"`,
		`import Page from `,
		`import { wrapperState } from `,
		`<L0 data={wrapperState.layoutData[0] ?? {}}>`,
		`<Page data={wrapperState.data} form={wrapperState.form} />`,
		`</L0>`,
	} {
		if !bytes.Contains(wrapperBytes, []byte(want)) {
			t.Errorf("wrapper.svelte missing %q:\n%s", want, wrapperBytes)
		}
	}
	// Hydration parity: the wrapper must render the page through a
	// STATIC <Page> reference. Dynamic dispatch ({#if}, {@const},
	// <svelte:component>) injects comment markers absent from the SSR
	// HTML and trips svelte/e/hydration_mismatch on first paint.
	for _, banned := range []string{"{#if", "{@const", "<svelte:component", "<PageSlot"} {
		if bytes.Contains(wrapperBytes, []byte(banned)) {
			t.Errorf("wrapper.svelte must not emit %q (hydration-mismatch hazard):\n%s", banned, wrapperBytes)
		}
	}

	// Entry must mount the wrapper, not Page directly, and forward layoutData.
	rootEntry := filepath.Join(root, ".gen", "client", "routes", "_page", "entry.ts")
	rootEntryBytes, err := os.ReadFile(rootEntry)
	if err != nil {
		t.Fatalf("entry.ts not emitted: %v", err)
	}
	if !bytes.Contains(rootEntryBytes, []byte(`import Root from "./wrapper.svelte";`)) {
		t.Errorf("entry.ts must import wrapper as Root:\n%s", rootEntryBytes)
	}
	if !bytes.Contains(rootEntryBytes, []byte("layoutData: payload.layoutData ?? []")) {
		t.Errorf("entry.ts must forward layoutData to wrapper:\n%s", rootEntryBytes)
	}
	if !bytes.Contains(rootEntryBytes, []byte(`chainKey: "`)) {
		t.Errorf("entry.ts must forward a chainKey to startRouter:\n%s", rootEntryBytes)
	}

	// Shared wrapper-store and the router's chainKeys table must be emitted.
	storePath := filepath.Join(root, ".gen", "client", "__router", "wrapper-store.svelte.ts")
	storeBytes, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("wrapper-store.svelte.ts not emitted: %v", err)
	}
	if !bytes.Contains(storeBytes, []byte("export const wrapperState = $state")) {
		t.Errorf("wrapper-store.svelte.ts missing wrapperState rune:\n%s", storeBytes)
	}

	routerBytes, err := os.ReadFile(filepath.Join(root, ".gen", "client", "__router", "router.ts"))
	if err != nil {
		t.Fatalf("router.ts not emitted: %v", err)
	}
	if !bytes.Contains(routerBytes, []byte("import { _setWrapperState } from './wrapper-store.svelte';")) {
		t.Errorf("router.ts missing wrapper-store import:\n%s", routerBytes)
	}
	if !bytes.Contains(routerBytes, []byte("const chainKeys: Record<string, string> = {")) {
		t.Errorf("router.ts missing chainKeys table:\n%s", routerBytes)
	}
	// The router's loader map must point at the wrapper, not the bare page.
	if !bytes.Contains(routerBytes, []byte("/wrapper.svelte")) {
		t.Errorf("router.ts loader map should target wrapper.svelte:\n%s", routerBytes)
	}
}

func TestBuild_EmbedSkippedWhenNoAssets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	// NoClient skips client entry emission so .gen/client/ is absent.
	// Without a static/ dir either, embed.go must not be written.
	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root, NoClient: true}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "embed.go")); err == nil {
		t.Errorf("embed.go should not be emitted without client/ or static/")
	}
}

// TestBuild_ReleaseRejectsLibDevImport verifies that a _page.svelte
// importing "$lib/dev/..." causes Build to return an error in release
// mode but succeeds in dev mode.
// TestBuild_ReleaseAllowsNonDevLibImport verifies that $lib imports that
// are not under dev/ pass through release mode unchanged.
func TestBuild_ReleaseAllowsNonDevLibImport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		`<script lang="go">
import (
	"context"
	"$lib/util"
)
</script>
<h1>Hello</h1>
`)
	writeFile(t, filepath.Join(root, "src", "lib", "util", "util.go"),
		"package util\n\nfunc Name() string { return \"lib\" }\n")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root, Release: true}); err != nil {
		t.Fatalf("release Build on non-dev $lib: unexpected error: %v", err)
	}
}

// TestCheckLibDevImports exercises the low-level helper directly.
func TestCheckLibDevImports(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"no-lib", `import "fmt"`, false},
		{"lib-util", `import "$lib/util"`, false},
		{"lib-bare", `import "$lib"`, false},
		{"lib-dev-exact", `import "$lib/dev"`, true},
		{"lib-dev-sub", `import "$lib/dev/panel"`, true},
		{"lib-developer", `import "$lib/developer"`, false}, // must NOT match
		{"lib-devtools", `import "$lib/devtools/x"`, false}, // must NOT match
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkLibDevImports(tc.body, "test.svelte")
			if (err != nil) != tc.wantErr {
				t.Errorf("checkLibDevImports(%q) error=%v wantErr=%v", tc.body, err, tc.wantErr)
			}
		})
	}
}

func snapshotGen(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	gen := filepath.Join(root, ".gen")
	err := filepath.Walk(gen, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(gen, path)
		if err != nil {
			return err
		}
		bs, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = bs
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", gen, err)
	}
	return out
}

func assertParsesAsGo(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, path, src, parser.AllErrors|parser.SkipObjectResolution); err != nil {
		t.Errorf("parse %s: %v\n--- src:\n%s", path, err, src)
	}
}

// TestBuild_PageDataNamedType reproduces the standalone-scaffold variant
// of #143: when the user's page.server.go declares a top-level
// `type PageData struct{...}`, the generated page.gen.go must alias to
// the mirrored type — `type PageData = usersrc.PageData` — instead of
// synthesizing an empty `type PageData = struct{}`. Without this, the
// gen file references `data.<UserField>` against the empty alias and
// fails to compile.
//
// The scaffold flow is the canonical repro: a fresh project's
// page.server.go uses the named-type form by default to keep Load()
// readable. See feedback_minimal_setup.md (2026-05-01).
// TestBuild_LayoutDataNamedType mirrors TestBuild_PageDataNamedType for
// layouts. A layout.server.go declaring `type LayoutData struct{...}`
// flows the named type through the SSR layout wire so the manifest
// bridge can type-assert against `usersrc.LayoutData`. Post-#478 the
// type lives in the layoutsrc mirror (which is the user's source file
// with the build tag stripped) and is referenced from the wire's
// dispatch.
func TestBuild_LayoutDataNamedType(t *testing.T) {
	t.Parallel()
	requireSSRSidecar(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/lyt\n\ngo 1.23\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_layout.svelte"),
		"<script lang=\"ts\">let { data, children } = $props();</script>\n"+
			"<header>{data.title}</header>{@render children()}\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_layout.server.go"),
		`//go:build sveltego

package routes

import (
	"context"
	"github.com/binsarjr/sveltego/exports/kit"
)

type LayoutData struct {
	Title string `+"`json:\"title\"`"+`
}

func Load(ctx *kit.LoadCtx) (LayoutData, error) {
	_ = ctx
	return LayoutData{Title: "x"}, nil
}
`)
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		"<h1>page</h1>\n")

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	mirror := filepath.Join(root, ".gen", "layoutsrc", "routes", "layout_server.go")
	mirrorBytes, err := os.ReadFile(mirror)
	if err != nil {
		t.Fatalf("read layout mirror: %v", err)
	}
	mirrorSrc := string(mirrorBytes)
	if !strings.Contains(mirrorSrc, "type LayoutData struct {") {
		t.Errorf("mirror missing named LayoutData declaration; got:\n%s", mirrorSrc)
	}
	if !strings.Contains(mirrorSrc, "Title string") {
		t.Errorf("mirror missing Title field; got:\n%s", mirrorSrc)
	}

	wire := filepath.Join(root, ".gen", "routes", "wire_layout_render.gen.go")
	wireBytes, err := os.ReadFile(wire)
	if err != nil {
		t.Fatalf("read SSR layout wire: %v", err)
	}
	wireSrc := string(wireBytes)
	const wantWireType = "usersrc.LayoutData"
	if !strings.Contains(wireSrc, wantWireType) {
		t.Errorf("expected %q in wire; got:\n%s", wantWireType, wireSrc)
	}
	const wantWireImport = `usersrc "example.com/lyt/.gen/layoutsrc/routes"`
	if !strings.Contains(wireSrc, wantWireImport) {
		t.Errorf("expected import %q in wire; got:\n%s", wantWireImport, wireSrc)
	}
	assertParsesAsGo(t, mirror)
	assertParsesAsGo(t, wire)
}
