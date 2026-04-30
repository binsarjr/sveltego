// Package adapterserver provides a build-time adapter that produces a
// standalone HTTP server binary. It is the reference adapter — sveltego's
// default deploy target since the framework already produces a single
// statically-linked Go binary.
//
// The adapter copies a pre-built binary (and any assets) into an output
// directory so downstream tooling can pick up a deployable artifact. It
// does not invoke `go build` itself; the caller (`sveltego build` or the
// `sveltego-adapter` driver) is responsible for compilation.
package adapterserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Name is the canonical target name for this adapter.
const Name = "server"

// BuildContext describes the inputs an adapter needs to assemble its
// output. The shape is intentionally small; each adapter ignores fields
// it does not use.
type BuildContext struct {
	// ProjectRoot is the absolute path of the user's project (the dir
	// containing go.mod and src/routes/).
	ProjectRoot string

	// BinaryPath is the absolute path to a pre-built sveltego binary the
	// adapter should package. Required for server, docker, and lambda
	// targets; ignored by static and cloudflare.
	BinaryPath string

	// OutputDir is the absolute path where the adapter writes its
	// artifacts. Created if missing.
	OutputDir string

	// AssetsDir, if non-empty, is the absolute path of a directory whose
	// contents should be packaged alongside the binary (typically static
	// public assets). Optional.
	AssetsDir string

	// BinaryName is the basename used for the output binary. Defaults to
	// "sveltego" when empty.
	BinaryName string
}

// Build copies the user's pre-built binary (and optional assets) into
// OutputDir. It is the reference implementation of the Adapter contract.
//
// The function is intentionally minimal: sveltego apps are
// single-binary by construction, so the "server" target is mostly a
// rename + relocate step. Returning an error here surfaces missing or
// unreadable inputs early.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bc.BinaryPath == "" {
		return errors.New("adapter-server: BinaryPath is required")
	}
	if bc.OutputDir == "" {
		return errors.New("adapter-server: OutputDir is required")
	}
	if _, err := os.Stat(bc.BinaryPath); err != nil {
		return fmt.Errorf("adapter-server: binary not found: %w", err)
	}

	if err := os.MkdirAll(bc.OutputDir, 0o755); err != nil {
		return fmt.Errorf("adapter-server: create output dir: %w", err)
	}

	binaryName := bc.BinaryName
	if binaryName == "" {
		binaryName = "sveltego"
	}
	dst := filepath.Join(bc.OutputDir, binaryName)
	if err := copyExecutable(bc.BinaryPath, dst); err != nil {
		return fmt.Errorf("adapter-server: copy binary: %w", err)
	}

	if bc.AssetsDir != "" {
		if _, err := os.Stat(bc.AssetsDir); err == nil {
			if err := copyTree(bc.AssetsDir, filepath.Join(bc.OutputDir, "assets")); err != nil {
				return fmt.Errorf("adapter-server: copy assets: %w", err)
			}
		}
	}
	return nil
}

// Doc returns deploy steps for the server target as plain text (suitable
// for inclusion in a README or printed by the CLI).
func Doc() string {
	return `Server target — single binary

  1. sveltego-adapter build --target=server --binary <path> --out dist/
  2. dist/<binary-name> listens on the address your main passes to
     server.ListenAndServe (typically :3000).
  3. Run anywhere a static Linux/macOS/Windows binary runs.

No external runtime; cross-compile via GOOS/GOARCH.`
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src comes from the caller's BuildContext, not user input
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // dst is OutputDir/<name>
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // src is enumerated from filepath.Walk(AssetsDir)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // dst is under OutputDir
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
