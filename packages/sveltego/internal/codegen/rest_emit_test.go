package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_EmitsRESTDispatcher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "users", "_server.go"),
		`//go:build sveltego

package users

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func GET(ev *kit.RequestEvent) *kit.Response {
	_ = ev
	return kit.JSON(200, kit.M{"ok": true})
}

func POST(ev *kit.RequestEvent) *kit.Response {
	_ = ev
	return kit.JSON(201, kit.M{"ok": true})
}
`)

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	mirror := filepath.Join(root, ".gen", "usersrc", "routes", "api", "users", "server.go")
	dispatcher := filepath.Join(root, ".gen", "routes", "api", "users", "server.gen.go")
	manifest := filepath.Join(root, ".gen", "manifest.gen.go")
	for _, p := range []string{mirror, dispatcher, manifest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	mirrorBytes, _ := os.ReadFile(mirror)
	if bytes.Contains(mirrorBytes, []byte("//go:build")) {
		t.Errorf("mirror retained build constraint:\n%s", mirrorBytes)
	}
	if !bytes.Contains(mirrorBytes, []byte("package users")) {
		t.Errorf("mirror package clause wrong:\n%s", mirrorBytes)
	}

	dispatcherBytes, _ := os.ReadFile(dispatcher)
	for _, want := range []string{
		"package users",
		`usersrc "example.com/app/.gen/usersrc/routes/api/users"`,
		`var Handlers = map[string]http.HandlerFunc{`,
		`"GET":  dispatch(usersrc.GET),`,
		`"POST": dispatch(usersrc.POST),`,
		`func dispatch(verb func(*kit.RequestEvent) *kit.Response) http.HandlerFunc {`,
	} {
		if !bytes.Contains(dispatcherBytes, []byte(want)) {
			t.Errorf("dispatcher missing %q:\n%s", want, dispatcherBytes)
		}
	}

	manifestBytes, _ := os.ReadFile(manifest)
	for _, want := range []string{
		`Pattern: ` + "`/api/users`",
		`Server: page_routes_api_users.Handlers,`,
	} {
		if !bytes.Contains(manifestBytes, []byte(want)) {
			t.Errorf("manifest missing %q:\n%s", want, manifestBytes)
		}
	}

	for _, p := range []string{mirror, dispatcher, manifest} {
		assertParsesAsGo(t, p)
	}
}

func TestBuild_RESTUnknownVerbErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "src", "routes", "api", "_server.go"),
		`//go:build sveltego

package api

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Get(ev *kit.RequestEvent) *kit.Response { return kit.NoContent() }
`)
	if _, err := Build(BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on unknown exported function")
	}
}
