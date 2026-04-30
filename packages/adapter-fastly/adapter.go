// Package adapterfastly provides a build-time adapter that targets
// Fastly Compute@Edge. Fastly Compute uses TinyGo to compile Go to
// WebAssembly; the resulting .wasm is uploaded to Fastly's Compute
// platform via the `fastly` CLI.
//
// The adapter orchestrates the `tinygo build` invocation that produces
// the Wasm artifact and emits a fastly.toml stub alongside it so the
// Fastly CLI can deploy the package without further manual setup.
//
// # TinyGo requirement
//
// TinyGo (https://tinygo.org) must be installed and on PATH. If it is
// absent, Build returns ErrTinyGoMissing. Install via:
//
//	brew install tinygo          # macOS
//	scoop install tinygo         # Windows
//	snap install tinygo --classic # Linux
//
// Minimum tested version: 0.32.0 (wasm target "wasi" renamed to "wasip1"
// in TinyGo ≥ 0.31).
package adapterfastly

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// Name is the canonical target name for this adapter.
const Name = "fastly"

// ErrTinyGoMissing is returned when the tinygo executable cannot be
// found on PATH. Wrap with errors.Is to distinguish from other errors.
var ErrTinyGoMissing = errors.New("adapter-fastly: tinygo not found on PATH — see https://tinygo.org/getting-started/install/")

// BuildContext describes the inputs needed to assemble a Fastly
// Compute@Edge package.
type BuildContext struct {
	// ProjectRoot is the absolute path of the user's project (the dir
	// containing go.mod and src/routes/).
	ProjectRoot string

	// EntryPoint is the absolute path to the main package that should be
	// compiled with TinyGo. Defaults to ProjectRoot when empty.
	EntryPoint string

	// OutputDir is the absolute path where the adapter writes its
	// artifacts (.wasm + fastly.toml). Created if missing.
	OutputDir string

	// ServiceName is embedded in the generated fastly.toml. Defaults to
	// "sveltego-app" when empty.
	ServiceName string

	// TinyGoPath overrides the tinygo binary location. When empty, the
	// adapter searches PATH.
	TinyGoPath string
}

// Build compiles the project to a Fastly Compute@Edge Wasm artifact and
// writes a fastly.toml stub into OutputDir.
//
// The compilation step requires TinyGo on PATH (or BuildContext.TinyGoPath).
// If TinyGo is absent, Build returns ErrTinyGoMissing without touching the
// output directory. All other errors are wrapped with context so the caller
// can use errors.Is / errors.As.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bc.OutputDir == "" {
		return errors.New("adapter-fastly: OutputDir is required")
	}
	if bc.ProjectRoot == "" {
		return errors.New("adapter-fastly: ProjectRoot is required")
	}

	tinygo, err := resolveTinyGo(bc.TinyGoPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(bc.OutputDir, 0o755); err != nil {
		return fmt.Errorf("adapter-fastly: create output dir: %w", err)
	}

	wasmOut := filepath.Join(bc.OutputDir, "main.wasm")
	if err := compileTinyGo(ctx, tinygo, bc.entryPoint(), wasmOut); err != nil {
		return fmt.Errorf("adapter-fastly: tinygo build: %w", err)
	}

	if err := writeFastlyTOML(bc.OutputDir, bc.serviceName(), wasmOut); err != nil {
		return fmt.Errorf("adapter-fastly: write fastly.toml: %w", err)
	}

	return nil
}

// Doc returns a plain-text deploy guide for the Fastly Compute target.
func Doc() string {
	return `Fastly Compute@Edge target — TinyGo Wasm

  Prerequisites:
    - TinyGo ≥ 0.32.0  (https://tinygo.org/getting-started/install/)
    - Fastly CLI        (https://developer.fastly.com/reference/cli/)

  Build:
    sveltego-adapter build --target=fastly --out dist/

  Deploy:
    fastly compute deploy --package dist/

  Wasm target: wasip1 (WASI preview 1), the Fastly Compute Wasm ABI.

  The adapter emits:
    dist/
      main.wasm        — TinyGo-compiled Wasm binary (wasip1 target)
      fastly.toml      — Fastly package manifest (edit before first deploy)

  Static assets:
    Fastly KV Store is the recommended mechanism for serving static files
    from Compute. Wire a KV store named "assets" in fastly.toml and the
    sveltego handler will resolve requests against it.

  Follow-up issues:
    - KV-backed static asset serving       (#65 prerender / SSG fallback)
    - Full smoke-test against Fastly local  (#191)

Track:
  https://github.com/binsarjr/sveltego/issues/191`
}

// resolveTinyGo returns the absolute path to the tinygo binary, returning
// ErrTinyGoMissing when it cannot be located.
func resolveTinyGo(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("%w: explicit path %q: %w", ErrTinyGoMissing, override, err)
		}
		return override, nil
	}
	path, err := exec.LookPath("tinygo")
	if err != nil {
		return "", ErrTinyGoMissing
	}
	return path, nil
}

// compileTinyGo runs:
//
//	tinygo build -o <wasmOut> -target wasip1 -scheduler none <entryPoint>
//
// The wasip1 target is the Fastly Compute Wasm ABI (WASI preview 1).
// -scheduler none disables goroutine scheduling, which is unsupported
// inside Fastly's Wasm sandbox.
//
// TODO(tinygo): once the runtime gap with net/http narrows (goroutines,
// net, os.Stat) switch -scheduler from none to asyncify and remove the
// stub handler shim from src/routes/hooks.server.go.
func compileTinyGo(ctx context.Context, tinygo, entryPoint, wasmOut string) error {
	//nolint:gosec // tinygo path is resolved via exec.LookPath or explicit caller override
	cmd := exec.CommandContext(ctx, tinygo, "build",
		"-o", wasmOut,
		"-target", "wasip1",
		"-scheduler", "none",
		entryPoint,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// fastlyTomlTmpl is a minimal fastly.toml manifest. The caller must fill
// in [setup.kv_stores] manually for production use.
var fastlyTomlTmpl = template.Must(template.New("fastly.toml").Parse(`# Generated by adapter-fastly. Edit before deploying to production.
# Docs: https://developer.fastly.com/reference/compute/fastly-toml/
manifest_version = 2

[package]
name        = "{{.ServiceName}}"
description = "sveltego Compute@Edge application"
language    = "other"

[scripts]
build = "true"  # build is handled by sveltego-adapter; Fastly CLI skips re-compilation

[[setup.object_stores]]
# Rename or remove if not using KV-backed static assets.
name = "assets"
`))

type tomlData struct {
	ServiceName string
}

func writeFastlyTOML(outputDir, serviceName, wasmOut string) error {
	dst := filepath.Join(outputDir, "fastly.toml")
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // dst is under the adapter's OutputDir
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	_ = wasmOut // referenced in future KV-store wiring
	return fastlyTomlTmpl.Execute(f, tomlData{ServiceName: serviceName})
}

func (bc BuildContext) entryPoint() string {
	if bc.EntryPoint != "" {
		return bc.EntryPoint
	}
	return bc.ProjectRoot
}

func (bc BuildContext) serviceName() string {
	if bc.ServiceName != "" {
		return bc.ServiceName
	}
	return "sveltego-app"
}
