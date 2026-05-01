package codegen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEnvLookup is a stub EnvLookup used across substituteStaticEnv unit tests.
var testEnvLookup = func(key string) (string, bool) {
	m := map[string]string{
		"PUBLIC_API_URL": "https://api.example.com",
		"PUBLIC_EMPTY":   "",
	}
	v, ok := m[key]
	return v, ok
}

func TestSubstituteStaticEnv(t *testing.T) {
	t.Run("no static calls → unchanged", func(t *testing.T) {
		src := `<p>{greeting}</p>`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != src {
			t.Errorf("want unchanged, got %q", got)
		}
	})

	t.Run("StaticPublic substituted to literal", func(t *testing.T) {
		src := `<p>{env.StaticPublic("PUBLIC_API_URL")}</p>`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, `"https://api.example.com"`) {
			t.Errorf("want literal URL in %q", got)
		}
		if strings.Contains(got, "env.StaticPublic") {
			t.Errorf("call site not removed in %q", got)
		}
	})

	t.Run("empty value is valid", func(t *testing.T) {
		src := `{env.StaticPublic("PUBLIC_EMPTY")}`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, `""`) {
			t.Errorf("want empty string literal in %q", got)
		}
	})

	t.Run("multiple calls in same source", func(t *testing.T) {
		src := `<a href={env.StaticPublic("PUBLIC_API_URL")}>{env.StaticPublic("PUBLIC_API_URL")}</a>`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(got, "env.StaticPublic") {
			t.Errorf("call site not fully removed in %q", got)
		}
		if strings.Count(got, `"https://api.example.com"`) != 2 {
			t.Errorf("expected 2 literal replacements in %q", got)
		}
	})

	t.Run("unset key returns error", func(t *testing.T) {
		src := `{env.StaticPublic("PUBLIC_MISSING_KEY")}`
		_, err := substituteStaticEnv(src, testEnvLookup)
		if err == nil {
			t.Fatal("expected error for unset key, got nil")
		}
		if !strings.Contains(err.Error(), "PUBLIC_MISSING_KEY") {
			t.Errorf("error should mention the key: %v", err)
		}
		if !strings.Contains(err.Error(), "unset in build environment") {
			t.Errorf("error should describe cause: %v", err)
		}
	})

	t.Run("StaticPrivate not substituted", func(t *testing.T) {
		// substituteStaticEnv only rewrites StaticPublic. StaticPrivate
		// must pass through so checkPrivateEnv can reject it.
		src := `{env.StaticPrivate("DATABASE_URL")}`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "env.StaticPrivate") {
			t.Errorf("StaticPrivate should be untouched, got %q", got)
		}
	})

	t.Run("DynamicPublic not substituted", func(t *testing.T) {
		src := `{env.DynamicPublic("PUBLIC_API_URL")}`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "env.DynamicPublic") {
			t.Errorf("DynamicPublic should be untouched, got %q", got)
		}
	})

	t.Run("DynamicPrivate not substituted", func(t *testing.T) {
		src := `{env.DynamicPrivate("X")}`
		got, err := substituteStaticEnv(src, testEnvLookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "env.DynamicPrivate") {
			t.Errorf("DynamicPrivate should be untouched, got %q", got)
		}
	})

	t.Run("value with special chars produces valid Go literal", func(t *testing.T) {
		lookup := func(string) (string, bool) { return `say "hi"`, true }
		src := `{env.StaticPublic("PUBLIC_GREET")}`
		got, err := substituteStaticEnv(src, lookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// strconv.Quote wraps in double quotes with escaping.
		if !strings.Contains(got, `"say \"hi\""`) {
			t.Errorf("want escaped literal in %q", got)
		}
	})
}

// ---- Build-level integration tests ----

// envSubstRoot returns a temp directory wired with a minimal go.mod + routes dir.
func envSubstRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/envtest\n\ngo 1.22\n")
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	return root
}

// writePlainPage writes a _page.svelte with the given body.
func writePlainPage(t *testing.T, root, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"), body)
}

// readGenPageSrc returns the generated page.gen.go content.
func readGenPageSrc(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".gen", "routes", "page.gen.go"))
	if err != nil {
		t.Fatalf("read page.gen.go: %v", err)
	}
	return string(b)
}

