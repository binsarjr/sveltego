package server

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// Log keys are named constants so sloglint's no-raw-keys rule passes
// and grep over log output finds every callsite for a given attribute.
const (
	logKeyMethod   = "method"
	logKeyPath     = "path"
	logKeyError    = "error"
	logKeyStatus   = "status"
	logKeyLocation = "location"
	logKeyFailCode = "fail_code"
	logKeyStreamID = "stream_id"
	logKeyEndpoint = "endpoint"
)

// httpStatuser lets user errors carry a non-500 status into the
// MVP error path without a sentinel-error coupling.
type httpStatuser interface {
	HTTPStatus() int
}

// methodNotAllowed writes a plain-text 405 with the Allow header for
// a _server.go route that lacks a handler for the request method.
func (s *Server) methodNotAllowed(w http.ResponseWriter, r *http.Request, allowed []string) {
	s.Logger.Info("server: method not allowed", logKeyMethod, r.Method, logKeyPath, r.URL.Path)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writePlain(w, http.StatusMethodNotAllowed, "405 method not allowed\n")
}

// handlePipelineError converts an error returned from anywhere inside
// the Handle pipeline (Handle itself, Load, Render, sentinel from
// resolve) into an HTTP response. Sentinel types route deterministically;
// everything else falls through to the user's HandleError hook (or the
// kit identity default) which produces a SafeError consumed by the
// generic writer.
func (s *Server) handlePipelineError(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, route *router.Route, err error) {
	var redir *kit.RedirectErr
	if errors.As(err, &redir) {
		s.Logger.Info("server: pipeline redirect",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyStatus, redir.Code,
			logKeyLocation, redir.Location)
		if ev != nil {
			if ev.Cookies != nil {
				ev.Cookies.Apply(w)
			}
			for k, vs := range ev.ResponseHeader() {
				w.Header()[k] = vs
			}
		}
		if redir.ForceReload {
			w.Header().Set("X-Sveltego-Reload", "1")
		}
		http.Redirect(w, r, redir.Location, redir.Code)
		return
	}
	var herr *kit.HTTPErr
	if errors.As(err, &herr) {
		s.Logger.Info("server: pipeline http error",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyStatus, herr.Code,
			logKeyError, herr.Message)
		if ev != nil {
			if ev.Cookies != nil {
				ev.Cookies.Apply(w)
			}
			for k, vs := range ev.ResponseHeader() {
				w.Header()[k] = vs
			}
		}
		writePlain(w, herr.Code, herr.Message+"\n")
		return
	}
	var fail *kit.FailErr
	if errors.As(err, &fail) {
		s.Logger.Warn("server: kit.Fail outside action context",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyFailCode, fail.Code)
		writePlain(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)+"\n")
		return
	}

	// Treat a SafeError thrown directly (e.g. "404 not found" sentinel
	// returned by resolve) as canonical without a second HandleError
	// pass — it's already user-shaped.
	var safeDirect kit.SafeError
	if errors.As(err, &safeDirect) {
		s.respondWithError(w, r, ev, route, safeDirect, err)
		return
	}

	// User-defined types implementing kit.HTTPError carry their own status
	// and safe public message. Convert them before falling through to
	// HandleError so the error boundary sees the right code.
	var httpErr kit.HTTPError
	if errors.As(err, &httpErr) {
		safe := kit.SafeError{Code: httpErr.Status(), Message: httpErr.Public()}
		s.respondWithError(w, r, ev, route, safe, err)
		return
	}

	safe, shortCircuit := s.hooks.HandleError(ev, err)

	// HandleError may short-circuit with a redirect or custom HTTP
	// response instead of rendering the error boundary.
	if shortCircuit != nil {
		var redir *kit.RedirectErr
		if errors.As(shortCircuit, &redir) {
			s.Logger.Info("server: handleerror redirect",
				logKeyMethod, r.Method,
				logKeyPath, r.URL.Path,
				logKeyStatus, redir.Code,
				logKeyLocation, redir.Location)
			if ev != nil && ev.Cookies != nil {
				ev.Cookies.Apply(w)
			}
			http.Redirect(w, r, redir.Location, redir.Code)
			return
		}
		var herr *kit.HTTPErr
		if errors.As(shortCircuit, &herr) {
			s.Logger.Info("server: handleerror http error",
				logKeyMethod, r.Method,
				logKeyPath, r.URL.Path,
				logKeyStatus, herr.Code,
				logKeyError, herr.Message)
			if ev != nil && ev.Cookies != nil {
				ev.Cookies.Apply(w)
			}
			writePlain(w, herr.Code, herr.Message+"\n")
			return
		}
		// Unknown short-circuit error type: treat as 500 to avoid a
		// silent no-op. Do not re-enter HandleError.
		s.Logger.Error("server: handleerror returned unknown short-circuit error",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyError, shortCircuit.Error())
		writePlain(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)+"\n")
		return
	}

	// Preserve the legacy httpStatuser observation. HandleError now takes
	// (SafeError, error) and may return a short-circuit error; when the
	// user did NOT author HandleError the identity default returns a 500
	// SafeError. Pre-hooks behavior promoted errors that implement
	// HTTPStatus() to that status, so we honor that path here. This branch
	// is therefore still reachable (not dead): it fires whenever HandleError
	// returns exactly the identity-default 500 and the original error
	// carries an HTTPStatus() method.
	if safe.Code == http.StatusInternalServerError && safe.Message == http.StatusText(http.StatusInternalServerError) {
		var hs httpStatuser
		if errors.As(err, &hs) {
			safe.Code = hs.HTTPStatus()
			safe.Message = http.StatusText(safe.Code)
		}
	}
	s.respondWithError(w, r, ev, route, safe, err)
}

