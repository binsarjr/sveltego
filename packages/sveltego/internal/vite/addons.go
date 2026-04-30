package vite

// Addon names a build-time integration the generated vite.config.gen.js
// must wire in. Addons compose: a project may enable any subset.
type Addon string

const (
	// AddonTailwindV4 enables the @tailwindcss/vite plugin (TW v4 default).
	AddonTailwindV4 Addon = "tailwindcss-v4"
	// AddonTailwindV3 enables the PostCSS-based TW v3 path. No Vite
	// plugin is added; PostCSS picks up postcss.config.js automatically.
	AddonTailwindV3 Addon = "tailwindcss-v3"
)

// hasAddon reports whether a is present in addons.
func hasAddon(addons []Addon, a Addon) bool {
	for _, x := range addons {
		if x == a {
			return true
		}
	}
	return false
}
