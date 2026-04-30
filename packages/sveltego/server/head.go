package server

import (
	"bytes"
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

// gatherHead invokes every layout-chain head handler (outer→inner) and
// the page head handler against a side render.Writer, runs the title
// dedupe pass, and returns the assembled bytes ready to splice between
// <head> and </head>. Returns nil when no handler is registered.
func gatherHead(rctx *kit.RenderCtx, route *router.Route, data any, layoutDatas []any) ([]byte, error) {
	if route == nil {
		return nil, nil
	}
	if route.Head == nil && !anyLayoutHead(route.LayoutHeads) {
		return nil, nil
	}

	buf := render.Acquire()
	defer render.Release(buf)

	for i, lh := range route.LayoutHeads {
		if lh == nil {
			continue
		}
		var ld any
		if i < len(layoutDatas) {
			ld = layoutDatas[i]
		}
		if err := lh(buf, rctx, ld); err != nil {
			return nil, err
		}
	}
	if route.Head != nil {
		if err := route.Head(buf, rctx, data); err != nil {
			return nil, err
		}
	}

	if buf.Len() == 0 {
		return nil, nil
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return dedupeTitle(out), nil
}

// anyLayoutHead reports whether at least one entry in heads is non-nil.
// Mirrors hasAnyLayoutLoader for the head-handler slice.
func anyLayoutHead(heads []router.LayoutHeadHandler) bool {
	for _, h := range heads {
		if h != nil {
			return true
		}
	}
	return false
}

// dedupeTitle collapses multiple <title>...</title> tags in the head
// buffer into the LAST occurrence. SvelteKit-style: the page wins over
// outer layouts, an inner layout wins over the root layout. The scan is
// case-insensitive on the tag name and tolerates leading whitespace
// inside the open tag (`<title >`). Content is taken verbatim — no
// HTML parsing, no attribute reshuffling.
func dedupeTitle(in []byte) []byte {
	matches := findTitleSpans(in)
	if len(matches) <= 1 {
		return in
	}
	last := matches[len(matches)-1]
	var out bytes.Buffer
	out.Grow(len(in))
	cursor := 0
	for _, m := range matches[:len(matches)-1] {
		out.Write(in[cursor:m.start])
		cursor = m.end
	}
	out.Write(in[cursor:last.start])
	out.Write(in[last.start:last.end])
	out.Write(in[last.end:])
	return out.Bytes()
}

type titleSpan struct {
	start int
	end   int
}

// findTitleSpans locates every `<title...>...</title>` span in src. The
// scan is case-insensitive and treats malformed input (open without
// close) as the absence of a span at that position.
func findTitleSpans(src []byte) []titleSpan {
	var out []titleSpan
	lower := strings.ToLower(string(src))
	cursor := 0
	for cursor < len(lower) {
		open := indexAt(lower, "<title", cursor)
		if open < 0 {
			break
		}
		// Verify the next char is `>`, whitespace, or `/` so we don't
		// match `<titlefoo>`. EOF after the prefix counts as no match.
		after := open + len("<title")
		if after >= len(lower) {
			break
		}
		c := lower[after]
		if c != '>' && c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '/' {
			cursor = after
			continue
		}
		gt := indexAt(lower, ">", after)
		if gt < 0 {
			break
		}
		closeAt := indexAt(lower, "</title>", gt+1)
		if closeAt < 0 {
			break
		}
		end := closeAt + len("</title>")
		out = append(out, titleSpan{start: open, end: end})
		cursor = end
	}
	return out
}

func indexAt(s, needle string, from int) int {
	if from < 0 {
		from = 0
	}
	if from >= len(s) {
		return -1
	}
	idx := strings.Index(s[from:], needle)
	if idx < 0 {
		return -1
	}
	return from + idx
}
