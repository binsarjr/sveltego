package routescan

import "testing"

func TestHasSSRFallbackMarker(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "exact marker",
			src:  "<!-- sveltego:ssr-fallback -->\n<h1>hello</h1>",
			want: true,
		},
		{
			name: "extra whitespace",
			src:  "<!--   sveltego:ssr-fallback\t-->",
			want: true,
		},
		{
			name: "leading and trailing newlines inside comment",
			src:  "<!--\n  sveltego:ssr-fallback\n-->",
			want: true,
		},
		{
			name: "later in file",
			src:  "<script>let x = 1;</script>\n<!-- sveltego:ssr-fallback -->\n<h1>hi</h1>",
			want: true,
		},
		{
			name: "marker in string literal not a comment",
			src:  `<script>const s = "sveltego:ssr-fallback";</script>`,
			want: false,
		},
		{
			name: "extra text inside comment is not a match",
			src:  "<!-- sveltego:ssr-fallback please -->",
			want: false,
		},
		{
			name: "no comment at all",
			src:  "<h1>hello</h1>",
			want: false,
		},
		{
			name: "different marker name",
			src:  "<!-- sveltego:other -->",
			want: false,
		},
		{
			name: "unterminated comment",
			src:  "<!-- sveltego:ssr-fallback",
			want: false,
		},
		{
			name: "second comment wins after a non-match",
			src:  "<!-- not us -->\n<!-- sveltego:ssr-fallback -->",
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasSSRFallbackMarker([]byte(tc.src))
			if got != tc.want {
				t.Fatalf("hasSSRFallbackMarker(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}
