package server

import (
	"strings"
	"testing"
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

	tags := m.assetTags(".gen/client/routes/_page/entry.ts", "/_app")

	for _, want := range []string{
		`<link rel="stylesheet" href="/_app/assets/app-def.css">`,
		`<link rel="stylesheet" href="/_app/assets/_page-xyz.css">`,
		`<script type="module" src="/_app/assets/_page-abc.js">`,
	} {
		if !strings.Contains(tags, want) {
			t.Errorf("assetTags missing %q; got:\n%s", want, tags)
		}
	}

	globalIdx := strings.Index(tags, "app-def.css")
	scopedIdx := strings.Index(tags, "_page-xyz.css")
	if globalIdx < 0 || scopedIdx < 0 || globalIdx > scopedIdx {
		t.Errorf("global CSS must precede scoped CSS; global=%d scoped=%d\n%s", globalIdx, scopedIdx, tags)
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

	tags := m.assetTags(".gen/client/routes/_page/entry.ts", "/_app")

	if strings.Contains(tags, "app-") {
		t.Errorf("unexpected global CSS tag in no-addon build:\n%s", tags)
	}
	if !strings.Contains(tags, `<script type="module" src="/_app/assets/_page-abc.js">`) {
		t.Errorf("missing module script tag:\n%s", tags)
	}
}
