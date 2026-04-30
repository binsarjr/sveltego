package kit

import (
	"sort"
	"strings"
)

// CSPMode controls how a sveltego server emits Content-Security-Policy
// headers. CSPOff (the zero value) keeps the existing behavior — no
// header, no per-request nonce. CSPStrict emits Content-Security-Policy.
// CSPReportOnly emits Content-Security-Policy-Report-Only so violations
// are observed without enforcement during rollout.
type CSPMode int

const (
	CSPOff CSPMode = iota
	CSPStrict
	CSPReportOnly
)

// CSPConfig configures the CSP middleware. Mode selects enforcement.
// Directives is merged over DefaultCSPDirectives so callers only specify
// the directives they want to override or extend; setting a directive to
// an empty slice removes it from the merged output. ReportTo, when
// non-empty, appends a `report-to <token>` directive.
type CSPConfig struct {
	Mode       CSPMode
	Directives map[string][]string
	ReportTo   string
}

// cspNonceKey is the ev.Locals key under which the per-request nonce is
// stored. Exported via NonceAttr / Nonce so user code reads through the
// helper rather than the bare key.
const cspNonceKey = "cspNonce"

// Nonce returns the per-request CSP nonce stored on ev by the server's
// CSP middleware, or the empty string when CSP is off or ev is nil.
func Nonce(ev *RequestEvent) string {
	if ev == nil || ev.Locals == nil {
		return ""
	}
	if v, ok := ev.Locals[cspNonceKey].(string); ok {
		return v
	}
	return ""
}

// SetNonce stores nonce on ev.Locals under the canonical key used by the
// CSP middleware. The server pipeline calls this once per request before
// Handle runs; user code does not.
func SetNonce(ev *RequestEvent, nonce string) {
	if ev == nil {
		return
	}
	if ev.Locals == nil {
		ev.Locals = map[string]any{}
	}
	ev.Locals[cspNonceKey] = nonce
}

// NonceAttr returns ` nonce="<nonce>"` for embedding in user-emitted
// inline <script> tags. Returns the empty string when CSP is off so
// templates compile uniformly across opt-in and opt-out builds.
func NonceAttr(ev *RequestEvent) string {
	n := Nonce(ev)
	if n == "" {
		return ""
	}
	return ` nonce="` + n + `"`
}

// DefaultCSPDirectives returns the baseline directive map applied under
// CSPStrict and CSPReportOnly. The script-src entry intentionally omits
// the nonce token; the middleware splices `'nonce-<n>'` per request.
func DefaultCSPDirectives() map[string][]string {
	return map[string][]string{
		"default-src": {"'self'"},
		"script-src":  {"'strict-dynamic'"},
		"style-src":   {"'self'", "'unsafe-inline'"},
		"img-src":     {"'self'", "data:"},
		"connect-src": {"'self'"},
		"base-uri":    {"'self'"},
		"form-action": {"'self'"},
	}
}

// BuildCSPHeader composes the Content-Security-Policy header value for
// the given config and per-request nonce. Directive order is
// deterministic (alphabetical) so two requests with equivalent input
// produce byte-identical output, which simplifies caching and tests.
// nonce is required: callers that want CSP off must skip the header
// entirely rather than passing an empty nonce.
func BuildCSPHeader(cfg CSPConfig, nonce string) string {
	merged := DefaultCSPDirectives()
	for k, v := range cfg.Directives {
		if len(v) == 0 {
			delete(merged, k)
			continue
		}
		merged[k] = append([]string(nil), v...)
	}
	if scripts, ok := merged["script-src"]; ok {
		merged["script-src"] = append([]string{"'nonce-" + nonce + "'"}, scripts...)
	} else {
		merged["script-src"] = []string{"'nonce-" + nonce + "'"}
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		parts = append(parts, k+" "+strings.Join(merged[k], " "))
	}
	if cfg.ReportTo != "" {
		parts = append(parts, "report-to "+cfg.ReportTo)
	}
	return strings.Join(parts, "; ")
}

// CSPHeaderName returns the response header name for the configured mode:
// Content-Security-Policy under CSPStrict, Content-Security-Policy-Report-Only
// under CSPReportOnly, or the empty string when CSP is off.
func CSPHeaderName(mode CSPMode) string {
	switch mode {
	case CSPStrict:
		return "Content-Security-Policy"
	case CSPReportOnly:
		return "Content-Security-Policy-Report-Only"
	default:
		return ""
	}
}
