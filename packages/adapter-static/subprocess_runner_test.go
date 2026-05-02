package adapterstatic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// TestSubprocessRunner_EnumeratesRoutes verifies that readDynamicRoutes
// correctly classifies routes from the routes.json sidecar emitted by
// the user binary's MaybePrerenderFromEnv. Routes that produced HTML
// are filtered out; the remainder is returned sorted as DynamicRoutes.
//
// We exercise the parsing helper directly rather than spawning a real
// Go build — the production runner path is covered end-to-end by the
// CLI integration tests.
func TestSubprocessRunner_EnumeratesRoutes(t *testing.T) {
	t.Parallel()

	scratch := t.TempDir()

	type summary struct {
		Pattern   string `json:"Pattern"`
		Prerender bool   `json:"Prerender"`
	}
	all := []summary{
		{Pattern: "/", Prerender: true},
		{Pattern: "/about", Prerender: true},
		{Pattern: "/post/[id]", Prerender: false},
		{Pattern: "/api/data", Prerender: false},
		{Pattern: "/dashboard", Prerender: false},
	}
	body, err := json.Marshal(all)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "routes.json"), body, 0o600); err != nil {
		t.Fatalf("write routes.json: %v", err)
	}

	prerendered := map[string]struct{}{
		"/":      {},
		"/about": {},
	}

	got, err := readDynamicRoutes(scratch, prerendered)
	if err != nil {
		t.Fatalf("readDynamicRoutes: %v", err)
	}

	want := []string{"/api/data", "/dashboard", "/post/[id]"}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dynamic routes = %v, want %v", got, want)
	}
}

func TestSubprocessRunner_RoutesManifestAbsent(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	got, err := readDynamicRoutes(scratch, map[string]struct{}{"/": {}})
	if err != nil {
		t.Fatalf("readDynamicRoutes: %v", err)
	}
	if got != nil {
		t.Errorf("dynamic = %v, want nil for missing routes.json (back-compat)", got)
	}
}

func TestSubprocessRunner_RoutesManifestMalformed(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	if err := os.WriteFile(filepath.Join(scratch, "routes.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := readDynamicRoutes(scratch, map[string]struct{}{})
	if err == nil {
		t.Errorf("expected parse error for malformed routes.json")
	}
}
