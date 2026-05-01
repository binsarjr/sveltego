package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTailwindFlavor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    TailwindFlavor
		wantErr bool
	}{
		{"", TailwindNone, false},
		{"none", TailwindNone, false},
		{"NONE", TailwindNone, false},
		{"v4", TailwindV4, false},
		{"V4", TailwindV4, false},
		{"4", TailwindV4, false},
		{"v3", TailwindV3, false},
		{"3", TailwindV3, false},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		got, err := ParseTailwindFlavor(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTailwindFlavor(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTailwindFlavor(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseTailwindFlavor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRun_TailwindV4_WritesPlugin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := Run(Options{Dir: dir, Module: "example.com/tw4", Tailwind: TailwindV4})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantFiles := []string{"package.json", "src/app.css"}
	for _, p := range wantFiles {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if !bytes.Contains(pkg, []byte("@tailwindcss/vite")) {
		t.Errorf("package.json missing @tailwindcss/vite, got: %s", pkg)
	}
	css, err := os.ReadFile(filepath.Join(dir, "src/app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	if !bytes.Contains(css, []byte(`@import "tailwindcss"`)) {
		t.Errorf("app.css missing @import \"tailwindcss\", got: %s", css)
	}
	if !bytes.Contains(css, []byte("@source")) {
		t.Errorf("app.css missing @source directive, got: %s", css)
	}
	if res.InstallCommand == "" {
		t.Errorf("expected install command for v4 flavor, got empty")
	}
	if !strings.Contains(res.InstallCommand, "install") {
		t.Errorf("install command shape wrong: %q", res.InstallCommand)
	}

	layout, err := os.ReadFile(filepath.Join(dir, "src/routes/_layout.svelte"))
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if !bytes.Contains(layout, []byte(`import './app.css'`)) {
		t.Errorf("_layout.svelte missing app.css import, got: %s", layout)
	}
}

func TestRun_TailwindV3_WritesPostCSSPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := Run(Options{Dir: dir, Module: "example.com/tw3", Tailwind: TailwindV3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantFiles := []string{
		"package.json",
		"src/app.css",
		"postcss.config.js",
		"tailwind.config.js",
	}
	for _, p := range wantFiles {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if bytes.Contains(pkg, []byte("@tailwindcss/vite")) {
		t.Errorf("package.json should not include @tailwindcss/vite for v3, got: %s", pkg)
	}
	for _, dep := range []string{"tailwindcss", "postcss", "autoprefixer"} {
		if !bytes.Contains(pkg, []byte(dep)) {
			t.Errorf("package.json missing %q, got: %s", dep, pkg)
		}
	}
	css, err := os.ReadFile(filepath.Join(dir, "src/app.css"))
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	for _, d := range []string{"@tailwind base", "@tailwind components", "@tailwind utilities"} {
		if !bytes.Contains(css, []byte(d)) {
			t.Errorf("app.css missing %q, got: %s", d, css)
		}
	}
	if res.InstallCommand == "" {
		t.Errorf("expected install command for v3 flavor")
	}
}

func TestRun_TailwindNone_OmitsTailwindDeps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := Run(Options{Dir: dir, Module: "example.com/none", Tailwind: TailwindNone})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("package.json should be written even without Tailwind: %v", err)
	}
	for _, dep := range []string{"@tailwindcss/vite", "tailwindcss", "postcss", "autoprefixer"} {
		if bytes.Contains(pkg, []byte(dep)) {
			t.Errorf("package.json should not include %q without Tailwind, got: %s", dep, pkg)
		}
	}
	for _, dep := range []string{"@sveltejs/vite-plugin-svelte", "svelte", "vite"} {
		if !bytes.Contains(pkg, []byte(dep)) {
			t.Errorf("package.json missing baseline dep %q, got: %s", dep, pkg)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "src/app.css")); err == nil {
		t.Errorf("src/app.css should not be written when Tailwind=none")
	}
	if res.InstallCommand == "" {
		t.Errorf("expected install command (always populated when package.json is written), got empty")
	}
}

func TestDetectPackageManager(t *testing.T) {
	t.Parallel()
	cases := []struct {
		seed string
		want PackageManager
	}{
		{"", PMnpm},
		{"pnpm-lock.yaml", PMpnpm},
		{"bun.lockb", PMbun},
		{"bun.lock", PMbun},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		if tc.seed != "" {
			if err := os.WriteFile(filepath.Join(dir, tc.seed), []byte{}, 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		got := DetectPackageManager(dir)
		if got != tc.want {
			t.Errorf("seed %q: got %q want %q", tc.seed, got, tc.want)
		}
	}
}
