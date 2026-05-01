package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
)

func TestEmitImage_BasicMarkup(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="hero.jpg" alt="Logo" width="800" height="600" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"hero.jpg": {
				Source:          "hero.jpg",
				IntrinsicWidth:  1600,
				IntrinsicHeight: 1200,
				Variants: []images.Variant{
					{Width: 320, Height: 240, URL: "/_app/immutable/assets/hero.deadbeef.320.jpg"},
					{Width: 640, Height: 480, URL: "/_app/immutable/assets/hero.deadbeef.640.jpg"},
					{Width: 1280, Height: 960, URL: "/_app/immutable/assets/hero.deadbeef.1280.jpg"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`src="/_app/immutable/assets/hero.deadbeef.1280.jpg"`,
		`srcset="/_app/immutable/assets/hero.deadbeef.320.jpg 320w, /_app/immutable/assets/hero.deadbeef.640.jpg 640w, /_app/immutable/assets/hero.deadbeef.1280.jpg 1280w"`,
		`width="800"`,
		`height="600"`,
		`alt="Logo"`,
		`loading="lazy"`,
		`decoding="async"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestEmitImage_EagerOptIn(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="hero.jpg" alt="" eager />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"hero.jpg": {
				Source:          "hero.jpg",
				IntrinsicWidth:  800,
				IntrinsicHeight: 600,
				Variants: []images.Variant{
					{Width: 800, Height: 600, URL: "/_app/immutable/assets/hero.deadbeef.800.jpg"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `loading="eager"`) {
		t.Errorf("expected loading=eager, got:\n%s", s)
	}
	if strings.Contains(s, `loading="lazy"`) {
		t.Errorf("expected no loading=lazy, got:\n%s", s)
	}
}

func TestEmitImage_IntrinsicDimsFallback(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="hero.jpg" alt="x" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"hero.jpg": {
				IntrinsicWidth:  1024,
				IntrinsicHeight: 768,
				Variants: []images.Variant{
					{Width: 1024, Height: 768, URL: "/_app/immutable/assets/hero.deadbeef.1024.jpg"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `width="1024"`) {
		t.Errorf("expected intrinsic width=1024, got:\n%s", s)
	}
	if !strings.Contains(s, `height="768"`) {
		t.Errorf("expected intrinsic height=768, got:\n%s", s)
	}
}

func TestEmitImage_SingleVariantNoSrcset(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="logo.png" alt="" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"logo.png": {
				IntrinsicWidth:  100,
				IntrinsicHeight: 100,
				Variants: []images.Variant{
					{Width: 100, Height: 100, URL: "/_app/immutable/assets/logo.cafe.100.png"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if strings.Contains(s, `srcset=`) {
		t.Errorf("expected no srcset for single variant, got:\n%s", s)
	}
}

func TestEmitImage_DynamicAlt(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="hero.jpg" alt={data.Title} />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"hero.jpg": {
				IntrinsicWidth:  800,
				IntrinsicHeight: 600,
				Variants: []images.Variant{
					{Width: 800, Height: 600, URL: "/_app/immutable/assets/hero.deadbeef.800.jpg"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `w.WriteEscapeAttr(data.Title)`) {
		t.Errorf("expected dynamic alt emission, got:\n%s", s)
	}
}

func TestEmitImage_UnknownSourceFails(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src="missing.jpg" alt="" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	_, err := Generate(frag, Options{
		PackageName:   "page",
		ImageVariants: map[string]images.Result{},
	})
	if err == nil {
		t.Fatal("expected error for unknown <Image> src, got nil")
	}
	if !strings.Contains(err.Error(), "missing.jpg") {
		t.Errorf("error should mention source, got: %v", err)
	}
}

func TestEmitImage_DynamicSrcRejected(t *testing.T) {
	t.Parallel()
	src := []byte(`<Image src={data.Hero} alt="" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected error for dynamic src, got nil")
	}
	if !strings.Contains(err.Error(), "dynamic src") {
		t.Errorf("error should mention dynamic src, got: %v", err)
	}
}

func TestCollectImageSources(t *testing.T) {
	t.Parallel()
	src := []byte(`
<Image src="a.jpg" />
<div>
  {#if Cond}
    <Image src="b.png" />
  {:else}
    <Image src="c.jpg" />
  {/if}
</div>
{#each List as item}
  <Image src="d.jpg" />
{/each}
`)
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	got := collectImageSources(frag)
	want := []string{"a.jpg", "b.png", "c.jpg", "d.jpg"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEmitImage_LeadingSlashNormalized(t *testing.T) {
	t.Parallel()
	// User can write src="/hero.jpg" or src="hero.jpg"; both must resolve.
	src := []byte(`<Image src="/hero.jpg" alt="" />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		ImageVariants: map[string]images.Result{
			"hero.jpg": {
				IntrinsicWidth:  100,
				IntrinsicHeight: 100,
				Variants: []images.Variant{
					{Width: 100, Height: 100, URL: "/_app/immutable/assets/hero.x.100.jpg"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(string(out), "hero.x.100.jpg") {
		t.Errorf("expected hashed URL, got:\n%s", out)
	}
}
