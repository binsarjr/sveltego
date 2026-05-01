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

var (
	commentsMu sync.RWMutex
	// comments stores in-memory threads keyed by post slug. The shape
	// matches the anonymous struct returned by Load so values pass
	// straight through to the template without conversion.
	comments = map[string][]struct {
		Author string
		Body   string
		Posted string
	}{}
)

func Load(ctx *kit.LoadCtx) (struct {
	Title    string
	Date     string
	HTML     string
	Comments []struct {
		Author string
		Body   string
		Posted string
	}
	Form any
},
	error,
) {
	// First return literal carries the struct shape codegen infers from.
	// Subsequent returns reuse `zero` to avoid restating the type seven
	// times.
	if ctx == nil || ctx.Params["slug"] == "" {
		return struct {
			Title    string
			Date     string
			HTML     string
			Comments []struct {
				Author string
				Body   string
				Posted string
			}
			Form any
		}{}, errors.New("missing slug param")
	}
	slug := ctx.Params["slug"]

	var zero struct {
		Title    string
		Date     string
		HTML     string
		Comments []struct {
			Author string
			Body   string
			Posted string
		}
		Form any
	}

	if !isSafeSlug(slug) {
		return zero, kit.Error(404, "post not found")
	}

	path := filepath.Join("content", "posts", slug+".md")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, kit.Error(404, "post not found")
		}
		return zero, kit.Error(500, "read post: "+err.Error())
	}

	fm, content := splitFrontmatter(body)
	html, err := renderMarkdown(content)
	if err != nil {
		return zero, kit.Error(500, "render markdown: "+err.Error())
	}

	commentsMu.RLock()
	existing := append([]struct {
		Author string
		Body   string
		Posted string
	}(nil), comments[slug]...)
	commentsMu.RUnlock()

	out := zero
	out.Title = fm["title"]
	out.Date = fm["date"]
	out.HTML = html
	out.Comments = existing
	return out, nil
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

		c := struct {
			Author string
			Body   string
			Posted string
		}{
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
