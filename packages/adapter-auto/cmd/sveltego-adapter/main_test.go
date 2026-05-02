package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTargets(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"targets"}, &stdout, &stderr); err != nil {
		t.Fatalf("targets: %v", err)
	}
	for _, want := range []string{"server", "docker", "lambda", "static", "cloudflare"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("targets output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(stdout.String(), "sveltego-adapter") {
		t.Fatalf("help output missing program name: %s", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"deploy"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected errUsage, got %v", err)
	}
}

func TestRunBuildRequiresTarget(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"build"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected errUsage, got %v", err)
	}
}

func TestRunBuildServer(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binSrc := filepath.Join(tmp, "bin")
	if err := os.WriteFile(binSrc, []byte("ELF"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out := filepath.Join(tmp, "dist")

	var stdout, stderr bytes.Buffer
	err := run([]string{
		"build",
		"--target=server",
		"--binary", binSrc,
		"--out", out,
		"--binary-name", "myapp",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "myapp")); err != nil {
		t.Fatalf("server adapter output missing: %v", err)
	}
}

func TestRunBuildDocker(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "--target=docker", "--out", out, "--port", "9000"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("build docker: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(out, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(body), "EXPOSE 9000") {
		t.Fatalf("Dockerfile did not pick up --port flag: %s", body)
	}
}

func TestRunBuildLambda(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"build", "--target=lambda",
		"--root", root,
		"--module", "github.com/example/app",
		"--memory-mb", "1024",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("build lambda: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "lambda", "main.go")); err != nil {
		t.Fatalf("lambda main.go missing: %v", err)
	}
}

func TestRunBuildStatic(t *testing.T) {
	t.Parallel()
	// Static target now drives the prerender pipeline (#447). The CLI
	// hands a synthetic project root that has no go.mod, so the
	// underlying `go build` fails — that surfaces as an adapter-static
	// error which is the signal we use to confirm dispatch landed
	// correctly.
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	err := run([]string{"build", "--target=static", "--root", root}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "adapter-static") {
		t.Fatalf("expected adapter-static error, got %v", err)
	}
}

func TestRunDoc(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"doc", "--target=server"}, &stdout, &stderr); err != nil {
		t.Fatalf("doc: %v", err)
	}
	if !strings.Contains(stdout.String(), "Server target") {
		t.Fatalf("doc output missing heading: %s", stdout.String())
	}
}

func TestRunDocUnknown(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"doc", "--target=vercel"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("expected unknown target, got %v", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected errUsage, got %v", err)
	}
}
