// Package sourcemap maps positions between a `.svelte` source file and the
// `.gen/<route>/page.go` Go file emitted by codegen, so LSP requests forwarded
// to gopls (run against the generated Go) can be translated back to the
// editor's view of the `.svelte` document.
//
// The scaffold ships an identity mapping per file. The full bidirectional
// mapping driven by codegen spans is the follow-up work tracked under #69.
package sourcemap

import "sort"

// Span pairs a half-open range in the `.svelte` source with the matching range
// in the generated Go file. Offsets are zero-based byte offsets.
type Span struct {
	SvelteStart, SvelteEnd int
	GoStart, GoEnd         int
}

// Map is the per-file mapping consulted on every LSP position translation.
type Map struct {
	SveltePath string
	GoPath     string
	Spans      []Span
}

// New builds a Map; the spans are sorted by SvelteStart for binary search.
func New(sveltePath, goPath string, spans []Span) *Map {
	out := make([]Span, len(spans))
	copy(out, spans)
	sort.Slice(out, func(i, j int) bool { return out[i].SvelteStart < out[j].SvelteStart })
	return &Map{SveltePath: sveltePath, GoPath: goPath, Spans: out}
}

// SvelteToGo translates a `.svelte` byte offset to a generated-Go byte offset.
// Returns ok=false when the offset is outside any recorded span.
func (m *Map) SvelteToGo(offset int) (int, bool) {
	if m == nil {
		return 0, false
	}
	idx := sort.Search(len(m.Spans), func(i int) bool {
		return m.Spans[i].SvelteEnd > offset
	})
	if idx == len(m.Spans) {
		return 0, false
	}
	span := m.Spans[idx]
	if offset < span.SvelteStart {
		return 0, false
	}
	return span.GoStart + (offset - span.SvelteStart), true
}

// GoToSvelte translates a generated-Go byte offset to a `.svelte` byte offset.
// Returns ok=false when the offset is outside any recorded span.
func (m *Map) GoToSvelte(offset int) (int, bool) {
	if m == nil {
		return 0, false
	}
	for _, span := range m.Spans {
		if offset >= span.GoStart && offset < span.GoEnd {
			return span.SvelteStart + (offset - span.GoStart), true
		}
	}
	return 0, false
}

// Identity returns a Map whose single span covers length bytes 1:1, useful for
// scaffold tests and for files that have not yet been codegen'd.
func Identity(sveltePath, goPath string, length int) *Map {
	if length <= 0 {
		return New(sveltePath, goPath, nil)
	}
	return New(sveltePath, goPath, []Span{{
		SvelteStart: 0, SvelteEnd: length,
		GoStart: 0, GoEnd: length,
	}})
}
