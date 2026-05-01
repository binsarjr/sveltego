//go:build sveltego

// Post detail Load + comment Action.
//
// Demonstrates: kit.LoadCtx.Params for [slug], goldmark + bluemonday
// for safe markdown rendering through {@html}, kit.ActionMap with a
// default action that BindForms a comment, kit.Cookies to remember the
// last author, and kit.ActionRedirect for the post-redirect-get cycle.
package _slug_

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

const Templates = "svelte"

type Comment struct {
	Author string `json:"author"`
	Body   string `json:"body"`
	Posted string `json:"posted"`
}

type PageData struct {
	Title    string    `json:"title"`
	Date     string    `json:"date"`
	HTML     string    `json:"html"`
	Comments []Comment `json:"comments"`
	Form     any       `json:"form"`
}

var (
	commentsMu sync.RWMutex
	comments   = map[string][]Comment{}
)

func Load(ctx *kit.LoadCtx) (PageData, error) {
	if ctx == nil || ctx.Params["slug"] == "" {
		return PageData{}, errors.New("missing slug param")
	}
	slug := ctx.Params["slug"]

	if !isSafeSlug(slug) {
		return PageData{}, kit.Error(404, "post not found")
	}

	path := filepath.Join("content", "posts", slug+".md")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PageData{}, kit.Error(404, "post not found")
		}
		return PageData{}, kit.Error(500, "read post: "+err.Error())
	}

	fm, content := splitFrontmatter(body)
	html, err := renderMarkdown(content)
	if err != nil {
		return PageData{}, kit.Error(500, "render markdown: "+err.Error())
	}

	commentsMu.RLock()
	existing := append([]Comment(nil), comments[slug]...)
	commentsMu.RUnlock()

	return PageData{
		Title:    fm["title"],
		Date:     fm["date"],
		HTML:     html,
		Comments: existing,
	}, nil
}

// Actions exposes the comment submission entry. The default key fires
// on POST without `?/<name>`. ev.BindForm reflects the form into a
// CommentForm; an empty body returns kit.ActionFail so the template
// re-renders with the error. Success appends to the in-memory store,
// stamps the author cookie, and redirects back to the post (303 PRG).
var Actions = kit.ActionMap{
	"default": func(ev *kit.RequestEvent) kit.ActionResult {
		var form struct {
			Author string `form:"author"`
			Body   string `form:"body"`
		}
		if err := ev.BindForm(&form); err != nil {
			return kit.ActionFail(400, map[string]string{"error": err.Error()})
		}
		form.Author = strings.TrimSpace(form.Author)
		form.Body = strings.TrimSpace(form.Body)
		if form.Author == "" || form.Body == "" {
			return kit.ActionFail(400, map[string]string{"error": "author and body are required"})
		}
		if len(form.Body) > 2000 {
			return kit.ActionFail(400, map[string]string{"error": "comment too long (max 2000 chars)"})
		}

		slug := ev.Params["slug"]
		if slug == "" || !isSafeSlug(slug) {
			return kit.ActionFail(404, map[string]string{"error": "post not found"})
		}

		c := Comment{
			Author: form.Author,
			Body:   form.Body,
			Posted: time.Now().UTC().Format(time.RFC3339),
		}
		commentsMu.Lock()
		comments[slug] = append(comments[slug], c)
		count := len(comments[slug])
		commentsMu.Unlock()

		if ev.Cookies != nil {
			ev.Cookies.Set("blog_last_author", form.Author, kit.CookieOpts{
				MaxAge: 7 * 24 * time.Hour,
			})
			ev.Cookies.Set("blog_comment_count", strconv.Itoa(count), kit.CookieOpts{
				MaxAge: 7 * 24 * time.Hour,
			})
		}

		return kit.ActionRedirect(303, "/"+slug)
	},
}

// renderMarkdown converts goldmark-produced HTML through bluemonday's
// UGC policy so the result is safe to splat into {@html}. UGC permits
// common formatting tags but strips scripts and event handlers.
func renderMarkdown(src []byte) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert(src, &buf); err != nil {
		return "", err
	}
	policy := bluemonday.UGCPolicy()
	return policy.Sanitize(buf.String()), nil
}

// isSafeSlug rejects slugs with path separators, dots, or anything that
// could escape the content/posts/ directory.
func isSafeSlug(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// splitFrontmatter pulls a tiny `key: value` block delimited by `---`
// from the head of body. Mirrors the helper in the index Load; kept
// local to avoid a shared package under src/routes that would break
// codegen's per-route mirror tree.
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
