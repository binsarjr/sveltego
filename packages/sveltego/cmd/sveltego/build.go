package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Compile templates and build production binary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "build: not implemented in this milestone (#21)")
			return nil
		},
	}
}
