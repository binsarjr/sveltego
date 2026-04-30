package kit

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestJSON_serializes(t *testing.T) {
	t.Parallel()
	res := JSON(http.StatusCreated, M{"name": "ada"})
	if res.Status != http.StatusCreated {
		t.Fatalf("status = %d", res.Status)
	}
	if got := res.Headers.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	if !bytes.Contains(res.Body, []byte(`"ada"`)) {
		t.Fatalf("body = %s", res.Body)
	}
}

func TestJSON_marshalErrorFalls500(t *testing.T) {
	t.Parallel()
	res := JSON(http.StatusOK, make(chan int))
	if res.Status != http.StatusInternalServerError {
		t.Fatalf("expected 500 on marshal error, got %d", res.Status)
	}
	if got := res.Headers.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("content-type = %q", got)
	}
}

func TestText_setsHeaders(t *testing.T) {
	t.Parallel()
	res := Text(http.StatusOK, "hi")
	if string(res.Body) != "hi" {
		t.Fatalf("body = %q", res.Body)
	}
	if got := res.Headers.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestXML_setsHeaders(t *testing.T) {
	t.Parallel()
	body := []byte(`<?xml version="1.0"?><root/>`)
	res := XML(http.StatusOK, body)
	if string(res.Body) != string(body) {
		t.Fatalf("body = %q", res.Body)
	}
	if got := res.Headers.Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestNoContent_status(t *testing.T) {
	t.Parallel()
	res := NoContent()
	if res.Status != http.StatusNoContent {
		t.Fatalf("status = %d", res.Status)
	}
	if len(res.Body) != 0 {
		t.Fatalf("body should be empty, got %q", res.Body)
	}
}

func TestMethodNotAllowed_AllowHeaderSorted(t *testing.T) {
	t.Parallel()
	res := MethodNotAllowed([]string{"POST", "GET", "DELETE"})
	if res.Status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", res.Status)
	}
	if got := res.Headers.Get("Allow"); got != "DELETE, GET, POST" {
		t.Fatalf("Allow = %q", got)
	}
}
