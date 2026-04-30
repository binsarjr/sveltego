package routescan

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanDeterministic(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(filepath.Join("testdata", "nested"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	in := ScanInput{RoutesDir: filepath.Join(abs, "routes")}

	first, err := Scan(in)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	for range 4 {
		next, err := Scan(in)
		if err != nil {
			t.Fatalf("repeat scan: %v", err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("non-deterministic scan:\nfirst=%+v\nnext=%+v", first, next)
		}
	}
}

func TestScanSortsAcrossFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []string{"nested", "groups", "optional", "rest"}
	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			t.Parallel()
			res := mustScan(t, f, "")
			for i := 1; i < len(res.Routes); i++ {
				if res.Routes[i-1].Pattern > res.Routes[i].Pattern {
					t.Fatalf("%s: routes not sorted: %v", f, patterns(res.Routes))
				}
			}
			for i := 1; i < len(res.Diagnostics); i++ {
				if res.Diagnostics[i-1].Path > res.Diagnostics[i].Path {
					t.Fatalf("%s: diagnostics not sorted by path", f)
				}
			}
		})
	}
}
