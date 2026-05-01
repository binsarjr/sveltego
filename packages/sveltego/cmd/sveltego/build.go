package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen"
)

func newBuildCmd() *cobra.Command {
	var (
		outPath  string
		mainPkg  string
		release  bool
		noClient bool
	)
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Compile templates and build production binary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			verbose := isVerbose(cmd)
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}
			if err := ensureSveltegoRequire(cmd, root, verbose); err != nil {
				return err
			}
			result, err := codegen.Build(cmd.Context(), codegen.BuildOptions{
				ProjectRoot: root,
				Verbose:     verbose,
				Release:     release || os.Getenv("SVELTEGO_RELEASE") == "1",
				NoClient:    noClient,
			})
			if err != nil {
				return err
			}
			for _, d := range result.Diagnostics {
				fmt.Fprintln(cmd.ErrOrStderr(), "warn:", d.String())
			}
			if err := codegen.EmitLinksFile(root, ""); err != nil {
				return fmt.Errorf("emit links: %w", err)
			}

			if result.ViteConfigPath != "" {
				if err := runViteBuild(cmd, root, result.ViteConfigPath, verbose); err != nil {
					return err
				}
			}

			outAbs := outPath
			if !filepath.IsAbs(outAbs) {
				outAbs = filepath.Join(root, outAbs)
			}
			if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(outAbs), err)
			}

			args := []string{"build", "-o", outAbs}
			if verbose {
				args = append(args, "-v")
			}
			args = append(args, mainPkg)

			gocmd := exec.Command("go", args...) //nolint:gosec // args composed from validated flags
			gocmd.Dir = root
			gocmd.Stdout = cmd.OutOrStdout()
			gocmd.Stderr = cmd.ErrOrStderr()
			if err := gocmd.Run(); err != nil {
				return fmt.Errorf("go build %s: %w", mainPkg, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "built:", outAbs)
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "build/app", "output binary path (relative to project root or absolute)")
	cmd.Flags().StringVar(&mainPkg, "main", "./cmd/app", "main package import path or directory")
	cmd.Flags().BoolVar(&release, "release", false, "production build: reject $lib/dev/** imports (also set by SVELTEGO_RELEASE=1)")
	cmd.Flags().BoolVar(&noClient, "no-client", false, "skip Vite client bundle step (server-only mode)")
	return cmd
}

// isVerbose reports whether the persistent --verbose count flag is at
// least 1. Build subcommands surface this both to the codegen driver
// (via [codegen.BuildOptions.Verbose]) and to `go build -v`.
func isVerbose(cmd *cobra.Command) bool {
	count, err := cmd.Flags().GetCount("verbose")
	if err != nil {
		return false
	}
	return count > 0
}

// resolveProjectRoot walks up from the current working directory until a
// go.mod is found. The returned path is absolute. An error is returned
// when no go.mod sits on the path to filesystem root.
func resolveProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("project root not found: no go.mod between cwd and filesystem root")
		}
		dir = parent
	}
}

// runViteBuild shells out to the detected package manager to run `vite build`.
// Node must be installed; a clear error is returned when not found on PATH.
func runViteBuild(cmd *cobra.Command, root, viteConfig string, verbose bool) error {
	pm := detectPackageManager(root)

	// Auto-install if package.json is present but node_modules is absent.
	pkgJSON := filepath.Join(root, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		if _, err := os.Stat(filepath.Join(root, "node_modules")); errors.Is(err, os.ErrNotExist) {
			if verbose {
				fmt.Fprintln(cmd.OutOrStdout(), "vite: installing dependencies via", pm)
			}
			install := exec.Command(pm, "install") //nolint:gosec
			install.Dir = root
			install.Stdout = cmd.OutOrStdout()
			install.Stderr = cmd.ErrOrStderr()
			if err := install.Run(); err != nil {
				return fmt.Errorf("vite: %s install: %w", pm, err)
			}
		}
	}

	viteArgs := []string{"vite", "build", "--config", viteConfig}
	var vitecmd *exec.Cmd
	switch pm {
	case "pnpm":
		vitecmd = exec.Command("pnpm", append([]string{"exec"}, viteArgs...)...) //nolint:gosec
	case "bun":
		vitecmd = exec.Command("bun", append([]string{"x"}, viteArgs...)...) //nolint:gosec
	default:
		vitecmd = exec.Command("npx", viteArgs...) //nolint:gosec
	}
	vitecmd.Dir = root
	vitecmd.Stdout = cmd.OutOrStdout()
	vitecmd.Stderr = cmd.ErrOrStderr()
	if err := vitecmd.Run(); err != nil {
		return fmt.Errorf("vite build: %w — ensure Node >=18 and @sveltejs/vite-plugin-svelte are installed", err)
	}
	return nil
}

