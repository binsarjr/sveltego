package server

import (
	"net/http"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// csrfTokenMaxAge bounds the lifetime of the issued cookie. One day is a
// pragmatic middle ground: long enough for normal browsing sessions,
// short enough that a leaked token expires before it becomes useful for
// long-tail attacks.
const csrfTokenMaxAge = 24 * time.Hour

// applyCSRF reads or seeds the CSRF token for the request, exposes it
// to renderers via ev.Locals, and on POST validates the submitted token
// against the cookie. Returns a non-nil 403 response when validation
// fails so the caller can short-circuit before dispatchAction.
//
// CSRF runs only for routes that declare an Actions map AND have CSRF
// enabled in their PageOptions. The default for every page is on; users
// opt out per-route with `const CSRF = false` or per-form with the
// `nocsrf` attribute (handled at codegen time).
func (s *Server) applyCSRF(r *http.Request, ev *kit.RequestEvent, route *router.Route) *kit.Response {
	if !csrfApplies(route) {
		return nil
	}

	cookieToken, _ := ev.Cookies.Get(kit.CSRFCookieName)

	if r.Method == http.MethodPost {
		submitted := r.PostFormValue(kit.CSRFFieldName)
		if submitted == "" {
			submitted = r.Header.Get("X-CSRF-Token")
		}
		if !kit.CSRFTokenEqual(cookieToken, submitted) {
			return forbiddenResponse("forbidden: csrf token missing or invalid")
		}
		// Re-publish the existing token so subsequent renders on the same
		// request can embed it (e.g. error-boundary re-renders that emit
		// another form).
		kit.SetCSRFToken(ev, cookieToken)
		return nil
	}

	if cookieToken == "" {
		fresh := kit.GenerateCSRFToken()
		if fresh == "" {
			return nil
		}
		ev.Cookies.Set(kit.CSRFCookieName, fresh, kit.CookieOpts{
			MaxAge:   csrfTokenMaxAge,
			SameSite: http.SameSiteLaxMode,
		})
		kit.SetCSRFToken(ev, fresh)
		return nil
	}
	kit.SetCSRFToken(ev, cookieToken)
	return nil
}

// csrfApplies reports whether the CSRF middleware should run for this
// route. It is gated by both the route having an Actions map (CSRF only
// matters for state-changing form submissions) and Options.CSRF being
// true (so users can opt out per-route).
func csrfApplies(route *router.Route) bool {
	if route == nil || route.Actions == nil {
		return false
	}
	// Zero-value PageOptions (older manifests, tests) is treated as
	// "CSRF off" because the field default is the result of the cascade
	// merge with DefaultPageOptions; tests that build Routes directly
	// have the zero value and should not see CSRF behavior unless they
	// opt in explicitly.
	return route.Options.CSRF
}

// forbiddenResponse builds a minimal 403 used for CSRF rejections.
func forbiddenResponse(msg string) *kit.Response {
	return &kit.Response{
		Status:  http.StatusForbidden,
		Headers: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:    []byte(msg),
	}
}
