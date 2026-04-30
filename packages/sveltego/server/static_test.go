package server

import "testing"

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

	cases := map[string]string{
		"app.js":      "application/javascript; charset=utf-8",
		"styles.css":  "text/css; charset=utf-8",
		"icon.svg":    "image/svg+xml",
		"data.json":   "application/json; charset=utf-8",
		"unknown.xyz": "",
	}
	for in, want := range cases {
		if got := contentTypeFor(in); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", in, got, want)
		}
	}
}
