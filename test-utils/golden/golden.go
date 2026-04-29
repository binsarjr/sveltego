package golden

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	goldenDir = "testdata/golden"
	goldenExt = ".golden"
	envUpdate = "GOLDEN_UPDATE"
	maxDiff   = 50
)

var (
	updateFlag *bool
	flagOnce   sync.Once
)

func init() {
	registerFlag()
}

func registerFlag() {
	flagOnce.Do(func() {
		if flag.CommandLine.Lookup("update") != nil {
			return
		}
		updateFlag = flag.Bool("update", false, "rewrite golden files in testdata/golden")
	})
}

// Equal asserts that got matches the golden fixture stored at testdata/golden/<name>.golden.
func Equal(t testing.TB, name string, got []byte) {
	t.Helper()
	path := goldenPath(name)
	if updating() {
		writeGolden(t, path, got)
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // path derived from test-controlled name
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("golden missing: %s; run: go test ./<pkg> -args -update", path)
		return
	}
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
		return
	}
	want = normalizeEOL(want)
	if bytes.Equal(want, got) {
		return
	}
	t.Errorf("golden mismatch in %s; run: go test ./<pkg> -args -update\n%s", name, diff(want, got))
}

// EqualString is the string-typed convenience wrapper around Equal.
func EqualString(t testing.TB, name string, got string) {
	t.Helper()
	Equal(t, name, []byte(got))
}

func goldenPath(name string) string {
	parts := strings.Split(name, "/")
	parts[len(parts)-1] += goldenExt
	return filepath.Join(append([]string{goldenDir}, parts...)...)
}

func updating() bool {
	if os.Getenv(envUpdate) == "1" {
		return true
	}
	if updateFlag != nil && *updateFlag {
		return true
	}
	if f := flag.CommandLine.Lookup("update"); f != nil {
		if g, ok := f.Value.(flag.Getter); ok {
			if b, ok := g.Get().(bool); ok && b {
				return true
			}
		}
	}
	return false
}

func writeGolden(t testing.TB, path string, got []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		return
	}
	if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // tests own this dir
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func normalizeEOL(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func diff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	var b strings.Builder
	b.WriteString("--- want\n+++ got\n")
	n := len(wantLines)
	if len(gotLines) > n {
		n = len(gotLines)
	}
	shown := 0
	for i := 0; i < n && shown < maxDiff; i++ {
		var w, g string
		var wOK, gOK bool
		if i < len(wantLines) {
			w, wOK = wantLines[i], true
		}
		if i < len(gotLines) {
			g, gOK = gotLines[i], true
		}
		if wOK && gOK && w == g {
			continue
		}
		if wOK {
			fmt.Fprintf(&b, "-%d: %s\n", i+1, w)
			shown++
		}
		if gOK && shown < maxDiff {
			fmt.Fprintf(&b, "+%d: %s\n", i+1, g)
			shown++
		}
	}
	if shown >= maxDiff {
		fmt.Fprintf(&b, "... (truncated at %d differing lines)\n", maxDiff)
	}
	return b.String()
}
