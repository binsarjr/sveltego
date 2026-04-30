package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
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
// handler (e.g. pure +server.go endpoints) are excluded from the SPA
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
		LayoutChain: []router.LayoutHandler{identityLayout},
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
