package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDocsAndGetDocPage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "guide", "form-actions.md"), "# Form Actions\n\nForm actions handle POST submissions in sveltego.\n")
	mustWrite(t, filepath.Join(dir, "guide", "hooks.md"), "# Hooks\n\nHooks intercept requests.\n")
	mustWrite(t, filepath.Join(dir, "intro.md"), "# Intro\n\nWelcome to sveltego.\n")

	srv := New(Config{Root: dir, DocsDir: dir, KitDir: dir, PlaygroundsDir: dir})

	res, err := srv.Call(context.Background(), "search_docs", json.RawMessage(`{"query":"form actions","limit":3}`))
	if err != nil {
		t.Fatalf("search_docs: %v", err)
	}
	if res.IsError {
		t.Fatalf("search_docs returned tool-error: %s", res.Text)
	}
	if !strings.Contains(res.Text, "form-actions") {
		t.Errorf("search results missing form-actions hit:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "Form Actions") {
		t.Errorf("search results missing extracted title:\n%s", res.Text)
	}

	res, err = srv.Call(context.Background(), "get_doc_page", json.RawMessage(`{"slug":"guide/form-actions"}`))
	if err != nil {
		t.Fatalf("get_doc_page: %v", err)
	}
	if !strings.Contains(res.Text, "# Form Actions") {
		t.Errorf("get_doc_page body missing heading: %q", res.Text)
	}

	res, err = srv.Call(context.Background(), "get_doc_page", json.RawMessage(`{"slug":"intro"}`))
	if err != nil {
		t.Fatalf("get_doc_page intro: %v", err)
	}
	if !strings.Contains(res.Text, "Welcome to sveltego") {
		t.Errorf("get_doc_page bare slug failed: %q", res.Text)
	}

	if _, err := srv.Call(context.Background(), "get_doc_page", json.RawMessage(`{"slug":"missing"}`)); err == nil {
		t.Errorf("expected error for missing doc page")
	}
	if _, err := srv.Call(context.Background(), "get_doc_page", json.RawMessage(`{"slug":"../etc/passwd"}`)); err == nil {
		t.Errorf("expected error for traversal slug")
	}
}

func TestSearchDocsEmptyDir(t *testing.T) {
	t.Parallel()

	srv := New(Config{DocsDir: t.TempDir()})
	res, err := srv.Call(context.Background(), "search_docs", json.RawMessage(`{"query":"anything"}`))
	if err != nil {
		t.Fatalf("search_docs: %v", err)
	}
	if !strings.Contains(res.Text, "no matches") {
		t.Errorf("expected no-matches message, got %q", res.Text)
	}
}

func TestLookupAPI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := `package kit

// Greeter says hello.
type Greeter struct {
	Name string
}

// Hello returns a greeting.
func (g *Greeter) Hello() string { return "hi " + g.Name }

// Redirect returns a redirect error.
func Redirect(code int, location string) error { return nil }

// MaxRetries caps retry attempts.
const MaxRetries = 5
`
	mustWrite(t, filepath.Join(dir, "kit.go"), src)

	srv := New(Config{KitDir: dir})

	cases := []struct {
		symbol     string
		wantSubstr []string
	}{
		{"Redirect", []string{"func Redirect", "Redirect returns a redirect error"}},
		{"Greeter", []string{"type Greeter", "Greeter says hello", "Hello"}},
		{"Hello", []string{"Hello", "greeting"}},
		{"MaxRetries", []string{"MaxRetries", "caps retry attempts"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.symbol, func(t *testing.T) {
			t.Parallel()
			args, _ := json.Marshal(map[string]string{"symbol": tc.symbol})
			res, err := srv.Call(context.Background(), "lookup_api", args)
			if err != nil {
				t.Fatalf("lookup_api(%s): %v", tc.symbol, err)
			}
			for _, want := range tc.wantSubstr {
				if !strings.Contains(res.Text, want) {
					t.Errorf("output for %s missing %q:\n%s", tc.symbol, want, res.Text)
				}
			}
		})
	}

	if _, err := srv.Call(context.Background(), "lookup_api", json.RawMessage(`{"symbol":"DoesNotExist"}`)); err == nil {
		t.Errorf("expected error for missing symbol")
	}
}

