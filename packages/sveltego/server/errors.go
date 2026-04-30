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

// notFound writes a plain-text 404 for an unmatched path.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.Logger.Info("server: route not found", logKeyMethod, r.Method, logKeyPath, r.URL.Path)
	writePlain(w, http.StatusNotFound, "404 not found\n")
}

// methodNotAllowed writes a plain-text 405 with the Allow header for
// a +server.go route that lacks a handler for the request method.
func (s *Server) methodNotAllowed(w http.ResponseWriter, r *http.Request, allowed []string) {
	s.Logger.Info("server: method not allowed", logKeyMethod, r.Method, logKeyPath, r.URL.Path)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writePlain(w, http.StatusMethodNotAllowed, "405 method not allowed\n")
}

// handleLoadError converts a Load() error into a plain-text response.
// Sentinel types (kit.RedirectErr, kit.HTTPErr, kit.FailErr) take
// precedence: redirect needs the Location header, HTTPErr writes the
// caller's message, FailErr outside an action context warns and 500s.
// Other errors implementing HTTPStatus() drive the status code; the
// rest fall through to 500.
func (s *Server) handleLoadError(w http.ResponseWriter, r *http.Request, err error) {
	var redir *kit.RedirectErr
	if errors.As(err, &redir) {
		s.Logger.Info("server: load redirect",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyStatus, redir.Code,
			logKeyLocation, redir.Location)
		http.Redirect(w, r, redir.Location, redir.Code)
		return
	}
	var herr *kit.HTTPErr
	if errors.As(err, &herr) {
		s.Logger.Info("server: load http error",
			logKeyMethod, r.Method,
			logKeyPath, r.URL.Path,
			logKeyStatus, herr.Code,
			logKeyError, herr.Message)
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

	status := http.StatusInternalServerError
	var hs httpStatuser
	if errors.As(err, &hs) {
		status = hs.HTTPStatus()
	}
	s.Logger.Error("server: load failed",
		logKeyMethod, r.Method,
		logKeyPath, r.URL.Path,
		logKeyError, err.Error(),
		logKeyStatus, status)
	writePlain(w, status, http.StatusText(status)+"\n")
}

// handleRenderError writes a plain-text 500 when a Page handler errors.
// The buffer is discarded by the caller; nothing is written before the
// header is set, so WriteHeader is safe here.
func (s *Server) handleRenderError(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.Error("server: render failed",
		logKeyMethod, r.Method,
		logKeyPath, r.URL.Path,
		logKeyError, err.Error())
	writePlain(w, http.StatusInternalServerError, "500 internal server error\n")
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
