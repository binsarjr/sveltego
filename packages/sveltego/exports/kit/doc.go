// Package kit is the public sveltego runtime API. Generated code and
// user route handlers depend on this package: RenderCtx threads request
// state through SSR templates, LoadCtx threads the same state through
// user-written Load functions in +page.server.go, RequestEvent backs
// the hooks pipeline (Handle, HandleError, HandleFetch, Reroute, Init),
// and Cookies provides the request/response cookie surface with secure
// defaults.
package kit
