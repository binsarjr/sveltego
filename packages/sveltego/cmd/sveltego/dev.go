package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/devserver"
)

func newDevCmd() *cobra.Command {
	var (
		port     int
		goPort   int
		vitePort int
		mainPkg  string
		noClient bool
	)
	cmd := &cobra.Command{
		Use:   "dev [project-dir]",
		Short: "Run the development server with HMR",
		Long: `dev starts a watching dev server: it codegens once, runs the user's
Go HTTP server as a child process, runs Vite as a sibling for client
HMR, and proxies the browser-facing port between the two. Edits to
.svelte files re-run codegen; edits to .go files re-run codegen and
restart the Go process. SIGINT and SIGTERM trigger a graceful shutdown.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveDevRoot(args)
			if err != nil {
				return err
			}
			opts := devserver.Options{
				ProjectRoot: root,
				MainPkg:     mainPkg,
				Port:        port,
				GoPort:      goPort,
				VitePort:    vitePort,
				NoClient:    noClient,
				Logger:      slog.Default(),
				Stdout:      cmd.OutOrStdout(),
				Stderr:      cmd.ErrOrStderr(),
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return devserver.Run(ctx, opts)
		},
	}
	cmd.Flags().IntVar(&port, "port", 5173, "browser-facing dev server port")
	cmd.Flags().IntVar(&goPort, "go-port", 5174, "internal Go server port")
	cmd.Flags().IntVar(&vitePort, "vite-port", 5175, "internal Vite dev server port")
	cmd.Flags().StringVar(&mainPkg, "main", "./cmd/app", "main package import path or directory")
	cmd.Flags().BoolVar(&noClient, "no-client", false, "skip Vite (server-only mode)")
	return cmd
}

// resolveDevRoot returns an absolute project root. With no positional
// arg the resolver walks up from cwd to find go.mod (matching the
// behaviour of `sveltego build`). With one arg the directory is taken
// verbatim and validated to contain go.mod.
func resolveDevRoot(args []string) (string, error) {
	if len(args) == 0 {
		return resolveProjectRoot()
	}
	abs, err := filepath.Abs(args[0])
	if err != nil {
		return "", fmt.Errorf("dev: resolve %s: %w", args[0], err)
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return "", fmt.Errorf("dev: no go.mod in %s: %w", abs, err)
	}
	return abs, nil
}
