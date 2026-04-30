package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
)

func writeTempServerGo(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.server.go")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestScanPageOptions_recognizesAll(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

const (
	Prerender     = true
	SSR           = false
	CSR           = false
	TrailingSlash = kit.TrailingSlashAlways
)
`
	path := writeTempServerGo(t, body)
	got, err := scanPageOptions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasPrerender || !got.Prerender {
		t.Errorf("Prerender missed: %+v", got)
	}
	if !got.HasSSR || got.SSR {
		t.Errorf("SSR missed or wrong value: %+v", got)
	}
	if !got.HasCSR || got.CSR {
		t.Errorf("CSR missed or wrong value: %+v", got)
	}
	if !got.HasTrailingSlash || got.TrailingSlash != kit.TrailingSlashAlways {
		t.Errorf("TrailingSlash missed: %+v", got)
	}
}

func TestScanPageOptions_partial(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

const SSR = false
`
	path := writeTempServerGo(t, body)
	got, err := scanPageOptions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasSSR || got.SSR {
		t.Errorf("SSR not flagged: %+v", got)
	}
	if got.HasCSR || got.HasPrerender || got.HasTrailingSlash {
		t.Errorf("only SSR expected, got %+v", got)
	}
}

func TestScanPageOptions_missingFile(t *testing.T) {
	t.Parallel()
	got, err := scanPageOptions(filepath.Join(t.TempDir(), "missing.go"))
	if err != nil {
		t.Fatalf("err on missing file: %v", err)
	}
	if got.Any() {
		t.Errorf("Any() = true on missing file, got %+v", got)
	}
}

func TestScanPageOptions_unknownTrailingSlashIdent(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

const TrailingSlash = kit.TrailingSlashWeird
`
	path := writeTempServerGo(t, body)
	if _, err := scanPageOptions(path); err == nil {
		t.Fatal("expected error on unknown TrailingSlash ident")
	} else if !strings.Contains(err.Error(), "unknown TrailingSlash") {
		t.Errorf("err = %v", err)
	}
}

func TestScanPageOptions_nonBoolValue(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

const SSR = 1
`
	path := writeTempServerGo(t, body)
	if _, err := scanPageOptions(path); err == nil {
		t.Fatal("expected error on non-bool SSR")
	}
}

func TestScanPageOptions_ssrOnly(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

const SSROnly = true
`
	path := writeTempServerGo(t, body)
	got, err := scanPageOptions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasSSROnly || !got.SSROnly {
		t.Errorf("SSROnly not set: %+v", got)
	}
	if got.HasPrerender || got.HasSSR || got.HasCSR || got.HasTrailingSlash {
		t.Errorf("only SSROnly expected, got %+v", got)
	}
}

func TestScanPageOptions_prerenderAutoAndProtected(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

const (
	Prerender          = true
	PrerenderAuto      = true
	PrerenderProtected = true
)
`
	path := writeTempServerGo(t, body)
	got, err := scanPageOptions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasPrerender || !got.Prerender {
		t.Errorf("Prerender missed: %+v", got)
	}
	if !got.HasPrerenderAuto || !got.PrerenderAuto {
		t.Errorf("PrerenderAuto missed: %+v", got)
	}
	if !got.HasPrerenderProtected || !got.PrerenderProtected {
		t.Errorf("PrerenderProtected missed: %+v", got)
	}
}

func TestScanPrerenderFromSvelte(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want kit.PageOptionsOverride
	}{
		{
			"true",
			`<svelte:options prerender />`,
			kit.PageOptionsOverride{HasPrerender: true, Prerender: true},
		},
		{
			"auto",
			`<svelte:options prerender="auto" />`,
			kit.PageOptionsOverride{HasPrerenderAuto: true, PrerenderAuto: true, HasPrerender: true, Prerender: true},
		},
		{
			"protected",
			`<svelte:options prerender="protected" />`,
			kit.PageOptionsOverride{HasPrerender: true, Prerender: true, HasPrerenderProtected: true, PrerenderProtected: true},
		},
		{
			"absent",
			`<h1>nothing</h1>`,
			kit.PageOptionsOverride{},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := filepath.Join(dir, "+page.svelte")
			if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := scanPrerenderFromSvelte(p)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestScanPageOptions_dotImportTrailingSlash(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package routes

const TrailingSlash = TrailingSlashIgnore
`
	path := writeTempServerGo(t, body)
	got, err := scanPageOptions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got.TrailingSlash != kit.TrailingSlashIgnore {
		t.Errorf("ident path missed: %+v", got)
	}
}
