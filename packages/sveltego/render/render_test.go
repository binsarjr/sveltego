package render_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/binsarjr/sveltego/render"
)

type stringerFn func() string

func (f stringerFn) String() string { return f() }

func TestNew_DefaultCapacity(t *testing.T) {
	w := render.New()
	if w == nil {
		t.Fatal("New returned nil")
	}
	if w.Len() != 0 {
		t.Errorf("Len = %d, want 0", w.Len())
	}
}

func TestWriteString(t *testing.T) {
	w := render.New()
	w.WriteString("<h1>")
	w.WriteString("hello")
	w.WriteString("</h1>")
	if got, want := string(w.Bytes()), "<h1>hello</h1>"; got != want {
		t.Errorf("Bytes = %q, want %q", got, want)
	}
}

func TestWriteRaw(t *testing.T) {
	w := render.New()
	w.WriteRaw("<script>x()</script>")
	if got, want := string(w.Bytes()), "<script>x()</script>"; got != want {
		t.Errorf("Bytes = %q, want %q", got, want)
	}
}

func TestWriteEscape(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string clean", "hello", "hello"},
		{"string lt", "<", "&lt;"},
		{"string gt", ">", "&gt;"},
		{"string amp", "&", "&amp;"},
		{"string quot", `"`, "&#34;"},
		{"string apos", "'", "&#39;"},
		{"string mixed", `<a href="x">"&'`, `&lt;a href=&#34;x&#34;&gt;&#34;&amp;&#39;`},
		{"string utf8", "café & crème", "café &amp; crème"},
		{"int", 42, "42"},
		{"int negative", -7, "-7"},
		{"int8", int8(-1), "-1"},
		{"int16", int16(300), "300"},
		{"int32", int32(-99999), "-99999"},
		{"int64", int64(9223372036854775807), "9223372036854775807"},
		{"uint", uint(7), "7"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float64", 3.14, "3.14"},
		{"float64 whole", 1.0, "1"},
		{"float32", float32(1.5), "1.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"bytes escapes", []byte("<x>"), "&lt;x&gt;"},
		{"stringer", stringerFn(func() string { return "<S>" }), "&lt;S&gt;"},
		{"error", errors.New("<oops>"), "&lt;oops&gt;"},
		{"struct fallback", struct{ X int }{7}, "{7}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := render.New()
			w.WriteEscape(tc.in)
			if got := string(w.Bytes()); got != tc.want {
				t.Errorf("WriteEscape(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteEscapeAttr(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string clean", "hello", "hello"},
		{"string apos preserved", "it's", "it's"},
		{"string lt", "<", "&lt;"},
		{"string gt", ">", "&gt;"},
		{"string amp", "&", "&amp;"},
		{"string quot", `"`, "&#34;"},
		{"string mixed", `<a "b">`, `&lt;a &#34;b&#34;&gt;`},
		{"int", 42, "42"},
		{"int64", int64(-1), "-1"},
		{"uint64", uint64(7), "7"},
		{"float64", 1.25, "1.25"},
		{"bool", true, "true"},
		{"bytes", []byte(`a"b`), "a&#34;b"},
		{"stringer", stringerFn(func() string { return `"S"` }), "&#34;S&#34;"},
		{"error", errors.New(`a"b`), "a&#34;b"},
		{"struct fallback", struct{ A string }{`<x>`}, "{&lt;x&gt;}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := render.New()
			w.WriteEscapeAttr(tc.in)
			if got := string(w.Bytes()); got != tc.want {
				t.Errorf("WriteEscapeAttr(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"object", map[string]int{"a": 1}, `{"a":1}`},
		{"slice", []int{1, 2, 3}, `[1,2,3]`},
		{"string", "hello", `"hello"`},
		{"html safe lt", "<script>", "\"\\u003cscript\\u003e\""},
		{"html safe amp", "a&b", "\"a\\u0026b\""},
		{"nil", nil, "null"},
		{"struct", struct {
			Name string `json:"name"`
		}{"x"}, `{"name":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := render.New()
			if err := w.WriteJSON(tc.in); err != nil {
				t.Fatalf("WriteJSON err = %v", err)
			}
			if got := string(w.Bytes()); got != tc.want {
				t.Errorf("WriteJSON(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteJSON_EncodeError(t *testing.T) {
	w := render.New()
	err := w.WriteJSON(make(chan int))
	if err == nil {
		t.Fatal("expected error for unencodable value, got nil")
	}
	if !strings.Contains(err.Error(), "render:") {
		t.Errorf("error %q missing package prefix", err.Error())
	}
}

func TestLenAndReset(t *testing.T) {
	w := render.New()
	if w.Len() != 0 {
		t.Errorf("initial Len = %d, want 0", w.Len())
	}
	w.WriteString("hello")
	if w.Len() != 5 {
		t.Errorf("Len after write = %d, want 5", w.Len())
	}
	w.Reset()
	if w.Len() != 0 {
		t.Errorf("Len after reset = %d, want 0", w.Len())
	}
	if len(w.Bytes()) != 0 {
		t.Errorf("Bytes after reset has len %d, want 0", len(w.Bytes()))
	}
}

func TestBytes_Aliasing(t *testing.T) {
	w := render.New()
	w.WriteString("abc")
	first := w.Bytes()
	w.WriteString("def")
	if got := string(w.Bytes()); got != "abcdef" {
		t.Errorf("Bytes after second write = %q, want abcdef", got)
	}
	// First slice header may now point to a stale prefix or grown buffer;
	// only the contract that Bytes() returns current state is asserted.
	_ = first
}

func TestAcquireRelease_Cycle(t *testing.T) {
	w := render.Acquire()
	w.WriteString("first")
	if got := string(w.Bytes()); got != "first" {
		t.Errorf("first cycle Bytes = %q", got)
	}
	render.Release(w)

	w2 := render.Acquire()
	t.Cleanup(func() { render.Release(w2) })
	if w2.Len() != 0 {
		t.Errorf("recycled Writer Len = %d, want 0", w2.Len())
	}
	if len(w2.Bytes()) != 0 {
		t.Errorf("recycled Writer Bytes len = %d, want 0", len(w2.Bytes()))
	}
	w2.WriteString("second")
	if got := string(w2.Bytes()); got != "second" {
		t.Errorf("second cycle Bytes = %q, want second", got)
	}
}

func TestRelease_NilSafe(_ *testing.T) {
	render.Release(nil)
}

func TestPool_Concurrent(t *testing.T) {
	const goroutines = 32
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for range iterations {
				w := render.Acquire()
				w.WriteString("<p>")
				w.WriteEscape("x&y")
				w.WriteString("</p>")
				if !strings.Contains(string(w.Bytes()), "x&amp;y") {
					t.Errorf("goroutine %d: missing escaped amp in %q", id, w.Bytes())
				}
				render.Release(w)
			}
		}(g)
	}
	wg.Wait()
}
