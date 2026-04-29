package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Type-check the project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "check: not implemented in this milestone")
			return nil
		},
	}
}
