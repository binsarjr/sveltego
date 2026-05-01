package kit

// TrailingSlash configures how the pipeline normalizes a request path's
// trailing "/" before resolving a route. Default leaves the request
// unchanged; Never strips a trailing slash via 308 redirect (except for
// "/"); Always appends a trailing slash via 308 redirect; Ignore disables
// any redirect — both forms reach the same route.
type TrailingSlash uint8

const (
	// TrailingSlashDefault behaves like TrailingSlashNever. The zero
	// value is named so manifest emission is unambiguous.
	TrailingSlashDefault TrailingSlash = iota
	// TrailingSlashNever redirects /foo/ to /foo with 308.
	TrailingSlashNever
	// TrailingSlashAlways redirects /foo to /foo/ with 308.
	TrailingSlashAlways
	// TrailingSlashIgnore disables trailing-slash normalization.
	TrailingSlashIgnore
)

// String returns the canonical token name for ts. The names match the
// constants users write in src/routes/**/page.server.go so log messages
// round-trip cleanly.
func (ts TrailingSlash) String() string {
	switch ts {
	case TrailingSlashNever:
		return "never"
	case TrailingSlashAlways:
		return "always"
	case TrailingSlashIgnore:
		return "ignore"
	default:
		return "default"
	}
}

// PageOptions carries the page- and layout-level settings declared as
// exported constants in *.server.go: Prerender, SSR, CSR, SSROnly,
// TrailingSlash, and CSRF. Layout-level values cascade to descendants;
// page-level values override the cascade.
//
// SSR, CSR, and CSRF default to true; Prerender and SSROnly default to
// false. Manifest emission stores the effective resolved value per
// route, so the pipeline does not re-walk the layout chain at request
// time.
//
// Prerender selects build-time HTML generation. PrerenderAuto, when
// true, switches the build to generate static HTML only when the route
// has no dynamic params and no server load — otherwise the route renders
// at request time. PrerenderProtected (#187) instructs the prerender
// pipeline to still emit the static HTML, but the runtime mux gates it
// behind the configured PrerenderAuthGate before serving.
//
// ImageWidths configures the variant widths produced by the build-time
// image pipeline for <Image src="..."> elements (#92). Empty applies the
// framework default (320, 640, 1280); the field is global rather than
// per-route because the variants live in a shared static/_app/immutable/
// pool and cache best when the width set is stable. Only the project
// root's default value is consulted; per-route overrides are ignored
// because the pool is not partitioned by route.
//
// Templates picks the per-route template pipeline (RFC #379 phase 3):
//   - "go-mustache" (Phase 3 default for backward compat): codegen parses
//     the .svelte body and emits Go SSR (Mustache-Go expressions).
//   - "svelte": codegen leaves the .svelte body for Vite + Svelte to
//     compile; the server returns the app.html shell plus a JSON
//     hydration payload, the client mounts and renders. Routes with
//     Prerender: true plus Templates: "svelte" are rendered to static
//     HTML at build time via a Node `svelte/server` sidecar; the runtime
//     stays JS-free.
//
// Phase 5 (#384) flips the default to "svelte" and removes the
// Mustache-Go path. Empty Templates defaults to "go-mustache" until
// then.
type PageOptions struct {
	Prerender          bool
	PrerenderAuto      bool
	PrerenderProtected bool
	SSR                bool
	CSR                bool
	SSROnly            bool
	CSRF               bool
	TrailingSlash      TrailingSlash
	ImageWidths        []int
	Templates          string
}

// TemplatesGoMustache is the legacy Mustache-Go template pipeline
// (Phase 3 default for backward compat). The .svelte body is parsed
// at codegen time and emitted as Go SSR.
const TemplatesGoMustache = "go-mustache"

// TemplatesSvelte is the pure-Svelte template pipeline (RFC #379).
// The .svelte body is left for Vite + Svelte to compile; the server
// returns app.html plus a JSON hydration payload.
const TemplatesSvelte = "svelte"

// DefaultPageOptions returns the framework defaults: SSR, CSR, and CSRF
// on, Prerender off, TrailingSlash never, Templates "go-mustache".
// Codegen seeds the cascade with this value so a project that declares
// no options gets the same behavior as today.
func DefaultPageOptions() PageOptions {
	return PageOptions{
		SSR:           true,
		CSR:           true,
		CSRF:          true,
		TrailingSlash: TrailingSlashNever,
		Templates:     TemplatesGoMustache,
	}
}

// Equal reports whether base and other carry the same options. It exists
// because PageOptions includes a slice (ImageWidths) which the language
// rejects in == comparisons.
func (base PageOptions) Equal(other PageOptions) bool {
	if base.Prerender != other.Prerender ||
		base.PrerenderAuto != other.PrerenderAuto ||
		base.PrerenderProtected != other.PrerenderProtected ||
		base.SSR != other.SSR ||
		base.CSR != other.CSR ||
		base.SSROnly != other.SSROnly ||
		base.CSRF != other.CSRF ||
		base.TrailingSlash != other.TrailingSlash ||
		base.Templates != other.Templates {
		return false
	}
	if len(base.ImageWidths) != len(other.ImageWidths) {
		return false
	}
	for i, w := range base.ImageWidths {
		if w != other.ImageWidths[i] {
			return false
		}
	}
	return true
}

// Merge returns base overlaid with override. Each scalar field in
// override replaces the corresponding base field when override declares
// a value (tracked by the optional companion type returned from the
// codegen scanner). Merge is a struct-value operation; both inputs are
// left unchanged.
func (base PageOptions) Merge(override PageOptionsOverride) PageOptions {
	out := base
	if override.HasPrerender {
		out.Prerender = override.Prerender
	}
	if override.HasPrerenderAuto {
		out.PrerenderAuto = override.PrerenderAuto
	}
	if override.HasPrerenderProtected {
		out.PrerenderProtected = override.PrerenderProtected
	}
	if override.HasSSR {
		out.SSR = override.SSR
	}
	if override.HasCSR {
		out.CSR = override.CSR
	}
	if override.HasSSROnly {
		out.SSROnly = override.SSROnly
	}
	if override.HasCSRF {
		out.CSRF = override.CSRF
	}
	if override.HasTrailingSlash {
		out.TrailingSlash = override.TrailingSlash
	}
	if override.HasTemplates {
		out.Templates = override.Templates
	}
	return out
}

// PageOptionsOverride is the parsed form of one *.server.go file's page
// options. Each Has* flag records whether the corresponding constant was
// declared so layouts and pages can express "inherit" by omission.
type PageOptionsOverride struct {
	Prerender             bool
	HasPrerender          bool
	PrerenderAuto         bool
	HasPrerenderAuto      bool
	PrerenderProtected    bool
	HasPrerenderProtected bool
	SSR                   bool
	HasSSR                bool
	CSR                   bool
	HasCSR                bool
	SSROnly               bool
	HasSSROnly            bool
	CSRF                  bool
	HasCSRF               bool
	TrailingSlash         TrailingSlash
	HasTrailingSlash      bool
	Templates             string
	HasTemplates          bool
}

// Any reports whether at least one option is declared. Codegen uses this
// to skip cascade resolution when the file declares no options at all.
func (o PageOptionsOverride) Any() bool {
	return o.HasPrerender || o.HasPrerenderAuto || o.HasPrerenderProtected ||
		o.HasSSR || o.HasCSR || o.HasSSROnly || o.HasCSRF ||
		o.HasTrailingSlash || o.HasTemplates
}
