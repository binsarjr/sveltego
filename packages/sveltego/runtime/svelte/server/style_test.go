package server

import "testing"

func TestToClass(t *testing.T) {
	cases := []struct {
		name       string
		value      any
		hash       string
		directives map[string]bool
		want       string
	}{
		{"nil", nil, "", nil, ""},
		{"value-only", "foo", "", nil, "foo"},
		{"value-and-hash", "foo", "h1", nil, "foo h1"},
		{"hash-only", nil, "h1", nil, "h1"},
		{"directives-add", "base", "", map[string]bool{"on": true}, "base on"},
		{"directives-remove", "base on tail", "", map[string]bool{"on": false}, "base tail"},
		{"directives-add-and-remove", "a b", "", map[string]bool{"b": false, "c": true}, "a c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToClass(tc.value, tc.hash, tc.directives); got != tc.want {
				t.Errorf("ToClass(%v,%q,%v) = %q, want %q", tc.value, tc.hash, tc.directives, got, tc.want)
			}
		})
	}
}

func TestToStyle(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		styles map[string]string
		want   string
	}{
		{"nil", nil, nil, ""},
		{"value-string", "color: red", nil, "color: red"},
		{"value-trimmed", "  color: red  ", nil, "color: red"},
		{"styles-only", nil, map[string]string{"color": "red"}, "color: red;"},
		{"merge", "font-size: 12px", map[string]string{"color": "red"}, "font-size: 12px; color: red;"},
		{"reserved-skipped", "color: blue; font-size: 12px", map[string]string{"color": "red"}, "font-size: 12px; color: red;"},
		{"strip-comments", "color: red /* note */", map[string]string{"x": "1"}, "color: red; x: 1;"},
		{"css-var", "--foo: 1", map[string]string{"--bar": "2"}, "--foo: 1; --bar: 2;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToStyle(tc.value, tc.styles); got != tc.want {
				t.Errorf("ToStyle(%v,%v) = %q, want %q", tc.value, tc.styles, got, tc.want)
			}
		})
	}
}

func TestSplitDecls(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a: 1; b: 2", []string{"a: 1", "b: 2"}},
		{"a: rgb(1, 2, 3); b: 4", []string{"a: rgb(1, 2, 3)", "b: 4"}},
		{`a: "x;y"; b: 4`, []string{`a: "x;y"`, "b: 4"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := splitDecls(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitDecls(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("splitDecls(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}
