package kit

import (
	"bytes"
	"strconv"
	"strings"
	"time"
)

// ChangeFreq is the sitemap.org `<changefreq>` enumeration. Use the
// exported constants rather than string literals so a typo fails to
// compile.
type ChangeFreq string

const (
	ChangeAlways  ChangeFreq = "always"
	ChangeHourly  ChangeFreq = "hourly"
	ChangeDaily   ChangeFreq = "daily"
	ChangeWeekly  ChangeFreq = "weekly"
	ChangeMonthly ChangeFreq = "monthly"
	ChangeYearly  ChangeFreq = "yearly"
	ChangeNever   ChangeFreq = "never"
)

// SitemapEntry is one URL in a sitemap. LastMod's zero value omits the
// `<lastmod>` element. Freq's empty string omits `<changefreq>`. Priority
// outside [0,1] omits `<priority>`.
type SitemapEntry struct {
	Loc      string
	LastMod  time.Time
	Freq     ChangeFreq
	Priority float64
}

// SitemapBuilder accumulates URLs and renders a valid sitemap.org XML
// document. Not safe for concurrent use; build per-request and discard.
type SitemapBuilder struct {
	baseURL string
	entries []SitemapEntry
}

// NewSitemap returns a builder rooted at baseURL. Add paths to it via
// Add or AddEntry. baseURL should not have a trailing slash; relative
// paths added via Add are joined with a single slash.
func NewSitemap(baseURL string) *SitemapBuilder {
	return &SitemapBuilder{baseURL: strings.TrimRight(baseURL, "/")}
}

// Add appends an entry. path may be absolute (already including the
// scheme/host) or relative (joined onto baseURL). lastMod's zero value
// is treated as "no <lastmod>"; freq's empty string omits <changefreq>;
// priority outside [0,1] silently omits the <priority> element -- no
// error is returned and no warning is logged. Valid priority range is
// [0.0, 1.0] inclusive.
func (s *SitemapBuilder) Add(path string, lastMod time.Time, freq ChangeFreq, priority float64) *SitemapBuilder {
	s.entries = append(s.entries, SitemapEntry{
		Loc:      s.resolve(path),
		LastMod:  lastMod,
		Freq:     freq,
		Priority: priority,
	})
	return s
}

// AddEntry appends a fully-formed entry without resolving Loc against
// baseURL. Use this when the caller already has an absolute URL or
// builds per-host sitemaps.
func (s *SitemapBuilder) AddEntry(e SitemapEntry) *SitemapBuilder {
	s.entries = append(s.entries, e)
	return s
}

// Len reports the number of entries added so far.
func (s *SitemapBuilder) Len() int { return len(s.entries) }

// Bytes renders the accumulated entries as a sitemap.org XML document.
// The output starts with the `<?xml version="1.0" encoding="UTF-8"?>`
// prologue and is suitable as the body of a `kit.XML(200, ...)` response.
func (s *SitemapBuilder) Bytes() []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, e := range s.entries {
		buf.WriteString("\n  <url>")
		buf.WriteString("\n    <loc>")
		writeXMLEscape(&buf, e.Loc)
		buf.WriteString("</loc>")
		if !e.LastMod.IsZero() {
			buf.WriteString("\n    <lastmod>")
			buf.WriteString(e.LastMod.UTC().Format("2006-01-02"))
			buf.WriteString("</lastmod>")
		}
		if e.Freq != "" {
			buf.WriteString("\n    <changefreq>")
			buf.WriteString(string(e.Freq))
			buf.WriteString("</changefreq>")
		}
		if e.Priority >= 0 && e.Priority <= 1 {
			buf.WriteString("\n    <priority>")
			buf.WriteString(strconv.FormatFloat(e.Priority, 'f', 1, 64))
			buf.WriteString("</priority>")
		}
		buf.WriteString("\n  </url>")
	}
	buf.WriteString("\n</urlset>\n")
	return buf.Bytes()
}

func (s *SitemapBuilder) resolve(path string) string {
	if path == "" {
		return s.baseURL
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return s.baseURL + path
}

// writeXMLEscape writes s to buf with the five XML predefined entities
// escaped. Sufficient for <loc> values; full XML attribute escaping is
// not needed because no caller embeds these inside attributes.
func writeXMLEscape(buf *bytes.Buffer, s string) {
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
}
