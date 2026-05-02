package adapterauto_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapterauto "github.com/binsarjr/sveltego/adapter-auto"
	adaptercloudflare "github.com/binsarjr/sveltego/adapter-cloudflare"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"explicit override", map[string]string{"SVELTEGO_ADAPTER": "docker"}, "docker"},
		{"lambda env", map[string]string{"AWS_LAMBDA_RUNTIME_API": "on"}, "lambda"},
		{"cf pages", map[string]string{"CF_PAGES": "1"}, "cloudflare"},
		{"default server", map[string]string{}, "server"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SVELTEGO_ADAPTER", "")
			t.Setenv("AWS_LAMBDA_RUNTIME_API", "")
			t.Setenv("CF_PAGES", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := adapterauto.Detect(); got != tc.want {
				t.Fatalf("Detect()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestBuildDispatchesServer(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binSrc := filepath.Join(tmp, "bin")
	if err := os.WriteFile(binSrc, []byte("ELF"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out := filepath.Join(tmp, "dist")

	err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target:     "server",
		BinaryPath: binSrc,
		OutputDir:  out,
	})
	if err != nil {
		t.Fatalf("Build server: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "sveltego")); err != nil {
		t.Fatalf("server adapter did not emit binary: %v", err)
	}
}

func TestBuildDispatchesDocker(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target:    "docker",
		OutputDir: out,
	}); err != nil {
		t.Fatalf("Build docker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "Dockerfile")); err != nil {
		t.Fatalf("docker adapter did not emit Dockerfile: %v", err)
	}
}

func TestBuildDispatchesLambda(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target:      "lambda",
		ProjectRoot: root,
		ModulePath:  "github.com/example/myapp",
	}); err != nil {
		t.Fatalf("Build lambda: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "lambda", "main.go")); err != nil {
		t.Fatalf("lambda adapter did not emit main.go: %v", err)
	}
}

func TestBuildDispatchesStatic(t *testing.T) {
	t.Parallel()
	// Static target now wires through to the prerender pipeline (#447).
	// Without ProjectRoot set the static adapter fails validation; that
	// is the signal we use to confirm dispatch landed in the static
	// adapter rather than the cloudflare/server stubs.
	err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target: "static",
	})
	if err == nil || !strings.Contains(err.Error(), "adapter-static") {
		t.Fatalf("expected adapter-static error, got %v", err)
	}
}

func TestBuildDispatchesCloudflareReturnsBlocker(t *testing.T) {
	t.Parallel()
	err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target: "cloudflare",
	})
	if !errors.Is(err, adaptercloudflare.ErrNotImplemented) {
		t.Fatalf("expected cloudflare blocker, got %v", err)
	}
}

func TestBuildUnknownTarget(t *testing.T) {
	t.Parallel()
	err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		Target: "vercel",
	})
	if !errors.Is(err, adapterauto.ErrUnknownTarget) {
		t.Fatalf("expected ErrUnknownTarget, got %v", err)
	}
	if !strings.Contains(err.Error(), "vercel") {
		t.Fatalf("error should mention the offending target, got %v", err)
	}
}

func TestBuildAutoDetectsToServer(t *testing.T) {
	t.Setenv("SVELTEGO_ADAPTER", "")
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "")
	t.Setenv("CF_PAGES", "")

	tmp := t.TempDir()
	binSrc := filepath.Join(tmp, "bin")
	if err := os.WriteFile(binSrc, []byte("ELF"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := adapterauto.Build(context.Background(), adapterauto.BuildContext{
		BinaryPath: binSrc,
		OutputDir:  filepath.Join(tmp, "dist"),
	}); err != nil {
		t.Fatalf("Build auto: %v", err)
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()
	for _, name := range adapterauto.Targets() {
		doc, err := adapterauto.Doc(name)
		if err != nil {
			t.Errorf("Doc(%q): %v", name, err)
			continue
		}
		if doc == "" {
			t.Errorf("Doc(%q) empty", name)
		}
	}
	if _, err := adapterauto.Doc("vercel"); !errors.Is(err, adapterauto.ErrUnknownTarget) {
		t.Errorf("Doc(\"vercel\") should error: %v", err)
	}
}

func TestTargets(t *testing.T) {
	t.Parallel()
	want := []string{"server", "docker", "lambda", "static", "cloudflare"}
	got := adapterauto.Targets()
	if len(got) != len(want) {
		t.Fatalf("Targets count %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Targets[%d]=%q want %q", i, got[i], w)
		}
	}
}
