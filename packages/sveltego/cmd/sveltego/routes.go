package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

func newRoutesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "routes",
		Short: "List route helpers emitted under .gen/links",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}
			scan, err := scanProjectRoutes(root)
			if err != nil {
				return err
			}
			routes := codegen.SortLinkRoutes(codegen.CollectLinkRoutes(scan))
			return writeRoutesTable(cmd.OutOrStdout(), routes)
		},
	}
	return cmd
}

func scanProjectRoutes(root string) (*routescan.ScanResult, error) {
	routesDir := filepath.Join(root, "src", "routes")
	if info, err := os.Stat(routesDir); err != nil {
		return nil, fmt.Errorf("src/routes/ not found at %s: %w", routesDir, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", routesDir)
	}
	paramsDir := filepath.Join(root, "src", "params")
	if _, err := os.Stat(paramsDir); err != nil {
		paramsDir = ""
	}
	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: routesDir, ParamsDir: paramsDir})
	if err != nil {
		return nil, fmt.Errorf("scan routes: %w", err)
	}
	return scan, nil
}

func writeRoutesTable(out io.Writer, routes []codegen.LinkRoute) error {
	if len(routes) == 0 {
		_, err := fmt.Fprintln(out, "no linkable routes found")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, r := range routes {
		if _, err := fmt.Fprintf(w, "%s\t%s\tlinks.%s\n", r.Method, r.Pattern, r.Helper); err != nil {
			return err
		}
	}
	return w.Flush()
}
