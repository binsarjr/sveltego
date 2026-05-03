package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestAssetTags_GlobalCSSInjection pins the contract that an addon-emitted
// global stylesheet (Tailwind / PostCSS via src/app.css) is injected as
// a <link rel="stylesheet"> alongside route-scoped CSS. Without this, a
// fresh `sveltego-init --tailwind=v4` scaffold would compile Tailwind to
// a hashed CSS chunk that the page never references.
func TestAssetTags_GlobalCSSInjection(t *testing.T) {
	t.Parallel()

	const manifest = `{
		".gen/client/routes/_page/entry.ts": {
			"file": "assets/_page-abc.js",
			"name": "routes/_page",
			"src": ".gen/client/routes/_page/entry.ts",
			"isEntry": true,
			"css": ["assets/_page-xyz.css"]
		},
		"src/app.css": {
			"file": "assets/app-def.css",
			"src": "src/app.css",
			"isEntry": true
		}
	}`

	m, err := parseViteManifest(manifest)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	headTags := m.headAssetTags(".gen/client/routes/_page/entry.ts", "/_app")
	bodyTag := m.bodyEntryTag(".gen/client/routes/_page/entry.ts", "/_app")

	for _, want := range []string{
		`<link rel="stylesheet" href="/_app/assets/app-def.css">`,
		`<link rel="stylesheet" href="/_app/assets/_page-xyz.css">`,
	} {
		if !strings.Contains(headTags, want) {
			t.Errorf("headAssetTags missing %q; got:\n%s", want, headTags)
		}
	}
	if !strings.Contains(bodyTag, `<script type="module" src="/_app/assets/_page-abc.js">`) {
		t.Errorf("bodyEntryTag missing module script; got:\n%s", bodyTag)
	}

	// Issue #521: entry <script> must NOT live in head fragment so the
	// browser paints body before any chunk executes.
	if strings.Contains(headTags, `<script type="module"`) {
		t.Errorf("headAssetTags must not include entry <script>; got:\n%s", headTags)
	}

	globalIdx := strings.Index(headTags, "app-def.css")
	scopedIdx := strings.Index(headTags, "_page-xyz.css")
	if globalIdx < 0 || scopedIdx < 0 || globalIdx > scopedIdx {
		t.Errorf("global CSS must precede scoped CSS; global=%d scoped=%d\n%s", globalIdx, scopedIdx, headTags)
	}
}

// TestAssetTags_NoGlobalCSSWhenAbsent pins the no-Tailwind case: when
// the manifest has no `src/app.css` entry, no extra stylesheet tag is
// injected. Guards against double-link regressions.
func TestAssetTags_NoGlobalCSSWhenAbsent(t *testing.T) {
	t.Parallel()

	const manifest = `{
		".gen/client/routes/_page/entry.ts": {
			"file": "assets/_page-abc.js",
			"name": "routes/_page",
			"src": ".gen/client/routes/_page/entry.ts",
			"isEntry": true
		}
	}`

	m, err := parseViteManifest(manifest)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	headTags := m.headAssetTags(".gen/client/routes/_page/entry.ts", "/_app")
	bodyTag := m.bodyEntryTag(".gen/client/routes/_page/entry.ts", "/_app")

	if strings.Contains(headTags, "app-") {
		t.Errorf("unexpected global CSS tag in no-addon build:\n%s", headTags)
	}
	if !strings.Contains(bodyTag, `<script type="module" src="/_app/assets/_page-abc.js">`) {
		t.Errorf("missing module script tag in body fragment:\n%s", bodyTag)
	}
	if strings.Contains(headTags, `<script type="module"`) {
		t.Errorf("entry script must not appear in head fragment; got:\n%s", headTags)
	}
}

