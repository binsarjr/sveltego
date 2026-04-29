// Package golden provides a minimal golden-file test harness for the sveltego project.
//
// Usage:
//
//	func TestCompile(t *testing.T) {
//	    got := compile("hello")
//	    golden.Equal(t, "compile/hello", got)
//	}
//
// Run "go test ./... -args -update" or set GOLDEN_UPDATE=1 to rewrite fixtures.
package golden
