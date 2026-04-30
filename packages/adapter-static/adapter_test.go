package adapterstatic_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	adapterstatic "github.com/binsarjr/sveltego/adapter-static"
)

func TestBuildReturnsNotImplemented(t *testing.T) {
	t.Parallel()
	err := adapterstatic.Build(context.Background(), adapterstatic.BuildContext{
		ProjectRoot: "/tmp/whatever",
		OutputDir:   "/tmp/out",
	})
	if err == nil {
		t.Fatalf("expected ErrNotImplemented")
	}
	if !errors.Is(err, adapterstatic.ErrNotImplemented) {
		t.Fatalf("expected errors.Is(err, ErrNotImplemented), got %v", err)
	}
	if !strings.Contains(err.Error(), "issues/65") {
		t.Fatalf("error missing tracking link: %v", err)
	}
}

func TestDocCallsOutBlocker(t *testing.T) {
	t.Parallel()
	doc := adapterstatic.Doc()
	wants := []string{"BLOCKED on #65", "Track:", "issues/65"}
	for _, w := range wants {
		if !strings.Contains(doc, w) {
			t.Errorf("Doc missing %q", w)
		}
	}
}

func TestBuildContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := adapterstatic.Build(ctx, adapterstatic.BuildContext{})
	if err == nil {
		t.Fatalf("expected ctx error")
	}
}
