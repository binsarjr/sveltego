package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TailwindFlavor selects how the scaffolder wires Tailwind.
//
//   - TailwindNone leaves Tailwind out of package.json. The generated
//     vite.config.gen.js does not import it.
//   - TailwindV4 writes the @tailwindcss/vite plugin path: app.css uses
//     `@import "tailwindcss";` plus `@source "../**/*.go";` so Tailwind
//     scans Go template literals.
//   - TailwindV3 writes the legacy PostCSS path with tailwind.config.js
//     content globs that include .go and .svelte files.
type TailwindFlavor string

const (
	TailwindNone TailwindFlavor = "none"
	TailwindV4   TailwindFlavor = "v4"
	TailwindV3   TailwindFlavor = "v3"
)

// ParseTailwindFlavor accepts the CLI input. The empty string and
// "none" both map to TailwindNone. Unknown values return an error so a
// typo never silently scaffolds the wrong project.
func ParseTailwindFlavor(s string) (TailwindFlavor, error) {
	switch strings.ToLower(s) {
	case "", "none":
		return TailwindNone, nil
	case "v4", "4", "tailwindcss-v4":
		return TailwindV4, nil
	case "v3", "3", "tailwindcss-v3":
		return TailwindV3, nil
	}
	return "", fmt.Errorf("scaffold: unknown tailwind flavor %q (want v4, v3, or none)", s)
}

// PackageManager names the tool the user should run after scaffolding.
type PackageManager string

const (
	PMnpm  PackageManager = "npm"
	PMpnpm PackageManager = "pnpm"
	PMbun  PackageManager = "bun"
)

// DetectPackageManager picks a tool based on lockfile presence in dir.
// Returns "npm" when no recognised lockfile is found.
func DetectPackageManager(dir string) PackageManager {
	for _, c := range []struct {
		lock string
		pm   PackageManager
	}{
		{"pnpm-lock.yaml", PMpnpm},
		{"bun.lockb", PMbun},
		{"bun.lock", PMbun},
	} {
		if _, err := os.Stat(filepath.Join(dir, c.lock)); err == nil {
			return c.pm
		}
	}
	return PMnpm
}

// InstallCommand returns the install command line, e.g. "npm install".
func (pm PackageManager) InstallCommand() string {
	return string(pm) + " install"
}

// Pinned Tailwind versions. v4 stays on a tested minor while the major
// stabilises; v3 sticks to the last known-good range. Any change here
// also updates the README scaffolded into the new project.
const (
	tailwindV4Range       = "^4.0.0"
	tailwindCSSViteRange  = "^4.0.0"
	tailwindV3Range       = "^3.4.0"
	postcssRange          = "^8.4.0"
	autoprefixerRange     = "^10.4.0"
	viteRange             = "^6.0.0"
	vitePluginSvelteRange = "^5.0.0"
	svelteRange           = "^5.0.0"
)

func tailwindFiles(flavor TailwindFlavor) []file {
	if flavor == TailwindNone {
		return nil
	}
	out := []file{
		{path: "package.json", body: []byte(renderPackageJSON(flavor))},
	}
	switch flavor {
	case TailwindV4:
		out = append(out,
			file{path: "src/app.css", body: []byte(appCSSv4Body)},
		)
	case TailwindV3:
		out = append(out,
			file{path: "src/app.css", body: []byte(appCSSv3Body)},
			file{path: "postcss.config.js", body: []byte(postcssConfigBody)},
			file{path: "tailwind.config.js", body: []byte(tailwindConfigBody)},
		)
	}
	return out
}

// renderLayoutSvelte returns the +layout.svelte body. When Tailwind is
// enabled the layout imports app.css so the bundle picks it up at build
// time. The plain layout has no imports.
func renderLayoutSvelte(flavor TailwindFlavor) string {
	if flavor == TailwindNone {
		return layoutSvelteBody
	}
	var b strings.Builder
	b.WriteString("<script lang=\"go\">\n")
	b.WriteString("  import \"./app.css\"\n")
	b.WriteString("</script>\n\n")
	b.WriteString("<slot />\n")
	return b.String()
}

// renderPagSvelte returns the +page.svelte body. With Tailwind we ship
// a short snippet exercising both a utility class and a scoped style so
// the smoke test confirms coexistence.
func renderPageSvelte(flavor TailwindFlavor) string {
	if flavor == TailwindNone {
		return pageSvelteBody
	}
	var b strings.Builder
	b.WriteString("<script lang=\"go\"></script>\n\n")
	b.WriteString("<h1 class=\"text-3xl font-bold underline\">{Data.Greeting}</h1>\n")
	b.WriteString("<p class=\"note\">Tailwind utilities + scoped &lt;style&gt; coexist.</p>\n\n")
	b.WriteString("<style>\n")
	b.WriteString("  .note { color: rgb(82 82 91); }\n")
	b.WriteString("</style>\n")
	return b.String()
}

func renderPackageJSON(flavor TailwindFlavor) string {
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString("  \"name\": \"sveltego-app\",\n")
	b.WriteString("  \"private\": true,\n")
	b.WriteString("  \"type\": \"module\",\n")
	b.WriteString("  \"scripts\": {\n")
	b.WriteString("    \"dev\": \"vite\",\n")
	b.WriteString("    \"build\": \"sveltego build\",\n")
	b.WriteString("    \"preview\": \"vite preview\"\n")
	b.WriteString("  },\n")
	b.WriteString("  \"devDependencies\": {\n")
	fmt.Fprintf(&b, "    %q: %q,\n", "@sveltejs/vite-plugin-svelte", vitePluginSvelteRange)
	fmt.Fprintf(&b, "    %q: %q,\n", "svelte", svelteRange)
	switch flavor {
	case TailwindV4:
		fmt.Fprintf(&b, "    %q: %q,\n", "@tailwindcss/vite", tailwindCSSViteRange)
		fmt.Fprintf(&b, "    %q: %q,\n", "tailwindcss", tailwindV4Range)
	case TailwindV3:
		fmt.Fprintf(&b, "    %q: %q,\n", "autoprefixer", autoprefixerRange)
		fmt.Fprintf(&b, "    %q: %q,\n", "postcss", postcssRange)
		fmt.Fprintf(&b, "    %q: %q,\n", "tailwindcss", tailwindV3Range)
	}
	fmt.Fprintf(&b, "    %q: %q\n", "vite", viteRange)
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

const appCSSv4Body = `@import "tailwindcss";

@source "../src/**/*.svelte";
@source "../src/**/*.go";
`

const appCSSv3Body = `@tailwind base;
@tailwind components;
@tailwind utilities;
`

const postcssConfigBody = `export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
`

const tailwindConfigBody = `/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './src/**/*.{svelte,go}',
  ],
  theme: { extend: {} },
  plugins: [],
};
`
