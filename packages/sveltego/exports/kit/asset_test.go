package kit_test

import (
	"sync"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func TestAsset_FallbackWhenUnregistered(t *testing.T) {
	kit.RegisterAssets(nil)
	t.Cleanup(func() { kit.RegisterAssets(nil) })

	cases := map[string]string{
		"logo.png":         "/logo.png",
		"/logo.png":        "/logo.png",
		"img/banner.webp":  "/img/banner.webp",
		"/img/banner.webp": "/img/banner.webp",
	}
	for in, want := range cases {
		if got := kit.Asset(in); got != want {
			t.Fatalf("Asset(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAsset_FallbackEmptyInput(t *testing.T) {
	kit.RegisterAssets(nil)
	t.Cleanup(func() { kit.RegisterAssets(nil) })

	if got := kit.Asset(""); got != "/" {
		t.Fatalf("Asset(empty) = %q, want %q", got, "/")
	}
}

func TestAsset_RegisteredLookup(t *testing.T) {
	kit.RegisterAssets(map[string]string{
		"logo.png":        "/_app/immutable/assets/logo.abc12345.png",
		"img/banner.webp": "/_app/immutable/assets/banner.deadbeef.webp",
	})
	t.Cleanup(func() { kit.RegisterAssets(nil) })

	cases := map[string]string{
		"logo.png":         "/_app/immutable/assets/logo.abc12345.png",
		"/logo.png":        "/_app/immutable/assets/logo.abc12345.png",
		"img/banner.webp":  "/_app/immutable/assets/banner.deadbeef.webp",
		"/img/banner.webp": "/_app/immutable/assets/banner.deadbeef.webp",
		"missing.svg":      "/missing.svg",
	}
	for in, want := range cases {
		if got := kit.Asset(in); got != want {
			t.Fatalf("Asset(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegisterAssets_DefensiveCopy(t *testing.T) {
	src := map[string]string{"logo.png": "/_app/immutable/assets/logo.abc12345.png"}
	kit.RegisterAssets(src)
	t.Cleanup(func() { kit.RegisterAssets(nil) })

	// Mutate the caller's map after registration; runtime lookup must be
	// unaffected because RegisterAssets shallow-copies.
	src["logo.png"] = "/wrong"
	delete(src, "logo.png")

	if got := kit.Asset("logo.png"); got != "/_app/immutable/assets/logo.abc12345.png" {
		t.Fatalf("registration is not isolated from caller mutation: got %q", got)
	}
}

func TestAsset_ConcurrentReads(t *testing.T) {
	kit.RegisterAssets(map[string]string{
		"logo.png": "/_app/immutable/assets/logo.abc12345.png",
	})
	t.Cleanup(func() { kit.RegisterAssets(nil) })

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if got := kit.Asset("logo.png"); got != "/_app/immutable/assets/logo.abc12345.png" {
				t.Errorf("concurrent Asset = %q", got)
			}
		}()
	}
	wg.Wait()
}
