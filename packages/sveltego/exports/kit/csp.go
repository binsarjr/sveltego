package kit

import (
	"sort"
	"strings"
	"sync"
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

// CSPTemplate is a precomputed Content-Security-Policy header split
// around the per-request nonce token. Build splices the nonce into the
// fixed prefix/suffix without rebuilding the directive map, so the hot
// path is one allocation (the joined string) instead of map alloc + sort
// + slice appends per request.
type CSPTemplate struct {
	prefix string
	suffix string
}

// NewCSPTemplate composes the deterministic prefix/suffix for cfg once.
// Callers store the returned template at server-construction time and
// invoke Build(nonce) per request.
func NewCSPTemplate(cfg CSPConfig) *CSPTemplate {
	prefix, suffix := composeCSP(cfg)
	return &CSPTemplate{prefix: prefix, suffix: suffix}
}

// Build returns the full Content-Security-Policy header value with nonce
// spliced into the cached script-src position.
func (t *CSPTemplate) Build(nonce string) string {
	if t == nil {
		return ""
	}
	return t.prefix + nonce + t.suffix
}

// cspTemplateCache memoizes CSPTemplate values keyed by a deterministic
// fingerprint of CSPConfig. BuildCSPHeader uses it so per-request callers
// pay the directive-merge + sort cost exactly once per distinct config.
var cspTemplateCache sync.Map

// BuildCSPHeader composes the Content-Security-Policy header value for
// the given config and per-request nonce. The merged directive map and
// sort order are computed once per distinct cfg and cached, so repeated
// calls with the same cfg only do a string concat. Directive order is
// deterministic (alphabetical) so two requests with equivalent input
// produce byte-identical output. nonce is required: callers that want
// CSP off must skip the header entirely rather than passing an empty
// nonce.
func BuildCSPHeader(cfg CSPConfig, nonce string) string {
	key := cspCacheKey(cfg)
	if cached, ok := cspTemplateCache.Load(key); ok {
		return cached.(*CSPTemplate).Build(nonce)
	}
	tpl := NewCSPTemplate(cfg)
	actual, _ := cspTemplateCache.LoadOrStore(key, tpl)
	return actual.(*CSPTemplate).Build(nonce)
}

// composeCSP returns the prefix/suffix surrounding the nonce token in the
// final header value. The split point is the script-src nonce slot: every
// directive before script-src plus the literal `script-src 'nonce-` lives
// in prefix; everything after the nonce string lives in suffix.
func composeCSP(cfg CSPConfig) (prefix, suffix string) {
	merged := DefaultCSPDirectives()
	for k, v := range cfg.Directives {
		if len(v) == 0 {
			delete(merged, k)
			continue
		}
		merged[k] = append([]string(nil), v...)
	}
	scripts := merged["script-src"]
	delete(merged, "script-src")

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	scriptIdx := sort.SearchStrings(keys, "script-src")

	var b strings.Builder
	for i := 0; i < scriptIdx; i++ {
		b.WriteString(keys[i])
		b.WriteByte(' ')
		b.WriteString(strings.Join(merged[keys[i]], " "))
		b.WriteString("; ")
	}
	b.WriteString("script-src 'nonce-")
	prefix = b.String()

	b.Reset()
	b.WriteByte('\'')
	if len(scripts) > 0 {
		b.WriteByte(' ')
		b.WriteString(strings.Join(scripts, " "))
	}
	for i := scriptIdx; i < len(keys); i++ {
		b.WriteString("; ")
		b.WriteString(keys[i])
		b.WriteByte(' ')
		b.WriteString(strings.Join(merged[keys[i]], " "))
	}
	if cfg.ReportTo != "" {
		b.WriteString("; report-to ")
		b.WriteString(cfg.ReportTo)
	}
	suffix = b.String()
	return prefix, suffix
}

// cspCacheKey returns a deterministic string fingerprint of cfg suitable
// as a sync.Map key. Map iteration order is non-deterministic so we sort
// the directive keys before serializing.
func cspCacheKey(cfg CSPConfig) string {
	var b strings.Builder
	b.WriteByte(byte('0' + int(cfg.Mode)))
	b.WriteByte('|')
	b.WriteString(cfg.ReportTo)
	b.WriteByte('|')
	keys := make([]string, 0, len(cfg.Directives))
	for k := range cfg.Directives {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		for i, v := range cfg.Directives[k] {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(v)
		}
		b.WriteByte(';')
	}
	return b.String()
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
