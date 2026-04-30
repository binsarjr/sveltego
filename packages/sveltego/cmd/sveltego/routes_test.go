package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutesCmd_FixtureProject(t *testing.T) {
	resetLoggerOnCleanup(t)
	root := stageExample(t)
	withCwd(t, root)

	stdout, stderr, err := runCmd(t, "routes")
	if err != nil {
		t.Fatalf("routes: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "GET") {
		t.Errorf("expected GET row in routes output, got %q", stdout)
	}
	if !strings.Contains(stdout, "links.Index") {
		t.Errorf("expected links.Index helper in routes output, got %q", stdout)
	}
}

func TestRoutesCmd_Help(t *testing.T) {
	resetLoggerOnCleanup(t)
	stdout, _, err := runCmd(t, "routes", "--help")
	if err != nil {
		t.Fatalf("routes --help: %v", err)
	}
	if !strings.Contains(stdout, "List route helpers") {
		t.Errorf("expected routes help to mention 'List route helpers', got %q", stdout)
	}
}

func TestRoutesCmd_NoRoutesDir(t *testing.T) {
	resetLoggerOnCleanup(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	withCwd(t, dir)
	_, _, err := runCmd(t, "routes")
	if err == nil {
		t.Fatal("expected error when src/routes is missing")
	}
}
