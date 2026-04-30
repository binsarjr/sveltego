package kit

import "testing"

func TestLink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pattern string
		params  map[string]string
		want    string
		wantErr bool
	}{
		{name: "root", pattern: "/", want: "/"},
		{name: "static", pattern: "/about", want: "/about"},
		{name: "param", pattern: "/blog/[slug]", params: map[string]string{"slug": "hello"}, want: "/blog/hello"},
		{name: "param-matcher", pattern: "/users/[id=int]", params: map[string]string{"id": "7"}, want: "/users/7"},
		{
			name:    "rest-with-slash",
			pattern: "/docs/[...path]",
			params:  map[string]string{"path": "/intro/setup"},
			want:    "/docs/intro/setup",
		},
		{name: "rest-empty", pattern: "/docs/[...path]", params: map[string]string{"path": ""}, want: "/docs"},
		{name: "rest-missing", pattern: "/docs/[...path]", want: "/docs"},
		{name: "optional-set", pattern: "/[[lang]]/about", params: map[string]string{"lang": "en"}, want: "/en/about"},
		{name: "optional-empty", pattern: "/[[lang]]/about", params: map[string]string{"lang": ""}, want: "/about"},
		{name: "optional-missing", pattern: "/[[lang]]/about", want: "/about"},
		{name: "missing-required", pattern: "/blog/[slug]", wantErr: true},
		{name: "no-leading-slash", pattern: "blog", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Link(tc.pattern, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Link(%q,%v) = %q, want %q", tc.pattern, tc.params, got, tc.want)
			}
		})
	}
}
