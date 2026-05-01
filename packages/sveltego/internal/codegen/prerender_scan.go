package codegen

import (
	"fmt"
	"os"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
)

// scanPrerenderFromSvelte parses a +page.svelte or +layout.svelte and
// returns the override fragment derived from the file's
// `<svelte:options prerender>` declaration. A missing file or absent
// attribute yields the zero override so callers can fold it into the
// cascade unconditionally. The svelte:options node IS NOT removed from
// the fragment here — full codegen runs separately and re-extracts the
// node via extractSvelteOptions.
func scanPrerenderFromSvelte(path string) (kit.PageOptionsOverride, error) {
	if path == "" {
		return kit.PageOptionsOverride{}, nil
	}
	src, err := os.ReadFile(path) //nolint:gosec // caller-controlled scan path
	if err != nil {
		if os.IsNotExist(err) {
			return kit.PageOptionsOverride{}, nil
		}
		return kit.PageOptionsOverride{}, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		// Surface as a non-fatal: the full codegen pass will produce the
		// authoritative parse error. Returning empty here lets the cascade
		// proceed and the real parser path own the diagnostic.
		return kit.PageOptionsOverride{}, nil
	}
	clone := *frag
	opts, err := extractSvelteOptions(&clone)
	if err != nil {
		return kit.PageOptionsOverride{}, nil
	}
	return prerenderOverrideFromSvelte(opts), nil
}

// prerenderOverrideFromSvelte folds an svelteOptions value into a
// PageOptionsOverride that the cascade can merge alongside server-file
// overrides. Only the prerender axis is set; every other field stays
// zero so non-prerender attributes do not leak into the cascade.
func prerenderOverrideFromSvelte(opts svelteOptions) kit.PageOptionsOverride {
	switch opts.Prerender {
	case "true":
		return kit.PageOptionsOverride{HasPrerender: true, Prerender: true}
	case "false":
		return kit.PageOptionsOverride{HasPrerender: true, Prerender: false}
	case "auto":
		return kit.PageOptionsOverride{HasPrerenderAuto: true, PrerenderAuto: true, HasPrerender: true, Prerender: true}
	case "protected":
		return kit.PageOptionsOverride{
			HasPrerender:          true,
			Prerender:             true,
			HasPrerenderProtected: true,
			PrerenderProtected:    true,
		}
	}
	return kit.PageOptionsOverride{}
}
