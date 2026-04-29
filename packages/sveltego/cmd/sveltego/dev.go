package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDevCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev",
		Short: "Run the development server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "dev: not implemented (deferred to v0.3, #42)")
			return nil
		},
	}
}
