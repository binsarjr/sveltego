package routescan

import (
	"errors"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func TestParseSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    router.Segment
		wantErr bool
		wantGrp bool
	}{
		{name: "static", input: "about", want: router.Segment{Kind: router.SegmentStatic, Value: "about"}},
		{name: "static-with-digits", input: "v1", want: router.Segment{Kind: router.SegmentStatic, Value: "v1"}},
		{name: "param", input: "[id]", want: router.Segment{Kind: router.SegmentParam, Name: "id"}},
		{name: "param-matcher", input: "[id=int]", want: router.Segment{Kind: router.SegmentParam, Name: "id", Matcher: "int"}},
		{name: "optional", input: "[[lang]]", want: router.Segment{Kind: router.SegmentOptional, Name: "lang"}},
		{name: "optional-matcher", input: "[[lang=locale]]", want: router.Segment{Kind: router.SegmentOptional, Name: "lang", Matcher: "locale"}},
		{name: "rest", input: "[...path]", want: router.Segment{Kind: router.SegmentRest, Name: "path"}},
		{name: "rest-matcher", input: "[...path=slug]", want: router.Segment{Kind: router.SegmentRest, Name: "path", Matcher: "slug"}},
		{name: "group", input: "(marketing)", wantGrp: true},
		{name: "empty", input: "", wantErr: true},
		{name: "empty-brackets", input: "[]", wantErr: true},
		{name: "unbalanced-open", input: "[id", wantErr: true},
		{name: "unbalanced-optional", input: "[[id]", wantErr: true},
		{name: "triple-bracket", input: "[[[id]]]", wantErr: true},
		{name: "go-keyword", input: "[func]", wantErr: true},
		{name: "non-ident", input: "[id-x]", wantErr: true},
		{name: "empty-group", input: "()", wantErr: true},
		{name: "unbalanced-group", input: "(marketing", wantErr: true},
		{name: "static-with-bracket", input: "abo[ut", wantErr: true},
		{name: "param-empty-name", input: "[]", wantErr: true},
		{name: "rest-empty-name", input: "[...]", wantErr: true},
		{name: "param-bad-matcher", input: "[id=bad-matcher]", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSegment(tc.input)
			switch {
			case tc.wantGrp:
				if !errors.Is(err, ErrGroup) {
					t.Fatalf("want ErrGroup, got %v", err)
				}
			case tc.wantErr:
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got %+v, want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestBuildPattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		segs []router.Segment
		want string
	}{
		{"empty", nil, "/"},
		{"static", []router.Segment{{Kind: router.SegmentStatic, Value: "about"}}, "/about"},
		{
			"mixed",
			[]router.Segment{
				{Kind: router.SegmentStatic, Value: "post"},
				{Kind: router.SegmentParam, Name: "id"},
				{Kind: router.SegmentRest, Name: "path"},
			},
			"/post/[id]/[...path]",
		},
		{
			"optional-matcher",
			[]router.Segment{
				{Kind: router.SegmentOptional, Name: "lang", Matcher: "locale"},
			},
			"/[[lang=locale]]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BuildPattern(tc.segs)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
