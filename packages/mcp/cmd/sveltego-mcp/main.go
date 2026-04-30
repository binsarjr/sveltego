// Command sveltego-mcp runs the sveltego Model Context Protocol server
// over stdio. AI clients (Claude Desktop, Cursor, Continue) launch this
// binary and exchange JSON-RPC messages on stdin/stdout to query
// sveltego docs, look up runtime APIs, fetch examples, scaffold routes,
// and validate template snippets.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/binsarjr/sveltego/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sveltego-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("sveltego-mcp", flag.ContinueOnError)
	root := fs.String("root", "", "sveltego repo root (auto-detected when empty)")
	docs := fs.String("docs", "", "documentation directory (defaults to <root>/documentation/docs)")
	kit := fs.String("kit", "", "kit package directory (defaults to <root>/packages/sveltego/exports/kit)")
	plays := fs.String("playgrounds", "", "playgrounds directory (defaults to <root>/playgrounds)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	cfg := mcp.Config{
		Root:           *root,
		DocsDir:        *docs,
		KitDir:         *kit,
		PlaygroundsDir: *plays,
	}
	cfg = cfg.WithDefaults()

	srv := mcp.New(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return srv.ServeStdio(ctx, os.Stdin, os.Stdout)
}
