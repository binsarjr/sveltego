package codegen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/testutils/golden"
)

func TestGenerateLayout_Fixtures(t *testing.T) {
	t.Parallel()
	root := "testdata/codegen/layout"
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".svelte") {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(matches) < 4 {
		t.Fatalf("expected >= 4 layout fixtures, found %d", len(matches))
	}
	sort.Strings(matches)
	for _, path := range matches {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".svelte")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			frag, errs := parser.Parse(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			out, err := GenerateLayout(frag, LayoutOptions{PackageName: "page"})
			if err != nil {
				t.Fatalf("GenerateLayout: %v", err)
			}
			golden.EqualString(t, "layout/"+name+".gen.go", string(out))
		})
	}
}

func TestGenerateLayout_NilFragment(t *testing.T) {
	t.Parallel()
	if _, err := GenerateLayout(nil, LayoutOptions{PackageName: "x"}); err == nil {
		t.Fatal("expected error on nil fragment")
	}
}

func TestGenerateLayout_EmptyPackageName(t *testing.T) {
	t.Parallel()
	frag, errs := parser.Parse([]byte("<slot />\n"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if _, err := GenerateLayout(frag, LayoutOptions{}); err == nil {
		t.Fatal("expected error on empty package name")
	}
}

func TestGenerateLayout_PageSlotIsTodo(t *testing.T) {
	t.Parallel()
	frag, errs := parser.Parse([]byte("<slot />\n"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(string(out), "TODO: <slot /> outside layout") {
		t.Fatalf("expected TODO marker for slot in page, got:\n%s", out)
	}
	if strings.Contains(string(out), "children(w)") {
		t.Fatalf("did not expect children() call in page output:\n%s", out)
	}
}
