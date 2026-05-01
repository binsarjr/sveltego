package kit

import "testing"

func TestPageOptions_DefaultsAndMerge(t *testing.T) {
	t.Parallel()
	base := DefaultPageOptions()
	if !base.SSR || !base.CSR || base.Prerender {
		t.Fatalf("defaults: %+v", base)
	}
	if base.TrailingSlash != TrailingSlashNever {
		t.Fatalf("trailing-slash default = %v, want never", base.TrailingSlash)
	}
	if base.Templates != TemplatesGoMustache {
		t.Fatalf("templates default = %q, want %q", base.Templates, TemplatesGoMustache)
	}

	out := base.Merge(PageOptionsOverride{HasSSR: true, SSR: false})
	if out.SSR {
		t.Fatalf("SSR override ignored: %+v", out)
	}
	if out.CSR != true {
		t.Fatalf("CSR overwritten without flag: %+v", out)
	}

	out = base.Merge(PageOptionsOverride{HasTrailingSlash: true, TrailingSlash: TrailingSlashAlways})
	if out.TrailingSlash != TrailingSlashAlways {
		t.Fatalf("trailing-slash override missed: %+v", out)
	}

	out = base.Merge(PageOptionsOverride{HasTemplates: true, Templates: TemplatesSvelte})
	if out.Templates != TemplatesSvelte {
		t.Fatalf("templates override missed: %+v", out)
	}

	if (PageOptionsOverride{}).Any() {
		t.Fatal("empty override reports Any()")
	}
	if !(PageOptionsOverride{HasPrerender: true}).Any() {
		t.Fatal("Prerender override missed by Any()")
	}
	if !(PageOptionsOverride{HasTemplates: true}).Any() {
		t.Fatal("Templates override missed by Any()")
	}
}

func TestTrailingSlash_String(t *testing.T) {
	t.Parallel()
	cases := map[TrailingSlash]string{
		TrailingSlashDefault: "default",
		TrailingSlashNever:   "never",
		TrailingSlashAlways:  "always",
		TrailingSlashIgnore:  "ignore",
	}
	for v, want := range cases {
		if got := v.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", v, got, want)
		}
	}
}
