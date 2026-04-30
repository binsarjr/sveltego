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
// exported constants in *.server.go: Prerender, SSR, CSR, and
// TrailingSlash. Layout-level values cascade to descendants; page-level
// values override the cascade.
//
// SSR and CSR default to true; Prerender defaults to false. Manifest
// emission stores the effective resolved value per route, so the
// pipeline does not re-walk the layout chain at request time.
type PageOptions struct {
	Prerender     bool
	SSR           bool
	CSR           bool
	TrailingSlash TrailingSlash
}

// DefaultPageOptions returns the framework defaults: SSR and CSR on,
// Prerender off, TrailingSlash never. Codegen seeds the cascade with
// this value so a project that declares no options gets the same
// behavior as today.
func DefaultPageOptions() PageOptions {
	return PageOptions{
		SSR:           true,
		CSR:           true,
		TrailingSlash: TrailingSlashNever,
	}
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
	if override.HasSSR {
		out.SSR = override.SSR
	}
	if override.HasCSR {
		out.CSR = override.CSR
	}
	if override.HasTrailingSlash {
		out.TrailingSlash = override.TrailingSlash
	}
	return out
}

// PageOptionsOverride is the parsed form of one *.server.go file's page
// options. Each Has* flag records whether the corresponding constant was
// declared so layouts and pages can express "inherit" by omission.
type PageOptionsOverride struct {
	Prerender        bool
	HasPrerender     bool
	SSR              bool
	HasSSR           bool
	CSR              bool
	HasCSR           bool
	TrailingSlash    TrailingSlash
	HasTrailingSlash bool
}

// Any reports whether at least one option is declared. Codegen uses this
// to skip cascade resolution when the file declares no options at all.
func (o PageOptionsOverride) Any() bool {
	return o.HasPrerender || o.HasSSR || o.HasCSR || o.HasTrailingSlash
}
