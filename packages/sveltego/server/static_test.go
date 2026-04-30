package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
)

func TestSafeRelPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"simple", "/app.js", "app.js", true},
		{"nested", "/_app/immutable/entry.js", "_app/immutable/entry.js", true},
		{"trailing slash", "/dir/", "dir", true},
		{"dot segment collapsed", "/a/./b.js", "a/b.js", true},
		{"absolute root rejected", "/", "", false},
		{"empty rejected", "", "", false},
		{"parent traversal rejected", "/../etc/passwd", "", false},
		{"embedded parent rejected", "/a/../../b", "", false},
		{"null byte rejected", "/a\x00b", "", false},
		{"raw dotdot rejected", "/..", "", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := safeRelPath(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("got = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAcceptsEncoding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		header string
		tok    string
		want   bool
	}{
		{"", "br", false},
		{"gzip", "gzip", true},
		{"gzip", "br", false},
		{"br, gzip", "br", true},
		{"br, gzip", "gzip", true},
		{"BR", "br", true},
		{"gzip;q=0", "gzip", false},
		{"gzip;q=0.5", "gzip", true},
		{"gzip; q=0.0", "gzip", false},
		{"deflate, br;q=0.9", "br", true},
		{"*", "br", true},
		{"identity", "br", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.header+"_"+tc.tok, func(t *testing.T) {
			t.Parallel()
			if got := acceptsEncoding(tc.header, tc.tok); got != tc.want {
				t.Errorf("acceptsEncoding(%q, %q) = %v, want %v", tc.header, tc.tok, got, tc.want)
			}
		})
	}
}

func TestContentTypeFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want string
	}{
		{"app.js", "application/javascript; charset=utf-8"},
		{"chunk.mjs", "application/javascript; charset=utf-8"},
		{"styles.css", "text/css; charset=utf-8"},
		{"index.html", "text/html; charset=utf-8"},
		{"index.htm", "text/html; charset=utf-8"},
		{"data.json", "application/json; charset=utf-8"},
		{"icon.svg", "image/svg+xml"},
		{"readme.txt", "text/plain; charset=utf-8"},
		{"feed.xml", "application/xml; charset=utf-8"},
		{"module.wasm", "application/wasm"},
		{"font.woff", "font/woff"},
		{"font.woff2", "font/woff2"},
		{"favicon.ico", "image/x-icon"},
		{"app.webmanifest", "application/manifest+json"},
		// unknown extension falls back to mime.TypeByExtension, then octet-stream.
		{"unknown.sveltegounknown", "application/octet-stream"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			if got := contentTypeFor(tc.file); got != tc.want {
				t.Errorf("contentTypeFor(%q) = %q, want %q", tc.file, got, tc.want)
			}
		})
	}
}

func TestStaticHandlerNosniff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteStatic(t, dir, "favicon.ico", []byte{0x00, 0x00, 0x01, 0x00})
	mustWriteStatic(t, dir, "module.wasm", []byte{0x00, 0x61, 0x73, 0x6d})
	mustWriteStatic(t, dir, "font.woff2", []byte("wOF2"))
	mustWriteStatic(t, dir, "app.webmanifest", []byte(`{"name":"test"}`))

	ts := httptest.NewServer(StaticHandler(kit.StaticConfig{Dir: dir}))
	t.Cleanup(ts.Close)

	cases := []struct {
		path   string
		wantCT string
	}{
		{"/favicon.ico", "image/x-icon"},
		{"/module.wasm", "application/wasm"},
		{"/font.woff2", "font/woff2"},
		{"/app.webmanifest", "application/manifest+json"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			resp.Body.Close()

			if got := resp.Header.Get("Content-Type"); got != tc.wantCT {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantCT)
			}
			if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}
		})
	}
}

func mustWriteStatic(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
