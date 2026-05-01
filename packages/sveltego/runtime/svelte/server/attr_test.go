package server

import "testing"

func TestAttr(t *testing.T) {
	cases := []struct {
		name      string
		attrName  string
		value     any
		isBoolean bool
		want      string
	}{
		{"plain", "id", "main", false, ` id="main"`},
		{"escape-quote", "title", `say "hi"`, false, ` title="say &quot;hi&quot;"`},
		{"escape-amp-lt", "data-x", "a&b<c", false, ` data-x="a&amp;b&lt;c"`},
		{"nil-value", "id", nil, false, ""},
		{"boolean-true", "disabled", true, true, ` disabled=""`},
		{"boolean-false", "disabled", false, true, ""},
		{"boolean-empty-string", "disabled", "", true, ""},
		{"hidden-bool-default", "hidden", true, false, ` hidden=""`},
		{"hidden-until-found", "hidden", "until-found", false, ` hidden="until-found"`},
		{"translate-true", "translate", true, false, ` translate="yes"`},
		{"translate-false", "translate", false, false, ` translate="no"`},
		{"int-value", "tabindex", 0, false, ` tabindex="0"`},
		{"float-value", "data-pi", 3.14, false, ` data-pi="3.14"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Attr(tc.attrName, tc.value, tc.isBoolean); got != tc.want {
				t.Errorf("Attr(%q,%v,%v) = %q, want %q", tc.attrName, tc.value, tc.isBoolean, got, tc.want)
			}
		})
	}
}

func TestClsx(t *testing.T) {
	cases := []struct {
		name string
		args []any
		want string
	}{
		{"empty", nil, ""},
		{"single-string", []any{"foo"}, "foo"},
		{"multi-string", []any{"foo", "bar"}, "foo bar"},
		{"nil-skipped", []any{nil, "foo", nil}, "foo"},
		{"empty-string-skipped", []any{"", "foo", ""}, "foo"},
		{"map-truthy-only", []any{map[string]any{"a": true, "b": false, "c": true}}, "a c"},
		{"map-bool", []any{map[string]bool{"a": true, "b": false}}, "a"},
		{"slice-of-string", []any{[]string{"x", "y"}}, "x y"},
		{"nested-slice", []any{[]any{"x", []any{"y", "z"}}}, "x y z"},
		{"mixed", []any{"base", map[string]any{"on": true, "off": false}, "trail"}, "base on trail"},
		{"bool-skipped", []any{"x", true, false, "y"}, "x y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Clsx(tc.args...); got != tc.want {
				t.Errorf("Clsx(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestMergeStyles(t *testing.T) {
	cases := []struct {
		name string
		args []any
		want string
	}{
		{"empty", nil, ""},
		{"single", []any{"color: red"}, "color: red;"},
		{"single-with-semicolon", []any{"color: red;"}, "color: red;"},
		{"two", []any{"color: red", "font-size: 12px"}, "color: red; font-size: 12px;"},
		{"trim-multiple-semis", []any{"color: red;;;"}, "color: red;"},
		{"nil-skipped", []any{nil, "color: red", ""}, "color: red;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MergeStyles(tc.args...); got != tc.want {
				t.Errorf("MergeStyles(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestSpreadAttributes(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"empty", nil, ""},
		{"single", map[string]any{"id": "main"}, ` id="main"`},
		{"multi-sorted", map[string]any{"id": "x", "class": "y"}, ` class="y" id="x"`},
		{"drop-on-handler", map[string]any{"onclick": func() {}, "id": "a"}, ` id="a"`},
		{"drop-dollar", map[string]any{"$$slots": map[string]any{}, "id": "a"}, ` id="a"`},
		{"boolean-attr", map[string]any{"disabled": true}, ` disabled=""`},
		{"boolean-attr-false", map[string]any{"disabled": false}, ""},
		{"escape-attr", map[string]any{"title": `a "b"`}, ` title="a &quot;b&quot;"`},
		{"class-clsx", map[string]any{"class": map[string]any{"a": true, "b": false}}, ` class="a"`},
		{"invalid-name-skipped", map[string]any{"bad name": "x", "id": "ok"}, ` id="ok"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SpreadAttributes(tc.props); got != tc.want {
				t.Errorf("SpreadAttributes(%v) = %q, want %q", tc.props, got, tc.want)
			}
		})
	}
}

func TestIsValidAttrName(t *testing.T) {
	good := []string{"id", "class", "data-x", "aria-label", "x:y"}
	bad := []string{"", "a b", "a\"b", "a'b", "a/b", "a=b", "a>b"}
	for _, n := range good {
		if !isValidAttrName(n) {
			t.Errorf("isValidAttrName(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if isValidAttrName(n) {
			t.Errorf("isValidAttrName(%q) = true, want false", n)
		}
	}
}
