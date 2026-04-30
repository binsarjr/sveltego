package router

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"io"
	"sort"
	"strconv"
)

var errEmptyPattern = errors.New("router: empty pattern")

type duplicatePatternError struct {
	pattern string
}

func (e *duplicatePatternError) Error() string {
	return "router: duplicate pattern " + strconv.Quote(e.pattern)
}

type missingMatcherError struct {
	pattern string
	matcher string
}

func (e *missingMatcherError) Error() string {
	return "router: route " + strconv.Quote(e.pattern) +
		" references unknown matcher " + strconv.Quote(e.matcher)
}

// node is one position in the radix tree. Children are split by kind so
// the matcher can iterate them in resolution order without per-call sort
// cost. Routes terminate on a node via the route field.
type node struct {
	staticChildren []*node
	paramChildren  []*node
	optional       *node
	rest           *node

	kind    SegmentKind
	value   string
	name    string
	matcher string

	route *Route

	// bestSpec is the lexicographically-greatest specificity vector
	// among routes terminating in this subtree. Used to break ties
	// between sibling param children whose specificity differs only in
	// later segments.
	bestSpec []int
}

// Tree is the radix-tree route table. It is read-only after NewTree
// returns; concurrent Match calls are safe.
type Tree struct {
	root     *node
	routes   []Route
	matchers Matchers
}

// NewTree builds a Tree from routes. Insert order does not affect match
// outcomes: children are sorted by specificity at build time and each
// Route.ID is populated with an 8-char FNV-1a hash of Pattern. Matcher
// validation happens later via WithMatchers; routes carrying a Matcher
// name are accepted here so callers can install matchers in any order.
func NewTree(routes []Route) (*Tree, error) {
	t := &Tree{root: &node{}, routes: make([]Route, len(routes))}
	copy(t.routes, routes)
	for i := range t.routes {
		r := &t.routes[i]
		r.ID = routeID(r.Pattern)
		if err := t.insert(r); err != nil {
			return nil, err
		}
	}
	propagateSpec(t.root)
	sortAll(t.root)
	return t, nil
}

// WithMatchers installs the matcher registry used by Match for segments
// declaring `[name=matcher]`. Validation runs immediately: any segment
// naming a missing matcher returns a build error so the failure
// surfaces at startup, not on first request.
func (t *Tree) WithMatchers(m Matchers) (*Tree, error) {
	t.matchers = m
	for i := range t.routes {
		for _, s := range t.routes[i].Segments {
			if s.Matcher == "" {
				continue
			}
			if _, ok := m[s.Matcher]; !ok {
				return nil, &missingMatcherError{pattern: t.routes[i].Pattern, matcher: s.Matcher}
			}
		}
	}
	return t, nil
}

// Routes returns a copy of the tree's route slice. Each call allocates
// a new slice header and backing array; callers are free to sort or
// truncate the result without affecting the tree.
func (t *Tree) Routes() []Route {
	out := make([]Route, len(t.routes))
	copy(out, t.routes)
	return out
}

func (t *Tree) insert(r *Route) error {
	if r.Pattern == "" {
		return errEmptyPattern
	}
	cur := t.root
	for _, seg := range r.Segments {
		cur = cur.childFor(seg)
	}
	if cur.route != nil {
		return &duplicatePatternError{pattern: r.Pattern}
	}
	cur.route = r
	return nil
}

func (n *node) childFor(seg Segment) *node {
	switch seg.Kind {
	case SegmentStatic:
		for _, c := range n.staticChildren {
			if c.value == seg.Value {
				return c
			}
		}
		c := &node{kind: SegmentStatic, value: seg.Value}
		n.staticChildren = append(n.staticChildren, c)
		return c
	case SegmentParam:
		for _, c := range n.paramChildren {
			if c.name == seg.Name && c.matcher == seg.Matcher {
				return c
			}
		}
		c := &node{kind: SegmentParam, name: seg.Name, matcher: seg.Matcher}
		n.paramChildren = append(n.paramChildren, c)
		return c
	case SegmentOptional:
		if n.optional != nil && n.optional.name == seg.Name && n.optional.matcher == seg.Matcher {
			return n.optional
		}
		c := &node{kind: SegmentOptional, name: seg.Name, matcher: seg.Matcher}
		n.optional = c
		return c
	case SegmentRest:
		if n.rest != nil && n.rest.name == seg.Name && n.rest.matcher == seg.Matcher {
			return n.rest
		}
		c := &node{kind: SegmentRest, name: seg.Name, matcher: seg.Matcher}
		n.rest = c
		return c
	}
	return n
}

// segmentScore returns a per-segment specificity score; higher is more
// specific. Lex-comparing the per-route vector implements
// "static beats matcher-param beats param beats optional beats rest"
// at every segment depth.
func segmentScore(s Segment) int {
	switch s.Kind {
	case SegmentStatic:
		return 4
	case SegmentParam:
		if s.Matcher != "" {
			return 3
		}
		return 2
	case SegmentOptional:
		return 1
	case SegmentRest:
		return 0
	}
	return 0
}

func routeSpec(r *Route) []int {
	out := make([]int, len(r.Segments))
	for i, s := range r.Segments {
		out[i] = segmentScore(s)
	}
	return out
}

// specCompare returns -1, 0, +1 by lexicographic compare of two
// specificity vectors. A nil vector compares as less than any non-nil
// vector at the same prefix, matching "no terminal route in subtree".
func specCompare(a, b []int) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func bestOf(a, b []int) []int {
	if specCompare(a, b) >= 0 {
		return a
	}
	return b
}

func propagateSpec(n *node) []int {
	var best []int
	if n.route != nil {
		best = bestOf(best, routeSpec(n.route))
	}
	for _, c := range n.staticChildren {
		best = bestOf(best, propagateSpec(c))
	}
	for _, c := range n.paramChildren {
		best = bestOf(best, propagateSpec(c))
	}
	if n.optional != nil {
		best = bestOf(best, propagateSpec(n.optional))
	}
	if n.rest != nil {
		best = bestOf(best, propagateSpec(n.rest))
	}
	n.bestSpec = best
	return best
}

func sortAll(n *node) {
	sort.SliceStable(n.staticChildren, func(i, j int) bool {
		a, b := n.staticChildren[i], n.staticChildren[j]
		if cmp := specCompare(a.bestSpec, b.bestSpec); cmp != 0 {
			return cmp > 0
		}
		return a.value < b.value
	})
	sort.SliceStable(n.paramChildren, func(i, j int) bool {
		a, b := n.paramChildren[i], n.paramChildren[j]
		if cmp := specCompare(a.bestSpec, b.bestSpec); cmp != 0 {
			return cmp > 0
		}
		if (a.matcher != "") != (b.matcher != "") {
			return a.matcher != ""
		}
		return a.name < b.name
	})
	for _, c := range n.staticChildren {
		sortAll(c)
	}
	for _, c := range n.paramChildren {
		sortAll(c)
	}
	if n.optional != nil {
		sortAll(n.optional)
	}
	if n.rest != nil {
		sortAll(n.rest)
	}
}

// routeID returns an 8-char hex tag derived from an FNV-1a 32-bit hash
// of the route pattern. The tag is used for fast logging and manifest
// correlation; it is NOT a collision-free identity. Two routes with
// different patterns may produce the same ID (p ≈ N²/2³² for N routes).
// Do not use ID as a unique key; use Pattern for that.
func routeID(pattern string) string {
	h := fnv.New32a()
	_, _ = io.WriteString(h, pattern)
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], h.Sum32())
	return hex.EncodeToString(buf[:])
}
