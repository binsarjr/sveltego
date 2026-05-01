// Package kit is the public sveltego runtime API. Generated code and
// user route handlers depend on this package: RenderCtx threads request
// state through SSR templates, LoadCtx threads the same state through
// user-written Load functions in _page.server.go, RequestEvent backs
// the hooks pipeline (Handle, HandleError, HandleFetch, Reroute, Init),
// and Cookies provides the request/response cookie surface with secure
// defaults.
//
// URL building: the typed helpers generated under <module>/.gen/links
// are the preferred way to build internal URLs because they fail at
// compile time when a route is renamed. [Link] is the runtime fallback
// for dynamic patterns that are not known at build time.
package kit
