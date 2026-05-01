package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen"
)

func newCompileCmd() *cobra.Command {
	var release bool
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile .svelte templates to Go source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			verbose := isVerbose(cmd)
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}
			result, err := codegen.Build(codegen.BuildOptions{
				ProjectRoot: root,
				Verbose:     verbose,
				Release:     release || os.Getenv("SVELTEGO_RELEASE") == "1",
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
			fmt.Fprintf(cmd.OutOrStdout(), "compiled: %d route(s) -> %s\n", result.Routes, result.ManifestPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&release, "release", false, "production build: reject $lib/dev/** imports (also set by SVELTEGO_RELEASE=1)")
	return cmd
}
