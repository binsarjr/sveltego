package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitWire_LoadOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := mirrorRoute{
		encodedSubpath: "routes/posts/_slug_",
		packageName:    "_slug_",
		wireDir:        dir,
		hasActions:     false,
	}
	if err := emitWire(".gen", "example.com/app", r); err != nil {
		t.Fatalf("emitWire: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "wire.gen.go"))
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	got := string(src)
	if !strings.Contains(got, "package _slug_") {
		t.Errorf("missing package clause:\n%s", got)
	}
	if !strings.Contains(got, `usersrc "example.com/app/.gen/usersrc/routes/posts/_slug_"`) {
		t.Errorf("missing aliased mirror import:\n%s", got)
	}
	if !strings.Contains(got, "func Load(ctx *kit.LoadCtx) (any, error) { return usersrc.Load(ctx) }") {
		t.Errorf("missing Load wrapper:\n%s", got)
	}
	if !strings.Contains(got, "func Actions() any { return nil }") {
		t.Errorf("missing Actions stub when hasActions=false:\n%s", got)
	}
	if strings.Contains(got, "usersrc.Actions") {
		t.Errorf("Actions stub should not invoke usersrc when hasActions=false:\n%s", got)
	}
	assertParsesAsGo(t, filepath.Join(dir, "wire.gen.go"))
}

func TestEmitWire_LoadAndActions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := mirrorRoute{
		encodedSubpath: "routes",
		packageName:    "routes",
		wireDir:        dir,
		hasActions:     true,
	}
	if err := emitWire(".gen", "myapp", r); err != nil {
		t.Fatalf("emitWire: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "wire.gen.go"))
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	got := string(src)
	if !strings.Contains(got, "func Actions() any { return usersrc.Actions }") {
		t.Errorf("missing Actions wrapper:\n%s", got)
	}
	if !strings.Contains(got, `usersrc "myapp/.gen/usersrc/routes"`) {
		t.Errorf("missing mirror import:\n%s", got)
	}
	assertParsesAsGo(t, filepath.Join(dir, "wire.gen.go"))
}

func TestMirrorUserSource_StripsBuildTagAndRewritesPackage(t *testing.T) {
	t.Parallel()
	src := []byte(`//go:build sveltego

package origpkg

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

var Actions = kit.ActionMap{}

func Load(ctx *kit.LoadCtx) (any, error) { return nil, nil }
`)
	dir := t.TempDir()
	userPath := filepath.Join(dir, "_page.server.go")
	if err := os.WriteFile(userPath, src, 0o600); err != nil {
		t.Fatalf("write user: %v", err)
	}
	usf := userSourceFile{
		UserPath:    userPath,
		MirrorPath:  filepath.Join(dir, "mirror", "_slug_", "page_server.go"),
		PackageName: "_slug_",
	}
	if err := mirrorUserSource(&usf); err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if !usf.HasActions {
		t.Errorf("expected HasActions=true")
	}
	got, err := os.ReadFile(usf.MirrorPath)
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if bytes.Contains(got, []byte("//go:build")) {
		t.Errorf("expected build constraint stripped:\n%s", got)
	}
	if !bytes.Contains(got, []byte("package _slug_")) {
		t.Errorf("expected package clause rewritten:\n%s", got)
	}
	if !bytes.Contains(got, []byte("func Load")) {
		t.Errorf("expected Load body preserved:\n%s", got)
	}
	assertParsesAsGo(t, usf.MirrorPath)
}

func TestMirrorUserSource_NoActions(t *testing.T) {
	t.Parallel()
	src := []byte(`//go:build sveltego

package whatever

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) { return nil, nil }
`)
	dir := t.TempDir()
	userPath := filepath.Join(dir, "_page.server.go")
	if err := os.WriteFile(userPath, src, 0o600); err != nil {
		t.Fatalf("write user: %v", err)
	}
	usf := userSourceFile{
		UserPath:    userPath,
		MirrorPath:  filepath.Join(dir, "mirror", "routes", "page_server.go"),
		PackageName: "routes",
	}
	if err := mirrorUserSource(&usf); err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if usf.HasActions {
		t.Errorf("expected HasActions=false")
	}
}
