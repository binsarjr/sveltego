package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/internal/codegen"
)

func newBuildCmd() *cobra.Command {
	var (
		outPath string
		mainPkg string
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
			result, err := codegen.Build(codegen.BuildOptions{
				ProjectRoot: root,
				Verbose:     verbose,
			})
			if err != nil {
				return err
			}
			for _, d := range result.Diagnostics {
				fmt.Fprintln(cmd.ErrOrStderr(), "warn:", d.String())
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
