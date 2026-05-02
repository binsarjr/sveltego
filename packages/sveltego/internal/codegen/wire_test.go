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
		hasLoad:        true,
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

// TestEmitWire_NoLoadNoActions covers #467: a route ships
// _page.server.go with PageOptions (e.g. Prerender = true) but neither
// Load nor Actions. emitWire must drop a stub Load + stub Actions and
// must not import usersrc (which would be unused and break the build).
func TestEmitWire_NoLoadNoActions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := mirrorRoute{
		encodedSubpath: "routes",
		packageName:    "routes",
		wireDir:        dir,
		hasActions:     false,
		hasLoad:        false,
	}
	if err := emitWire(".gen", "example.com/app", r); err != nil {
		t.Fatalf("emitWire: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "wire.gen.go"))
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	got := string(src)
	if strings.Contains(got, "usersrc") {
		t.Errorf("expected no usersrc import when hasLoad=false && hasActions=false:\n%s", got)
	}
	if !strings.Contains(got, "func Load(ctx *kit.LoadCtx) (any, error)") {
		t.Errorf("missing Load stub:\n%s", got)
	}
	if !strings.Contains(got, "return nil, nil") {
		t.Errorf("Load stub must return nil, nil:\n%s", got)
	}
	if !strings.Contains(got, "func Actions() any { return nil }") {
		t.Errorf("missing Actions stub:\n%s", got)
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
		hasLoad:        true,
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
	if !usf.HasLoad {
		t.Errorf("expected HasLoad=true")
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

// TestEmitSSRLayoutWire verifies issue #440's children-callback layout
// wire helper writes a Go file with `RenderLayoutSSR(payload, data,
// inner)` that bridges the typed usersrc.Render to the manifest's
// future payload-shaped LayoutHandler dispatch.
func TestEmitSSRLayoutWire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := mirrorRoute{
		encodedSubpath: "routes/_layout",
		packageName:    "_layout",
		wireDir:        dir,
		hasSSRRender:   true,
	}
	if err := emitSSRLayoutWire(".gen", "example.com/app", r); err != nil {
		t.Fatalf("emitSSRLayoutWire: %v", err)
	}
	target := filepath.Join(dir, "wire_layout_render.gen.go")
	src, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	got := string(src)
	if !strings.Contains(got, "package _layout") {
		t.Errorf("missing package clause:\n%s", got)
	}
	if !strings.Contains(got, `usersrc "example.com/app/.gen/layoutsrc/routes/_layout"`) {
		t.Errorf("missing layoutsrc import:\n%s", got)
	}
	if !strings.Contains(got, "func RenderLayoutSSR(payload *server.Payload, data any, inner func(*server.Payload)) error {") {
		t.Errorf("missing RenderLayoutSSR signature:\n%s", got)
	}
	if !strings.Contains(got, "usersrc.Render(payload, typed, inner)") {
		t.Errorf("expected typed Render dispatch with inner callback:\n%s", got)
	}
	assertParsesAsGo(t, target)
}

// TestEmitSSRErrorWire verifies issue #412's error-boundary wire
// helper writes a Go file with `RenderErrorSSR(payload, safe)` that
// bridges the typed errorsrc.Render to the manifest's payload-shaped
// ErrorHandler dispatch. Both the kit and runtime/svelte/server
// imports must be present so SafeError and Payload resolve at compile
// time.
func TestEmitSSRErrorWire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := mirrorRoute{
		encodedSubpath: "routes",
		packageName:    "routes",
		wireDir:        dir,
	}
	if err := emitSSRErrorWire(".gen", "example.com/app", r); err != nil {
		t.Fatalf("emitSSRErrorWire: %v", err)
	}
	target := filepath.Join(dir, "wire_error_render.gen.go")
	src, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	got := string(src)
	if !strings.Contains(got, "package routes") {
		t.Errorf("missing package clause:\n%s", got)
	}
	if !strings.Contains(got, `errorsrc "example.com/app/.gen/errorsrc/routes"`) {
		t.Errorf("missing errorsrc import:\n%s", got)
	}
	if !strings.Contains(got, `"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`) {
		t.Errorf("missing kit import:\n%s", got)
	}
	if !strings.Contains(got, `server "github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/server"`) {
		t.Errorf("missing server import:\n%s", got)
	}
	if !strings.Contains(got, "func RenderErrorSSR(payload *server.Payload, safe kit.SafeError) {") {
		t.Errorf("missing RenderErrorSSR signature:\n%s", got)
	}
	if !strings.Contains(got, "errorsrc.Render(payload, safe)") {
		t.Errorf("expected typed Render dispatch:\n%s", got)
	}
	assertParsesAsGo(t, target)
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
	if !usf.HasLoad {
		t.Errorf("expected HasLoad=true")
	}
}

// TestMirrorUserSource_NoLoad covers #467: a route's _page.server.go
// declares only PageOptions (Templates, Prerender) and no Load. The
// mirror still copies the file but flags HasLoad=false so the wire
// emitter drops a stub Load instead of dispatching to usersrc.Load.
func TestMirrorUserSource_NoLoad(t *testing.T) {
	t.Parallel()
	src := []byte(`//go:build sveltego

package whatever

const (
	Templates = "svelte"
	Prerender = true
)
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
	if usf.HasLoad {
		t.Errorf("expected HasLoad=false (no Load func declared)")
	}
	if usf.HasActions {
		t.Errorf("expected HasActions=false")
	}
}
