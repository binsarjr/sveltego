package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compile",
		Short: "Compile .svelte templates to Go source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "compile: not implemented in this milestone (#21)")
			return nil
		},
	}
}
