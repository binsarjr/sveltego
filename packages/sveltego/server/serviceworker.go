package server

// serviceWorkerScriptBody is the inline JS payload registering the
// service worker (#89). It feature-gates registration on
// `'serviceWorker' in navigator` so non-supporting browsers no-op,
// scope is rooted at "/" so SPA navigation under any sub-path is
// covered, and registration runs on `load` to avoid contending with
// the page's initial render. Errors are logged at warn level rather
// than thrown so a SW fault does not block hydration.
const serviceWorkerScriptBody = `if('serviceWorker' in navigator){` +
	`addEventListener('load',function(){` +
	`navigator.serviceWorker.register('/service-worker.js',{scope:'/'})` +
	`.catch(function(e){console.warn('sveltego: service worker registration failed',e)});` +
	`});` +
	`}`

// serviceWorkerTag returns the inline registration <script> tag with
// an optional CSP nonce attribute. Returns the empty string when the
// service worker is disabled for this server. The nonce is the
// per-request CSP nonce (kit.Nonce(ev)); empty for off-CSP requests.
func (s *Server) serviceWorkerTag(nonce string) string {
	if !s.serviceWorker {
		return ""
	}
	if nonce == "" {
		return `<script>` + serviceWorkerScriptBody + `</script>`
	}
	return `<script nonce="` + nonce + `">` + serviceWorkerScriptBody + `</script>`
}
