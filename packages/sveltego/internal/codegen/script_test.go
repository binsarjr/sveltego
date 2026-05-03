package codegen

import (
	"reflect"
	"testing"
)

func TestExtractModuleExports(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "const-binding",
			body: `export const snapshot = { capture, restore };`,
			want: []string{"snapshot"},
		},
		{
			name: "let-binding",
			body: `export let counter = 0;`,
			want: []string{"counter"},
		},
		{
			name: "var-binding",
			body: `export var legacy = {};`,
			want: []string{"legacy"},
		},
		{
			name: "function-binding",
			body: `export function helper() { return 1; }`,
			want: []string{"helper"},
		},
		{
			name: "async-function-binding",
			body: `export async function load() { return 1; }`,
			want: []string{"load"},
		},
		{
			name: "generator-function-binding",
			body: `export function* gen() { yield 1; }`,
			want: []string{"gen"},
		},
		{
			name: "class-binding",
			body: `export class Widget {}`,
			want: []string{"Widget"},
		},
		{
			name: "named-reexport",
			body: `const snapshot = {}; export { snapshot };`,
			want: []string{"snapshot"},
		},
		{
			name: "renamed-reexport",
			body: `const internal = 1; export { internal as snapshot };`,
			want: []string{"snapshot"},
		},
		{
			name: "multi-name-reexport",
			body: `export { a, b, c };`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "mixed-bindings-and-reexports",
			body: `export const a = 1; export function b() {} const c = 1; export { c };`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "no-exports",
			body: `const local = 1; function helper() {}`,
			want: nil,
		},
		{
			name: "snapshot-as-prop-not-export",
			body: `const obj = { snapshot: 1 };`,
			want: nil,
		},
		{
			name: "commented-out-line",
			body: "// export const snapshot = {};\nexport const other = 1;",
			want: []string{"other"},
		},
		{
			name: "commented-out-block",
			body: "/* export const snapshot = {}; */ export const other = 1;",
			want: []string{"other"},
		},
		{
			name: "snapshot-substring-other-name",
			body: `export const snapshotMaker = () => ({});`,
			want: []string{"snapshotMaker"},
		},
		{
			name: "default-export-ignored",
			body: `export default function () { return 1; }`,
			want: nil,
		},
		{
			name: "duplicate-names-deduped",
			body: `export const a = 1; export { a };`,
			want: []string{"a"},
		},
		{
			name: "result-sorted",
			body: `export const z = 1; export const a = 2;`,
			want: []string{"a", "z"},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := extractModuleExports(c.body)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("extractModuleExports(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

func TestStripJSComments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"// hi\nfoo", "\nfoo"},
		{"/* hi */foo", "foo"},
		{"a // tail", "a "},
		{"a /* mid */ b", "a  b"},
		{"no comments here", "no comments here"},
	}
	for _, c := range cases {
		got := stripJSComments(c.in)
		if got != c.want {
			t.Errorf("stripJSComments(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
