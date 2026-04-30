package codegen

import "testing"

func TestDetectSnapshotExport(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "const-binding",
			body: `export const snapshot = { capture, restore };`,
			want: true,
		},
		{
			name: "let-binding",
			body: `export let snapshot = { capture: () => 0, restore: () => {} };`,
			want: true,
		},
		{
			name: "var-binding",
			body: `export var snapshot = {};`,
			want: true,
		},
		{
			name: "named-reexport",
			body: `const snapshot = {}; export { snapshot };`,
			want: true,
		},
		{
			name: "renamed-reexport",
			body: `const s = {}; export { s as snapshot };`,
			want: true,
		},
		{
			name: "no-snapshot",
			body: `export const greeting = "hello";`,
			want: false,
		},
		{
			name: "snapshot-as-prop-not-export",
			body: `const obj = { snapshot: 1 };`,
			want: false,
		},
		{
			name: "commented-out-line",
			body: "// export const snapshot = {};\nexport const other = 1;",
			want: false,
		},
		{
			name: "commented-out-block",
			body: "/* export const snapshot = {}; */ export const other = 1;",
			want: false,
		},
		{
			name: "snapshot-substring-other-name",
			body: `export const snapshotMaker = () => ({});`,
			want: false,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := detectSnapshotExport(c.body)
			if got != c.want {
				t.Errorf("detectSnapshotExport(%q) = %v, want %v", c.body, got, c.want)
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
