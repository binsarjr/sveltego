package fallback

import "strings"

// fragmentOpen / fragmentClose are the hydration markers svelte/server
// always wraps around its rendered component output. The client mounts
// each fallback page as a CHILD component of the per-route wrapper
// (#508), and Svelte's runtime walker doesn't expect those markers on a
// child render — leaving them in the SSR HTML trips
// svelte/e/hydration_mismatch on first paint.
const (
	fragmentOpen  = "<!--[-->"
	fragmentClose = "<!--]-->"
)

// StripFragmentMarkers removes the leading <!--[--> and trailing
// <!--]--> the sidecar's `svelte/server.render` always wraps around the
// page output. Surrounding whitespace is preserved so the rendered
// markup stays byte-identical to a transpile-SSR Render() emit.
//
// The function is a no-op when the markers are absent (e.g. an upstream
// rewrite already stripped them), so callers can apply it
// unconditionally without checking the response shape first.
func StripFragmentMarkers(body string) string {
	trimmedLeft := strings.TrimLeft(body, " \t\r\n")
	if !strings.HasPrefix(trimmedLeft, fragmentOpen) {
		return body
	}
	leftPad := body[:len(body)-len(trimmedLeft)]
	rest := trimmedLeft[len(fragmentOpen):]

	trimmedRight := strings.TrimRight(rest, " \t\r\n")
	if !strings.HasSuffix(trimmedRight, fragmentClose) {
		// Open marker without a matching close — leave the body
		// unchanged so the mismatch surfaces in dev rather than the
		// stripper silently producing half-balanced output.
		return body
	}
	rightPad := rest[len(trimmedRight):]
	inner := trimmedRight[:len(trimmedRight)-len(fragmentClose)]
	return leftPad + inner + rightPad
}
