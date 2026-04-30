package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/internal/codegen"
)

// prerenderTriggerEnv is the env var the user binary looks for to
// switch from "serve" to "prerender" mode. The CLI exports it before
// running the freshly-built app.
const prerenderTriggerEnv = "SVELTEGO_PRERENDER"

// prerenderOutDirEnv overrides the output directory the user binary
// writes prerendered HTML into. Optional; defaults to
// server.DefaultPrerenderOutDir when unset.
const prerenderOutDirEnv = "SVELTEGO_PRERENDER_OUT"

// prerenderTolerateEnv mirrors --tolerate at the env-var layer so the
// user binary can read it without re-parsing flags.
const prerenderTolerateEnv = "SVELTEGO_PRERENDER_TOLERATE"

// prerenderReportEnv carries the optional --report=<path> flag through
// to the user binary.
const prerenderReportEnv = "SVELTEGO_PRERENDER_REPORT"

func newPrerenderCmd() *cobra.Command {
	var (
		mainPkg    string
		outDir     string
		tolerate   int
		reportPath string
	)
	cmd := &cobra.Command{
		Use:   "prerender",
		Short: "Generate static HTML for routes opted into prerender",
		Long: "prerender compiles the user app, runs it in prerender mode, and " +
			"writes static HTML for every route declared with Prerender = true " +
			"or <svelte:options prerender>. Errors are aggregated and reported " +
			"at the end (#185).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			verbose := isVerbose(cmd)
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}
			result, err := codegen.Build(codegen.BuildOptions{
				ProjectRoot: root,
				Verbose:     verbose,
				Release:     os.Getenv("SVELTEGO_RELEASE") == "1",
				NoClient:    true,
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

			tmpBin := filepath.Join(root, ".gen", "_prerender.bin")
			if err := os.MkdirAll(filepath.Dir(tmpBin), 0o755); err != nil {
				return fmt.Errorf("mkdir prerender bin dir: %w", err)
			}
			args := []string{"build", "-o", tmpBin}
			if verbose {
				args = append(args, "-v")
			}
			args = append(args, mainPkg)
			gocmd := exec.Command("go", args...) //nolint:gosec
			gocmd.Dir = root
			gocmd.Stdout = cmd.OutOrStdout()
			gocmd.Stderr = cmd.ErrOrStderr()
			if err := gocmd.Run(); err != nil {
				return fmt.Errorf("go build %s: %w", mainPkg, err)
			}
			defer os.Remove(tmpBin)

			runCmd := exec.Command(tmpBin) //nolint:gosec
			runCmd.Dir = root
			runCmd.Stdout = cmd.OutOrStdout()
			runCmd.Stderr = cmd.ErrOrStderr()
			env := os.Environ()
			env = append(env,
				prerenderTriggerEnv+"=1",
				prerenderOutDirEnv+"="+outDir,
				prerenderTolerateEnv+"="+strconv.Itoa(tolerate),
			)
			if reportPath != "" {
				env = append(env, prerenderReportEnv+"="+reportPath)
			}
			runCmd.Env = env
			if err := runCmd.Run(); err != nil {
				return fmt.Errorf("prerender run: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "prerender: done")
			return nil
		},
	}
	cmd.Flags().StringVar(&mainPkg, "main", "./cmd/app", "main package import path or directory")
	cmd.Flags().StringVar(&outDir, "out", "static/_prerendered", "output directory relative to project root")
	cmd.Flags().IntVar(&tolerate, "tolerate", 0, "absorb up to N prerender errors before failing the build (-1 = unlimited)")
	cmd.Flags().StringVar(&reportPath, "report", "", "write JSON error report to this path when failures occur")
	return cmd
}
