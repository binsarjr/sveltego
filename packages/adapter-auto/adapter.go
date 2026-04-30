// Package adapterauto auto-detects the deploy target from environment
// hints and dispatches to the matching adapter. It does not implement
// build logic itself; it imports each per-target adapter and forwards
// to its Build function.
//
// The detection rules favour explicit signal over guesswork:
//
//	SVELTEGO_ADAPTER set      → use that name verbatim
//	AWS_LAMBDA_RUNTIME_API set → "lambda"
//	CF_PAGES set              → "cloudflare"
//	otherwise                 → "server"
package adapterauto

import (
	"context"
	"errors"
	"fmt"
	"os"

	adaptercloudflare "github.com/binsarjr/sveltego/adapter-cloudflare"
	adapterdocker "github.com/binsarjr/sveltego/adapter-docker"
	adapterlambda "github.com/binsarjr/sveltego/adapter-lambda"
	adapterserver "github.com/binsarjr/sveltego/adapter-server"
	adapterstatic "github.com/binsarjr/sveltego/adapter-static"
)

// BuildContext is a superset of every per-adapter BuildContext. The
// dispatcher copies only the fields the chosen adapter needs.
type BuildContext struct {
	Target         string
	ProjectRoot    string
	OutputDir      string
	BinaryPath     string
	BinaryName     string
	AssetsDir      string
	ModulePath     string
	MainPackage    string
	GoVersion      string
	Port           int
	HandlerName    string
	MemoryMB       int
	TimeoutSeconds int
}

// ErrUnknownTarget is returned when Target does not match any known
// adapter name.
var ErrUnknownTarget = errors.New("adapter-auto: unknown target")

// Detect returns the adapter Name implied by the current environment.
// Override by setting SVELTEGO_ADAPTER directly.
func Detect() string {
	if name := os.Getenv("SVELTEGO_ADAPTER"); name != "" {
		return name
	}
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		return adapterlambda.Name
	}
	if os.Getenv("CF_PAGES") != "" {
		return adaptercloudflare.Name
	}
	return adapterserver.Name
}

// Build resolves bc.Target (or the Detect() result if empty) and
// forwards to the matching adapter.
func Build(ctx context.Context, bc BuildContext) error {
	target := bc.Target
	if target == "" {
		target = Detect()
	}
	switch target {
	case adapterserver.Name:
		return adapterserver.Build(ctx, adapterserver.BuildContext{
			ProjectRoot: bc.ProjectRoot,
			BinaryPath:  bc.BinaryPath,
			OutputDir:   bc.OutputDir,
			AssetsDir:   bc.AssetsDir,
			BinaryName:  bc.BinaryName,
		})
	case adapterdocker.Name:
		return adapterdocker.Build(ctx, adapterdocker.BuildContext{
			ProjectRoot: bc.ProjectRoot,
			OutputDir:   bc.OutputDir,
			BinaryName:  bc.BinaryName,
			MainPackage: bc.MainPackage,
			AssetsDir:   bc.AssetsDir,
			GoVersion:   bc.GoVersion,
			Port:        bc.Port,
		})
	case adapterlambda.Name:
		return adapterlambda.Build(ctx, adapterlambda.BuildContext{
			ProjectRoot:    bc.ProjectRoot,
			OutputDir:      bc.OutputDir,
			ModulePath:     bc.ModulePath,
			HandlerName:    bc.HandlerName,
			MemoryMB:       bc.MemoryMB,
			TimeoutSeconds: bc.TimeoutSeconds,
		})
	case adapterstatic.Name:
		return adapterstatic.Build(ctx, adapterstatic.BuildContext{
			ProjectRoot: bc.ProjectRoot,
			OutputDir:   bc.OutputDir,
		})
	case adaptercloudflare.Name:
		return adaptercloudflare.Build(ctx, adaptercloudflare.BuildContext{
			ProjectRoot: bc.ProjectRoot,
			OutputDir:   bc.OutputDir,
		})
	default:
		return fmt.Errorf("%w: %q (known: server, docker, lambda, static, cloudflare)", ErrUnknownTarget, target)
	}
}

// Doc returns the deploy steps for the given target name, or
// ErrUnknownTarget wrapped in an error string when the name is unknown.
func Doc(target string) (string, error) {
	switch target {
	case adapterserver.Name:
		return adapterserver.Doc(), nil
	case adapterdocker.Name:
		return adapterdocker.Doc(), nil
	case adapterlambda.Name:
		return adapterlambda.Doc(), nil
	case adapterstatic.Name:
		return adapterstatic.Doc(), nil
	case adaptercloudflare.Name:
		return adaptercloudflare.Doc(), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownTarget, target)
	}
}

// Targets enumerates every known target name in stable order.
func Targets() []string {
	return []string{
		adapterserver.Name,
		adapterdocker.Name,
		adapterlambda.Name,
		adapterstatic.Name,
		adaptercloudflare.Name,
	}
}
