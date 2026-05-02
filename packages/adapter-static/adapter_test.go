package adapterstatic_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	adapterstatic "github.com/binsarjr/sveltego/adapter-static"
)

// fakeRunner mimics what Server.Prerender writes into scratchDir so the
// adapter unit tests can verify the packaging logic without compiling
// or executing a real user binary. The shape mirrors
// packages/sveltego/server/prerender.go's manifest output exactly.
type fakeRunner struct {
	prerendered  map[string]string // route -> HTML body
	dynamic      []string
	prerenderErr error
}

type scratchManifest struct {
	GeneratedAt string         `json:"generatedAt"`
	Entries     []scratchEntry `json:"entries"`
}

type scratchEntry struct {
	Route     string `json:"route"`
	Path      string `json:"path"`
	File      string `json:"file"`
	Protected bool   `json:"protected,omitempty"`
}

func (r *fakeRunner) Prerender(_ context.Context, _, scratchDir string) (adapterstatic.RunInfo, error) {
	if r.prerenderErr != nil {
		return adapterstatic.RunInfo{}, r.prerenderErr
	}
	routes := make([]string, 0, len(r.prerendered))
	for k := range r.prerendered {
		routes = append(routes, k)
	}
	sort.Strings(routes)

	entries := make([]scratchEntry, 0, len(routes))
	for _, route := range routes {
		body := r.prerendered[route]
		urlPath := route
		if urlPath == "" {
			urlPath = "/"
		}
		relDir := strings.Trim(urlPath, "/")
		if relDir == "" {
			relDir = "index"
		}
		htmlPath := filepath.Join(scratchDir, relDir, "index.html")
		if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
			return adapterstatic.RunInfo{}, err
		}
		if err := os.WriteFile(htmlPath, []byte(body), 0o644); err != nil {
			return adapterstatic.RunInfo{}, err
		}
		entries = append(entries, scratchEntry{
			Route: route,
			Path:  urlPath,
			File:  filepath.ToSlash(filepath.Join(relDir, "index.html")),
		})
	}

	manifest := scratchManifest{
		GeneratedAt: "2026-05-02T00:00:00Z",
		Entries:     entries,
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return adapterstatic.RunInfo{}, err
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(scratchDir, "manifest.json"), body, 0o644); err != nil {
		return adapterstatic.RunInfo{}, err
	}

	return adapterstatic.RunInfo{
		PrerenderedRoutes: routes,
		DynamicRoutes:     append([]string(nil), r.dynamic...),
	}, nil
}

