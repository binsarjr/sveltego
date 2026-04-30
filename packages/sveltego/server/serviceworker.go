package server

// serviceWorkerRegisterScript is the inline <script> emitted before
// </body> when Config.ServiceWorker is true. It feature-gates registration
// on `'serviceWorker' in navigator` so non-supporting browsers no-op,
// scope is rooted at "/" so SPA navigation under any sub-path is covered,
// and registration runs on `load` to avoid contending with the page's
// initial render. Errors are logged at warn level rather than thrown so
// a SW fault does not block hydration (#89).
const serviceWorkerRegisterScript = `<script>` +
	`if('serviceWorker' in navigator){` +
	`addEventListener('load',function(){` +
	`navigator.serviceWorker.register('/service-worker.js',{scope:'/'})` +
	`.catch(function(e){console.warn('sveltego: service worker registration failed',e)});` +
	`});` +
	`}</script>`
