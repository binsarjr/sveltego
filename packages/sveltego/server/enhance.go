package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// EnhanceHeader is the request header the @sveltego/client enhance
// runtime sets when posting a form via fetch. The pipeline returns the
// action result as a JSON envelope when this header is "1".
const EnhanceHeader = "X-Sveltego-Action"

// enhanceEnvelope is the JSON shape returned to the client-side
// `use:enhance` runtime. Type discriminates the variant; Data is set on
// success/failure; Location is set on redirect.
type enhanceEnvelope struct {
	Type     string `json:"type"`
	Status   int    `json:"status"`
	Data     any    `json:"data,omitempty"`
	Location string `json:"location,omitempty"`
}

// isEnhanceRequest reports whether the request was issued by the client
// `use:enhance` runtime. The header is forgeable, so this is a request
// shape signal — not a security boundary.
func isEnhanceRequest(r *http.Request) bool {
	return r != nil && r.Header.Get(EnhanceHeader) == "1"
}

// enhanceResponse builds a JSON envelope from a dispatch outcome. res
// covers the redirect / not-found / 405 paths (where dispatchAction
// returned a Response); fd covers the success / failure paths (where
// the page would have re-rendered with form data).
func enhanceResponse(res *kit.Response, fd *formData) *kit.Response {
	if res != nil {
		// Redirect short-circuit: surface as type=redirect so the client
		// can perform an SPA navigation (or fall back to location.href).
		// Status is forced to 200 so fetch() does not follow the redirect
		// — the client takes over navigation.
		if loc := res.Headers.Get("Location"); loc != "" {
			return jsonEnvelope(enhanceEnvelope{
				Type:     "redirect",
				Status:   res.Status,
				Location: loc,
			})
		}
		// 404 / 405 / other non-action errors: pass through the status as
		// type=error with the body as a plain string the client can show.
		return jsonEnvelope(enhanceEnvelope{
			Type:   "error",
			Status: res.Status,
			Data:   string(res.Body),
		})
	}
	if fd == nil {
		// HandleAction returned nil — nothing to do. Surface as success
		// with empty data so the client doesn't get stuck on a hanging
		// promise.
		return jsonEnvelope(enhanceEnvelope{Type: "success", Status: http.StatusOK})
	}
	envType := "success"
	if fd.code >= 400 {
		envType = "failure"
	}
	return jsonEnvelope(enhanceEnvelope{
		Type:   envType,
		Status: fd.code,
		Data:   fd.data,
	})
}

// enhanceForbiddenResponse is the envelope used when CSRF validation
// fails on an enhance-driven submission.
func enhanceForbiddenResponse() *kit.Response {
	return jsonEnvelope(enhanceEnvelope{
		Type:   "error",
		Status: http.StatusForbidden,
		Data:   "csrf token missing or invalid",
	})
}

// jsonEnvelope wraps env as the HTTP response body. The HTTP status is
// always 200: the original action status (303 redirect, 422 failure,
// etc.) lives inside the envelope so fetch() does not auto-follow
// redirects or mark the response as an error.
func jsonEnvelope(env enhanceEnvelope) *kit.Response {
	body, err := json.Marshal(env)
	if err != nil {
		body = []byte(`{"type":"error","status":500,"data":"server: marshal action envelope"}`)
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	headers.Set("X-Sveltego-Action", "1")
	return &kit.Response{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    body,
	}
}
