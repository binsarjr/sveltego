package kit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
)

// wrapErr is a minimal error wrapper used by tests to avoid the fmt import.
type wrapErr struct {
	msg string
	err error
}

func (w *wrapErr) Error() string { return w.msg + ": " + w.err.Error() }
func (w *wrapErr) Unwrap() error { return w.err }

func TestRedirect_Helper(t *testing.T) {
	t.Parallel()

	err := kit.Redirect(303, "/login")
	if err == nil {
		t.Fatal("Redirect returned nil")
	}

	var redir *kit.RedirectErr
	if !errors.As(err, &redir) {
		t.Fatalf("errors.As(*RedirectErr) failed: %v", err)
	}
	if redir.Code != 303 {
		t.Errorf("Code = %d, want 303", redir.Code)
	}
	if redir.Location != "/login" {
		t.Errorf("Location = %q, want /login", redir.Location)
	}
	if redir.HTTPStatus() != 303 {
		t.Errorf("HTTPStatus = %d, want 303", redir.HTTPStatus())
	}
	msg := redir.Error()
	if !strings.Contains(msg, "303") || !strings.Contains(msg, "/login") {
		t.Errorf("Error() = %q, want substrings 303 and /login", msg)
	}
}

func TestError_WithMessage(t *testing.T) {
	t.Parallel()

	err := kit.Error(404, "post not found")
	if err == nil {
		t.Fatal("Error returned nil")
	}

	var herr *kit.HTTPErr
	if !errors.As(err, &herr) {
		t.Fatalf("errors.As(*HTTPErr) failed: %v", err)
	}
	if herr.Code != 404 {
		t.Errorf("Code = %d, want 404", herr.Code)
	}
	if herr.Message != "post not found" {
		t.Errorf("Message = %q, want post not found", herr.Message)
	}
	if herr.HTTPStatus() != 404 {
		t.Errorf("HTTPStatus = %d, want 404", herr.HTTPStatus())
	}
	if herr.Error() != "post not found" {
		t.Errorf("Error() = %q, want post not found", herr.Error())
	}
}

func TestError_DefaultMessage(t *testing.T) {
	t.Parallel()

	err := kit.Error(404)
	if err == nil {
		t.Fatal("Error returned nil")
	}

	var herr *kit.HTTPErr
	if !errors.As(err, &herr) {
		t.Fatalf("errors.As(*HTTPErr) failed: %v", err)
	}
	if herr.Code != 404 {
		t.Errorf("Code = %d, want 404", herr.Code)
	}
	if herr.Message != "Not Found" {
		t.Errorf("Message = %q, want \"Not Found\"", herr.Message)
	}
	if herr.HTTPStatus() != 404 {
		t.Errorf("HTTPStatus = %d, want 404", herr.HTTPStatus())
	}
	if herr.Error() != "Not Found" {
		t.Errorf("Error() = %q, want \"Not Found\"", herr.Error())
	}
}

func TestError_DefaultMessage_500(t *testing.T) {
	t.Parallel()

	err := kit.Error(500)
	var herr *kit.HTTPErr
	if !errors.As(err, &herr) {
		t.Fatalf("errors.As(*HTTPErr) failed: %v", err)
	}
	if herr.Message != "Internal Server Error" {
		t.Errorf("Message = %q, want \"Internal Server Error\"", herr.Message)
	}
}

func TestFail_Helper(t *testing.T) {
	t.Parallel()

	data := map[string]string{"email": "required"}
	err := kit.Fail(400, data)
	if err == nil {
		t.Fatal("Fail returned nil")
	}

	var fe *kit.FailErr
	if !errors.As(err, &fe) {
		t.Fatalf("errors.As(*FailErr) failed: %v", err)
	}
	if fe.Code != 400 {
		t.Errorf("Code = %d, want 400", fe.Code)
	}
	got, ok := fe.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data type = %T, want map[string]string", fe.Data)
	}
	if got["email"] != "required" {
		t.Errorf("Data[email] = %q, want required", got["email"])
	}
	if fe.HTTPStatus() != 400 {
		t.Errorf("HTTPStatus = %d, want 400", fe.HTTPStatus())
	}
	if !strings.Contains(fe.Error(), "400") {
		t.Errorf("Error() = %q, want 400 substring", fe.Error())
	}
}

func TestSentinels_DistinctTypes(t *testing.T) {
	t.Parallel()

	red := kit.Redirect(303, "/x")
	herr := kit.Error(404, "no")
	fe := kit.Fail(400, nil)

	var r *kit.RedirectErr
	var h *kit.HTTPErr
	var f *kit.FailErr

	if !errors.As(red, &r) || errors.As(red, &h) || errors.As(red, &f) {
		t.Error("Redirect must match only *RedirectErr")
	}
	if !errors.As(herr, &h) || errors.As(herr, &r) || errors.As(herr, &f) {
		t.Error("Error must match only *HTTPErr")
	}
	if !errors.As(fe, &f) || errors.As(fe, &r) || errors.As(fe, &h) {
		t.Error("Fail must match only *FailErr")
	}
}

func TestRedirectReload_SetsForceReload(t *testing.T) {
	t.Parallel()

	err := kit.Redirect(303, "/login", kit.RedirectReload())

	var redir *kit.RedirectErr
	if !errors.As(err, &redir) {
		t.Fatalf("errors.As(*RedirectErr) failed")
	}
	if !redir.ForceReload {
		t.Error("ForceReload = false, want true")
	}
	if redir.Code != 303 {
		t.Errorf("Code = %d, want 303", redir.Code)
	}
	if redir.Location != "/login" {
		t.Errorf("Location = %q, want /login", redir.Location)
	}
}

func TestRedirect_WithoutReload_ForceReloadFalse(t *testing.T) {
	t.Parallel()

	err := kit.Redirect(303, "/login")

	var redir *kit.RedirectErr
	if !errors.As(err, &redir) {
		t.Fatalf("errors.As(*RedirectErr) failed")
	}
	if redir.ForceReload {
		t.Error("ForceReload = true, want false for plain Redirect")
	}
}

// userNotFoundError is a domain error type that implements kit.HTTPError.
type userNotFoundError struct {
	ID string
}

func (e *userNotFoundError) Error() string  { return "not found: " + e.ID }
func (e *userNotFoundError) Status() int    { return 404 }
func (e *userNotFoundError) Public() string { return "The requested item does not exist." }

func TestHTTPError_InterfaceSatisfied(t *testing.T) {
	t.Parallel()

	var _ kit.HTTPError = (*userNotFoundError)(nil)

	err := &userNotFoundError{ID: "abc"}
	var he kit.HTTPError
	if !errors.As(err, &he) {
		t.Fatal("errors.As(kit.HTTPError) failed")
	}
	if he.Status() != 404 {
		t.Errorf("Status = %d, want 404", he.Status())
	}
	if he.Public() != "The requested item does not exist." {
		t.Errorf("Public = %q, unexpected", he.Public())
	}
	if he.Error() != "not found: abc" {
		t.Errorf("Error() = %q, want 'not found: abc'", he.Error())
	}
}

func TestHTTPError_WrappedErrorDetected(t *testing.T) {
	t.Parallel()

	inner := &userNotFoundError{ID: "xyz"}
	wrapped := &wrapErr{msg: "load failed", err: inner}

	var he kit.HTTPError
	if !errors.As(wrapped, &he) {
		t.Fatal("errors.As through wrapping failed")
	}
	if he.Status() != 404 {
		t.Errorf("Status = %d, want 404 through wrapping", he.Status())
	}
}
