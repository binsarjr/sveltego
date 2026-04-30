// Build tag `integration` gates the subprocess-style end-to-end test
// that runs `sveltego build` against the example fixture and asserts a
// real binary is produced. Default `go test` skips it because go build
// inside a test is multi-second and not fit for the inner dev loop.
// Run with `go test -tags=integration -run TestBuildCmdIntegration ./cmd/sveltego/...`.
//go:build integration

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildCmdIntegration(t *testing.T) {
	resetLoggerOnCleanup(t)
	root := stageExample(t)
	withCwd(t, root)

	outRel := filepath.Join("build", "app")
	if runtime.GOOS == "windows" {
		outRel += ".exe"
	}

	stdout, stderr, err := runCmd(t, "build", "--out", outRel, "--main", "./cmd/app")
	if err != nil {
		t.Fatalf("build: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	binPath := filepath.Join(root, outRel)
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("expected binary at %s: %v", binPath, err)
	}
	if info.Size() == 0 {
		t.Errorf("binary at %s is empty", binPath)
	}
}
