// Build tag `integration` gates this subprocess-style smoke test. Run with
// `go test -tags=integration ./internal/codegen/...`. Default `go test`
// skips the file entirely.
//go:build integration

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
)

// TestIntegrationSmoke writes generated `.gen.go` files for a handful of
// fixtures into a temporary module, then runs `go build ./...` against the
// real render + kit packages to prove the codegen output is compilable.
func TestIntegrationSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration smoke in -short mode")
	}

	sveltegoModuleDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs sveltego module: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sveltegoModuleDir, "go.mod")); err != nil {
		t.Fatalf("expected sveltego go.mod at %s: %v", sveltegoModuleDir, err)
	}

	sandbox := t.TempDir()

	goMod := strings.Join([]string{
		"module sveltegosmoke",
		"",
		"go 1.22",
		"",
		"require github.com/binsarjr/sveltego/packages/sveltego v0.0.0-00010101000000-000000000000",
		"",
		"replace github.com/binsarjr/sveltego/packages/sveltego => " + sveltegoModuleDir,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(sandbox, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Fixtures chosen to exercise text + element + mustache + if + each
	// without rune placeholders or non-compiling stubs. Companion `decls.go`
	// supplies the unbound names referenced in mustache expressions so the
	// sandbox compiles in isolation; in production those names come from a
	// sibling _page.server.go.
	type fixture struct {
		dir    string
		svelte string
		decls  string
	}
	fixtures := []fixture{
		{
			dir:    "minimal",
			svelte: "01-empty.svelte",
		},
		{
			dir:    "mustacheif",
			svelte: "49-if-with-mustache-body.svelte",
			decls:  "package page\n\nvar Data = struct {\n\tOk  bool\n\tMsg string\n}{Ok: true, Msg: \"hello\"}\n",
		},
		{
			dir:    "eachelement",
			svelte: "57-each-with-element-body.svelte",
			decls:  "package page\n\nvar Posts = []struct{ Title string }{{Title: \"first\"}}\n",
		},
	}

	for _, fx := range fixtures {
		fxPath := filepath.Join("testdata", "codegen", fx.svelte)
		src, err := os.ReadFile(fxPath)
		if err != nil {
			t.Fatalf("read fixture %s: %v", fx.svelte, err)
		}
		frag, perrs := parser.Parse(src)
		if len(perrs) > 0 {
			t.Fatalf("parse %s: %v", fx.svelte, perrs)
		}
		out, err := Generate(frag, Options{PackageName: "page"})
		if err != nil {
			t.Fatalf("generate %s: %v", fx.svelte, err)
		}
		if strings.Contains(string(out), runePrefix) {
			t.Fatalf("fixture %s leaked %s placeholder; smoke must not hit rune fixtures", fx.svelte, runePrefix)
		}
		if strings.Contains(string(out), "// TODO:") {
			t.Fatalf("fixture %s emitted TODO stub:\n%s", fx.svelte, out)
		}

		dir := filepath.Join(sandbox, fx.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "page.gen.go"), out, 0o644); err != nil {
			t.Fatalf("write page.gen.go for %s: %v", fx.dir, err)
		}
		if fx.decls != "" {
			if err := os.WriteFile(filepath.Join(dir, "decls.go"), []byte(fx.decls), 0o644); err != nil {
				t.Fatalf("write decls.go for %s: %v", fx.dir, err)
			}
		}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s (%s/%s):\n%s", sandbox, runtime.GOOS, runtime.GOARCH, output)
	}
}

// TestIntegrationManifestComposes runs Build end-to-end on a synthetic
// project tree exercising the cases that broke Phase 0j: a typed
// PageData inferred from a _page.server.go inline-struct return, a
// route directory with a `[id]` segment whose .go files carry
// //go:build sveltego, and a manifest that wires Render adapters plus
// wire.gen.go Load/Actions wrappers. The sandbox is then built with
// GOWORK=off + go build ./... to prove the entire .gen tree composes
// against the real router/render/kit packages.
func TestIntegrationManifestComposes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration smoke in -short mode")
	}

	sveltegoModuleDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs sveltego module: %v", err)
	}
	sandbox := t.TempDir()

	goMod := strings.Join([]string{
		"module manifestsmoke",
		"",
		"go 1.22",
		"",
		"require github.com/binsarjr/sveltego/packages/sveltego v0.0.0-00010101000000-000000000000",
		"",
		"replace github.com/binsarjr/sveltego/packages/sveltego => " + sveltegoModuleDir,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(sandbox, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	mustWrite := func(rel, body string) {
		t.Helper()
		full := filepath.Join(sandbox, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	mustWrite("src/routes/_page.svelte", "<h1>hello</h1>\n")
	mustWrite("src/routes/_page.server.go", `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct{ Greeting string }{Greeting: "hello"}, nil
}
`)

	mustWrite("src/routes/post/[id]/_page.svelte", "<h2>post id</h2>\n")
	mustWrite("src/routes/post/[id]/_page.server.go", `//go:build sveltego

package _id_

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct{ ID string }{ID: ctx.Params["id"]}, nil
}

func Actions() any { return nil }
`)

	if _, err := Build(BuildOptions{ProjectRoot: sandbox}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s (%s/%s):\n%s", sandbox, runtime.GOOS, runtime.GOARCH, out)
	}

	// Probe binary that uses gen.Routes() -> router.NewTree to prove the
	// manifest's Page handlers satisfy router.PageHandler at compile time.
	mustWrite("cmd/probe/main.go", `package main

import (
	"manifestsmoke/.gen"

	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func main() {
	if _, err := router.NewTree(gen.Routes()); err != nil {
		panic(err)
	}
}
`)
	cmd = exec.Command("go", "build", "./cmd/probe/...")
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("probe build failed in %s:\n%s", sandbox, out)
	}
}
