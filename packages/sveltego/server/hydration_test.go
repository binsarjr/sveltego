package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestRenderPage_payloadScriptTag asserts that SSR HTML contains a
// <script id="sveltego-data" type="application/json"> tag with the
// correct routeId and data fields (#35).
func TestRenderPage_payloadScriptTag(t *testing.T) {
	t.Parallel()

	type pageData struct {
		Title string `json:"title"`
	}

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/blog",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "blog"}},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<h1>blog</h1>")
			return nil
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return pageData{Title: "Hello"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/blog")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	bs := string(body)
	if !strings.Contains(bs, `<script id="sveltego-data" type="application/json">`) {
		t.Fatalf("body missing payload script tag; got:\n%s", bs)
	}

	// Extract payload JSON.
	start := strings.Index(bs, `<script id="sveltego-data" type="application/json">`)
	start += len(`<script id="sveltego-data" type="application/json">`)
	end := strings.Index(bs[start:], `</script>`)
	if end < 0 {
		t.Fatalf("payload script tag not closed")
	}
	raw := bs[start : start+end]

	var payload struct {
		RouteID string          `json:"routeId"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse payload: %v; raw=%s", err, raw)
	}
	if payload.RouteID != "/blog" {
		t.Errorf("routeId = %q, want /blog", payload.RouteID)
	}
	var pd pageData
	if err := json.Unmarshal(payload.Data, &pd); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if pd.Title != "Hello" {
		t.Errorf("data.title = %q, want Hello", pd.Title)
	}
}

// TestRenderPage_payloadIncludesManifest asserts that the SSR payload
// carries the SPA route manifest so the client router can match link
// URLs without a separate manifest fetch (#37).
func TestRenderPage_payloadIncludesManifest(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page:     staticPage("home"),
		},
		{
			Pattern: "/post/[id]",
			Segments: []router.Segment{
				{Kind: router.SegmentStatic, Value: "post"},
				{Kind: router.SegmentParam, Name: "id"},
			},
			Page: staticPage("post"),
		},
		{
			Pattern: "/docs/[...rest]",
			Segments: []router.Segment{
				{Kind: router.SegmentStatic, Value: "docs"},
				{Kind: router.SegmentRest, Name: "rest"},
			},
			Page: staticPage("docs"),
		},
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	bs := string(body)
	start := strings.Index(bs, `<script id="sveltego-data" type="application/json">`)
	if start < 0 {
		t.Fatalf("payload tag not found")
	}
	start += len(`<script id="sveltego-data" type="application/json">`)
	end := strings.Index(bs[start:], `</script>`)
	raw := bs[start : start+end]

	var payload struct {
		Manifest []struct {
			Pattern  string `json:"pattern"`
			Segments []struct {
				Kind  uint8  `json:"kind"`
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"segments"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse: %v; raw=%s", err, raw)
	}
	if len(payload.Manifest) != 3 {
		t.Fatalf("manifest entries = %d, want 3; raw=%s", len(payload.Manifest), raw)
	}
	patterns := map[string]bool{}
	for _, e := range payload.Manifest {
		patterns[e.Pattern] = true
	}
	for _, want := range []string{"/", "/post/[id]", "/docs/[...rest]"} {
		if !patterns[want] {
			t.Errorf("manifest missing pattern %q; got %+v", want, patterns)
		}
	}
}

// TestDataJSON_carriesDeps asserts the client payload ships the dep
// tags Load declared via ctx.Depends, so $app/navigation.invalidate can
// match them on the client (#85).
func TestDataJSON_carriesDeps(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/posts",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "posts"}},
		Page:     staticPage("posts"),
		Load: func(ctx *kit.LoadCtx) (any, error) {
			ctx.Depends("posts:list", "feed")
			return map[string]string{"k": "v"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/posts/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var payload struct {
		Deps []string `json:"deps"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse: %v; body=%s", err, body)
	}
	want := []string{"posts:list", "feed"}
	if len(payload.Deps) != len(want) {
		t.Fatalf("deps = %v, want %v", payload.Deps, want)
	}
	for i, w := range want {
		if payload.Deps[i] != w {
			t.Errorf("deps[%d] = %q, want %q", i, payload.Deps[i], w)
		}
	}
}

