package codegen

import (
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

func TestResolvePageOptions_cascade(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(filepath.Join("testdata", "page-options"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(abs, "routes")})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got, err := resolvePageOptions(scan)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	rootDefaults := kit.PageOptions{SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashAlways, Templates: kit.TemplatesSvelte}
	if !got["/"].Equal(rootDefaults) {
		t.Errorf("/ effective options = %+v, want %+v", got["/"], rootDefaults)
	}

	billing := kit.PageOptions{
		Prerender:     true,
		SSR:           true,
		CSR:           true,
		SSROnly:       true,
		CSRF:          true,
		TrailingSlash: kit.TrailingSlashIgnore,
		Templates:     kit.TemplatesSvelte,
	}
	if !got["/dash/billing"].Equal(billing) {
		t.Errorf("/dash/billing effective options = %+v, want %+v", got["/dash/billing"], billing)
	}
}
