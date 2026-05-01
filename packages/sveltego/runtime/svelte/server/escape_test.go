package server

import "testing"

func TestEscapeHTMLContent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"amp", "a & b", "a &amp; b"},
		{"lt", "a < b", "a &lt; b"},
		{"both", "a & b < c", "a &amp; b &lt; c"},
		{"gt-not-escaped", "a > b", "a > b"},
		{"quote-not-escaped-in-content", `she said "hi"`, `she said "hi"`},
		{"apos-not-escaped", "it's", "it's"},
		{"nil", nil, ""},
		{"int", 42, "42"},
		{"bool-true", true, "true"},
		{"bool-false", false, "false"},
		{"unicode", "héllo & wörld", "héllo &amp; wörld"},
		{"multibyte-amp", "日本 & 語", "日本 &amp; 語"},
		{"already-escaped", "&amp;", "&amp;amp;"},
		{"only-amps", "&&&", "&amp;&amp;&amp;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeHTML(tc.in); got != tc.want {
				t.Errorf("EscapeHTML(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeHTMLAttr(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"amp", "a & b", "a &amp; b"},
		{"lt", "a < b", "a &lt; b"},
		{"quote", `she said "hi"`, `she said &quot;hi&quot;`},
		{"all-three", `& < "`, `&amp; &lt; &quot;`},
		{"gt-not-escaped", "a > b", "a > b"},
		{"apos-not-escaped", "it's", "it's"},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeHTMLAttr(tc.in); got != tc.want {
				t.Errorf("EscapeHTMLAttr(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeHTMLString(t *testing.T) {
	if got := EscapeHTMLString("a & b"); got != "a &amp; b" {
		t.Errorf("EscapeHTMLString = %q", got)
	}
	if got := EscapeHTMLAttrString(`a "b"`); got != `a &quot;b&quot;` {
		t.Errorf("EscapeHTMLAttrString = %q", got)
	}
}

func BenchmarkEscapeHTMLPlain(b *testing.B) {
	s := "hello world this is a normal sentence with no special characters at all"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EscapeHTMLString(s)
	}
}

func BenchmarkEscapeHTMLEscaping(b *testing.B) {
	s := `<script>alert("x & y")</script>`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EscapeHTMLString(s)
	}
}

func BenchmarkEscapeHTMLLong(b *testing.B) {
	var s string
	for i := 0; i < 100; i++ {
		s += "lorem ipsum dolor sit amet & friends < everyone > "
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EscapeHTMLString(s)
	}
}
