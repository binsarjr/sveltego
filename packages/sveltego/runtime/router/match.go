package router

import (
	"net/url"
	"strings"
)

// maxStackParams caps the number of captures kept on the stack frame.
// Eight covers any realistic SvelteKit-style route depth and lets the
// matcher stay allocation-free for static and param paths.
const maxStackParams = 8

type capture struct {
	name  string
	value string
}

// matchState carries per-Match scratch state. Fields are addressable
// from Match's stack frame and never escape, so the matcher itself
// allocates zero on the static and param hot paths.
type matchState struct {
	caps [maxStackParams]capture
	n    int
	over []capture
}

func (s *matchState) push(c capture) {
	if s.n < maxStackParams {
		s.caps[s.n] = c
		s.n++
		return
	}
	s.over = append(s.over, c)
	s.n++
}

// truncate restores the state to the size it had before some pushes
// were made. The invariant maintained is len(s.over) == max(0, s.n -
// maxStackParams).
func (s *matchState) truncate(to int) {
	over := to - maxStackParams
	if over < 0 {
		over = 0
	}
	if over < len(s.over) {
		s.over = s.over[:over]
	}
	s.n = to
}

func (s *matchState) at(i int) capture {
	if i < maxStackParams {
		return s.caps[i]
	}
	return s.over[i-maxStackParams]
}

// Match resolves path against the tree. The first terminal route in
// resolution order (static > param-with-matcher > param > optional >
// rest) wins. The returned param map is keyed by Segment.Name; absent
// optional segments map to "" and rest segments map to the joined
// remainder. Match returns nil, nil, false when no route matches.
//
// path is split on literal '/' boundaries first; each segment is then
// URL-decoded. URL-encoded slashes (%2F) survive splitting and end up
// inside the segment value (e.g. /files/a%2Fb becomes one segment
// "a/b").
func (t *Tree) Match(path string) (*Route, map[string]string, bool) {
	var segs [maxStackParams]string
	parsed, ok := splitAndDecodeInto(path, &segs)
	if !ok {
		return nil, nil, false
	}
	var st matchState
	r := t.matchNode(t.root, parsed, &st)
	if r == nil {
		return nil, nil, false
	}
	if st.n == 0 {
		return r, nil, true
	}
	params := make(map[string]string, st.n)
	for i := 0; i < st.n; i++ {
		c := st.at(i)
		params[c.name] = c.value
	}
	return r, params, true
}

func (t *Tree) matchNode(n *node, segs []string, st *matchState) *Route {
	if len(segs) == 0 {
		if n.route != nil {
			return n.route
		}
		if n.optional != nil {
			before := st.n
			st.push(capture{name: n.optional.name, value: ""})
			if r := t.matchNode(n.optional, nil, st); r != nil {
				return r
			}
			st.truncate(before)
		}
		if n.rest != nil && n.rest.route != nil {
			st.push(capture{name: n.rest.name, value: ""})
			return n.rest.route
		}
		return nil
	}

	seg := segs[0]
	for _, c := range n.staticChildren {
		if c.value != seg {
			continue
		}
		if r := t.matchNode(c, segs[1:], st); r != nil {
			return r
		}
	}
	for _, c := range n.paramChildren {
		if c.matcher != "" {
			if t.matchers == nil {
				continue
			}
			m, ok := t.matchers[c.matcher]
			if !ok || !m.Match(seg) {
				continue
			}
		}
		before := st.n
		st.push(capture{name: c.name, value: seg})
		if r := t.matchNode(c, segs[1:], st); r != nil {
			return r
		}
		st.truncate(before)
	}
	if n.optional != nil {
		c := n.optional
		eligible := true
		if c.matcher != "" {
			eligible = false
			if t.matchers != nil {
				if m, found := t.matchers[c.matcher]; found && m.Match(seg) {
					eligible = true
				}
			}
		}
		if eligible {
			before := st.n
			st.push(capture{name: c.name, value: seg})
			if r := t.matchNode(c, segs[1:], st); r != nil {
				return r
			}
			st.truncate(before)
		}
		before := st.n
		st.push(capture{name: c.name, value: ""})
		if r := t.matchNode(c, segs, st); r != nil {
			return r
		}
		st.truncate(before)
	}
	if n.rest != nil && n.rest.route != nil {
		c := n.rest
		joined := joinSegs(segs)
		if c.matcher != "" {
			if t.matchers == nil {
				return nil
			}
			m, found := t.matchers[c.matcher]
			if !found || !m.Match(joined) {
				return nil
			}
		}
		st.push(capture{name: c.name, value: joined})
		return c.route
	}
	return nil
}

func joinSegs(segs []string) string {
	switch len(segs) {
	case 0:
		return ""
	case 1:
		return segs[0]
	}
	return strings.Join(segs, "/")
}

// splitAndDecodeInto splits p on literal '/' bytes, then URL-decodes
// each segment into the caller-provided fixed-size buffer. When the
// path has more segments than maxStackParams, the function falls back
// to a heap slice (rare). The leading slash is consumed; "" and "/"
// both yield a nil slice.
func splitAndDecodeInto(p string, buf *[maxStackParams]string) ([]string, bool) {
	if p == "" || p == "/" {
		return nil, true
	}
	if p[0] == '/' {
		p = p[1:]
	}
	if p == "" {
		return nil, true
	}
	count := 0
	start := 0
	var heap []string
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			s, err := url.PathUnescape(p[start:i])
			if err != nil {
				return nil, false
			}
			if count < maxStackParams && heap == nil {
				buf[count] = s
			} else {
				if heap == nil {
					heap = make([]string, maxStackParams, count*2)
					copy(heap, buf[:])
				}
				heap = append(heap, s)
			}
			count++
			start = i + 1
		}
	}
	s, err := url.PathUnescape(p[start:])
	if err != nil {
		return nil, false
	}
	if count < maxStackParams && heap == nil {
		buf[count] = s
		count++
		return buf[:count], true
	}
	if heap == nil {
		heap = make([]string, maxStackParams, count+1)
		copy(heap, buf[:])
	}
	heap = append(heap, s)
	return heap, true
}
