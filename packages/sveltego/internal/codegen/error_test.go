package codegen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
	"github.com/binsarjr/sveltego/test-utils/golden"
)

func TestGenerateErrorPage_Fixtures(t *testing.T) {
	t.Parallel()
	root := "testdata/codegen/errorpage"
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
	if len(matches) < 1 {
		t.Fatalf("expected >= 1 errorpage fixture, found %d", len(matches))
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
			out, err := GenerateErrorPage(frag, ErrorPageOptions{PackageName: "page"})
			if err != nil {
				t.Fatalf("GenerateErrorPage: %v", err)
			}
			golden.EqualString(t, "errorpage/"+name+".gen.go", string(out))
		})
	}
}

func TestGenerateErrorPage_NilFragment(t *testing.T) {
	t.Parallel()
	if _, err := GenerateErrorPage(nil, ErrorPageOptions{PackageName: "x"}); err == nil {
		t.Fatal("expected error on nil fragment")
	}
}

func TestGenerateErrorPage_EmptyPackageName(t *testing.T) {
	t.Parallel()
	frag, errs := parser.Parse([]byte("<h1>err</h1>"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if _, err := GenerateErrorPage(frag, ErrorPageOptions{}); err == nil {
		t.Fatal("expected error on empty package name")
	}
}
