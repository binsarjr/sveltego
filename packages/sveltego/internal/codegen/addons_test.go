package codegen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/vite"
)

func TestDetectAddons(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want []vite.Addon
	}{
		{
			name: "missing-package-json",
			body: "",
			want: nil,
		},
		{
			name: "no-tailwind",
			body: `{"devDependencies": {"vite": "^6"}}`,
			want: nil,
		},
		{
			name: "tailwind-v4-via-dev",
			body: `{"devDependencies": {"@tailwindcss/vite": "^4"}}`,
			want: []vite.Addon{vite.AddonTailwindV4},
		},
		{
			name: "tailwind-v4-via-runtime",
			body: `{"dependencies": {"@tailwindcss/vite": "^4"}}`,
			want: []vite.Addon{vite.AddonTailwindV4},
		},
		{
			name: "tailwind-v3",
			body: `{"devDependencies": {"tailwindcss": "^3", "postcss": "^8"}}`,
			want: []vite.Addon{vite.AddonTailwindV3},
		},
		{
			name: "v4-wins-over-v3-when-both-listed",
			body: `{"devDependencies": {"@tailwindcss/vite": "^4", "tailwindcss": "^4"}}`,
			want: []vite.Addon{vite.AddonTailwindV4},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tc.body != "" {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tc.body), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			got, err := detectAddons(dir)
			if err != nil {
				t.Fatalf("detectAddons: %v", err)
			}
			if !equalAddons(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestDetectAddons_BadJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := detectAddons(dir); err == nil {
		t.Errorf("expected parse error")
	}
}

func TestResolveCSSEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if got := resolveCSSEntry(dir); got != "" {
		t.Errorf("missing app.css: got %q want \"\"", got)
	}
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "app.css"), []byte("@import \"tailwindcss\";\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := resolveCSSEntry(dir); got != "src/app.css" {
		t.Errorf("got %q want %q", got, "src/app.css")
	}
}

func equalAddons(a, b []vite.Addon) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuild_TailwindV4_WiresViteConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/tw4")
	if err := os.WriteFile(filepath.Join(root, "package.json"),
		[]byte(`{"devDependencies": {"@tailwindcss/vite": "^4"}}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.css"),
		[]byte(`@import "tailwindcss";`+"\n"), 0o600); err != nil {
		t.Fatalf("seed app.css: %v", err)
	}

	res, err := Build(context.Background(), BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.ViteConfigPath == "" {
		t.Fatalf("expected ViteConfigPath, got empty")
	}
	cfg, err := os.ReadFile(res.ViteConfigPath)
	if err != nil {
		t.Fatalf("read vite config: %v", err)
	}
	if !bytes.Contains(cfg, []byte(`import tailwindcss from '@tailwindcss/vite'`)) {
		t.Errorf("vite config missing tailwindcss import:\n%s", cfg)
	}
	if !bytes.Contains(cfg, []byte(`tailwindcss()`)) {
		t.Errorf("vite config missing tailwindcss() plugin call:\n%s", cfg)
	}
	if !bytes.Contains(cfg, []byte(`"app": path.resolve(__dirname, "src/app.css")`)) {
		t.Errorf("vite config missing app.css rollup input:\n%s", cfg)
	}
}

func TestBuild_TailwindV3_WiresPostCSS(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/tw3")
	if err := os.WriteFile(filepath.Join(root, "package.json"),
		[]byte(`{"devDependencies": {"tailwindcss": "^3", "postcss": "^8"}}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.css"),
		[]byte("@tailwind base;\n@tailwind components;\n@tailwind utilities;\n"), 0o600); err != nil {
		t.Fatalf("seed app.css: %v", err)
	}

	res, err := Build(context.Background(), BuildOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg, err := os.ReadFile(res.ViteConfigPath)
	if err != nil {
		t.Fatalf("read vite config: %v", err)
	}
	if bytes.Contains(cfg, []byte(`@tailwindcss/vite`)) {
		t.Errorf("v3 path must not import @tailwindcss/vite plugin:\n%s", cfg)
	}
	if !bytes.Contains(cfg, []byte(`css: { postcss: './postcss.config.js' }`)) {
		t.Errorf("v3 path must wire postcss config:\n%s", cfg)
	}
}
