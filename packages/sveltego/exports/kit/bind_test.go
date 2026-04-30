package kit_test

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
)

func newPostRequest(t *testing.T, body string, contentType string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	return req
}

func TestBindForm_HappyPath(t *testing.T) {
	t.Parallel()
	values := url.Values{
		"email":  {"alice@example.com"},
		"age":    {"30"},
		"notify": {"true"},
		"score":  {"4.5"},
		"tags":   {"go", "kit"},
	}
	req := newPostRequest(t, values.Encode(), "application/x-www-form-urlencoded")
	ev := kit.NewRequestEvent(req, nil)

	type form struct {
		Email  string   `form:"email"`
		Age    int      `form:"age"`
		Notify bool     `form:"notify"`
		Score  float64  `form:"score"`
		Tags   []string `form:"tags"`
	}
	var got form
	if err := ev.BindForm(&got); err != nil {
		t.Fatalf("BindForm: %v", err)
	}
	if got.Email != "alice@example.com" || got.Age != 30 || !got.Notify || got.Score != 4.5 {
		t.Fatalf("scalar mismatch: %#v", got)
	}
	if !equalStringSlice(got.Tags, []string{"go", "kit"}) {
		t.Fatalf("Tags = %v", got.Tags)
	}
}

func TestBindForm_DefaultLowercaseTag(t *testing.T) {
	t.Parallel()
	values := url.Values{"name": {"jdoe"}}
	req := newPostRequest(t, values.Encode(), "application/x-www-form-urlencoded")
	ev := kit.NewRequestEvent(req, nil)

	type form struct {
		Name string
	}
	var got form
	if err := ev.BindForm(&got); err != nil {
		t.Fatalf("BindForm: %v", err)
	}
	if got.Name != "jdoe" {
		t.Errorf("Name = %q, want jdoe", got.Name)
	}
}

func TestBindForm_InvalidIntAggregates(t *testing.T) {
	t.Parallel()
	values := url.Values{"age": {"notanumber"}, "max": {"alsobad"}}
	req := newPostRequest(t, values.Encode(), "application/x-www-form-urlencoded")
	ev := kit.NewRequestEvent(req, nil)

	type form struct {
		Age int `form:"age"`
		Max int `form:"max"`
	}
	var dst form
	err := ev.BindForm(&dst)
	if err == nil {
		t.Fatal("expected error")
	}
	var be *kit.BindError
	if !errors.As(err, &be) {
		t.Fatalf("expected *BindError, got %T (%v)", err, err)
	}
	if _, ok := be.FieldErrors["age"]; !ok {
		t.Errorf("missing age error: %#v", be.FieldErrors)
	}
	if _, ok := be.FieldErrors["max"]; !ok {
		t.Errorf("missing max error: %#v", be.FieldErrors)
	}
}

func TestBindForm_TimeRFC3339(t *testing.T) {
	t.Parallel()
	values := url.Values{"when": {"2026-04-30T12:00:00Z"}, "bad": {"not-a-time"}}
	req := newPostRequest(t, values.Encode(), "application/x-www-form-urlencoded")
	ev := kit.NewRequestEvent(req, nil)

	type form struct {
		When time.Time `form:"when"`
		Bad  time.Time `form:"bad"`
	}
	var dst form
	err := ev.BindForm(&dst)
	if err == nil {
		t.Fatal("expected aggregated time error")
	}
	var be *kit.BindError
	if !errors.As(err, &be) {
		t.Fatalf("expected *BindError, got %T", err)
	}
	if _, ok := be.FieldErrors["bad"]; !ok {
		t.Errorf("missing bad error: %#v", be.FieldErrors)
	}
	if !dst.When.Equal(time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("when = %v, want 2026-04-30T12:00:00Z", dst.When)
	}
}

func TestBindForm_NilDstRejected(t *testing.T) {
	t.Parallel()
	req := newPostRequest(t, "", "application/x-www-form-urlencoded")
	ev := kit.NewRequestEvent(req, nil)

	type form struct{ Email string }
	var nilPtr *form
	if err := ev.BindForm(nilPtr); err == nil {
		t.Fatal("expected error on nil pointer")
	}
	if err := ev.BindForm(form{}); err == nil {
		t.Fatal("expected error on non-pointer")
	}
}

func TestBindMultipart_FilesAndValues(t *testing.T) {
	t.Parallel()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("title", "hello"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := w.WriteField("tags", "a"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := w.WriteField("tags", "b"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	fw, err := w.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte("PNGDATA")); err != nil {
		t.Fatalf("file write: %v", err)
	}
	for _, n := range []string{"a.txt", "b.txt"} {
		fp, err := w.CreateFormFile("photos", n)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fp.Write([]byte(n)); err != nil {
			t.Fatalf("file write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	ev := kit.NewRequestEvent(req, nil)

	type form struct {
		Title  string                  `form:"title"`
		Tags   []string                `form:"tags"`
		Avatar *multipart.FileHeader   `form:"avatar"`
		Photos []*multipart.FileHeader `form:"photos"`
	}
	var got form
	if err := ev.BindMultipart(&got, 0); err != nil {
		t.Fatalf("BindMultipart: %v", err)
	}
	if got.Title != "hello" {
		t.Errorf("Title = %q", got.Title)
	}
	if !equalStringSlice(got.Tags, []string{"a", "b"}) {
		t.Errorf("Tags = %v", got.Tags)
	}
	if got.Avatar == nil || got.Avatar.Filename != "avatar.png" {
		t.Fatalf("Avatar = %#v", got.Avatar)
	}
	f, err := got.Avatar.Open()
	if err != nil {
		t.Fatalf("open avatar: %v", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read avatar: %v", err)
	}
	if string(data) != "PNGDATA" {
		t.Errorf("avatar bytes = %q", data)
	}
	if len(got.Photos) != 2 || got.Photos[0].Filename != "a.txt" || got.Photos[1].Filename != "b.txt" {
		t.Errorf("Photos = %#v", got.Photos)
	}

	files := ev.Files()
	if len(files["photos"]) != 2 {
		t.Errorf("Files()[photos] = %d, want 2", len(files["photos"]))
	}
}

func TestBindError_String(t *testing.T) {
	t.Parallel()
	be := &kit.BindError{FieldErrors: map[string]string{"b": "x", "a": "y"}}
	got := be.Error()
	if got != "bind: a: y; b: x" {
		t.Errorf("Error() = %q", got)
	}
	empty := &kit.BindError{}
	if empty.Error() == "" {
		t.Error("empty BindError.Error should not be empty string")
	}
}

func TestFiles_NoMultipartParsed(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	ev := kit.NewRequestEvent(req, nil)
	if got := ev.Files(); got != nil {
		t.Errorf("Files() before parse = %#v, want nil", got)
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