// TestRenderPage_entryScriptAtBodyEnd pins the issue #521 contract end-to-end:
// the per-route entry <script type="module"> lands at end of <body>, not in
// <head>. Modulepreload hints stay in <head> for parallel discovery, and the
// inline JSON payload sits just before the entry script so the entry can
// JSON.parse(document.getElementById('sveltego-data').textContent).
func TestRenderPage_entryScriptAtBodyEnd(t *testing.T) {
	t.Parallel()

	const manifest = `{
		"src/routes/_page.svelte": {
			"file": "_app/page-abc.js",
			"css": ["_app/page-xyz.css"],
			"imports": ["_shared"],
			"isEntry": true
		},
		"_shared": {
			"file": "_app/shared-def.js"
		}
	}`

	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:   "/",
			Segments:  []router.Segment{},
			ClientKey: "src/routes/_page.svelte",
			Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString(`<main><h1>SSR'd content</h1></main>`)
				return nil
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

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	bs := string(body)

	headEnd := strings.Index(bs, `</head>`)
	bodyOpen := strings.Index(bs, `<body>`)
	bodyClose := strings.Index(bs, `</body>`)
	if headEnd < 0 || bodyOpen < 0 || bodyClose < 0 {
		t.Fatalf("missing structural markers; head=%d body=%d /body=%d in:\n%s", headEnd, bodyOpen, bodyClose, bs)
	}

	const entryScript = `<script type="module" src="/static/_app/_app/page-abc.js">`
	const preloadHint = `<link rel="modulepreload" href="/static/_app/_app/shared-def.js">`
	const cssHint = `<link rel="stylesheet" href="/static/_app/_app/page-xyz.css">`
	const payloadTag = `<script id="sveltego-data" type="application/json">`
	const renderedContent = `<main><h1>SSR'd content</h1></main>`

	entryIdx := strings.Index(bs, entryScript)
	preloadIdx := strings.Index(bs, preloadHint)
	cssIdx := strings.Index(bs, cssHint)
	payloadIdx := strings.Index(bs, payloadTag)
	contentIdx := strings.Index(bs, renderedContent)

	if entryIdx < 0 {
		t.Fatalf("entry script %q not found:\n%s", entryScript, bs)
	}
	if preloadIdx < 0 {
		t.Fatalf("modulepreload hint %q not found:\n%s", preloadHint, bs)
	}
	if cssIdx < 0 {
		t.Fatalf("css link %q not found:\n%s", cssHint, bs)
	}
	if payloadIdx < 0 {
		t.Fatalf("payload tag %q not found:\n%s", payloadTag, bs)
	}
	if contentIdx < 0 {
		t.Fatalf("rendered content %q not found:\n%s", renderedContent, bs)
	}

	// Issue #521 acceptance criteria:
	// 1. Entry <script type="module"> lands in <body>, not <head>.
	if entryIdx < headEnd {
		t.Errorf("entry script must be in body, not head; entry=%d </head>=%d", entryIdx, headEnd)
	}
	if entryIdx > bodyClose {
		t.Errorf("entry script must precede </body>; entry=%d </body>=%d", entryIdx, bodyClose)
	}

	// 2. <link rel="modulepreload"> hints stay in <head>.
	if preloadIdx > headEnd {
		t.Errorf("modulepreload hint must stay in head; preload=%d </head>=%d", preloadIdx, headEnd)
	}
	// Stylesheet links also belong in <head>.
	if cssIdx > headEnd {
		t.Errorf("css link must stay in head; css=%d </head>=%d", cssIdx, headEnd)
	}

	// 3. Payload tag sits just before the entry script (so JSON.parse works).
	if payloadIdx > entryIdx {
		t.Errorf("payload tag must precede entry script; payload=%d entry=%d", payloadIdx, entryIdx)
	}
	if payloadIdx < contentIdx {
		t.Errorf("payload tag must come AFTER SSR'd content; payload=%d content=%d", payloadIdx, contentIdx)
	}
	// 4. SSR'd content comes before payload + entry script.
	if contentIdx > entryIdx {
		t.Errorf("SSR'd content must precede entry script; content=%d entry=%d", contentIdx, entryIdx)
	}
}