// sveltegoModulePath is the canonical Go module path of the framework
// runtime. The fresh-scaffold go.mod (init #110) intentionally omits the
// require line so the proxy can resolve whatever pseudo-version is
// current. ensureSveltegoRequire bridges that gap on first build.
const sveltegoModulePath = "github.com/binsarjr/sveltego/packages/sveltego"

// sveltegoRequireRef is the version selector passed to `go get` when the
// require line is missing. Pinned to `@main` until release-please cuts a
// real tag: `@latest` resolves to the bootstrap pseudo-version whose
// module path mismatches the post-#378 layout, so the proxy silently
// drops the require directive (#413). `@main` always resolves the
// current commit on the default branch.
const sveltegoRequireRef = "@main"

// ensureSveltegoRequire seeds the project's go.mod with a require line for
// the framework runtime when one is missing. The fresh-scaffold (#110)
// emits a bare `module ... / go 1.23` clause so we do not pin a literal
// `v0.0.0` that the proxy cannot resolve. On the first `sveltego build`,
// shell out to `go get <module>@main` to let the proxy fill in the
// current pseudo-version and seed go.sum at the same time.
//
// A go.mod that already requires the framework module is left untouched
// so subsequent builds do not chase a moving target.
func ensureSveltegoRequire(cmd *cobra.Command, root string, verbose bool) error {
	goModPath := filepath.Join(root, "go.mod")
	body, err := os.ReadFile(goModPath) //nolint:gosec // root is resolved from cwd
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	if hasRequire(body, sveltegoModulePath) {
		return nil
	}
	if verbose {
		fmt.Fprintf(cmd.OutOrStdout(), "go.mod: adding require %s%s\n", sveltegoModulePath, sveltegoRequireRef)
	}
	getCmd := exec.Command("go", "get", sveltegoModulePath+sveltegoRequireRef) //nolint:gosec // module path and ref are fixed constants
	getCmd.Dir = root
	getCmd.Stdout = cmd.OutOrStdout()
	getCmd.Stderr = cmd.ErrOrStderr()
	if err := getCmd.Run(); err != nil {
		return fmt.Errorf("go get %s%s: %w", sveltegoModulePath, sveltegoRequireRef, err)
	}
	body, err = os.ReadFile(goModPath) //nolint:gosec // root is resolved from cwd
	if err != nil {
		return fmt.Errorf("re-read go.mod after go get: %w", err)
	}
	if !hasRequire(body, sveltegoModulePath) {
		return fmt.Errorf("go get %s%s succeeded but go.mod still missing require %s", sveltegoModulePath, sveltegoRequireRef, sveltegoModulePath)
	}
	return nil
}

// hasRequire reports whether goModBody declares a require directive for
// the given module path. The check is line-based and tolerates both the
// single-line `require <path> v1` and the parenthesized block form.
func hasRequire(goModBody []byte, modulePath string) bool {
	for _, line := range strings.Split(string(goModBody), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// `require github.com/foo/bar v1.2.3`
		if strings.HasPrefix(trimmed, "require "+modulePath) || strings.HasPrefix(trimmed, "require\t"+modulePath) {
			return true
		}
		// Inside a `require ( ... )` block: bare `<module> v1.2.3` lines.
		if strings.HasPrefix(trimmed, modulePath+" ") || strings.HasPrefix(trimmed, modulePath+"\t") {
			return true
		}
	}
	return false
}

// detectPackageManager returns "pnpm", "bun", or "npm" based on lockfile
// presence. The SVELTEGO_PM env var overrides detection.
func detectPackageManager(root string) string {
	if pm := os.Getenv("SVELTEGO_PM"); pm != "" {
		return pm
	}
	for _, c := range []struct {
		lock string
		pm   string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
	} {
		if _, err := os.Stat(filepath.Join(root, c.lock)); err == nil {
			return c.pm
		}
	}
	return "npm"
}
