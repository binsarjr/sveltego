package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"sveltego v%s (go%s, %s/%s)\n",
				version.Version, goVersion(), runtime.GOOS, runtime.GOARCH,
			)
			return nil
		},
	}
}

// goVersion returns the Go runtime version without the leading "go" prefix
// so the formatted output reads "go1.22" rather than "gogo1.22".
func goVersion() string {
	return strings.TrimPrefix(runtime.Version(), "go")
}