// respondWithError dispatches a SafeError to the route's _error.svelte
// boundary when one applies; otherwise it falls back to the plain-text
// writer that has handled error responses since Phase 0h.
func (s *Server) respondWithError(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, route *router.Route, safe kit.SafeError, original error) {
	if route == nil || route.RenderError == nil {
		s.writeSafeError(w, r, ev, safe, original)
		return
	}
	if err := s.renderErrorBoundary(w, r, ev, route, safe, original); err != nil {
		s.Logger.Error("server: error boundary render failed",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyError, err.Error())
		s.writeSafeError(w, r, ev, safe, original)
	}
}

// renderErrorBoundary writes the route's _error.svelte response. The
// generated RenderError closure embeds the surviving outer-layout
// composition; the pipeline only passes context, the SafeError, and
// the layoutDatas slice. Status mirrors safe.HTTPStatus(); cookies
// queued on ev are flushed before the write.
func (s *Server) renderErrorBoundary(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, route *router.Route, safe kit.SafeError, original error) error {
	buf := render.Acquire()
	defer render.Release(buf)

	var rctx *kit.RenderCtx
	if ev != nil {
		rctx = &kit.RenderCtx{
			Locals:      ev.Locals,
			URL:         ev.URL,
			OriginalURL: ev.OriginalURL,
			Params:      ev.Params,
			RawParams:   ev.RawParams,
			Cookies:     ev.Cookies,
			Request:     r,
		}
	} else {
		rctx = &kit.RenderCtx{Request: r}
	}

	buf.WriteString(s.shellHead)
	buf.WriteString(s.shellMid)
	if err := route.RenderError(buf, rctx, safe, nil); err != nil {
		return err
	}
	buf.WriteString(s.shellTail)

	body := buf.Bytes()
	if ev != nil {
		if ev.Cookies != nil {
			ev.Cookies.Apply(w)
		}
		for k, vs := range ev.ResponseHeader() {
			w.Header()[k] = vs
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	status := safe.HTTPStatus()
	w.WriteHeader(status)
	_, _ = w.Write(body)

	logger := s.Logger.Error
	if status >= 400 && status < 500 {
		logger = s.Logger.Info
	}
	logger("server: pipeline error",
		logKeyMethod, r.Method,
		logKeyPath, r.URL.Path,
		logKeyStatus, status,
		logKeyError, original.Error())
	return nil
}

// writeSafeError flushes a SafeError to the response and logs it once.
// Cookies and user-set response headers from ev are applied before WriteHeader
// so they appear on error responses just as they would on success responses.
func (s *Server) writeSafeError(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, safe kit.SafeError, original error) {
	status := safe.HTTPStatus()
	msg := safe.Message
	if msg == "" {
		msg = http.StatusText(status)
	}
	logger := s.Logger.Error
	if status >= 400 && status < 500 {
		logger = s.Logger.Info
	}
	logger("server: pipeline error",
		logKeyMethod, r.Method,
		logKeyPath, r.URL.Path,
		logKeyStatus, status,
		logKeyError, original.Error())
	if ev != nil {
		if ev.Cookies != nil {
			ev.Cookies.Apply(w)
		}
		for k, vs := range ev.ResponseHeader() {
			w.Header()[k] = vs
		}
	}
	writePlain(w, status, msg+"\n")
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// methodsOf returns sorted method keys from a _server.go handler map
// for use in the Allow header.
func methodsOf(m map[string]http.HandlerFunc) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
