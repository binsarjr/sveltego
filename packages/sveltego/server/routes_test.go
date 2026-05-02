package server

import (
	"reflect"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func TestServer_Routes_StaticAndDynamic(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("home"),
			Options: kit.PageOptions{
				Prerender: true,
				SSR:       true,
				CSR:       true,
			},
		},
		{
			Pattern:  "/post/[id=int]",
			Segments: segmentsFor("/post/[id=int]"),
			Page:     paramPage(),
			Options: kit.PageOptions{
				SSR: true,
				CSR: true,
			},
		},
		{
			Pattern:  "/docs/[...path]",
			Segments: segmentsFor("/docs/[...path]"),
			Page:     staticPage("docs"),
			Options: kit.PageOptions{
				SSR: true,
			},
		},
	})

	got := srv.Routes()
	if len(got) != 3 {
		t.Fatalf("len(Routes()) = %d, want 3", len(got))
	}

	byPattern := make(map[string]RouteSummary, len(got))
	for _, r := range got {
		byPattern[r.Pattern] = r
	}

	root, ok := byPattern["/"]
	if !ok {
		t.Fatalf("missing summary for /")
	}
	if !root.Prerender {
		t.Errorf("/ Prerender = false, want true")
	}
	if !root.SSR {
		t.Errorf("/ SSR = false, want true")
	}
	if len(root.DynamicParams) != 0 {
		t.Errorf("/ DynamicParams = %v, want empty", root.DynamicParams)
	}
	if !root.PageOptions.CSR {
		t.Errorf("/ PageOptions.CSR = false, want true")
	}

	post, ok := byPattern["/post/[id=int]"]
	if !ok {
		t.Fatalf("missing summary for /post/[id=int]")
	}
	if post.Prerender {
		t.Errorf("/post/[id=int] Prerender = true, want false")
	}
	if !reflect.DeepEqual(post.DynamicParams, []string{"id"}) {
		t.Errorf("/post/[id=int] DynamicParams = %v, want [id]", post.DynamicParams)
	}

	docs, ok := byPattern["/docs/[...path]"]
	if !ok {
		t.Fatalf("missing summary for /docs/[...path]")
	}
	if !reflect.DeepEqual(docs.DynamicParams, []string{"path"}) {
		t.Errorf("/docs/[...path] DynamicParams = %v, want [path]", docs.DynamicParams)
	}
}

func TestServer_Routes_ReturnsCopy(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("home"),
			Options:  kit.PageOptions{SSR: true},
		},
	})

	first := srv.Routes()
	if len(first) != 1 {
		t.Fatalf("len = %d", len(first))
	}
	first[0].Pattern = "/mutated"
	second := srv.Routes()
	if second[0].Pattern != "/" {
		t.Errorf("Routes mutation leaked: pattern = %q", second[0].Pattern)
	}
}
