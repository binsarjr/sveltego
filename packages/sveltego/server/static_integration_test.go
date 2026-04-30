package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
)

// staticFixture lays out a directory with raw + precompressed siblings
// matching the production layout (`_app/immutable/...` plus a top-level
// asset). All bytes are made distinguishable so tests can assert which
// variant the handler picked.
func staticFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "app.js"), []byte("console.log('raw');\n"))
	mustWrite(t, filepath.Join(dir, "app.js.gz"), []byte("GZIP-BODY-app.js"))
	mustWrite(t, filepath.Join(dir, "app.js.br"), []byte("BROTLI-BODY-app.js"))

	imm := filepath.Join(dir, "_app", "immutable")
	if err := os.MkdirAll(imm, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWrite(t, filepath.Join(imm, "entry.abc.js"), []byte("export const v = 1;\n"))
	mustWrite(t, filepath.Join(imm, "entry.abc.js.gz"), []byte("GZIP-BODY-entry"))

	mustWrite(t, filepath.Join(dir, "plain.txt"), []byte("hello plain world\n"))

	return dir
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newStaticServer(t *testing.T, cfg kit.StaticConfig) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(StaticHandler(cfg))
	t.Cleanup(ts.Close)
	return ts
}

// rawClient returns an *http.Client whose transport does NOT
// transparently decompress gzip. The default net/http transport
// silently injects Accept-Encoding: gzip and decompresses, which both
// rewrites the response body and strips Content-Encoding — making the
// gzip-vs-raw assertions meaningless.
func rawClient() *http.Client {
	return &http.Client{Transport: &http.Transport{DisableCompression: true}}
}

func staticGet(t *testing.T, url string, headers map[string]string, method string) *http.Response {
	t.Helper()
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := rawClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func TestStatic_RawFallback_NoEncoding(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Brotli: true, Gzip: true, ETag: true})

	resp := staticGet(t, ts.URL+"/app.js", nil, "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty", got)
	}
	if !strings.Contains(string(body), "raw") {
		t.Errorf("body = %q, want raw payload", body)
	}
	if got := resp.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
	if got := resp.Header.Get("ETag"); got == "" {
		t.Errorf("ETag missing")
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "must-revalidate") {
		t.Errorf("Cache-Control = %q, want must-revalidate", got)
	}
}

func TestStatic_GzipNegotiated(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Gzip: true})

	resp := staticGet(t, ts.URL+"/app.js", map[string]string{"Accept-Encoding": "gzip"}, "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if string(body) != "GZIP-BODY-app.js" {
		t.Errorf("body = %q, want gzip sibling bytes", body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/javascript") {
		t.Errorf("Content-Type = %q, want javascript", got)
	}
}

func TestStatic_BrotliPreferredOverGzip(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Brotli: true, Gzip: true})

	resp := staticGet(t, ts.URL+"/app.js", map[string]string{"Accept-Encoding": "gzip, br"}, "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if got := resp.Header.Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if string(body) != "BROTLI-BODY-app.js" {
		t.Errorf("body = %q, want brotli sibling bytes", body)
	}
}

func TestStatic_BrotliFallsBackWhenSiblingMissing(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	// entry.abc.js has only a .gz sibling; even if br is accepted,
	// handler must fall through to gzip.
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Brotli: true, Gzip: true})

	resp := staticGet(t, ts.URL+"/_app/immutable/entry.abc.js",
		map[string]string{"Accept-Encoding": "br, gzip"}, "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip fallback", got)
	}
	if string(body) != "GZIP-BODY-entry" {
		t.Errorf("body = %q", body)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable for hashed asset", got)
	}
}

func TestStatic_GzipDisabledServesRaw(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/app.js", map[string]string{"Accept-Encoding": "gzip, br"}, "")
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty (compression disabled)", got)
	}
}

func TestStatic_MissingFile404(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/nope.js", nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestStatic_DirectoryIs404NotListing(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/_app", nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (no directory listings)", resp.StatusCode)
	}
}

func TestStatic_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	mustWrite(t, filepath.Join(outer, "secret.txt"), []byte("TOP SECRET"))
	inner := filepath.Join(outer, "static")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWrite(t, filepath.Join(inner, "ok.txt"), []byte("ok"))

	ts := newStaticServer(t, kit.StaticConfig{Dir: inner})

	cases := []string{
		"/../secret.txt",
		"/..%2Fsecret.txt",
		"/foo/../../secret.txt",
	}
	for _, p := range cases {
		resp := staticGet(t, ts.URL+p, nil, "")
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "TOP SECRET") {
			t.Errorf("traversal succeeded for %q: body = %q", p, body)
		}
		if resp.StatusCode == http.StatusOK {
			t.Errorf("status %d for traversal %q, want non-200", resp.StatusCode, p)
		}
	}
}

func TestStatic_RangeRequest(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/plain.txt", map[string]string{"Range": "bytes=0-4"}, "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q, want %q", body, "hello")
	}
	if got := resp.Header.Get("Content-Range"); !strings.HasPrefix(got, "bytes 0-4/") {
		t.Errorf("Content-Range = %q", got)
	}
}

func TestStatic_ETagConditional304(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, ETag: true})

	resp := staticGet(t, ts.URL+"/plain.txt", nil, "")
	resp.Body.Close()
	tag := resp.Header.Get("ETag")
	if tag == "" {
		t.Fatalf("ETag missing on first response")
	}

	resp2 := staticGet(t, ts.URL+"/plain.txt", map[string]string{"If-None-Match": tag}, "")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("status = %d, want 304", resp2.StatusCode)
	}
}

func TestStatic_ETagDistinctPerEncoding(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Gzip: true, Brotli: true, ETag: true})

	resp := staticGet(t, ts.URL+"/app.js", nil, "")
	resp.Body.Close()
	rawTag := resp.Header.Get("ETag")

	gz := staticGet(t, ts.URL+"/app.js", map[string]string{"Accept-Encoding": "gzip"}, "")
	gz.Body.Close()
	gzTag := gz.Header.Get("ETag")

	if rawTag == "" || gzTag == "" {
		t.Fatalf("missing ETags raw=%q gzip=%q", rawTag, gzTag)
	}
	if rawTag == gzTag {
		t.Errorf("raw and gzip share ETag %q; cache poisoning risk", rawTag)
	}
}

func TestStatic_ImmutableCacheForHashedPrefix(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/_app/immutable/entry.abc.js", nil, "")
	defer resp.Body.Close()
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control = %q, want immutable + max-age", cc)
	}
}

func TestStatic_HEADRequest(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir, Gzip: true})

	resp := staticGet(t, ts.URL+"/app.js", map[string]string{"Accept-Encoding": "gzip"}, http.MethodHead)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("HEAD body len = %d, want 0", len(body))
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}

func TestStatic_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	dir := staticFixture(t)
	ts := newStaticServer(t, kit.StaticConfig{Dir: dir})

	resp := staticGet(t, ts.URL+"/app.js", nil, http.MethodPost)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); !strings.Contains(got, "GET") {
		t.Errorf("Allow = %q, want GET", got)
	}
}

func TestStatic_EmptyDirReturnsErrorHandler(t *testing.T) {
	t.Parallel()
	ts := newStaticServer(t, kit.StaticConfig{Dir: ""})

	resp := staticGet(t, ts.URL+"/anything", nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}
