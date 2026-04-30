package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// RootCmd is the package-level root command used by Execute.
var RootCmd = NewRootCmd()

// Execute runs the root command and returns its error.
func Execute() error {
	return RootCmd.Execute()
}

// NewRootCmd builds a fresh sveltego command tree. Tests use this to avoid
// state bleed across cobra invocations.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sveltego",
		Short: "Pure-Go SvelteKit-shape framework",
		Long:  "sveltego is a pure-Go rewrite of SvelteKit's shape: parser, codegen, runtime, router, and CLI.",
	}

	cmd.SilenceUsage = true
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.PersistentFlags().StringP("config", "c", "", "path to sveltego config (no-op until #21)")
	cmd.PersistentFlags().String("cwd", "", "working directory override (no-op until #21)")
	cmd.PersistentFlags().CountP("verbose", "v", "increase log verbosity (-v info, -vv debug, -vvv debug+source)")

	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		verbose, err := c.Flags().GetCount("verbose")
		if err != nil {
			return err
		}
		configureLogger(verbose)
		return nil
	}

	cmd.AddCommand(
		newBuildCmd(),
		newCompileCmd(),
		newDevCmd(),
		newCheckCmd(),
		newPrerenderCmd(),
		newRoutesCmd(),
		newVersionCmd(),
	)

	return cmd
}

func configureLogger(verbose int) {
	level := slog.LevelWarn
	addSource := false
	switch {
	case verbose >= 3:
		level = slog.LevelDebug
		addSource = true
	case verbose == 2:
		level = slog.LevelDebug
	case verbose == 1:
		level = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	})
	slog.SetDefault(slog.New(handler))
}
