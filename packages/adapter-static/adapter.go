// Package adapterstatic provides a build-time adapter that produces
// prerendered static output (SSG). It is currently a stub: the adapter
// depends on issue #65 (prerender mode) to walk every route, render its
// HTML at build time, and write the result to disk. Until #65 lands,
// Build returns an error so callers fail fast rather than producing
// silently empty output.
package adapterstatic

import (
	"context"
	"errors"
	"fmt"
)

// Name is the canonical target name for this adapter.
const Name = "static"

// ErrNotImplemented is returned by Build until prerender mode (#65)
// ships. Wrap with errors.Is to distinguish from input-validation
// errors.
var ErrNotImplemented = errors.New("adapter-static: static target requires prerender mode (#65), not yet shipped")

// BuildContext describes the inputs a future static adapter will need.
// The shape is reserved so the public API is stable across the stub →
// implementation transition.
type BuildContext struct {
	// ProjectRoot is the user's project root.
	ProjectRoot string

	// OutputDir is where the rendered .html / asset tree will land.
	OutputDir string

	// FailOnDynamic, when true, fails the build if any route is not
	// fully prerenderable. The implementation will surface the offending
	// route paths.
	FailOnDynamic bool
}

// Build returns ErrNotImplemented. The signature is stable so callers
// can wire the adapter today and have it start working when #65 lands.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = bc
	return fmt.Errorf("%w: track https://github.com/binsarjr/sveltego/issues/65", ErrNotImplemented)
}

// Doc explains the current blocker and how to migrate once prerender
// mode lands.
func Doc() string {
	return `Static target — SSG (BLOCKED on #65 prerender)

  The static adapter walks every route, renders its HTML at build time,
  and writes the result to OutputDir as a flat tree of .html files +
  hashed assets. It depends on issue #65 (prerender mode), which adds
  a per-route Prerender flag and a build-time crawler that resolves
  every load parameter.

Until #65 ships:

  - Use --target=server and put a CDN (Cloudflare, Bunny, Fastly) in
    front of the binary.
  - Or render selected routes manually via httptest and ship the bytes
    to S3 / Pages / Netlify Edge.

Track:
  https://github.com/binsarjr/sveltego/issues/65`
}
