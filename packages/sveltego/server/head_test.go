package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func TestDedupeTitle_KeepsLast(t *testing.T) {
	t.Parallel()
	in := []byte(`<meta charset="utf-8"><title>A</title><title>B</title><title>C</title><meta name="x">`)
	got := string(dedupeTitle(in))
	if want := `<meta charset="utf-8"><title>C</title><meta name="x">`; got != want {
		t.Fatalf("dedupeTitle mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDedupeTitle_NoChangeWhenSingle(t *testing.T) {
	t.Parallel()
	in := []byte(`<title>Solo</title>`)
	got := dedupeTitle(in)
	if !bytes.Equal(got, in) {
		t.Fatalf("dedupeTitle mutated single title: %q", got)
	}
}

func TestDedupeTitle_MixedCase(t *testing.T) {
	t.Parallel()
	in := []byte(`<TITLE>A</TITLE><Title>B</Title>`)
	got := string(dedupeTitle(in))
	if !strings.Contains(got, "<Title>B</Title>") {
		t.Fatalf("expected last (mixed-case) title preserved, got %q", got)
	}
	if strings.Contains(got, "<TITLE>A</TITLE>") {
		t.Fatalf("expected first (mixed-case) title dropped, got %q", got)
	}
}

func TestDedupeTitle_DoesNotMatchTitleBar(t *testing.T) {
	t.Parallel()
	in := []byte(`<titlebar>x</titlebar><title>real</title>`)
	got := string(dedupeTitle(in))
	if got != string(in) {
		t.Fatalf("expected <titlebar> ignored, got %q", got)
	}
}

func TestGatherHead_NilRoute(t *testing.T) {
	t.Parallel()
	got, err := gatherHead(&kit.RenderCtx{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil head, got %q", got)
	}
}

func TestGatherHead_LayoutThenPage(t *testing.T) {
	t.Parallel()
	route := &router.Route{
		Head: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString(`<title>page</title>`)
			return nil
		},
		LayoutHeads: []router.LayoutHeadHandler{
			func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString(`<title>layout</title>`)
				return nil
			},
		},
	}
	got, err := gatherHead(&kit.RenderCtx{}, route, nil, []any{nil})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "<title>page</title>") {
		t.Fatalf("expected page title preserved, got %q", s)
	}
	if strings.Contains(s, "<title>layout</title>") {
		t.Fatalf("expected layout title deduped, got %q", s)
	}
}

// TestServer_SvelteHeadInjection is the integration assertion that the
// pipeline's head buffer ends up between <head> and </head> in the
// rendered HTML. A minimal Route with Head handlers is registered and
// the response body is grepped for the head injection.
func TestServer_SvelteHeadInjection(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString(`<main>hello</main>`)
			return nil
		},
		Head: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString(`<title>page</title>`)
			return nil
		},
		LayoutHeads: []router.LayoutHeadHandler{
			func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString(`<meta name="theme" content="dark">`)
				w.WriteString(`<title>layout</title>`)
				return nil
			},
		},
		LayoutChain: []router.LayoutHandler{
			func(w *render.Writer, _ *kit.RenderCtx, _ any, children func(*render.Writer) error) error {
				return children(w)
			},
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	s := readAll(t, resp)
	if !strings.Contains(s, `<meta name="theme" content="dark">`) {
		t.Errorf("missing layout meta in body:\n%s", s)
	}
	if !strings.Contains(s, `<title>page</title>`) {
		t.Errorf("expected page title, got:\n%s", s)
	}
	if strings.Contains(s, `<title>layout</title>`) {
		t.Errorf("layout title should have been deduped, got:\n%s", s)
	}
	headIdx := strings.Index(s, `<title>page</title>`)
	endHead := strings.Index(s, "</head>")
	if headIdx < 0 || endHead < 0 || headIdx > endHead {
		t.Errorf("page title must appear before </head>, got idx=%d endHead=%d body=%s", headIdx, endHead, s)
	}
}
