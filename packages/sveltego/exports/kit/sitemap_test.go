package kit

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestSitemap_BuildsValidXML(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	sm.Add("/", time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), ChangeDaily, 1.0)
	sm.Add("/blog/post", time.Time{}, ChangeWeekly, 0.5)

	body := sm.Bytes()
	var doc struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc      string `xml:"loc"`
			LastMod  string `xml:"lastmod"`
			Freq     string `xml:"changefreq"`
			Priority string `xml:"priority"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if len(doc.URLs) != 2 {
		t.Fatalf("urls = %d, want 2", len(doc.URLs))
	}
	if doc.URLs[0].Loc != "https://example.com/" {
		t.Errorf("loc[0] = %q", doc.URLs[0].Loc)
	}
	if doc.URLs[0].LastMod != "2026-04-30" {
		t.Errorf("lastmod[0] = %q", doc.URLs[0].LastMod)
	}
	if doc.URLs[0].Freq != "daily" {
		t.Errorf("freq[0] = %q", doc.URLs[0].Freq)
	}
	if doc.URLs[0].Priority != "1.0" {
		t.Errorf("priority[0] = %q", doc.URLs[0].Priority)
	}
	if doc.URLs[1].LastMod != "" {
		t.Errorf("lastmod[1] should be empty, got %q", doc.URLs[1].LastMod)
	}
}

func TestSitemap_BaseURLTrim(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com/")
	sm.Add("/x", time.Time{}, "", -1)
	out := string(sm.Bytes())
	if !strings.Contains(out, "<loc>https://example.com/x</loc>") {
		t.Errorf("trailing slash not trimmed: %q", out)
	}
}

func TestSitemap_AbsoluteURLPreserved(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	sm.Add("https://other.example/foo", time.Time{}, "", -1)
	out := string(sm.Bytes())
	if !strings.Contains(out, "<loc>https://other.example/foo</loc>") {
		t.Errorf("absolute url rewritten: %q", out)
	}
}

func TestSitemap_PriorityOutOfRangeOmitted(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	sm.Add("/a", time.Time{}, "", 5)
	sm.Add("/b", time.Time{}, "", -1)
	out := string(sm.Bytes())
	if strings.Contains(out, "<priority>") {
		t.Errorf("priority emitted out of range: %q", out)
	}
}

func TestSitemap_XMLEscapesLoc(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	sm.Add("/search?q=a&b=<x>", time.Time{}, "", -1)
	out := string(sm.Bytes())
	if !strings.Contains(out, "&amp;b=&lt;x&gt;") {
		t.Errorf("loc not XML-escaped: %q", out)
	}
}

func TestSitemap_LenTracksEntries(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	if sm.Len() != 0 {
		t.Errorf("empty Len = %d", sm.Len())
	}
	sm.Add("/", time.Time{}, "", -1)
	sm.Add("/x", time.Time{}, "", -1)
	if sm.Len() != 2 {
		t.Errorf("Len after 2 Adds = %d", sm.Len())
	}
}

func TestSitemap_PrologueAndRoot(t *testing.T) {
	t.Parallel()
	sm := NewSitemap("https://example.com")
	out := string(sm.Bytes())
	if !strings.HasPrefix(out, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("missing prologue: %q", out)
	}
	if !strings.Contains(out, `xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`) {
		t.Errorf("missing xmlns: %q", out)
	}
}
