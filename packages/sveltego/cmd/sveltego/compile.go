package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/internal/codegen"
)

func newCompileCmd() *cobra.Command {
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
			})
			if err != nil {
				return err
			}
			for _, d := range result.Diagnostics {
				fmt.Fprintln(cmd.ErrOrStderr(), "warn:", d.String())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "compiled: %d route(s) -> %s\n", result.Routes, result.ManifestPath)
			return nil
		},
	}
	return cmd
}
