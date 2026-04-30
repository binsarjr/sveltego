package kit

// ActionFn is the signature of one entry in an [ActionMap]. It runs on
// POST to a page route after dispatch resolves the entry from the
// request's `?/<name>` query (or the default key when absent). The
// returned [ActionResult] tells the pipeline how to finish the request.
type ActionFn = func(ev *RequestEvent) ActionResult

// ActionMap is the type a +page.server.go exports as `var Actions`.
// Keys are action names (`"default"`, `"submit"`, ...); the dispatcher
// matches the request's `?/<name>` query against them.
type ActionMap = map[string]ActionFn

// ActionResult is the sealed sum returned from an [ActionFn]. The three
// concrete types — [ActionData], [ActionFailData], [ActionRedirectResult]
// — are constructed via [ActionDataResult], [ActionFail], and
// [ActionRedirect]. The unexported sealedAction method keeps user code
// from defining additional variants.
type ActionResult interface {
	sealedAction()
}

// ActionData re-renders the page with Code as the response status and
// Data merged into the page's Form field.
type ActionData struct {
	Code int
	Data any
}

func (ActionData) sealedAction() {}

// ActionFailData re-renders the page with Code as the response status
// (typically 4xx) and Data merged into the page's Form field. Used to
// surface validation errors back to the form.
type ActionFailData struct {
	Code int
	Data any
}

func (ActionFailData) sealedAction() {}

// ActionRedirectResult short-circuits the page render and writes a
// redirect with Location and Code (defaulting to 303 when zero).
type ActionRedirectResult struct {
	Code     int
	Location string
}

func (ActionRedirectResult) sealedAction() {}

// ActionDataResult builds an [ActionData] result. Use code 200 for the
// happy path; the page re-renders with data bound to its Form field.
func ActionDataResult(code int, data any) ActionResult {
	return ActionData{Code: code, Data: data}
}

// ActionFail builds an [ActionFailData] result. The page re-renders
// with the same Form data and the response carries the failure code.
func ActionFail(code int, data any) ActionResult {
	return ActionFailData{Code: code, Data: data}
}

// ActionRedirect builds an [ActionRedirectResult]. Default Code is 303
// (POST -> GET) when the caller passes 0.
func ActionRedirect(code int, location string) ActionResult {
	if code == 0 {
		code = 303
	}
	return ActionRedirectResult{Code: code, Location: location}
}
