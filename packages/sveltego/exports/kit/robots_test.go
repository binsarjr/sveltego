package kit

import (
	"strings"
	"testing"
)

func TestRobots_BasicAllowDisallow(t *testing.T) {
	t.Parallel()
	r := NewRobots().
		UserAgent("*").
		Allow("/").
		Disallow("/admin/")
	got := r.String()
	want := "User-agent: *\nAllow: /\nDisallow: /admin/\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRobots_MultipleGroupsSeparatedByBlankLine(t *testing.T) {
	t.Parallel()
	r := NewRobots().
		UserAgent("Googlebot").
		Allow("/").
		UserAgent("BadBot").
		Disallow("/")
	got := r.String()
	if !strings.Contains(got, "User-agent: Googlebot\nAllow: /\n\nUser-agent: BadBot\nDisallow: /\n") {
		t.Errorf("groups not separated: %q", got)
	}
}

func TestRobots_SitemapAfterGroups(t *testing.T) {
	t.Parallel()
	r := NewRobots().
		UserAgent("*").
		Allow("/").
		Sitemap("https://example.com/sitemap.xml")
	got := r.String()
	if !strings.HasSuffix(got, "\nSitemap: https://example.com/sitemap.xml\n") {
		t.Errorf("sitemap not at end: %q", got)
	}
}

func TestRobots_SitemapWithoutGroups(t *testing.T) {
	t.Parallel()
	r := NewRobots().Sitemap("https://example.com/sitemap.xml")
	got := r.String()
	if got != "Sitemap: https://example.com/sitemap.xml\n" {
		t.Errorf("sitemap-only output: %q", got)
	}
}

func TestRobots_ImplicitWildcardOnRulelessAdd(t *testing.T) {
	t.Parallel()
	r := NewRobots().Disallow("/private/")
	got := r.String()
	if !strings.HasPrefix(got, "User-agent: *\n") {
		t.Errorf("implicit wildcard missing: %q", got)
	}
}

func TestRobots_MultipleSitemaps(t *testing.T) {
	t.Parallel()
	r := NewRobots().
		Sitemap("https://example.com/sitemap-1.xml").
		Sitemap("https://example.com/sitemap-2.xml")
	got := r.String()
	if !strings.Contains(got, "Sitemap: https://example.com/sitemap-1.xml\nSitemap: https://example.com/sitemap-2.xml\n") {
		t.Errorf("multiple sitemaps not preserved: %q", got)
	}
}

func TestRobots_EmptyBuilder(t *testing.T) {
	t.Parallel()
	if got := NewRobots().String(); got != "" {
		t.Errorf("empty builder = %q, want empty", got)
	}
}