func TestBuildSubstitutesPublicEnv(t *testing.T) {
	t.Skip("Mustache-Go env.StaticPublic() body emitter unreachable after #384; rewrite against pure-Svelte expectations in #406")
	root := envSubstRoot(t)
	writePlainPage(t, root, `<p>{env.StaticPublic("PUBLIC_SITE_NAME")}</p>`)

	lookup := func(key string) (string, bool) {
		if key == "PUBLIC_SITE_NAME" {
			return "MySite", true
		}
		return "", false
	}

	result, err := Build(context.Background(), BuildOptions{ProjectRoot: root, EnvLookup: lookup})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Routes != 1 {
		t.Fatalf("want 1 route, got %d", result.Routes)
	}

	src := readGenPageSrc(t, root)
	if !strings.Contains(src, `"MySite"`) {
		t.Errorf("generated source missing literal; got:\n%s", src)
	}
	if strings.Contains(src, "env.StaticPublic") {
		t.Errorf("env.StaticPublic call not removed; got:\n%s", src)
	}
}

func TestBuildRejectsUnsetKey(t *testing.T) {
	t.Skip("Mustache-Go env.StaticPublic() body emitter unreachable after #384; rewrite against pure-Svelte expectations in #406")
	root := envSubstRoot(t)
	writePlainPage(t, root, `<p>{env.StaticPublic("PUBLIC_MISSING")}</p>`)

	_, err := Build(context.Background(), BuildOptions{
		ProjectRoot: root,
		EnvLookup:   func(string) (string, bool) { return "", false },
	})
	if err == nil {
		t.Fatal("expected error for unset key, got nil")
	}
	if !strings.Contains(err.Error(), "PUBLIC_MISSING") {
		t.Errorf("error should mention key name, got: %v", err)
	}
}

// TestBuildRejectsPrivateEnvInTemplate verifies that env.StaticPrivate in a
// template is rejected by the private-leak guard. substituteStaticEnv does
// not rewrite StaticPrivate, so checkPrivateEnv sees the call and aborts.
func TestBuildRejectsPrivateEnvInTemplate(t *testing.T) {
	t.Skip("Mustache-Go body env.StaticPrivate() check unreachable after #384; rewrite against pure-Svelte expectations in #406")
	root := envSubstRoot(t)
	writePlainPage(t, root, `<p>{env.StaticPrivate("DATABASE_URL")}</p>`)

	_, err := Build(context.Background(), BuildOptions{
		ProjectRoot: root,
		EnvLookup:   func(_ string) (string, bool) { return "postgres://x", true },
	})
	if err == nil {
		t.Fatal("expected private-env diagnostic, got nil")
	}
	if !strings.Contains(err.Error(), "private env access not allowed") {
		t.Errorf("want private-env diagnostic, got: %v", err)
	}
}

// TestBuildDefaultEnvLookupUsesOsEnv verifies that nil EnvLookup falls
// back to os.LookupEnv.
func TestBuildDefaultEnvLookupUsesOsEnv(t *testing.T) {
	t.Skip("Mustache-Go env.StaticPublic() body emitter unreachable after #384; rewrite against pure-Svelte expectations in #406")
	root := envSubstRoot(t)
	writePlainPage(t, root, `<p>{env.StaticPublic("PUBLIC_SVELTEGO_TEST_SUBST")}</p>`)

	t.Setenv("PUBLIC_SVELTEGO_TEST_SUBST", "hello-from-os")

	result, err := Build(context.Background(), BuildOptions{ProjectRoot: root}) // no EnvLookup → os.LookupEnv
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Routes != 1 {
		t.Fatalf("want 1 route, got %d", result.Routes)
	}

	src := readGenPageSrc(t, root)
	if !strings.Contains(src, `"hello-from-os"`) {
		t.Errorf("generated source missing os env literal; got:\n%s", src)
	}
}

// TestBuildSubstitutesPublicEnvInLayout verifies that env.StaticPublic
// calls in _layout.svelte are also inlined at build time.
func TestBuildSubstitutesPublicEnvInLayout(t *testing.T) {
	root := envSubstRoot(t)

	writeFile(t, filepath.Join(root, "src", "routes", "_layout.svelte"),
		`<div class={env.StaticPublic("PUBLIC_THEME")}><slot/></div>`)
	writePlainPage(t, root, `<p>hello</p>`)

	lookup := func(key string) (string, bool) {
		if key == "PUBLIC_THEME" {
			return "dark", true
		}
		return "", false
	}

	_, err := Build(context.Background(), BuildOptions{ProjectRoot: root, EnvLookup: lookup})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(root, ".gen", "routes", "layout.gen.go"))
	if err != nil {
		t.Fatalf("read layout.gen.go: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, `"dark"`) {
		t.Errorf("layout.gen.go missing literal; got:\n%s", src)
	}
	if strings.Contains(src, "env.StaticPublic") {
		t.Errorf("env.StaticPublic not removed from layout; got:\n%s", src)
	}
}
