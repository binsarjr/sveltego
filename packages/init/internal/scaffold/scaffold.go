// Package scaffold writes a fresh sveltego project tree into a target
// directory. It owns both the baseline project files (src/routes, src/lib,
// hooks, config, go.mod, README) and the optional AI-assistant templates
// embedded under packages/init/internal/aitemplates.
package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/init/internal/aitemplates"
)

const fileMode = 0o600

// Options controls what Run writes and how it handles conflicts.
type Options struct {
	// Dir is the target project directory. Created if missing.
	Dir string
	// Module is the Go module path written into the generated go.mod.
	// Empty falls back to the directory base name.
	Module string
	// AI copies the embedded AI-template FS contents into Dir.
	AI bool
	// Force overwrites existing files. Default skips them and reports
	// the list via the returned Result.
	Force bool
	// Tailwind selects the Tailwind CSS scaffolding flavor. Defaults to
	// TailwindNone (Tailwind deps and config files omitted, but a
	// baseline package.json with the Vite client toolchain is still
	// emitted so `sveltego build` can drive the client bundle).
	Tailwind TailwindFlavor
	// ServiceWorker, when true, emits a starter src/service-worker.ts
	// with a no-op install/activate handler the user can extend with
	// their own caching strategy. Codegen auto-detects the file on the
	// next build and wires the registration <script> + Vite Rollup
	// input (#89). Default false — service workers are opt-in.
	ServiceWorker bool
}

// Result reports what Run wrote and what it skipped.
type Result struct {
	Written []string
	Skipped []string
	// InstallCommand is the suggested package-manager command to run
	// after scaffolding. Always populated because the scaffold always
	// writes a package.json with at least the Vite + Svelte toolchain.
	InstallCommand string
}

// Run materializes the scaffold described by opts. It returns the
// per-file outcome and the first I/O error it could not skip past.
func Run(opts Options) (Result, error) {
	res := Result{}
	if opts.Dir == "" {
		return res, errors.New("scaffold: empty target dir")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return res, fmt.Errorf("scaffold: mkdir %s: %w", opts.Dir, err)
	}

	module := opts.Module
	if module == "" {
		module = filepath.Base(opts.Dir)
		if module == "." || module == "/" || module == "" {
			module = "sveltego-app"
		}
	}

	flavor := opts.Tailwind
	if flavor == "" {
		flavor = TailwindNone
	}

	for _, f := range baseFiles(module, flavor) {
		if err := writeFile(opts.Dir, f.path, f.body, opts.Force, &res); err != nil {
			return res, err
		}
	}
	for _, f := range tailwindFiles(flavor) {
		if err := writeFile(opts.Dir, f.path, f.body, opts.Force, &res); err != nil {
			return res, err
		}
	}
	if opts.ServiceWorker {
		if err := writeFile(opts.Dir, "src/service-worker.ts", []byte(serviceWorkerStarter), opts.Force, &res); err != nil {
			return res, err
		}
	}

	if opts.AI {
		if err := writeAITemplates(opts.Dir, opts.Force, &res); err != nil {
			return res, err
		}
	}

	res.InstallCommand = DetectPackageManager(opts.Dir).InstallCommand()

	sort.Strings(res.Written)
	sort.Strings(res.Skipped)
	return res, nil
}

