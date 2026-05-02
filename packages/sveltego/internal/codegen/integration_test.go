// Build tag `integration` gates this subprocess-style smoke test. Run with
// `go test -tags=integration ./internal/codegen/...`. Default `go test`
// skips the file entirely.
//go:build integration

package codegen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

import (
	"context"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct{ Greeting string }{Greeting: "hello"}, nil
}
`)

	mustWrite("src/routes/post/[id]/_page.svelte", "<h2>post id</h2>\n")
	mustWrite("src/routes/post/[id]/_page.server.go", `//go:build sveltego

package _id_

import (
	"context"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct{ ID string }{ID: ctx.Params["id"]}, nil
}

func Actions() any { return nil }
`)

	if _, err := Build(context.Background(), BuildOptions{ProjectRoot: sandbox}); err != nil {
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
