package mcp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DocsIndex is a tiny in-memory text index over markdown files. Search
// is case-insensitive substring with simple ranking by hit count.
type DocsIndex struct {
	pages []docPage
}

type docPage struct {
	Slug    string
	Title   string
	Body    string
	BodyLow string
}

// SearchHit is a ranked search result.
type SearchHit struct {
	Slug    string
	Title   string
	Snippet string
}

// Len reports the number of indexed pages.
func (i *DocsIndex) Len() int { return len(i.pages) }

// Search returns up to limit hits for q ranked by occurrence count.
// Ties break by slug for determinism.
func (i *DocsIndex) Search(q string, limit int) []SearchHit {
	if i == nil || len(i.pages) == 0 {
		return nil
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	if needle == "" {
		return nil
	}
	type scored struct {
		page  *docPage
		count int
		first int
	}
	scoredHits := make([]scored, 0, len(i.pages))
	for idx := range i.pages {
		p := &i.pages[idx]
		count := strings.Count(p.BodyLow, needle)
		if count == 0 && !strings.Contains(strings.ToLower(p.Title), needle) {
			continue
		}
		first := strings.Index(p.BodyLow, needle)
		scoredHits = append(scoredHits, scored{page: p, count: count, first: first})
	}
	sort.Slice(scoredHits, func(a, b int) bool {
		if scoredHits[a].count != scoredHits[b].count {
			return scoredHits[a].count > scoredHits[b].count
		}
		return scoredHits[a].page.Slug < scoredHits[b].page.Slug
	})
	if limit > 0 && len(scoredHits) > limit {
		scoredHits = scoredHits[:limit]
	}
	out := make([]SearchHit, 0, len(scoredHits))
	for _, s := range scoredHits {
		out = append(out, SearchHit{
			Slug:    s.page.Slug,
			Title:   s.page.Title,
			Snippet: snippetAround(s.page.Body, s.first, 200),
		})
	}
	return out
}

func snippetAround(body string, idx, radius int) string {
	if idx < 0 {
		idx = 0
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius
	if end > len(body) {
		end = len(body)
	}
	out := body[start:end]
	out = strings.ReplaceAll(out, "\n", " ")
	out = strings.TrimSpace(out)
	if start > 0 {
		out = "…" + out
	}
	if end < len(body) {
		out += "…"
	}
	return out
}

// LoadDocsIndex walks dir for markdown files and returns an index. A
// missing dir returns an empty (but non-nil) index so callers can
// degrade gracefully.
func LoadDocsIndex(dir string) (*DocsIndex, error) {
	idx := &DocsIndex{}
	if dir == "" {
		return idx, nil
	}
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return idx, nil
		}
		return nil, fmt.Errorf("stat docs dir: %w", err)
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		body, err := os.ReadFile(path) //nolint:gosec // markdown sources under DocsDir
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		slug := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		bodyStr := string(body)
		idx.pages = append(idx.pages, docPage{
			Slug:    slug,
			Title:   extractTitle(bodyStr, slug),
			Body:    bodyStr,
			BodyLow: strings.ToLower(bodyStr),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(idx.pages, func(i, j int) bool { return idx.pages[i].Slug < idx.pages[j].Slug })
	return idx, nil
}

func extractTitle(body, fallback string) string {
	for _, line := range strings.SplitN(body, "\n", 12) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallback
}

// docsIndex loads the docs index lazily on first access and memoises the
// result. The loader runs outside s.mu so a slow filesystem walk does not
// block the JSON-RPC writer.
func (s *Server) docsIndex() (*DocsIndex, error) {
	s.docsOnce.Do(func() {
		s.docsIdx, s.docsErr = LoadDocsIndex(s.cfg.DocsDir)
	})
	return s.docsIdx, s.docsErr
}

func readDocPage(dir, slug string) (string, error) {
	if dir == "" {
		return "", errors.New("docs directory not configured")
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(slug, "/"))
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid slug: %s", slug)
	}
	rel := strings.TrimPrefix(clean, "/")
	if !strings.HasSuffix(rel, ".md") {
		rel += ".md"
	}
	full := filepath.Join(dir, rel)
	if !strings.HasPrefix(full, filepath.Clean(dir)+string(filepath.Separator)) && full != filepath.Clean(dir) {
		return "", fmt.Errorf("invalid slug: %s", slug)
	}
	body, err := os.ReadFile(full) //nolint:gosec // docs path validated against dir prefix
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("doc page not found: %s", slug)
		}
		return "", fmt.Errorf("read doc page: %w", err)
	}
	return string(body), nil
}
