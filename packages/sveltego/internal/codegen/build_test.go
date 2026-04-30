package codegen

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// scaffoldProject builds a synthetic project tree under root with one
// root +page.svelte, one [id]/+page.svelte + +page.server.go, and a
// lib/db/posts.go file. The project go.mod declares module path module.
func scaffoldProject(t *testing.T, root, module string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+module+"\n\ngo 1.22\n")

	writeFile(t, filepath.Join(root, "src", "routes", "+page.svelte"),
		`<script lang="go">
import "$lib/db"
</script>
<h1>{db.Title()}</h1>
`)

	writeFile(t, filepath.Join(root, "src", "routes", "[id]", "+page.svelte"),
		"<h2>id page</h2>\n")
	writeFile(t, filepath.Join(root, "src", "routes", "[id]", "page.server.go"),
		`//go:build sveltego

package _id_

import "github.com/binsarjr/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (PageData, error) {
	return struct{ ID string }{ID: "x"}, nil
}
`)

	writeFile(t, filepath.Join(root, "lib", "db", "posts.go"),
		"package db\n\nfunc Title() string { return \"hi\" }\n")
}

func TestBuild_HappyPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")

	res, err := Build(BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Routes != 2 {
		t.Errorf("Routes = %d, want 2", res.Routes)
	}

	rootPage := filepath.Join(root, ".gen", "routes", "page.gen.go")
	idPage := filepath.Join(root, ".gen", "routes", "_id_", "page.gen.go")
	manifest := filepath.Join(root, ".gen", "manifest.gen.go")
	for _, p := range []string{rootPage, idPage, manifest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	rootBytes, err := os.ReadFile(rootPage)
	if err != nil {
		t.Fatalf("read root page: %v", err)
	}
	if !bytes.Contains(rootBytes, []byte(`"example.com/app/lib/db"`)) {
		t.Errorf("expected $lib import rewritten to module path, got:\n%s", rootBytes)
	}
	if bytes.Contains(rootBytes, []byte("$lib")) {
		t.Errorf("expected no remaining $lib literal, got:\n%s", rootBytes)
	}
	if !bytes.Contains(rootBytes, []byte("package routes")) {
		t.Errorf("expected `package routes`, got:\n%s", rootBytes)
	}

	idBytes, err := os.ReadFile(idPage)
	if err != nil {
		t.Fatalf("read id page: %v", err)
	}
	if !bytes.Contains(idBytes, []byte("package _id_")) {
		t.Errorf("expected `package _id_`, got:\n%s", idBytes)
	}

	// Mirror + wire emitted for the route with page.server.go.
	idMirror := filepath.Join(root, ".gen", "usersrc", "routes", "_id_", "page_server.go")
	idWire := filepath.Join(root, ".gen", "routes", "_id_", "wire.gen.go")
	for _, p := range []string{idMirror, idWire} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
	mirrorBytes, err := os.ReadFile(idMirror)
	if err != nil {
		t.Fatalf("read id mirror: %v", err)
	}
	if bytes.Contains(mirrorBytes, []byte("//go:build")) {
		t.Errorf("mirror retained build constraint:\n%s", mirrorBytes)
	}
	if !bytes.Contains(mirrorBytes, []byte("package _id_")) {
		t.Errorf("mirror package clause not rewritten:\n%s", mirrorBytes)
	}
	wireBytes, err := os.ReadFile(idWire)
	if err != nil {
		t.Fatalf("read id wire: %v", err)
	}
	if !bytes.Contains(wireBytes, []byte(`usersrc "example.com/app/.gen/usersrc/routes/_id_"`)) {
		t.Errorf("wire missing mirror import:\n%s", wireBytes)
	}
	if !bytes.Contains(wireBytes, []byte("func Load(ctx *kit.LoadCtx)")) {
		t.Errorf("wire missing Load wrapper:\n%s", wireBytes)
	}

	manifestBytes, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, want := range []string{
		`"example.com/app/.gen/routes"`,
		`"example.com/app/.gen/routes/_id_"`,
		`func render__page_routes`,
		`func render__page_routes__id_`,
		`Page:    render__page_routes__id_`,
	} {
		if !bytes.Contains(manifestBytes, []byte(want)) {
			t.Errorf("manifest missing %q:\n%s", want, manifestBytes)
		}
	}

	for _, p := range []string{rootPage, idPage, manifest, idWire, idMirror} {
		assertParsesAsGo(t, p)
	}
}

func TestBuild_MissingGoMod(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "routes", "+page.svelte"), "<h1>x</h1>\n")
	if _, err := Build(BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on missing go.mod")
	}
}

func TestBuild_MissingRoutesDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n\ngo 1.22\n")
	if _, err := Build(BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on missing src/routes/")
	}
}

func TestBuild_RelativeProjectRoot(t *testing.T) {
	t.Parallel()
	if _, err := Build(BuildOptions{ProjectRoot: "relative/path"}); err == nil {
		t.Fatal("expected error on relative ProjectRoot")
	}
}

