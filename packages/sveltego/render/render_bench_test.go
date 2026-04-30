package render_test

import (
	"testing"

	"github.com/binsarjr/sveltego/render"
)

const (
	benchClean = "the quick brown fox jumps over the lazy dog"
	benchDirty = `<a href="x">Tom & Jerry's "best"</a>`
	benchAttr  = `Tom "Tornado" O'Brien & Sons`
)

func BenchmarkWriteString(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteString(benchClean)
		render.Release(w)
	}
}

func BenchmarkWriteEscape_Clean(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteEscape(benchClean)
		render.Release(w)
	}
}

func BenchmarkWriteEscape_Dirty(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteEscape(benchDirty)
		render.Release(w)
	}
}

func BenchmarkWriteEscape_Int(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteEscape(123456)
		render.Release(w)
	}
}

func BenchmarkWriteEscapeAttr_Clean(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteEscapeAttr(benchClean)
		render.Release(w)
	}
}

func BenchmarkWriteEscapeAttr_Dirty(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		w := render.Acquire()
		w.WriteEscapeAttr(benchAttr)
		render.Release(w)
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	payload := map[string]any{"name": "Tom", "tags": []string{"a", "b", "c"}, "n": 42}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := render.Acquire()
		_ = w.WriteJSON(payload)
		render.Release(w)
	}
}

func BenchmarkWriteRawBytes(b *testing.B) {
	payload := []byte("<title>page</title><meta name=\"x\" content=\"y\">")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := render.Acquire()
		w.WriteRawBytes(payload)
		render.Release(w)
	}
}

type benchStruct struct {
	A int
	B string
}

func BenchmarkWriteEscape_Default(b *testing.B) {
	v := benchStruct{A: 7, B: "hello"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := render.Acquire()
		w.WriteEscape(v)
		render.Release(w)
	}
}

func BenchmarkWriteEscapeAttr_Default(b *testing.B) {
	v := benchStruct{A: 7, B: "hello"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := render.Acquire()
		w.WriteEscapeAttr(v)
		render.Release(w)
	}
}
