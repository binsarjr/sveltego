// Command sveltego-lsp runs the sveltego Language Server over stdio.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/binsarjr/sveltego/lsp/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sveltego-lsp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	srv := server.New(os.Stdin, os.Stdout, os.Stderr)
	return srv.Serve(ctx)
}
