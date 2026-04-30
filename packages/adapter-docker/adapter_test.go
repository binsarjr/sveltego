package adapterdocker_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapterdocker "github.com/binsarjr/sveltego/adapter-docker"
)

func TestBuildEmitsDockerfileAndIgnore(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	err := adapterdocker.Build(context.Background(), adapterdocker.BuildContext{
		OutputDir:   out,
		BinaryName:  "myapp",
		MainPackage: "./cmd/myapp",
		AssetsDir:   "static",
		GoVersion:   "1.23",
		Port:        9000,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	dockerfile, err := os.ReadFile(filepath.Join(out, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	got := string(dockerfile)
	wants := []string{
		"ARG GO_VERSION=1.23",
		"FROM golang:${GO_VERSION}-alpine AS builder",
		"go build -trimpath -ldflags='-s -w' -o /out/myapp ./cmd/myapp",
		"FROM gcr.io/distroless/static-debian12:nonroot",
		"COPY --from=builder /out/myapp /app/myapp",
		"COPY --from=builder /src/static /app/assets",
		"EXPOSE 9000",
		"USER nonroot:nonroot",
		"HEALTHCHECK",
		"ENTRYPOINT [\"/app/myapp\"]",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("Dockerfile missing %q\n--- got ---\n%s", w, got)
		}
	}

	di, err := os.ReadFile(filepath.Join(out, ".dockerignore"))
	if err != nil {
		t.Fatalf("read .dockerignore: %v", err)
	}
	for _, w := range []string{".git", "node_modules", ".gen/embed/_dev", ".claude"} {
		if !strings.Contains(string(di), w) {
			t.Errorf(".dockerignore missing %q", w)
		}
	}
}

func TestBuildDefaults(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := adapterdocker.Build(context.Background(), adapterdocker.BuildContext{
		OutputDir: out,
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	dockerfile, _ := os.ReadFile(filepath.Join(out, "Dockerfile"))
	got := string(dockerfile)
	wants := []string{
		"ARG GO_VERSION=1.23",
		"go build -trimpath -ldflags='-s -w' -o /out/sveltego ./cmd/app",
		"EXPOSE 8080",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("default Dockerfile missing %q", w)
		}
	}
	if strings.Contains(got, "/app/assets") {
		t.Errorf("AssetsDir empty should skip assets COPY")
	}
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()
	if err := adapterdocker.Build(context.Background(), adapterdocker.BuildContext{}); err == nil {
		t.Fatalf("expected error for missing OutputDir")
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()
	if !strings.Contains(adapterdocker.Doc(), "Docker target") {
		t.Fatalf("Doc missing target heading")
	}
}

func TestBuildContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapterdocker.Build(ctx, adapterdocker.BuildContext{OutputDir: t.TempDir()}); err == nil {
		t.Fatalf("expected ctx error")
	}
}
