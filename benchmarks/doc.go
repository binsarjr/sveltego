// Package benchmarks holds performance regression tests for sveltego.
//
// Real benchmarks land per-feature (#60). The noop benchmark in this directory
// exists only to keep the bench gate workflow exercising end-to-end.
//
// Run locally:
//
//	go test -bench=. -benchmem -count=6 ./benchmarks/...
//
// Compare two runs:
//
//	bench-compare -base main.txt -head pr.txt
package benchmarks
