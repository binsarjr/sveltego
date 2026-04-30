package codegen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
)

func TestRunA11yChecks_NilFragment(t *testing.T) {
	if got := RunA11yChecks(nil); got != nil {
		t.Fatalf("nil fragment should produce no diagnostics, got %v", got)
	}
}

func TestRunA11yChecks_Rules(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantCode string
		wantNone bool
	}{
		{
			name:     "img missing alt",
			src:      `<img src="logo.png" />`,
			wantCode: A11yImgAlt,
		},
		{
			name:     "img with alt is fine",
			src:      `<img src="logo.png" alt="Logo" />`,
			wantNone: true,
		},
		{
			name:     "img with empty alt is fine (decorative)",
			src:      `<img src="logo.png" alt="" />`,
			wantNone: true,
		},
		{
			name:     "img with dynamic alt is fine",
			src:      `<img src="logo.png" alt={Caption} />`,
			wantNone: true,
		},
		{
			name:     "anchor without text",
			src:      `<a href="/about"></a>`,
			wantCode: A11yAnchorContent,
		},
		{
			name:     "anchor with text is fine",
			src:      `<a href="/about">About</a>`,
			wantNone: true,
		},
		{
			name:     "anchor with aria-label is fine",
			src:      `<a href="/about" aria-label="About"></a>`,
			wantNone: true,
		},
		{
			name:     "anchor wrapping img with alt is fine",
			src:      `<a href="/"><img src="logo.png" alt="Home" /></a>`,
			wantNone: true,
		},
		{
			name:     "button without text",
			src:      `<button></button>`,
			wantCode: A11yButtonContent,
		},
		{
			name:     "button with text is fine",
			src:      `<button>Submit</button>`,
			wantNone: true,
		},
		{
			name:     "button with mustache is fine",
			src:      `<button>{Label}</button>`,
			wantNone: true,
		},
		{
			name:     "input without label",
			src:      `<input type="text" />`,
			wantCode: A11yInputLabel,
		},
		{
			name:     "input with id is fine",
			src:      `<input type="text" id="email" />`,
			wantNone: true,
		},
		{
			name:     "input with aria-label is fine",
			src:      `<input type="text" aria-label="Email" />`,
			wantNone: true,
		},
		{
			name:     "input type=hidden is exempt",
			src:      `<input type="hidden" name="csrf" />`,
			wantNone: true,
		},
		{
			name:     "input type=submit is exempt",
			src:      `<input type="submit" />`,
			wantNone: true,
		},
		{
			name:     "html without lang",
			src:      `<html><body></body></html>`,
			wantCode: A11yHTMLLang,
		},
		{
			name:     "html with lang is fine",
			src:      `<html lang="en"><body></body></html>`,
			wantNone: true,
		},
		{
			name:     "role with invalid value",
			src:      `<div role="bogus">x</div>`,
			wantCode: A11yRoleValid,
		},
		{
			name:     "role with valid value is fine",
			src:      `<div role="navigation">x</div>`,
			wantNone: true,
		},
		{
			name:     "role with dynamic value is skipped",
			src:      `<div role={Role}>x</div>`,
			wantNone: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frag, errs := parser.Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			diags := RunA11yChecks(frag)
			if tc.wantNone {
				if len(diags) != 0 {
					t.Fatalf("want no diagnostics, got %v", diags)
				}
				return
			}
			if len(diags) == 0 {
				t.Fatalf("want diagnostic %q, got none", tc.wantCode)
			}
			found := false
			for _, d := range diags {
				if strings.HasPrefix(d.Message, tc.wantCode+":") {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("want diagnostic %q, got %v", tc.wantCode, diags)
			}
		})
	}
}

func TestRunA11yChecks_DeterministicOrdering(t *testing.T) {
	src := `<img src="b.png" />
<img src="a.png" />
<button></button>`
	frag, errs := parser.Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	diags := RunA11yChecks(frag)
	if len(diags) != 3 {
		t.Fatalf("want 3 diagnostics, got %d: %v", len(diags), diags)
	}
	for i := 1; i < len(diags); i++ {
		prev, cur := diags[i-1].Pos, diags[i].Pos
		if prev.Line > cur.Line || (prev.Line == cur.Line && prev.Col > cur.Col) {
			t.Fatalf("not sorted by position: %v", diags)
		}
	}
}

func TestA11yFixtures(t *testing.T) {
	root := "testdata/a11y"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	var names []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svelte") {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Fatalf("no fixtures found in %s", root)
	}
	sort.Strings(names)
	for _, name := range names {
		base := strings.TrimSuffix(name, ".svelte")
		t.Run(base, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			frag, errs := parser.Parse(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			diags := RunA11yChecks(frag)
			got := formatA11yDiagnostics(diags)
			goldPath := filepath.Join("testdata/golden/a11y", base+".golden")
			if os.Getenv("GOLDEN_UPDATE") == "1" {
				if err := os.MkdirAll(filepath.Dir(goldPath), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(goldPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return
			}
			raw, err := os.ReadFile(goldPath)
			if err != nil {
				t.Fatalf("read %s: %v (run with GOLDEN_UPDATE=1 to create)", goldPath, err)
			}
			if string(raw) != got {
				t.Fatalf("golden mismatch\n--- want ---\n%s--- got ---\n%s", string(raw), got)
			}
		})
	}
}

func formatA11yDiagnostics(ds []Diagnostic) string {
	if len(ds) == 0 {
		return ""
	}
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.Pos.String())
		b.WriteByte(' ')
		b.WriteString(d.Severity.String())
		b.WriteByte(' ')
		b.WriteString(d.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
