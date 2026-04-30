package server

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/runtime/router"
)

// injectFormField sets the `Form` field on data (when present) to
// formValue and returns the resulting value. data is typically a
// PageData struct alias the codegen emits with `Form any`; routes
// without that field receive data unchanged. nil data falls through:
// PageData zero value carries Form=nil so the page reads it as nil.
//
// The reflective path is the v0.1 fallback. Once #143 (form-field
// collision detection) lands, codegen can emit a typed setter that
// skips reflection entirely.
func injectFormField(data, formValue any) any {
	if data == nil {
		return nil
	}
	rv := reflect.ValueOf(data)
	if rv.Kind() != reflect.Struct {
		return data
	}
	formField := rv.FieldByName("Form")
	if !formField.IsValid() {
		return data
	}
	cp := reflect.New(rv.Type()).Elem()
	cp.Set(rv)
	target := cp.FieldByName("Form")
	if !target.CanSet() {
		return data
	}
	if formValue == nil {
		target.Set(reflect.Zero(target.Type()))
	} else {
		target.Set(reflect.ValueOf(formValue))
	}
	return cp.Interface()
}

// dispatchAction resolves the form action keyed by the request URL's
// `?/<name>` query (default: "default") against route.Actions and runs
// it through the HandleAction middleware. The returned response or error
// follows the same shape as renderPage so the surrounding pipeline writes
// one Response.
//
// Returning (nil, nil) means the caller should fall through to
// renderPage with the `Form` payload installed via formData.
func (s *Server) dispatchAction(r *http.Request, ev *kit.RequestEvent, route *router.Route) (*kit.Response, *formData, error) {
	if route.Actions == nil {
		return kit.MethodNotAllowed([]string{http.MethodGet}), nil, nil
	}
	rawMap := route.Actions()
	actions, _ := rawMap.(kit.ActionMap)
	if len(actions) == 0 {
		return kit.MethodNotAllowed([]string{http.MethodGet}), nil, nil
	}

	name := actionNameFromQuery(r.URL.RawQuery)
	fn, ok := actions[name]
	if !ok {
		return &kit.Response{
			Status:  http.StatusNotFound,
			Headers: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			Body:    []byte("action not found: " + name),
		}, nil, nil
	}

	result := s.hooks.HandleAction(ev, name, fn)
	switch v := result.(type) {
	case kit.ActionRedirectResult:
		code := v.Code
		if code == 0 {
			code = http.StatusSeeOther
		}
		return &kit.Response{
			Status:  code,
			Headers: http.Header{"Location": []string{v.Location}},
		}, nil, nil
	case kit.ActionData:
		return nil, &formData{code: codeOr(v.Code, http.StatusOK), data: v.Data}, nil
	case kit.ActionFailData:
		return nil, &formData{code: codeOr(v.Code, http.StatusBadRequest), data: v.Data}, nil
	case nil:
		return nil, nil, nil
	default:
		return nil, nil, errUnknownActionResult{}
	}
}

// formData carries an ActionData / ActionFailData payload from the
// dispatcher into renderPage so the page can expose it as PageData.Form.
type formData struct {
	code int
	data any
}

// errUnknownActionResult is returned when a custom ActionResult escapes
// the sealed sum. Defensive only: kit's unexported sealedAction method
// already prevents this in well-behaved code.
type errUnknownActionResult struct{}

func (errUnknownActionResult) Error() string {
	return "server: unknown ActionResult variant"
}

// actionNameFromQuery extracts the leading `/<name>` token from a raw
// query string. SvelteKit encodes the action name as a bare key (e.g.
// `?/submit`), not as a value, so url.ParseQuery sees it as the empty
// value of `/submit`. The default action key is "default".
func actionNameFromQuery(raw string) string {
	for raw != "" {
		var part string
		if i := strings.IndexByte(raw, '&'); i >= 0 {
			part, raw = raw[:i], raw[i+1:]
		} else {
			part, raw = raw, ""
		}
		if len(part) == 0 || part[0] != '/' {
			continue
		}
		name := part[1:]
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if name == "" {
			return "default"
		}
		return name
	}
	return "default"
}

func codeOr(code, fallback int) int {
	if code == 0 {
		return fallback
	}
	return code
}
