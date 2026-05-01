package svelterender

import (
	"errors"
	"testing"
)

func TestEnsureNode(t *testing.T) {
	t.Parallel()
	// Node availability depends on the host. The contract under test is:
	// either the lookup succeeds and the path is non-empty, or it fails
	// with errNodeMissing wrapped via %w. Both outcomes are healthy.
	path, err := EnsureNode()
	if err != nil {
		if !errors.Is(err, errNodeMissing) {
			t.Fatalf("expected errNodeMissing wrap, got %v", err)
		}
		return
	}
	if path == "" {
		t.Fatal("EnsureNode succeeded but returned empty path")
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()
	if got := Plan(nil); got != nil {
		t.Fatalf("Plan(nil) = %v, want nil", got)
	}
	if got := Plan([]Job{{}}); got != nil {
		t.Fatalf("Plan(empty job) = %v, want nil", got)
	}
	jobs := []Job{
		{Path: "/", Pattern: "/", SSRBundle: ".gen/ssr/index.js"},
		{Path: "/about", Pattern: "/about", SSRBundle: ".gen/ssr/about.js"},
	}
	got := Plan(jobs)
	if len(got) != 2 {
		t.Fatalf("Plan(2 valid) length = %d, want 2", len(got))
	}
}
