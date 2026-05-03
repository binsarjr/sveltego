package scaffold

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/init/internal/aitemplates"
)

func TestRun_BaseScaffold(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(Options{Dir: dir, Module: "example.com/hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("unexpected skipped on fresh dir: %v", res.Skipped)
	}
	wantPaths := []string{
		"go.mod",
		"README.md",
		".gitignore",
		"app.html",
		"package.json",
		"vite.config.js",
		"sveltego.config.go",
		"src/hooks.server.go",
		"cmd/app/main.go",
		"src/routes/_page.svelte",
		"src/routes/_page.server.go",
		"src/routes/_layout.svelte",
		"src/lib/.gitkeep",
	}
	for _, p := range wantPaths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !bytes.Contains(gomod, []byte("module example.com/hello")) {
		t.Errorf("go.mod missing module line, got: %s", gomod)
	}
	if !bytes.Contains(gomod, []byte("go 1.25")) {
		t.Errorf("go.mod missing go 1.25 directive, got: %s", gomod)
	}
}

// TestRun_GoModNoLiteralV000 pins the contract that the scaffold never
// emits a `require ... v0.0.0` line for the framework module. The
// release-please bootstrap pseudo-version churns on every commit to
// main, and the proxy cannot resolve the literal v0.0.0 — pinning it
// silently breaks every fresh project on first `go build`. See #110.
//
// The fix from the standalone-scaffold repro (#110): drop the require
// line entirely. `sveltego build` shells out to `go get @latest` on
// first invocation to seed it.
func TestRun_GoModNoLiteralV000(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if bytes.Contains(gomod, []byte("v0.0.0")) {
		t.Errorf("go.mod contains literal v0.0.0; the proxy cannot resolve it. body:\n%s", gomod)
	}
	// And no require line for the framework module at all — sveltego
	// build will add the resolved pseudo-version on first run.
	if bytes.Contains(gomod, []byte("github.com/binsarjr/sveltego")) {
		t.Errorf("go.mod references sveltego before first build; expected bare module clause. body:\n%s", gomod)
	}
}

// TestRun_MainGoCompiles parses cmd/app/main.go to confirm the scaffold
// emits syntactically valid Go and that the import path for the generated
// package picks up the user's module path. Full type-check would require
// the workspace replace + the .gen package to exist, which is the
// `sveltego build` integration's job; parsing alone catches the common
// scaffold corruption cases (broken raw strings, stale module slot).
func TestRun_MainGoCompiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mainPath := filepath.Join(dir, "cmd", "app", "main.go")
	body, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, mainPath, body, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse main.go: %v\nbody:\n%s", err, body)
	}
	if f.Name.Name != "main" {
		t.Errorf("main.go package = %q, want main", f.Name.Name)
	}
	wantImports := []string{
		`"example.com/hello/.gen"`,
		`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`,
		`"github.com/binsarjr/sveltego/packages/sveltego/server"`,
	}
	got := make(map[string]bool, len(f.Imports))
	for _, imp := range f.Imports {
		got[imp.Path.Value] = true
	}
	for _, want := range wantImports {
		if !got[want] {
			t.Errorf("main.go missing import %s; got %v", want, got)
		}
	}
}

func TestRun_AppHTMLPlaceholders(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "app.html"))
	if err != nil {
		t.Fatalf("read app.html: %v", err)
	}
	for _, marker := range []string{"%sveltego.head%", "%sveltego.body%", "<!DOCTYPE html>"} {
		if !bytes.Contains(body, []byte(marker)) {
			t.Errorf("app.html missing %q; got:\n%s", marker, body)
		}
	}
}

func TestRun_PackageJSONNameDerivedFromModule(t *testing.T) {
	cases := []struct {
		module string
		want   string
	}{
		{"example.com/hello", "hello"},
		{"github.com/foo/My-App", "my-app"},
		{"example.com/Bar", "bar"},
		{"weird!!chars", "weirdchars"},
		{"example.com/!!!", "sveltego-app"},
	}
	for _, tc := range cases {
		t.Run(tc.module, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Run(Options{Dir: dir, Module: tc.module}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			body, err := os.ReadFile(filepath.Join(dir, "package.json"))
			if err != nil {
				t.Fatalf("read package.json: %v", err)
			}
			needle := `"name": "` + tc.want + `"`
			if !strings.Contains(string(body), needle) {
				t.Errorf("package.json missing %q; got:\n%s", needle, body)
			}
		})
	}
}