// WriteAITemplates copies every file in [aitemplates.FS] into dir.
// Existing files are skipped unless force is true; their paths are
// returned in Result.Skipped.
func WriteAITemplates(dir string, force bool) (Result, error) {
	res := Result{}
	if dir == "" {
		return res, errors.New("scaffold: empty target dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("scaffold: mkdir %s: %w", dir, err)
	}
	if err := writeAITemplates(dir, force, &res); err != nil {
		return res, err
	}
	sort.Strings(res.Written)
	sort.Strings(res.Skipped)
	return res, nil
}

func writeAITemplates(dir string, force bool, res *Result) error {
	for _, name := range aitemplates.Files {
		body, err := fs.ReadFile(aitemplates.FS, name)
		if err != nil {
			return fmt.Errorf("scaffold: read template %q: %w", name, err)
		}
		if err := writeFile(dir, name, body, force, res); err != nil {
			return err
		}
	}
	return nil
}

type file struct {
	path string
	body []byte
}

func baseFiles(module string, flavor TailwindFlavor) []file {
	return []file{
		{path: "go.mod", body: []byte(renderGoMod(module))},
		{path: "README.md", body: []byte(renderReadme(module))},
		{path: ".gitignore", body: []byte(gitignoreBody)},
		{path: "app.html", body: []byte(appHTMLBody)},
		{path: "package.json", body: []byte(renderPackageJSON(module, flavor))},
		{path: "vite.config.js", body: []byte(viteConfigBody)},
		{path: "tsconfig.json", body: []byte(tsconfigBody)},
		{path: "sveltego.config.go", body: []byte(configBody)},
		{path: "src/hooks.server.go", body: []byte(hooksBody)},
		{path: "cmd/app/main.go", body: []byte(renderMainGo(module))},
		{path: "src/routes/_page.svelte", body: []byte(renderPageSvelte(flavor))},
		{path: "src/routes/_page.server.go", body: []byte(pageServerBody)},
		{path: "src/routes/_layout.svelte", body: []byte(renderLayoutSvelte(flavor))},
		{path: "src/lib/.gitkeep", body: []byte{}},
	}
}

func writeFile(root, rel string, body []byte, force bool, res *Result) error {
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !force {
		if _, err := os.Stat(target); err == nil {
			res.Skipped = append(res.Skipped, rel)
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("scaffold: stat %s: %w", target, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("scaffold: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, body, fileMode); err != nil {
		return fmt.Errorf("scaffold: write %s: %w", target, err)
	}
	res.Written = append(res.Written, rel)
	return nil
}

func renderGoMod(module string) string {
	var b strings.Builder
	b.WriteString("module ")
	b.WriteString(module)
	b.WriteString("\n\ngo 1.25\n")
	return b.String()
}

func renderReadme(module string) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(module)
	b.WriteString("\n\n")
	b.WriteString("Generated by `sveltego-init`.\n\n")
	b.WriteString("## Develop\n\n")
	b.WriteString("```sh\nsveltego build\n./build/app\n```\n\n")
	b.WriteString("`sveltego build` runs codegen, builds the Vite client bundle, and produces `build/app`.\n\n")
	b.WriteString("## Layout\n\n")
	b.WriteString("- `src/routes/` — pages, layouts, server load/actions.\n")
	b.WriteString("- `src/lib/` — shared modules (`$lib` alias).\n")
	b.WriteString("- `cmd/app/main.go` — Go entrypoint that wires the generated routes to an HTTP server.\n")
	b.WriteString("- `app.html` — root HTML shell with `%sveltego.head%` / `%sveltego.body%` placeholders.\n")
	b.WriteString("- `src/hooks.server.go` — request lifecycle hooks.\n")
	b.WriteString("- `sveltego.config.go` — project config.\n")
	b.WriteString("- `package.json`, `vite.config.js` — Vite client bundle config (consumed by `sveltego build`).\n")
	b.WriteString("- `.gen/` — generated Go from `.svelte` (gitignored).\n")
	return b.String()
}

func renderMainGo(module string) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\t\"log\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"time\"\n\n")
	b.WriteString("\tgen \"")
	b.WriteString(module)
	b.WriteString("/.gen\"\n\n")
	b.WriteString("\t\"github.com/binsarjr/sveltego/packages/sveltego/exports/kit\"\n")
	b.WriteString("\t\"github.com/binsarjr/sveltego/packages/sveltego/server\"\n")
	b.WriteString(")\n\n")
	b.WriteString("func main() {\n")
	b.WriteString("\tshell, err := os.ReadFile(\"app.html\")\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"read app.html: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\tmanifest, err := os.ReadFile(\"static/_app/.vite/manifest.json\")\n")
	b.WriteString("\tif err != nil && !errors.Is(err, os.ErrNotExist) {\n")
	b.WriteString("\t\tlog.Fatalf(\"read vite manifest: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\ts, err := server.New(server.Config{\n")
	b.WriteString("\t\tRoutes:        gen.Routes(),\n")
	b.WriteString("\t\tMatchers:      gen.Matchers(),\n")
	b.WriteString("\t\tShell:         string(shell),\n")
	b.WriteString("\t\tHooks:         gen.Hooks(),\n")
	b.WriteString("\t\tServiceWorker: gen.HasServiceWorker,\n")
	b.WriteString("\t\tViteManifest:  string(manifest),\n")
	b.WriteString("\t\tViteBase:      \"/_app\",\n")
	b.WriteString("\t})\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"server.New: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\tmux := http.NewServeMux()\n")
	b.WriteString("\tmux.Handle(\"/_app/\", http.StripPrefix(\"/_app\", server.StaticHandler(kit.StaticConfig{\n")
	b.WriteString("\t\tDir:  \"static/_app\",\n")
	b.WriteString("\t\tETag: true,\n")
	b.WriteString("\t})))\n")
	b.WriteString("\tmux.Handle(\"/\", s)\n")
	b.WriteString("\taddr := \":3000\"\n")
	b.WriteString("\tlog.Printf(\"listening on %s\", addr)\n")
	b.WriteString("\thttpSrv := &http.Server{\n")
	b.WriteString("\t\tAddr:              addr,\n")
	b.WriteString("\t\tHandler:           mux,\n")
	b.WriteString("\t\tReadHeaderTimeout: 10 * time.Second,\n")
	b.WriteString("\t}\n")
	b.WriteString("\tlog.Fatal(httpSrv.ListenAndServe())\n")
	b.WriteString("}\n")
	return b.String()
}

// projectName derives a package.json-safe name from the module path. It
// takes the last path segment, lowercases it, and strips characters npm
// rejects in package names. Empty results fall back to "sveltego-app".
func projectName(module string) string {
	base := module
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	var out strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out.WriteRune(r)
		}
	}
	name := out.String()
	if name == "" {
		return "sveltego-app"
	}
	return name
}

const gitignoreBody = `.gen/
build/
node_modules/
dist/
static/_app/
*.test
# go build ./cmd/app from project root drops a binary named after cmd/app.
/app
`

const appHTMLBody = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>sveltego app</title>
%sveltego.head%
</head>
<body>
%sveltego.body%
</body>
</html>
`

// tsconfigBody seeds the TypeScript project so Svelte LSP and
// vscode-svelte pick up the auto-generated `_page.svelte.d.ts` /
// `_layout.svelte.d.ts` files (RFC #379 phase 2 typegen output) for
// Svelte-mode routes. The include glob covers .svelte sources plus
// the generated declarations alongside them. Users may extend the
// strictness flags freely; sveltego does not re-read this file.
const tsconfigBody = `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "verbatimModuleSyntax": true,
    "isolatedModules": true,
    "allowJs": true,
    "checkJs": false
  },
  "include": [
    "src/**/*.ts",
    "src/**/*.js",
    "src/**/*.svelte",
    "src/**/*.svelte.d.ts",
    "vite.config.js"
  ]
}
`

const viteConfigBody = `import { svelte } from '@sveltejs/vite-plugin-svelte';