func TestBuild_ConflictAborts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "+page.svelte"), "<h1>p</h1>\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "server.go"),
		"//go:build sveltego\n\npackage api\n\nimport \"net/http\"\n\nvar Handlers = map[string]http.HandlerFunc{}\n")
	_, err := Build(BuildOptions{ProjectRoot: root})
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

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	first := snapshotGen(t, root)

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
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

func TestBuild_LibMissingWarning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "+page.svelte"),
		`<script lang="go">
import "$lib/db"
</script>
<h1>{db.Title()}</h1>
`)
	res, err := Build(BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "$lib referenced") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected $lib-missing warning, got %v", res.Diagnostics)
	}
}

func TestBuild_EmitsLayoutChain(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "+layout.svelte"),
		`<header>root</header><slot />`+"\n")
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "+layout.svelte"),
		`<nav>dash</nav><slot />`+"\n")
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "+page.svelte"),
		`<h1>Dash</h1>`+"\n")

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	rootLayout := filepath.Join(root, ".gen", "routes", "layout.gen.go")
	dashLayout := filepath.Join(root, ".gen", "routes", "dash", "layout.gen.go")
	dashPage := filepath.Join(root, ".gen", "routes", "dash", "page.gen.go")
	manifest := filepath.Join(root, ".gen", "manifest.gen.go")
	for _, p := range []string{rootLayout, dashLayout, dashPage, manifest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	rootBytes, err := os.ReadFile(rootLayout)
	if err != nil {
		t.Fatalf("read root layout: %v", err)
	}
	for _, want := range []string{
		"type Layout struct{}",
		"type LayoutData = struct{}",
		"children func(*render.Writer) error",
		"if children != nil",
	} {
		if !bytes.Contains(rootBytes, []byte(want)) {
			t.Errorf("root layout missing %q:\n%s", want, rootBytes)
		}
	}

	manifestBytes, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, want := range []string{
		`render__layout__page_routes`,
		`render__layout__page_routes_dash`,
		`LayoutChain: []router.LayoutHandler{`,
	} {
		if !bytes.Contains(manifestBytes, []byte(want)) {
			t.Errorf("manifest missing %q:\n%s", want, manifestBytes)
		}
	}

	for _, p := range []string{rootLayout, dashLayout, dashPage, manifest} {
		assertParsesAsGo(t, p)
	}
}

// TestBuild_EmitsLayoutServer covers Phase 0k-B: a +layout.server.go
// adjacent to +layout.svelte produces a layoutsrc mirror, a sibling
// wire_layout.gen.go in the gen package, a typed LayoutData alias from
// the inferred Load() return, and a manifest LayoutLoaders entry.
func TestBuild_EmitsLayoutServer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "+layout.svelte"),
		`<header>root</header><slot />`+"\n")
	writeFile(t, filepath.Join(root, "src", "routes", "layout.server.go"),
		`//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (LayoutData, error) {
	return struct{ User string }{User: "alice"}, nil
}
`)
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "+layout.svelte"),
		`<nav>dash</nav><slot />`+"\n")
	writeFile(t, filepath.Join(root, "src", "routes", "dash", "+page.svelte"),
		`<h1>Dash</h1>`+"\n")

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	rootLayout := filepath.Join(root, ".gen", "routes", "layout.gen.go")
	mirror := filepath.Join(root, ".gen", "layoutsrc", "routes", "layout_server.go")
	wire := filepath.Join(root, ".gen", "routes", "wire_layout.gen.go")
	manifest := filepath.Join(root, ".gen", "manifest.gen.go")
	for _, p := range []string{rootLayout, mirror, wire, manifest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	rootBytes, err := os.ReadFile(rootLayout)
	if err != nil {
		t.Fatalf("read root layout: %v", err)
	}
	for _, want := range []string{
		"type LayoutData = struct {",
		"User string",
	} {
		if !bytes.Contains(rootBytes, []byte(want)) {
			t.Errorf("root layout missing %q:\n%s", want, rootBytes)
		}
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

	for _, p := range []string{rootLayout, mirror, wire, manifest} {
		assertParsesAsGo(t, p)
	}
}

func TestBuild_EmbedSkippedWhenNoAssets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "embed.go")); err == nil {
		t.Errorf("embed.go should not be emitted without client/ or static/")
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

func TestRewriteLibImports(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     string
		module string
		want   string
		hit    bool
	}{
		{
			name:   "single-pkg",
			in:     `import "$lib/db"`,
			module: "example.com/app",
			want:   `import "example.com/app/lib/db"`,
			hit:    true,
		},
		{
			name:   "bare",
			in:     `import "$lib"`,
			module: "myapp",
			want:   `import "myapp/lib"`,
			hit:    true,
		},
		{
			name:   "no-hit",
			in:     `import "fmt"`,
			module: "myapp",
			want:   `import "fmt"`,
			hit:    false,
		},
		{
			name:   "multiple",
			in:     `"$lib/a" "$lib/b"`,
			module: "m",
			want:   `"m/lib/a" "m/lib/b"`,
			hit:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, hit := rewriteLibImports(tc.in, tc.module)
			if got != tc.want {
				t.Errorf("rewrite = %q want %q", got, tc.want)
			}
			if hit != tc.hit {
				t.Errorf("hit = %v want %v", hit, tc.hit)
			}
		})
	}
}
