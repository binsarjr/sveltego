// Package adapterdocker provides a build-time adapter that emits a
// Dockerfile and .dockerignore alongside the user's binary so the
// project can be containerized with a single `docker build`.
//
// The adapter does NOT invoke `docker build`; the caller is expected
// to run the docker CLI in OutputDir. This keeps the adapter free of
// any runtime dependency on the Docker daemon.
package adapterdocker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/adapter-docker/internal/fsutil"
)

// Name is the canonical target name for this adapter.
const Name = "docker"

// BuildContext describes the inputs the docker adapter needs.
type BuildContext struct {
	// ProjectRoot is the absolute path of the user's project. The
	// generated Dockerfile is written here.
	ProjectRoot string

	// OutputDir is where the artefacts (Dockerfile, .dockerignore, and
	// optional pre-built binary copy) live. Created if missing.
	OutputDir string

	// BinaryName is the basename used inside the container. Defaults to
	// "sveltego" when empty.
	BinaryName string

	// MainPackage is the Go package path passed to `go build` inside the
	// container. Defaults to "./cmd/app".
	MainPackage string

	// AssetsDir, if non-empty (relative to ProjectRoot), is COPY'd into
	// /app/assets in the runtime image. Optional.
	AssetsDir string

	// GoVersion is the golang base image tag. Defaults to "1.23".
	GoVersion string

	// Port is the port the binary listens on, written into EXPOSE.
	// Defaults to 8080.
	Port int
}

// Build emits Dockerfile and .dockerignore into OutputDir using the
// fields of bc.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bc.OutputDir == "" {
		return errors.New("adapter-docker: OutputDir is required")
	}
	if err := os.MkdirAll(bc.OutputDir, 0o755); err != nil {
		return fmt.Errorf("adapter-docker: create output dir: %w", err)
	}

	bc.applyDefaults()

	dockerfile := renderDockerfile(bc)
	if err := fsutil.WriteFile(filepath.Join(bc.OutputDir, "Dockerfile"), dockerfile, 0o644); err != nil {
		return fmt.Errorf("adapter-docker: write Dockerfile: %w", err)
	}
	if err := fsutil.WriteFile(filepath.Join(bc.OutputDir, ".dockerignore"), dockerignoreTemplate, 0o644); err != nil {
		return fmt.Errorf("adapter-docker: write .dockerignore: %w", err)
	}
	return nil
}

// Doc returns deploy steps for the docker target.
func Doc() string {
	return `Docker target — multi-stage build → distroless runtime

  1. sveltego-adapter build --target=docker --out .
     Emits Dockerfile + .dockerignore in the current directory.
  2. docker build -t myapp:latest .
  3. docker run --rm -p 8080:8080 myapp:latest

The runtime image is gcr.io/distroless/static-debian12:nonroot
(~2MB, no shell). HEALTHCHECK calls the binary with --healthcheck;
add a corresponding flag handler in main.go OR replace the directive
with one that hits /healthz over HTTP. The framework does not generate
a /healthz handler — add it via src/routes/healthz/server.go.`
}

func (bc *BuildContext) applyDefaults() {
	if bc.BinaryName == "" {
		bc.BinaryName = "sveltego"
	}
	if bc.MainPackage == "" {
		bc.MainPackage = "./cmd/app"
	}
	if bc.GoVersion == "" {
		bc.GoVersion = "1.23"
	}
	if bc.Port == 0 {
		bc.Port = 8080
	}
}

func renderDockerfile(bc BuildContext) string {
	assetsCopy := ""
	if bc.AssetsDir != "" {
		assetsCopy = fmt.Sprintf("COPY --from=builder /src/%s /app/assets", bc.AssetsDir)
	}
	out := dockerfileTemplate
	out = strings.ReplaceAll(out, "{{GoVersion}}", bc.GoVersion)
	out = strings.ReplaceAll(out, "{{BinaryName}}", bc.BinaryName)
	out = strings.ReplaceAll(out, "{{MainPackage}}", bc.MainPackage)
	out = strings.ReplaceAll(out, "{{AssetsCopy}}", assetsCopy)
	out = strings.ReplaceAll(out, "{{Port}}", strconv.Itoa(bc.Port))
	return out
}
