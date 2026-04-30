package server

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
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
)

// httpStatuser lets user errors carry a non-500 status into the
// MVP error path without a sentinel-error coupling.
type httpStatuser interface {
	HTTPStatus() int
}

// methodNotAllowed writes a plain-text 405 with the Allow header for
// a +server.go route that lacks a handler for the request method.
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
func (s *Server) handlePipelineError(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, err error) {
	var redir *kit.RedirectErr
	if errors.As(err, &redir) {
		s.Logger.Info("server: pipeline redirect",
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
	if errors.As(err, &herr) {
		s.Logger.Info("server: pipeline http error",
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
		s.writeSafeError(w, r, safeDirect, err)
		return
	}

	safe := s.hooks.HandleError(ev, err)

	// Preserve the legacy httpStatuser observation: when the user did
	// not author HandleError, the identity default returns 500 — but
	// pre-hooks behavior promoted any error implementing HTTPStatus()
	// to that status. Honor that path so existing user errors that
	// expose a status keep doing so.
	if safe.Code == http.StatusInternalServerError && safe.Message == http.StatusText(http.StatusInternalServerError) {
		var hs httpStatuser
		if errors.As(err, &hs) {
			safe.Code = hs.HTTPStatus()
			safe.Message = http.StatusText(safe.Code)
		}
	}
	s.writeSafeError(w, r, safe, err)
}

// writeSafeError flushes a SafeError to the response and logs it once.
func (s *Server) writeSafeError(w http.ResponseWriter, r *http.Request, safe kit.SafeError, original error) {
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
	writePlain(w, status, msg+"\n")
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// methodsOf returns sorted method keys from a +server.go handler map
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
