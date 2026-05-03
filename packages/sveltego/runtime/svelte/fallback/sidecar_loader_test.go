package fallback

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSidecarRecursiveSvelteLoader boots the real ssr_serve.mjs against
// a fixture project whose `_page.svelte` imports a child `.svelte`
// component via the `$lib` alias. Without the Node loader hook
// registered in svelte_loader.mjs the child import fails with
// `ERR_UNKNOWN_FILE_EXTENSION` (issue #512). The test asserts the
// /render call returns 200 and the rendered body contains content
// emitted by the child.
//
// Skipped when:
//   - `node` is not on PATH
//   - the sidecar's vendored `node_modules` (svelte) hasn't been
//     installed (npm ci runs in CI; locally `npm install` in the
//     sidecar dir is the equivalent).
func TestSidecarRecursiveSvelteLoader(t *testing.T) {
	t.Parallel()
	nodePath := skipIfNoNode(t)
	sidecarDir := locateRealSidecar(t)
	skipIfSidecarMissingDeps(t, sidecarDir)

	// Build a fixture project with a parent _page.svelte that imports
	// a child Stat.svelte via $lib. Layout mirrors what kitchen-sink
	// produces: src/lib/Stat.svelte and src/routes/sidetest/_page.svelte.
	root := t.TempDir()
	libDir := filepath.Join(root, "src", "lib")
	pageDir := filepath.Join(root, "src", "routes", "sidetest")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const childSrc = `<script>
  let { label, value } = $props();
</script>
<span class="stat" data-label={label}>STAT:{label}={value}</span>
`
	const parentSrc = `<script>
  import Stat from '$lib/Stat.svelte';
  let { data } = $props();
</script>
<h1>SideTest</h1>
<Stat label="demo" value={data?.n ?? 0} />
`
	if err := os.WriteFile(filepath.Join(libDir, "Stat.svelte"), []byte(childSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(pageDir, "_page.svelte")
	if err := os.WriteFile(parentPath, []byte(parentSrc), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	side, err := Start(ctx, SidecarOptions{
		NodePath:    nodePath,
		SidecarDir:  sidecarDir,
		ProjectRoot: root,
	})
	if err != nil {
		t.Fatalf("Start sidecar: %v", err)
	}
	defer side.Stop()

	c := NewClient(ClientOptions{Endpoint: side.Endpoint(), CacheSize: 4, TTL: time.Minute})
	resp, err := c.Render(ctx, RenderRequest{
		Route:  "/sidetest",
		Source: parentPath,
		Data:   map[string]any{"n": 7},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The child Stat.svelte emits "STAT:demo=7" — verifying both the
	// child compiled successfully and the prop wiring made it through.
	if !strings.Contains(resp.Body, "STAT:demo=7") {
		t.Fatalf("response body missing child output: %q", resp.Body)
	}
	if !strings.Contains(resp.Body, "SideTest") {
		t.Fatalf("response body missing parent output: %q", resp.Body)
	}
}

// locateRealSidecar walks up from the current test file to find the
// vendored sidecar tree at packages/sveltego/internal/codegen/svelterender/sidecar.
func locateRealSidecar(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	candidate := filepath.Join(dir, "..", "..", "..", "internal", "codegen", "svelterender", "sidecar")
	abs, err := filepath.Abs(candidate)
	if err != nil {
		t.Fatalf("abs sidecar dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "index.mjs")); err != nil {
		t.Fatalf("vendored sidecar not at %s: %v", abs, err)
	}
	return abs
}

// skipIfSidecarMissingDeps short-circuits the test when the sidecar's
// node_modules tree (svelte, acorn) hasn't been installed. CI runs
// `npm ci` in the sidecar dir before invoking go test; locally
// developers may not.
func skipIfSidecarMissingDeps(t *testing.T, sidecarDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(sidecarDir, "node_modules", "svelte", "package.json")); err != nil {
		t.Skip("sidecar node_modules/svelte missing; run `npm ci` in " + sidecarDir + " to enable this test")
	}
}