/** @type {import('vite').UserConfig} */
export default {
  plugins: [svelte()],
  build: {
    outDir: 'static/_app',
    manifest: true,
  },
};
`

const configBody = `//go:build sveltego

// Package config holds the sveltego project configuration. The build tool
// reads this file at compile time.
package config

// Config returns the project-level settings consumed by ` + "`sveltego compile`" + `.
func Config() map[string]any {
	return map[string]any{
		"appDir": "_app",
	}
}
`

const hooksBody = `//go:build sveltego

package hooks

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	return resolve(ev)
}
`

const pageSvelteBody = `<script lang="ts">
  let { data } = $props();
</script>

<h1>{data.greeting}</h1>
`

const pageServerBody = `package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type PageData struct {
	Greeting string ` + "`json:\"greeting\"`" + `
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{Greeting: "Hello, sveltego!"}, nil
}
`

const layoutSvelteBody = `<script lang="ts">
  let { children } = $props();
</script>

{@render children()}
`

// serviceWorkerStarter is the opt-in Service Worker scaffold written when
// Options.ServiceWorker is true. The default behavior is intentionally
// conservative: install/activate handlers no-op so the worker takes
// control without breaking pages, and `fetch` falls through to the
// network. Users plug in a caching strategy here. Codegen wires it into
// the Vite build and emits the registration <script> automatically (#89).
const serviceWorkerStarter = `// src/service-worker.ts — sveltego service worker scaffold.
//
// Sveltego auto-detects this file at build time and:
//   - bundles it as a separate Vite Rollup input → /service-worker.js
//   - injects an auto-registration <script> into every SSR page
//
// Customize the install / activate / fetch handlers below to add a
// caching strategy. The default is a no-op pass-through so the worker
// never breaks navigation while you iterate.
//
// References: https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API

/// <reference lib="webworker" />
const sw = self as unknown as ServiceWorkerGlobalScope;

sw.addEventListener('install', (_event) => {
  // Activate this worker immediately, replacing any previous version.
  sw.skipWaiting();
});

sw.addEventListener('activate', (event) => {
  // Take control of every open client without requiring a reload.
  event.waitUntil(sw.clients.claim());
});

sw.addEventListener('fetch', (_event) => {
  // Default: pass through to the network. Plug in a caching strategy here.
});
`
