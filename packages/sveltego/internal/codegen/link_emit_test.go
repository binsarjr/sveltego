package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

var linkFixtures = []string{
	"simple",
	"nested",
	"groups",
	"optional",
	"rest",
	"layout-chain",
}

func TestGenerateLinks_Goldens(t *testing.T) {
	t.Parallel()
	for _, name := range linkFixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scan := scanFixture(t, name)
			got, err := GenerateLinks(scan, LinkEmitOptions{})
			if err != nil {
				t.Fatalf("GenerateLinks: %v", err)
			}
			assertLinksGolden(t, name, got)
		})
	}
}

func TestGenerateLinks_Deterministic(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "nested")
	a, err := GenerateLinks(scan, LinkEmitOptions{})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := GenerateLinks(scan, LinkEmitOptions{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic output:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
}

func TestGenerateLinks_NilScan(t *testing.T) {
	t.Parallel()
	if _, err := GenerateLinks(nil, LinkEmitOptions{}); err == nil {
		t.Fatal("expected error on nil scan")
	}
}

func TestRouteIdent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		segs []router.Segment
		want string
	}{
		{"root", nil, "Index"},
		{"static", []router.Segment{{Kind: router.SegmentStatic, Value: "about"}}, "About"},
		{
			"static-deep",
			[]router.Segment{
				{Kind: router.SegmentStatic, Value: "blog"},
				{Kind: router.SegmentParam, Name: "slug"},
			},
			"BlogSlug",
		},
		{
			"rest",
			[]router.Segment{
				{Kind: router.SegmentStatic, Value: "docs"},
				{Kind: router.SegmentRest, Name: "path"},
			},
			"DocsPath",
		},
		{
			"optional",
			[]router.Segment{
				{Kind: router.SegmentOptional, Name: "lang"},
				{Kind: router.SegmentStatic, Value: "about"},
			},
			"LangAbout",
		},
		{
			"hyphenated-static",
			[]router.Segment{
				{Kind: router.SegmentStatic, Value: "user-profile"},
			},
			"UserProfile",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := routeIdent(tc.segs)
			if got != tc.want {
				t.Fatalf("routeIdent(%v) = %q, want %q", tc.segs, got, tc.want)
			}
		})
	}
}

func TestPascal(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":              "",
		"slug":          "Slug",
		"user-profile":  "UserProfile",
		"user_profile":  "UserProfile",
		"path":          "Path",
		"123abc":        "_123Abc",
		"alreadyPascal": "AlreadyPascal",
	}
	for in, want := range cases {
		in, want := in, want
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got := pascal(in); got != want {
				t.Fatalf("pascal(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestCollectLinkRoutes_DisambiguatesHelperCollisions(t *testing.T) {
	t.Parallel()
	scan := &routescan.ScanResult{
		Routes: []routescan.ScannedRoute{
			{
				Pattern: "/blog",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "blog"},
				},
				HasPage: true,
			},
			{
				Pattern: "/blog",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "blog"},
				},
				HasPage: true,
			},
		},
	}
	got := CollectLinkRoutes(scan)
	if len(got) != 2 {
		t.Fatalf("want 2 routes, got %d", len(got))
	}
	if got[0].Helper != "Blog" {
		t.Fatalf("first helper want Blog, got %s", got[0].Helper)
	}
	if got[1].Helper != "Blog_2" {
		t.Fatalf("second helper want Blog_2, got %s", got[1].Helper)
	}
}

func TestCollectLinkRoutes_SkipsOrphans(t *testing.T) {
	t.Parallel()
	scan := &routescan.ScanResult{
		Routes: []routescan.ScannedRoute{
			{Pattern: "/orphan"},
			{Pattern: "/page", HasPage: true, Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "page"}}},
		},
	}
	got := CollectLinkRoutes(scan)
	if len(got) != 1 {
		t.Fatalf("want 1 route, got %d", len(got))
	}
	if got[0].Pattern != "/page" {
		t.Fatalf("want /page, got %s", got[0].Pattern)
	}
}

func TestSortLinkRoutes(t *testing.T) {
	t.Parallel()
	in := []LinkRoute{
		{Pattern: "/b"},
		{Pattern: "/a"},
		{Pattern: "/c"},
	}
	got := SortLinkRoutes(in)
	want := []string{"/a", "/b", "/c"}
	for i, r := range got {
		if r.Pattern != want[i] {
			t.Fatalf("idx %d: want %s, got %s", i, want[i], r.Pattern)
		}
	}
}

func assertLinksGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "links", name+".golden")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch in %s; run GOLDEN_UPDATE=1\n--- want:\n%s\n--- got:\n%s", path, want, got)
	}
}
