package codegen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuild_ServiceWorker_DetectedAndWired covers the happy path for issue
// #89: when src/service-worker.ts exists, Build sets HasServiceWorker on
// the result, the generated manifest declares `const HasServiceWorker = true`,
// and vite.config.gen.js carries the SW Rollup input plus the
// `../service-worker.js` entryFileNames hook so the worker lands at the
// site root with no hash.
func TestBuild_ServiceWorker_DetectedAndWired(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	writeFile(t, filepath.Join(root, "src", "service-worker.ts"),
		`// minimal worker
self.addEventListener('install', () => {});
`)

	res, err := Build(context.Background(), BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !res.HasServiceWorker {
		t.Fatal("BuildResult.HasServiceWorker = false; want true")
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, ".gen", "manifest.gen.go"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifestBytes), "const HasServiceWorker = true") {
		t.Errorf("manifest missing `const HasServiceWorker = true`:\n%s", manifestBytes)
	}

	viteCfg, err := os.ReadFile(filepath.Join(root, "vite.config.gen.js"))
	if err != nil {
		t.Fatalf("read vite config: %v", err)
	}
	cfg := string(viteCfg)
	for _, want := range []string{
		`"service-worker": path.resolve(__dirname, "src/service-worker.ts")`,
		`'../service-worker.js'`,
		`entryFileNames`,
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("vite.config.gen.js missing %q\n--- config:\n%s", want, cfg)
		}
	}
}

// TestBuild_ServiceWorker_AbsentLeavesConfigUnchanged covers the inverse:
// no src/service-worker.ts means HasServiceWorker = false on both the
// BuildResult and the generated manifest, and the vite config has no
// service-worker input or entryFileNames hook (so route chunks still get
// the default hashed filenames).
func TestBuild_ServiceWorker_AbsentLeavesConfigUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")

	res, err := Build(context.Background(), BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.HasServiceWorker {
		t.Error("BuildResult.HasServiceWorker = true; want false (no src/service-worker.ts)")
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, ".gen", "manifest.gen.go"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifestBytes), "const HasServiceWorker = false") {
		t.Errorf("manifest missing `const HasServiceWorker = false`:\n%s", manifestBytes)
	}

	viteCfg, err := os.ReadFile(filepath.Join(root, "vite.config.gen.js"))
	if err != nil {
		t.Fatalf("read vite config: %v", err)
	}
	cfg := string(viteCfg)
	for _, banned := range []string{`"service-worker"`, `entryFileNames`, `service-worker.ts`} {
		if strings.Contains(cfg, banned) {
			t.Errorf("vite.config.gen.js unexpectedly contains %q\n--- config:\n%s", banned, cfg)
		}
	}
}
