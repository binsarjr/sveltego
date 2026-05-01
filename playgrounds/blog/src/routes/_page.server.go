//go:build sveltego

// Index Load: scans content/posts/*.md, sorts by date desc, paginates
// at PageSize per page. Demonstrates kit.LoadCtx URL access for
// `?page=N` and a small in-memory derived collection.
package routes

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

const Templates = "svelte"

const pageSize = 3

type Post struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Date    string `json:"date"`
}

type PageData struct {
	Posts      []Post `json:"posts"`
	Page       int    `json:"page"`
	TotalPages int    `json:"totalPages"`
	HasPrev    bool   `json:"hasPrev"`
	HasNext    bool   `json:"hasNext"`
	PrevHref   string `json:"prevHref"`
	NextHref   string `json:"nextHref"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	all, err := readAllPosts()
	if err != nil {
		return PageData{}, kit.Error(500, "failed to load posts: "+err.Error())
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Date > all[j].Date })

	pageNum := 1
	if ctx != nil && ctx.URL != nil {
		if v := ctx.URL.Query().Get("page"); v != "" {
			if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
				pageNum = n
			}
		}
	}

	total := (len(all) + pageSize - 1) / pageSize
	if total == 0 {
		total = 1
	}
	if pageNum > total {
		pageNum = total
	}

	start := (pageNum - 1) * pageSize
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	prevHref := ""
	if pageNum > 1 {
		prevHref = "/?page=" + strconv.Itoa(pageNum-1)
	}
	nextHref := ""
	if pageNum < total {
		nextHref = "/?page=" + strconv.Itoa(pageNum+1)
	}

	return PageData{
		Posts:      all[start:end],
		Page:       pageNum,
		TotalPages: total,
		HasPrev:    pageNum > 1,
		HasNext:    pageNum < total,
		PrevHref:   prevHref,
		NextHref:   nextHref,
	}, nil
}

// readAllPosts walks content/posts for *.md files and parses the
// frontmatter into Post values.
func readAllPosts() ([]Post, error) {
	entries, err := os.ReadDir("content/posts")
	if err != nil {
		return nil, err
	}
	out := make([]Post, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join("content", "posts", e.Name())
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, rerr
		}
		fm, _ := splitFrontmatter(body)
		slug := strings.TrimSuffix(e.Name(), ".md")
		out = append(out, Post{
			Slug:    slug,
			Title:   fm["title"],
			Summary: fm["summary"],
			Date:    fm["date"],
		})
	}
	return out, nil
}

// splitFrontmatter extracts a tiny `key: value` YAML block delimited by
// `---` lines from the head of body. Returns (frontmatter map, rest).
// Good enough for the demo; production would use a real YAML parser.
func splitFrontmatter(body []byte) (map[string]string, []byte) {
	src := string(body)
	if !strings.HasPrefix(src, "---\n") {
		return map[string]string{}, body
	}
	rest := src[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return map[string]string{}, body
	}
	header := rest[:end]
	out := map[string]string{}
	for _, line := range strings.Split(header, "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		out[key] = val
	}
	return out, []byte(rest[end+5:])
}