func TestRun_AICopiesEmbedFSByteEqual(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, AI: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range aitemplates.Files {
		want, err := fs.ReadFile(aitemplates.FS, name)
		if err != nil {
			t.Fatalf("embed read %q: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("read scaffolded %q: %v", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("template %q drifted from embed.FS bytes", name)
		}
	}
}

func TestRun_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	hooksPath := filepath.Join(hooksDir, "hooks.server.go")
	if err := os.WriteFile(hooksPath, []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := Run(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(res.Skipped, "src/hooks.server.go") {
		t.Errorf("expected src/hooks.server.go in skipped, got %v", res.Skipped)
	}
	body, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if string(body) != "// pre-existing\n" {
		t.Errorf("src/hooks.server.go was overwritten without --force; got %q", body)
	}
}

func TestRun_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	hooksPath := filepath.Join(hooksDir, "hooks.server.go")
	if err := os.WriteFile(hooksPath, []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := Run(Options{Dir: dir, Force: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if contains(res.Skipped, "src/hooks.server.go") {
		t.Errorf("src/hooks.server.go skipped despite --force: %v", res.Skipped)
	}
	body, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if !bytes.Contains(body, []byte("func Handle(")) {
		t.Errorf("src/hooks.server.go not overwritten; got %q", body)
	}
}

func TestWriteAITemplates_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(target, []byte("local notes\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := WriteAITemplates(dir, false)
	if err != nil {
		t.Fatalf("WriteAITemplates: %v", err)
	}
	if !contains(res.Skipped, "CLAUDE.md") {
		t.Errorf("expected CLAUDE.md in skipped, got %v", res.Skipped)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if string(body) != "local notes\n" {
		t.Errorf("CLAUDE.md overwritten without --force; got %q", body)
	}
}

// TestRun_ServiceWorkerOptIn covers the --service-worker scaffold flag
// from issue #89: opt-in emits src/service-worker.ts; default omits it.
func TestRun_ServiceWorkerOptIn(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/sw", ServiceWorker: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	swPath := filepath.Join(dir, "src", "service-worker.ts")
	body, err := os.ReadFile(swPath)
	if err != nil {
		t.Fatalf("read service-worker.ts: %v", err)
	}
	for _, want := range []string{
		"<reference lib=\"webworker\"",
		"sw.addEventListener('install'",
		"sw.skipWaiting()",
		"sw.clients.claim()",
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("service-worker.ts missing %q\n--- body:\n%s", want, body)
		}
	}
}

// TestRun_ServiceWorkerOmittedByDefault asserts the scaffold does NOT
// write src/service-worker.ts unless ServiceWorker is set, so existing
// users do not get a worker registered they did not opt into (#89).
func TestRun_ServiceWorkerOmittedByDefault(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/sw"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "service-worker.ts")); err == nil {
		t.Fatal("expected src/service-worker.ts to be absent without --service-worker")
	}
}

func TestRun_EmptyDirRejected(t *testing.T) {
	if _, err := Run(Options{}); err == nil {
		t.Errorf("expected error on empty Dir")
	}
}

// TestRun_HooksUseFuncDecl pins the Handle hook to the canonical
// `func Handle(...)` declaration so the codegen scanner (which prefers
// FuncDecls) wires it. Drift back to `var Handle = ...` would silently
// drop hook wiring (#415).
func TestRun_HooksUseFuncDecl(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "src", "hooks.server.go"))
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if !bytes.Contains(body, []byte("func Handle(")) {
		t.Errorf("hooks.server.go missing `func Handle(` form; body:\n%s", body)
	}
	if bytes.Contains(body, []byte("var Handle")) {
		t.Errorf("hooks.server.go uses obsolete `var Handle` form; body:\n%s", body)
	}
}

// TestRun_PageSvelteNoGoScript pins the scaffolded _page.svelte and
// _layout.svelte to the pure-Svelte template. The obsolete
// `<script lang="go">` block from the Mustache-Go era confuses Svelte's
// parser when users add a normal `<script>` block (#416).
func TestRun_PageSvelteNoGoScript(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, rel := range []string{"src/routes/_page.svelte", "src/routes/_layout.svelte"} {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if bytes.Contains(body, []byte(`<script lang="go"`)) {
			t.Errorf("%s still contains obsolete <script lang=\"go\"> block:\n%s", rel, body)
		}
	}
}

// TestRun_MainGoWiresViteAndStatic asserts cmd/app/main.go reads the
// Vite manifest and mounts a static handler at /_app/. Without this the
// SSR shell omits asset tags and asset URLs 404 (#417).
func TestRun_MainGoWiresViteAndStatic(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, Module: "example.com/hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "cmd", "app", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, want := range []string{
		"static/_app/.vite/manifest.json",
		"ViteManifest:",
		"ViteBase:",
		"server.StaticHandler(",
		`"/_app/"`,
		`http.StripPrefix("/_app"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("main.go missing %q; body:\n%s", want, body)
		}
	}
}

// TestRun_PageDataMatchesJSONTagContract pins the scaffold to the
// Go ↔ TypeScript boundary the SSR transpiler enforces (ADR 0008): every
// PageData field carries an explicit `json:"..."` tag, and the template
// reads `data.<lowercase>` matching that tag. Without this, the welcome
// page fails to build out of the box with "<Field> not in PageData JSON
// tag map" (#524).
func TestRun_PageDataMatchesJSONTagContract(t *testing.T) {
	for _, flavor := range []TailwindFlavor{TailwindNone, TailwindV4, TailwindV3} {
		t.Run(string(flavor), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Run(Options{Dir: dir, Module: "example.com/hello", Tailwind: flavor}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			server, err := os.ReadFile(filepath.Join(dir, "src/routes/_page.server.go"))
			if err != nil {
				t.Fatalf("read _page.server.go: %v", err)
			}
			if !bytes.Contains(server, []byte("`json:\"greeting\"`")) {
				t.Errorf("_page.server.go missing `json:\"greeting\"` tag on PageData.Greeting; body:\n%s", server)
			}
			page, err := os.ReadFile(filepath.Join(dir, "src/routes/_page.svelte"))
			if err != nil {
				t.Fatalf("read _page.svelte: %v", err)
			}
			if !bytes.Contains(page, []byte("{data.greeting}")) {
				t.Errorf("_page.svelte must read {data.greeting} (lowercase, matching json tag); body:\n%s", page)
			}
			if bytes.Contains(page, []byte("{data.Greeting}")) {
				t.Errorf("_page.svelte references {data.Greeting} (uppercase) — transpiler rejects this; body:\n%s", page)
			}
		})
	}
}

func contains(xs []string, s string) bool {
	i := sort.SearchStrings(xs, s)
	return i < len(xs) && xs[i] == s
}
