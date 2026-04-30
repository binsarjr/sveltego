package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_TailwindBareFlag_DefaultsToV4(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run([]string{"--tailwind", "--non-interactive", dir}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if !bytes.Contains(pkg, []byte("@tailwindcss/vite")) {
		t.Errorf("expected v4 plugin pinning, got: %s", pkg)
	}
}

func TestRun_TailwindEqualsV3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run([]string{"--tailwind=v3", "--non-interactive", dir}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if bytes.Contains(pkg, []byte("@tailwindcss/vite")) {
		t.Errorf("v3 must not pull @tailwindcss/vite, got: %s", pkg)
	}
	if !bytes.Contains(pkg, []byte(`"tailwindcss"`)) {
		t.Errorf("v3 must include tailwindcss, got: %s", pkg)
	}
	if _, err := os.Stat(filepath.Join(dir, "tailwind.config.js")); err != nil {
		t.Errorf("tailwind.config.js missing: %v", err)
	}
	if !strings.Contains(stdout.String(), "next step:") {
		t.Errorf("missing install hint in stdout: %s", stdout.String())
	}
}

func TestRun_TailwindEqualsNone_OmitsTailwindDeps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run([]string{"--tailwind=none", "--non-interactive", dir}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("package.json should be written for --tailwind=none: %v", err)
	}
	if bytes.Contains(pkg, []byte("tailwindcss")) {
		t.Errorf("package.json should not include tailwindcss for --tailwind=none, got: %s", pkg)
	}
	if !strings.Contains(stdout.String(), "next step:") {
		t.Errorf("expected install hint in stdout (package.json is always emitted): %s", stdout.String())
	}
}

func TestRun_TailwindBogusValue_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := run([]string{"--tailwind=bogus", "--non-interactive", dir}, strings.NewReader(""), stdout, stderr)
	if err == nil {
		t.Fatalf("expected error on unknown flavor, stderr: %s", stderr.String())
	}
}
