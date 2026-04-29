// Real benchmarks land per-feature (#60) and per-package. The noop benchmark
// here exists only to keep the bench-regression workflow exercising end-to-end
// before any real work-loads exist.
package benchmarks

import "testing"

func BenchmarkNoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i
	}
}