func TestScaffoldRoute(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	cases := []struct {
		path, kind string
		want       []string
		notWant    []string
	}{
		{
			"about", "page",
			[]string{"src/routes/about/_page.svelte", "src/routes/about/page.server.go", "//go:build sveltego", "func Load(ctx *kit.LoadCtx)"},
			[]string{"_page.server.go"},
		},
		{
			"dash", "layout",
			[]string{"src/routes/dash/_layout.svelte", "src/routes/dash/layout.server.go", "<slot />"},
			nil,
		},
		{
			"api/health", "server",
			[]string{"src/routes/api/health/server.go", "func GET(ev *kit.RequestEvent)", "//go:build sveltego"},
			[]string{"_server.go"},
		},
		{
			"", "error",
			[]string{"src/routes/_error.svelte", "Data.Status"},
			nil,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path+"/"+tc.kind, func(t *testing.T) {
			t.Parallel()
			args, _ := json.Marshal(map[string]string{"path": tc.path, "kind": tc.kind})
			if tc.path == "" {
				args, _ = json.Marshal(map[string]string{"path": "/", "kind": tc.kind})
			}
			res, err := srv.Call(context.Background(), "scaffold_route", args)
			if err != nil {
				t.Fatalf("scaffold_route: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(res.Text, w) {
					t.Errorf("missing %q in output:\n%s", w, res.Text)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(res.Text, nw) {
					t.Errorf("output should not contain %q:\n%s", nw, res.Text)
				}
			}
		})
	}

	if _, err := srv.Call(context.Background(), "scaffold_route", json.RawMessage(`{"path":"x","kind":"weird"}`)); err == nil {
		t.Errorf("expected error for unknown kind")
	}
}

func TestGetExample(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "demo", "main.go"), "package main\n\nfunc main() {}\n")
	mustWrite(t, filepath.Join(dir, "demo", "src", "app.html"), "<html></html>\n")
	mustWrite(t, filepath.Join(dir, "demo", "node_modules", "fake.js"), "should be skipped")

	srv := New(Config{PlaygroundsDir: dir})

	res, err := srv.Call(context.Background(), "get_example", json.RawMessage(`{"name":"demo"}`))
	if err != nil {
		t.Fatalf("get_example: %v", err)
	}
	if !strings.Contains(res.Text, "demo/main.go") {
		t.Errorf("missing main.go header in output:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "demo/src/app.html") {
		t.Errorf("missing app.html header in output:\n%s", res.Text)
	}
	if strings.Contains(res.Text, "node_modules") {
		t.Errorf("node_modules should be skipped, got:\n%s", res.Text)
	}

	if _, err := srv.Call(context.Background(), "get_example", json.RawMessage(`{"name":"missing"}`)); err == nil {
		t.Errorf("expected error for missing example")
	}
	if _, err := srv.Call(context.Background(), "get_example", json.RawMessage(`{"name":"../etc"}`)); err == nil {
		t.Errorf("expected error for traversal example name")
	}
}

func TestValidateTemplateStub(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	res, err := srv.Call(context.Background(), "validate_template", json.RawMessage(`{"source":"{#if x}"}`))
	if err != nil {
		t.Fatalf("validate_template: %v", err)
	}
	if !strings.Contains(res.Text, "follow-up") {
		t.Errorf("expected stub message mentioning follow-up, got %q", res.Text)
	}
}

func TestToolsCallEndToEnd(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	args, _ := json.Marshal(map[string]string{"path": "x", "kind": "page"})
	params, _ := json.Marshal(toolCallParams{Name: "scaffold_route", Arguments: args})
	res, err := srv.onToolsCall(context.Background(), params)
	if err != nil {
		t.Fatalf("onToolsCall: %v", err)
	}
	tcr, ok := res.(toolCallResult)
	if !ok {
		t.Fatalf("result type = %T, want toolCallResult", res)
	}
	if tcr.IsError {
		t.Errorf("unexpected tool error: %+v", tcr)
	}
	if len(tcr.Content) == 0 || tcr.Content[0].Type != "text" {
		t.Errorf("unexpected content: %+v", tcr.Content)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}