func TestBuild_TreeLayoutAndManifest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "static", "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "static", "img", "logo.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	runner := &fakeRunner{prerendered: map[string]string{
		"/":      "<main>home</main>",
		"/about": "<main>about</main>",
	}}

	err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: projectRoot,
		OutputDir:   outDir,
		Runner:      runner,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	homeBody, err := os.ReadFile(filepath.Join(outDir, "index", "index.html"))
	if err != nil {
		t.Fatalf("home file: %v", err)
	}
	if string(homeBody) != "<main>home</main>" {
		t.Errorf("home body = %q", homeBody)
	}
	if _, err := os.Stat(filepath.Join(outDir, "about", "index.html")); err != nil {
		t.Errorf("about file missing: %v", err)
	}

	logoBody, err := os.ReadFile(filepath.Join(outDir, "static", "img", "logo.png"))
	if err != nil {
		t.Fatalf("static asset: %v", err)
	}
	if string(logoBody) != "PNG" {
		t.Errorf("logo body = %q", logoBody)
	}

	manifestPath := filepath.Join(outDir, adapterstatic.PrerenderManifestFilename)
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var m struct {
		Version   int    `json:"version"`
		SourceSHA string `json:"sourceSHA"`
		Entries   []struct {
			Route  string `json:"route"`
			Path   string `json:"path"`
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
		} `json:"entries"`
		DynamicRoutes []string `json:"dynamicRoutes"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	if m.SourceSHA == "" {
		t.Errorf("sourceSHA empty")
	}
	if got, want := len(m.Entries), 2; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	for _, e := range m.Entries {
		if e.SHA256 == "" {
			t.Errorf("entry %s missing sha256", e.Route)
		}
		// Verify the per-entry SHA matches the file on disk.
		fileBody, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(e.File)))
		if err != nil {
			t.Errorf("read entry file %s: %v", e.File, err)
			continue
		}
		sum := sha256.Sum256(fileBody)
		if got := hex.EncodeToString(sum[:]); got != e.SHA256 {
			t.Errorf("entry %s: SHA mismatch (file=%s manifest=%s)", e.Route, got, e.SHA256)
		}
	}
}

func TestBuild_Idempotent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "static", "asset.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	runner := &fakeRunner{prerendered: map[string]string{
		"/":    "<h1>home</h1>",
		"/x":   "<h1>x</h1>",
		"/x/y": "<h1>xy</h1>",
	}}

	bc := adapterstatic.BuildContext{
		ProjectRoot: projectRoot,
		OutputDir:   outDir,
		Runner:      runner,
	}
	if err := adapterstatic.Build(context.Background(), bc); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	first := hashTree(t, outDir)

	if err := adapterstatic.Build(context.Background(), bc); err != nil {
		t.Fatalf("second Build: %v", err)
	}
	second := hashTree(t, outDir)

	if len(first) != len(second) {
		t.Fatalf("tree length differs: first=%d second=%d", len(first), len(second))
	}
	for path, sumA := range first {
		sumB, ok := second[path]
		if !ok {
			t.Errorf("file %s present in first but not second", path)
			continue
		}
		if sumA != sumB {
			t.Errorf("file %s differs: first=%s second=%s", path, sumA, sumB)
		}
	}
	for path := range second {
		if _, ok := first[path]; !ok {
			t.Errorf("file %s present in second but not first", path)
		}
	}
}

func TestBuild_FailOnDynamic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	runner := &fakeRunner{
		prerendered: map[string]string{"/": "<h1>home</h1>"},
		dynamic:     []string{"/api/items", "/dashboard"},
	}

	err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot:   projectRoot,
		OutputDir:     outDir,
		Runner:        runner,
		FailOnDynamic: true,
	})
	if err == nil {
		t.Fatalf("expected ErrDynamicRoutes")
	}
	var e *adapterstatic.ErrDynamicRoutes
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *ErrDynamicRoutes", err)
	}
	if got, want := len(e.Routes), 2; got != want {
		t.Fatalf("routes = %d, want %d", got, want)
	}
	for i, want := range []string{"/api/items", "/dashboard"} {
		if e.Routes[i] != want {
			t.Errorf("routes[%d] = %s, want %s", i, e.Routes[i], want)
		}
	}
	// Without FailOnDynamic the dynamic routes are still surfaced via
	// the manifest but Build succeeds.
	if err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: projectRoot,
		OutputDir:   outDir,
		Runner:      runner,
	}); err != nil {
		t.Fatalf("Build w/o FailOnDynamic: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(outDir, adapterstatic.PrerenderManifestFilename))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if !strings.Contains(string(body), "/api/items") || !strings.Contains(string(body), "/dashboard") {
		t.Errorf("manifest missing dynamicRoutes: %s", body)
	}
}

func TestBuild_StaticPrerenderedScratchSkipped(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "static", "_prerendered", "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Stale runtime artifact from a prior `sveltego prerender` run.
	if err := os.WriteFile(filepath.Join(projectRoot, "static", "_prerendered", "old", "index.html"), []byte("STALE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "static", "good.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")
	runner := &fakeRunner{prerendered: map[string]string{"/": "<h1>home</h1>"}}

	if err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: projectRoot,
		OutputDir:   outDir,
		Runner:      runner,
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "static", "_prerendered")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("_prerendered scratch dir should not be in output: stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "static", "good.txt")); err != nil {
		t.Errorf("legitimate static asset missing: %v", err)
	}
}

func TestBuild_RejectsRelativePaths(t *testing.T) {
	t.Parallel()
	err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: "relative/path",
		OutputDir:   "/abs",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("expected absolute-path error, got %v", err)
	}
	err = adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: "/abs",
		OutputDir:   "rel",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("expected absolute-path error, got %v", err)
	}
}

func TestBuild_ContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := adapterstatic.Build(ctx, adapterstatic.BuildContext{})
	if err == nil {
		t.Fatalf("expected context error")
	}
}

func TestBuild_RunnerError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{prerenderErr: errors.New("boom")}
	err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: projectRoot,
		OutputDir:   filepath.Join(tmp, "out"),
		Runner:      runner,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestErrDynamicRoutes_EmptyMessage(t *testing.T) {
	t.Parallel()
	var e *adapterstatic.ErrDynamicRoutes
	if e.Error() == "" {
		t.Errorf("nil receiver should still produce a message")
	}
	e = &adapterstatic.ErrDynamicRoutes{}
	if e.Error() == "" {
		t.Errorf("empty receiver should still produce a message")
	}
	e = &adapterstatic.ErrDynamicRoutes{Routes: []string{"/a", "/b"}}
	if !strings.Contains(e.Error(), "/a") || !strings.Contains(e.Error(), "/b") {
		t.Errorf("error missing route patterns: %s", e.Error())
	}
}

func TestDocMentionsKeyConcepts(t *testing.T) {
	t.Parallel()
	doc := adapterstatic.Doc()
	for _, want := range []string{"Static target", "Idempotent", "FailOnDynamic"} {
		if !strings.Contains(doc, want) {
			t.Errorf("Doc missing %q", want)
		}
	}
}

// hashTree returns relative-path -> sha256(file contents) for every
// regular file under root. Directories collapse into entries via their
// children; entries sorted by path so map iteration is deterministic
// for the caller.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}
