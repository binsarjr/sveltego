package adaptercloudflare_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	adaptercloudflare "github.com/binsarjr/sveltego/adapter-cloudflare"
)

func TestBuildReturnsNotImplemented(t *testing.T) {
	t.Parallel()
	err := adaptercloudflare.Build(context.Background(), adaptercloudflare.BuildContext{
		ProjectRoot: "/tmp/whatever",
		OutputDir:   "/tmp/out",
	})
	if err == nil {
		t.Fatalf("expected ErrNotImplemented")
	}
	if !errors.Is(err, adaptercloudflare.ErrNotImplemented) {
		t.Fatalf("expected errors.Is(err, ErrNotImplemented), got %v", err)
	}
}

func TestDocCallsOutRuntimeGap(t *testing.T) {
	t.Parallel()
	doc := adaptercloudflare.Doc()
	wants := []string{
		"BLOCKED on runtime support",
		"~1MB",
		"TinyGo",
		"--target=docker",
	}
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
	if err := adaptercloudflare.Build(ctx, adaptercloudflare.BuildContext{}); err == nil {
		t.Fatalf("expected ctx error")
	}
}
