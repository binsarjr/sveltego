package adapterfastly_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adapterfastly "github.com/binsarjr/sveltego/adapter-fastly"
)

// hasTinyGo reports whether tinygo is available on PATH in this test environment.
func hasTinyGo() bool {
	_, err := exec.LookPath("tinygo")
	return err == nil
}

func TestBuildRequiresOutputDir(t *testing.T) {
	t.Parallel()
	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		ProjectRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected error for missing OutputDir")
	}
	if !strings.Contains(err.Error(), "OutputDir is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildRequiresProjectRoot(t *testing.T) {
	t.Parallel()
	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected error for missing ProjectRoot")
	}
	if !strings.Contains(err.Error(), "ProjectRoot is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := adapterfastly.Build(ctx, adapterfastly.BuildContext{
		ProjectRoot: t.TempDir(),
		OutputDir:   t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected context error")
	}
}

func TestBuildReturnsTinyGoMissingWhenAbsent(t *testing.T) {
	t.Parallel()
	if hasTinyGo() {
		t.Skip("tinygo present on PATH — skipping ErrTinyGoMissing test")
	}
	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		ProjectRoot: t.TempDir(),
		OutputDir:   t.TempDir(),
	})
	if !errors.Is(err, adapterfastly.ErrTinyGoMissing) {
		t.Fatalf("expected ErrTinyGoMissing, got %v", err)
	}
}

func TestBuildTinyGoPathNotFound(t *testing.T) {
	t.Parallel()
	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		ProjectRoot: t.TempDir(),
		OutputDir:   t.TempDir(),
		TinyGoPath:  "/nonexistent/tinygo",
	})
	if !errors.Is(err, adapterfastly.ErrTinyGoMissing) {
		t.Fatalf("expected ErrTinyGoMissing for bad explicit path, got %v", err)
	}
}

// TestBuildInvokesTinyGoWithCorrectFlags verifies that when tinygo IS
// available, Build drives it with the expected flags. We use a fake
// tinygo script that records its argv to a temp file instead of actually
// compiling, then assert on the captured arguments.
//
// Skipped when tinygo is absent — TestBuildReturnsTinyGoMissingWhenAbsent
// covers that path.
func TestBuildInvokesTinyGoWithCorrectFlags(t *testing.T) {
	t.Parallel()
	if !hasTinyGo() {
		t.Skip("tinygo not on PATH — skipping invocation-flags test")
	}

	tmp := t.TempDir()
	argFile := filepath.Join(tmp, "args.txt")

	// Write a fake tinygo shell script that records argv.
	fakeTinyGo := filepath.Join(tmp, "tinygo")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argFile + "\n"
	if err := os.WriteFile(fakeTinyGo, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tinygo: %v", err)
	}

	outDir := filepath.Join(tmp, "dist")
	projRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		ProjectRoot: projRoot,
		OutputDir:   outDir,
		TinyGoPath:  fakeTinyGo,
		ServiceName: "my-service",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	raw, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatalf("read arg file: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")

	wantArgs := map[string]bool{
		"build":    false,
		"-target":  false,
		"wasip1":   false,
		"-o":       false,
		"-scheduler": false,
		"none":     false,
	}
	for _, a := range args {
		if _, ok := wantArgs[a]; ok {
			wantArgs[a] = true
		}
	}
	for arg, found := range wantArgs {
		if !found {
			t.Errorf("tinygo invocation missing arg %q; got: %v", arg, args)
		}
	}
}

// TestBuildWritesFastlyTOML checks that a fastly.toml is emitted alongside
// the wasm artifact when tinygo succeeds. Uses the same fake-tinygo trick.
func TestBuildWritesFastlyTOML(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	wasmOut := filepath.Join(tmp, "dist", "main.wasm")

	// Fake tinygo that just creates the wasm file.
	fakeTinyGo := filepath.Join(tmp, "tinygo")
	script := "#!/bin/sh\ntouch " + wasmOut + "\n"
	if err := os.WriteFile(fakeTinyGo, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tinygo: %v", err)
	}

	projRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	outDir := filepath.Join(tmp, "dist")
	err := adapterfastly.Build(context.Background(), adapterfastly.BuildContext{
		ProjectRoot: projRoot,
		OutputDir:   outDir,
		TinyGoPath:  fakeTinyGo,
		ServiceName: "hello-world",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	tomlPath := filepath.Join(outDir, "fastly.toml")
	raw, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("fastly.toml not written: %v", err)
	}

	wants := []string{"manifest_version", "hello-world", "language", "assets"}
	for _, w := range wants {
		if !strings.Contains(string(raw), w) {
			t.Errorf("fastly.toml missing %q\n%s", w, raw)
		}
	}
}

func TestDocContent(t *testing.T) {
	t.Parallel()
	doc := adapterfastly.Doc()
	wants := []string{
		"Fastly Compute@Edge",
		"TinyGo",
		"wasip1",
		"fastly.toml",
		"KV",
	}
	for _, w := range wants {
		if !strings.Contains(doc, w) {
			t.Errorf("Doc missing %q", w)
		}
	}
}

func TestNameConstant(t *testing.T) {
	t.Parallel()
	if adapterfastly.Name != "fastly" {
		t.Fatalf("Name = %q, want %q", adapterfastly.Name, "fastly")
	}
}
