package server

import (
	"bytes"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
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
// close) as the absence of a span at that position. The scan walks src
// in place: no string conversion, no full lowercase copy, zero
// allocation on the no-title fast path.
func findTitleSpans(src []byte) []titleSpan {
	var (
		out     []titleSpan
		titleLC = []byte("title")
		closeLC = []byte("</title>")
	)
	cursor := 0
	for cursor < len(src) {
		lt := bytes.IndexByte(src[cursor:], '<')
		if lt < 0 {
			break
		}
		open := cursor + lt
		after := open + 1 + len(titleLC) // position of the byte after "<title"
		if after >= len(src) {
			break
		}
		if !bytes.EqualFold(src[open+1:after], titleLC) {
			cursor = open + 1
			continue
		}
		c := src[after]
		if c != '>' && c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '/' {
			cursor = after
			continue
		}
		gt := bytes.IndexByte(src[after:], '>')
		if gt < 0 {
			break
		}
		closeStart := indexFold(src, closeLC, after+gt+1)
		if closeStart < 0 {
			break
		}
		end := closeStart + len(closeLC)
		out = append(out, titleSpan{start: open, end: end})
		cursor = end
	}
	return out
}

// indexFold returns the index of the first case-insensitive match of
// needle in src starting at from, or -1 if absent. needle's first byte
// must be a literal (non-letter) anchor for a cheap IndexByte scan; the
// remaining bytes are compared with bytes.EqualFold.
func indexFold(src, needle []byte, from int) int {
	if from < 0 {
		from = 0
	}
	if len(needle) == 0 || from+len(needle) > len(src) {
		return -1
	}
	anchor := needle[0]
	rest := needle[1:]
	for i := from; i+len(needle) <= len(src); {
		idx := bytes.IndexByte(src[i:], anchor)
		if idx < 0 {
			return -1
		}
		j := i + idx
		if j+len(needle) > len(src) {
			return -1
		}
		if bytes.EqualFold(src[j+1:j+len(needle)], rest) {
			return j
		}
		i = j + 1
	}
	return -1
}
