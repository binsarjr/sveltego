// Command sveltego-adapter drives the deploy adapters as a standalone
// CLI. It exists so adapters can be exercised end-to-end before the
// `sveltego build --target=<name>` flag lands in the main CLI (Phase
// 0ee owns that integration).
//
// Usage:
//
//	sveltego-adapter build --target=server   --binary <path> --out dist/
//	sveltego-adapter build --target=docker   --out dist/
//	sveltego-adapter build --target=lambda   --module <path> --root <project>
//	sveltego-adapter build --target=static   --root <project> --out dist/
//	sveltego-adapter build --target=cloudflare                     # blocked
//	sveltego-adapter doc   --target=<name>
//	sveltego-adapter targets
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	adapterauto "github.com/binsarjr/sveltego/adapter-auto"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sveltego-adapter: %v\n", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errUsage
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "doc":
		return runDoc(args[1:], stdout, stderr)
	case "targets":
		for _, t := range adapterauto.Targets() {
			fmt.Fprintln(stdout, t)
		}
		return nil
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
	}
}

func runBuild(args []string, _, _ io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("target", "", "deploy target (server, docker, lambda, static, cloudflare)")
	root := fs.String("root", ".", "project root (path to dir containing go.mod)")
	out := fs.String("out", "", "output directory (defaults: server/docker='dist', lambda='<root>/.gen/lambda')")
	binary := fs.String("binary", "", "pre-built binary path (server target)")
	binaryName := fs.String("binary-name", "", "output binary name (defaults to 'sveltego')")
	assets := fs.String("assets", "", "assets directory to copy/reference")
	modulePath := fs.String("module", "", "user's Go module path (lambda target)")
	mainPkg := fs.String("main-package", "", "main package path inside Dockerfile (default './cmd/app')")
	goVersion := fs.String("go-version", "", "Go base image tag (default '1.23')")
	port := fs.Int("port", 0, "port to EXPOSE in Dockerfile (default 8080)")
	handler := fs.String("handler", "", "SAM logical-id (lambda target)")
	memory := fs.Int("memory-mb", 0, "Lambda memory size MB (default 512)")
	timeout := fs.Int("timeout", 0, "Lambda timeout seconds (default 30)")
	failOnDynamic := fs.Bool("fail-on-dynamic", false, "static target: fail if any non-prerenderable route exists")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %s", errUsage, err.Error())
	}
	if *target == "" {
		return fmt.Errorf("%w: --target is required", errUsage)
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve --root: %w", err)
	}
	outAbs := defaultOutDir(*target, *out, rootAbs)
	if outAbs != "" {
		outAbs, err = filepath.Abs(outAbs)
		if err != nil {
			return fmt.Errorf("resolve --out: %w", err)
		}
	}
	binaryAbs := *binary
	if binaryAbs != "" {
		binaryAbs, err = filepath.Abs(binaryAbs)
		if err != nil {
			return fmt.Errorf("resolve --binary: %w", err)
		}
	}
	assetsAbs := *assets
	if assetsAbs != "" {
		assetsAbs, err = filepath.Abs(assetsAbs)
		if err != nil {
			return fmt.Errorf("resolve --assets: %w", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	return adapterauto.Build(ctx, adapterauto.BuildContext{
		Target:         *target,
		ProjectRoot:    rootAbs,
		OutputDir:      outAbs,
		BinaryPath:     binaryAbs,
		BinaryName:     *binaryName,
		AssetsDir:      assetsAbs,
		ModulePath:     *modulePath,
		MainPackage:    *mainPkg,
		GoVersion:      *goVersion,
		Port:           *port,
		HandlerName:    *handler,
		MemoryMB:       *memory,
		TimeoutSeconds: *timeout,
		FailOnDynamic:  *failOnDynamic,
	})
}

// defaultOutDir resolves the output directory when --out is empty.
// server / docker / static / cloudflare default to "dist"; lambda
// defers to the adapter's own default ("<root>/.gen/lambda").
func defaultOutDir(target, explicit, root string) string {
	if explicit != "" {
		return explicit
	}
	switch target {
	case "lambda":
		return ""
	default:
		return filepath.Join(root, "dist")
	}
}

func runDoc(args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("doc", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("target", "", "deploy target")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %s", errUsage, err.Error())
	}
	if *target == "" {
		return fmt.Errorf("%w: --target is required", errUsage)
	}
	doc, err := adapterauto.Doc(*target)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, doc)
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(`
sveltego-adapter — deploy adapter driver

Commands:
  build --target=<name> [flags]   Build for the named target
  doc   --target=<name>           Print deploy steps for the target
  targets                          List known target names
  help                             Show this message

Targets:
  server, docker, lambda, static, cloudflare (blocked)

Examples:
  sveltego-adapter build --target=server --binary ./app --out ./dist
  sveltego-adapter build --target=docker --out ./dist --port 8080
  sveltego-adapter build --target=lambda --module github.com/me/app --root .
`))
}