// TestDataJSON_omitsDepsWhenNone asserts the deps field is absent when
// Load did not call Depends, so the wire stays small for the common case.
func TestDataJSON_omitsDepsWhenNone(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/silent",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "silent"}},
		Page:     staticPage("silent"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return map[string]string{"k": "v"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/silent/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if strings.Contains(string(body), `"deps"`) {
		t.Errorf("payload should omit deps when none declared; got: %s", body)
	}
}

// TestDataJSON_omitsManifest asserts that __data.json responses do NOT
// include the manifest (the client already has it from the initial paint).
func TestDataJSON_omitsManifest(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/blog",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "blog"}},
		Page:     staticPage("blog"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return map[string]string{"k": "v"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/blog/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if strings.Contains(string(body), `"manifest"`) {
		t.Errorf("__data.json should not include manifest; got: %s", body)
	}
}

// TestBuildClientManifest_skipsServerOnly verifies routes without a Page
// handler (e.g. pure _server.go endpoints) are excluded from the SPA
// manifest, since the client cannot mount a component for them.
func TestBuildClientManifest_skipsServerOnly(t *testing.T) {
	t.Parallel()

	routes := []router.Route{
		{Pattern: "/", Segments: []router.Segment{}, Page: staticPage("home")},
		{
			Pattern:  "/api",
			Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "api"}},
			Server:   router.ServerHandlers{},
		},
	}
	m := buildClientManifest(routes)
	if len(m) != 1 {
		t.Fatalf("entries = %d, want 1 (server-only route should be skipped)", len(m))
	}
	if m[0].Pattern != "/" {
		t.Errorf("pattern = %q, want /", m[0].Pattern)
	}
}

// TestRenderPage_payloadJSONEscape asserts that script-breaking sequences
// in data values are escaped so "</script>" can't break out of the tag (#35).
func TestRenderPage_payloadJSONEscape(t *testing.T) {
	t.Parallel()

	type pageData struct {
		Evil string `json:"evil"`
	}

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/evil",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "evil"}},
		Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
			return nil
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return pageData{Evil: "</script><script>alert(1)</script>"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/evil")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	bs := string(body)
	// The literal "</script>" must not appear inside the payload script tag.
	// Find the payload tag.
	start := strings.Index(bs, `<script id="sveltego-data"`)
	if start < 0 {
		t.Fatalf("no payload tag found")
	}
	end := strings.Index(bs[start+len(`<script id="sveltego-data">`):], `</script>`)
	tagContent := bs[start : start+end+len(`<script id="sveltego-data">`)]
	if strings.Contains(tagContent, "</script>") {
		t.Errorf("payload contains raw </script>; XSS escape not applied:\n%s", tagContent)
	}
}

// TestDataJSON_basicRoute asserts that GET /<route>/__data.json returns the
// client payload as JSON with the correct Content-Type and X-Sveltego-Data
// header (#38).
func TestDataJSON_basicRoute(t *testing.T) {
	t.Parallel()

	type pageData struct {
		Count int `json:"count"`
	}

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/counter",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "counter"}},
		Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
			return nil
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return pageData{Count: 42}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/counter/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if resp.Header.Get("X-Sveltego-Data") != "1" {
		t.Errorf("X-Sveltego-Data header missing")
	}

	var payload struct {
		RouteID string          `json:"routeId"`
		Data    json.RawMessage `json:"data"`
		URL     string          `json:"url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse response: %v; body=%s", err, body)
	}
	if payload.RouteID != "/counter" {
		t.Errorf("routeId = %q, want /counter", payload.RouteID)
	}
	if !strings.Contains(payload.URL, "/counter/__data.json") {
		t.Errorf("url = %q, want to contain /counter/__data.json", payload.URL)
	}
	var pd pageData
	if err := json.Unmarshal(payload.Data, &pd); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if pd.Count != 42 {
		t.Errorf("data.count = %d, want 42", pd.Count)
	}
}

// TestDataJSON_parameterizedRoute verifies that route params are populated
// correctly for __data.json fetches (#38).
func TestDataJSON_parameterizedRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern: "/item/[id]",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "item"},
			{Kind: router.SegmentParam, Name: "id"},
		},
		Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
			return nil
		},
		Load: func(ctx *kit.LoadCtx) (any, error) {
			return map[string]string{"id": ctx.Params["id"]}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/item/99/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse: %v", err)
	}
	var pd map[string]string
	if err := json.Unmarshal(payload.Data, &pd); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if pd["id"] != "99" {
		t.Errorf("data.id = %q, want 99", pd["id"])
	}
}

// TestDataJSON_layoutChain verifies that layout loader data appears in
// layoutData and page data appears in data (#38).
func TestDataJSON_layoutChain(t *testing.T) {
	t.Parallel()

	type rootData struct {
		Root string `json:"root"`
	}
	type leafData struct {
		Page string `json:"page"`
	}

	identityLayout := func(w *render.Writer, _ *kit.RenderCtx, _ any, children func(*render.Writer) error) error {
		return children(w)
	}

	srv := newTestServer(t, []router.Route{{
		Pattern:     "/chain",
		Segments:    []router.Segment{{Kind: router.SegmentStatic, Value: "chain"}},
		Page:        staticPage(""),
		RenderChain: composeChain([]router.LayoutHandler{identityLayout}),
		LayoutLoaders: []router.LayoutLoadHandler{
			func(_ *kit.LoadCtx) (any, error) {
				return rootData{Root: "rootval"}, nil
			},
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return leafData{Page: "pageval"}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/chain/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	var payload struct {
		Data       json.RawMessage   `json:"data"`
		LayoutData []json.RawMessage `json:"layoutData"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(payload.LayoutData) != 1 {
		t.Fatalf("layoutData len = %d, want 1; body=%s", len(payload.LayoutData), body)
	}
	var rd rootData
	if err := json.Unmarshal(payload.LayoutData[0], &rd); err != nil {
		t.Fatalf("parse layoutData[0]: %v", err)
	}
	if rd.Root != "rootval" {
		t.Errorf("layoutData[0].root = %q, want rootval", rd.Root)
	}
	var ld leafData
	if err := json.Unmarshal(payload.Data, &ld); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if ld.Page != "pageval" {
		t.Errorf("data.page = %q, want pageval", ld.Page)
	}
}

// TestDataJSON_ssrOnlyRejects asserts that SSROnly routes return 404 for
// __data.json requests (#38).
func TestDataJSON_ssrOnlyRejects(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/private",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "private"}},
		Page:     staticPage("<h1>secret</h1>"),
		Options:  kit.PageOptions{SSR: true, CSR: true, SSROnly: true},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/private/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("SSROnly __data.json status = %d, want 404", resp.StatusCode)
	}
}

// TestDataJSON_postMethodNotAllowed verifies 405 for POST to __data.json (#38).
func TestDataJSON_postMethodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/page",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "page"}},
		Page:     staticPage("<h1>page</h1>"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/page/__data.json", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST __data.json status = %d, want 405", resp.StatusCode)
	}
}

// streamingHydrationData is a Load result with a Streamed field, exercising
// the streaming render path for hydration-payload tests.
type streamingHydrationData struct {
	Title string                  `json:"title"`
	Posts *kit.Streamed[[]string] `json:"-"`
}

// TestStreaming_emitsHydrationPayload asserts that the streaming render
// path emits the <script id="sveltego-data"> hydration tag with the
// correct routeId, data, and Deps fields. Without this tag the client
// hydrates with an empty payload and the page boots with no state.
func TestStreaming_emitsHydrationPayload(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/streamed",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "streamed"}},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString(`<h1>shell</h1>`)
			return nil
		},
		Load: func(lctx *kit.LoadCtx) (any, error) {
			lctx.Depends("posts:list")
			return streamingHydrationData{
				Title: "Hello",
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					return []string{"a", "b"}, nil
				}),
			}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/streamed")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	bs := string(body)
	if !strings.Contains(bs, `<script id="sveltego-data" type="application/json">`) {
		t.Fatalf("streaming body missing hydration payload tag; got:\n%s", bs)
	}
	if !strings.Contains(bs, `__sveltego__resolve(`) {
		t.Fatalf("streaming body missing resolve script; got:\n%s", bs)
	}

	start := strings.Index(bs, `<script id="sveltego-data" type="application/json">`)
	start += len(`<script id="sveltego-data" type="application/json">`)
	end := strings.Index(bs[start:], `</script>`)
	if end < 0 {
		t.Fatalf("payload tag not closed")
	}
	raw := bs[start : start+end]

	var payload struct {
		RouteID string          `json:"routeId"`
		Data    json.RawMessage `json:"data"`
		Deps    []string        `json:"deps"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse payload: %v; raw=%s", err, raw)
	}
	if payload.RouteID != "/streamed" {
		t.Errorf("routeId = %q, want /streamed", payload.RouteID)
	}
	if len(payload.Deps) != 1 || payload.Deps[0] != "posts:list" {
		t.Errorf("deps = %v, want [posts:list]", payload.Deps)
	}
	var pd streamingHydrationData
	if err := json.Unmarshal(payload.Data, &pd); err != nil {
		t.Fatalf("parse data: %v; raw=%s", err, payload.Data)
	}
	if pd.Title != "Hello" {
		t.Errorf("data.title = %q, want Hello", pd.Title)
	}

	// Payload tag must precede the resolve script so the client has state
	// before any out-of-order patches arrive.
	payloadIdx := strings.Index(bs, `<script id="sveltego-data"`)
	resolveIdx := strings.Index(bs, `__sveltego__resolve(`)
	if payloadIdx < 0 || resolveIdx < 0 || payloadIdx > resolveIdx {
		t.Errorf("payload tag must come before resolve script; payload=%d resolve=%d", payloadIdx, resolveIdx)
	}
}

// TestStreaming_emitsViteAssetTags asserts that the streaming render path
// injects the Vite manifest's <link rel="stylesheet"> and
// <script type="module"> tags into the early-flush prefix. Without these,
// streaming routes load the page body but never pull in the client SPA
// bundle or compiled CSS, so hydration silently no-ops and Tailwind styles
// never apply.
func TestStreaming_emitsViteAssetTags(t *testing.T) {
	t.Parallel()

	const manifest = `{
		"src/routes/streamed/_page.svelte": {
			"file": "_app/streamed-abc.js",
			"css": ["_app/streamed-xyz.css"],
			"imports": ["_shared"],
			"isEntry": true
		},
		"_shared": {
			"file": "_app/shared-def.js"
		}
	}`

	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:   "/streamed",
			Segments:  []router.Segment{{Kind: router.SegmentStatic, Value: "streamed"}},
			ClientKey: "src/routes/streamed/_page.svelte",
			Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString(`<h1>shell</h1>`)
				return nil
			},
			Load: func(lctx *kit.LoadCtx) (any, error) {
				return streamingHydrationData{
					Title: "Hello",
					Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
						return []string{"a", "b"}, nil
					}),
				}, nil
			},
		}},
		Shell:        testShell,
		Logger:       quietLogger(),
		ViteManifest: manifest,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/streamed")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	bs := string(body)

	const cssTag = `<link rel="stylesheet" href="/static/_app/_app/streamed-xyz.css">`
	const jsTag = `<script type="module" src="/static/_app/_app/streamed-abc.js">`
	const preloadTag = `<link rel="modulepreload" href="/static/_app/_app/shared-def.js">`

	if !strings.Contains(bs, cssTag) {
		t.Errorf("body missing CSS link tag %q; got:\n%s", cssTag, bs)
	}
	if !strings.Contains(bs, jsTag) {
		t.Errorf("body missing module script tag %q; got:\n%s", jsTag, bs)
	}
	if !strings.Contains(bs, preloadTag) {
		t.Errorf("body missing modulepreload tag for imported chunk %q; got:\n%s", preloadTag, bs)
	}

	// CSS link must precede page content (FOUC prevention) and the
	// content must precede the resolve script (out-of-order patches
	// arrive after first flush).
	contentIdx := strings.Index(bs, `<h1>shell</h1>`)
	cssIdx := strings.Index(bs, cssTag)
	jsIdx := strings.Index(bs, jsTag)
	resolveIdx := strings.Index(bs, `__sveltego__resolve(`)
	if cssIdx < 0 || contentIdx < 0 || jsIdx < 0 || resolveIdx < 0 {
		t.Fatalf("missing markers; css=%d content=%d js=%d resolve=%d", cssIdx, contentIdx, jsIdx, resolveIdx)
	}
	if cssIdx > contentIdx {
		t.Errorf("CSS link must precede page content; css=%d content=%d", cssIdx, contentIdx)
	}
	if jsIdx > resolveIdx {
		t.Errorf("module script must precede resolve patches; js=%d resolve=%d", jsIdx, resolveIdx)
	}
}

// TestRenderPage_payloadCarriesAppState pins the wire shape of the
// $app/state surface (#312): every SSR payload must populate routeId,
// url, params, status, error, data, and form so the client-side `page`
// rune reads valid values on first paint without a follow-up round-trip.
func TestRenderPage_payloadCarriesAppState(t *testing.T) {
	t.Parallel()

	type pageData struct {
		Title string `json:"title"`
	}

	srv := newTestServer(t, []router.Route{{
		Pattern: "/post/[id]",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "post"},
			{Kind: router.SegmentParam, Name: "id"},
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<article>post</article>")
			return nil
		},
		Load: func(ctx *kit.LoadCtx) (any, error) {
			return pageData{Title: "post-" + ctx.Params["id"]}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/post/42?ref=feed")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	bs := string(body)
	start := strings.Index(bs, `<script id="sveltego-data" type="application/json">`)
	if start < 0 {
		t.Fatalf("payload script tag missing; body=%s", bs)
	}
	start += len(`<script id="sveltego-data" type="application/json">`)
	end := strings.Index(bs[start:], `</script>`)
	raw := bs[start : start+end]

	var payload struct {
		RouteID string                    `json:"routeId"`
		URL     string                    `json:"url"`
		Params  map[string]string         `json:"params"`
		Status  int                       `json:"status"`
		Error   *struct{ Message string } `json:"error"`
		Form    json.RawMessage           `json:"form"`
		Data    json.RawMessage           `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse payload: %v; raw=%s", err, raw)
	}
	if payload.RouteID != "/post/[id]" {
		t.Errorf("routeId = %q, want /post/[id]", payload.RouteID)
	}
	if !strings.HasSuffix(payload.URL, "/post/42?ref=feed") {
		t.Errorf("url = %q, want path /post/42?ref=feed", payload.URL)
	}
	if payload.Params["id"] != "42" {
		t.Errorf("params[id] = %q, want 42", payload.Params["id"])
	}
	if payload.Status != 200 {
		t.Errorf("status = %d, want 200", payload.Status)
	}
	if payload.Error != nil {
		t.Errorf("error = %+v, want nil on success path", payload.Error)
	}
	if string(payload.Form) != "null" {
		t.Errorf("form = %s, want null on GET", payload.Form)
	}
}

// TestDataJSON_carriesAppState mirrors TestRenderPage_payloadCarriesAppState
// for the __data.json endpoint so SPA-router refetches see the same nine
// fields as the initial SSR render.
func TestDataJSON_carriesAppState(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern: "/item/[id]",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "item"},
			{Kind: router.SegmentParam, Name: "id"},
		},
		Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error { return nil },
		Load: func(ctx *kit.LoadCtx) (any, error) {
			return map[string]string{"id": ctx.Params["id"]}, nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/item/7/__data.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var payload struct {
		RouteID string            `json:"routeId"`
		Params  map[string]string `json:"params"`
		Status  int               `json:"status"`
		Error   any               `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse: %v; body=%s", err, body)
	}
	if payload.RouteID != "/item/[id]" {
		t.Errorf("routeId = %q, want /item/[id]", payload.RouteID)
	}
	if payload.Params["id"] != "7" {
		t.Errorf("params[id] = %q, want 7", payload.Params["id"])
	}
	if payload.Status != 200 {
		t.Errorf("status = %d, want 200", payload.Status)
	}
	if payload.Error != nil {
		t.Errorf("error = %v, want nil", payload.Error)
	}
}
