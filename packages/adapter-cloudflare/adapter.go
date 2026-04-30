// Package adaptercloudflare provides a build-time adapter that targets
// Cloudflare Workers. It is currently a stub: Workers's Go runtime is
// limited (no full net/http server, restricted stdlib, ~1MB script
// size) and a sveltego app cannot run unmodified. The adapter exposes
// a stable signature so callers can wire it today and have it start
// working when the runtime gap closes (or when a TinyGo + WASI WASM
// shim lands as a follow-up RFC).
package adaptercloudflare

import (
	"context"
	"errors"
	"fmt"
)

// Name is the canonical target name for this adapter.
const Name = "cloudflare"

// ErrNotImplemented is returned by Build. Wrap with errors.Is to
// distinguish from input-validation errors.
var ErrNotImplemented = errors.New("adapter-cloudflare: Cloudflare Workers Go runtime is too restricted for sveltego (no net/http server, ~1MB script limit)")

// BuildContext describes the inputs a future Workers adapter will need.
type BuildContext struct {
	// ProjectRoot is the user's project root.
	ProjectRoot string

	// OutputDir is where wrangler.toml + entry script will land.
	OutputDir string

	// AccountID is the Cloudflare account identifier (optional, only
	// required if the wrangler.toml stub should be fully populated).
	AccountID string
}

// Build returns ErrNotImplemented.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = bc
	return fmt.Errorf("%w: see https://developers.cloudflare.com/workers/languages/go/ for runtime status", ErrNotImplemented)
}

// Doc explains the runtime constraints and points to alternatives.
func Doc() string {
	return `Cloudflare target — Workers (BLOCKED on runtime support)

  Cloudflare's Go runtime currently:

    - Does not expose a full net/http server (only a fetch-style entry).
    - Caps deployed script size at ~1MB compressed (sveltego's
      typical binary is 8–12MB).
    - Restricts the stdlib (no os.Open, no goroutines beyond the
      fetch handler scope).

  A sveltego app cannot run on Workers without a major rewrite. Two
  paths the project may pursue once the gap narrows:

    - TinyGo + WASI shim: compile to WASM, run under Workers' WASM
      runtime. Requires reflect-free codegen.
    - Pages Functions (Node): not in scope (Node-only platform).

  In the meantime, deploy with --target=docker and put Cloudflare in
  front as a CDN (Workers KV / R2 still attachable via your origin).

Track:
  https://github.com/binsarjr/sveltego/issues?q=adapter-cloudflare`
}
