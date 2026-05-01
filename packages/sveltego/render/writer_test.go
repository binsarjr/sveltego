package render_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/render"
)

func TestFlushToCopiesAndResets(t *testing.T) {
	t.Parallel()
	w := render.New()
	w.WriteString("hello")
	var dst bytes.Buffer
	if err := w.FlushTo(&dst); err != nil {
		t.Fatalf("FlushTo err = %v", err)
	}
	if got := dst.String(); got != "hello" {
		t.Fatalf("dst = %q, want %q", got, "hello")
	}
	if w.Len() != 0 {
		t.Fatalf("Len after flush = %d, want 0", w.Len())
	}
}

func TestFlushToInvokesHTTPFlusher(t *testing.T) {
	t.Parallel()
	w := render.New()
	w.WriteString("chunk")
	rec := &flushSpy{}
	if err := w.FlushTo(rec); err != nil {
		t.Fatalf("FlushTo err = %v", err)
	}
	if !rec.flushed {
		t.Fatalf("Flush not invoked on http.Flusher")
	}
	if rec.buf.String() != "chunk" {
		t.Fatalf("dst = %q, want %q", rec.buf.String(), "chunk")
	}
}

func TestFlushToEmptyBufferStillFlushes(t *testing.T) {
	t.Parallel()
	w := render.New()
	rec := &flushSpy{}
	if err := w.FlushTo(rec); err != nil {
		t.Fatalf("FlushTo err = %v", err)
	}
	if !rec.flushed {
		t.Fatalf("Flush should fire even when buffer empty")
	}
}

func TestFlushToNilWriterIsNoop(t *testing.T) {
	t.Parallel()
	var w *render.Writer
	if err := w.FlushTo(&bytes.Buffer{}); err != nil {
		t.Fatalf("FlushTo nil err = %v", err)
	}
}

func TestFlushToWriteError(t *testing.T) {
	t.Parallel()
	w := render.New()
	w.WriteString("hi")
	want := errors.New("disk full")
	got := w.FlushTo(errWriter{err: want})
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
	if w.Len() == 0 {
		t.Fatalf("buffer reset on write error; should be preserved")
	}
}

type flushSpy struct {
	buf     bytes.Buffer
	flushed bool
}

func (f *flushSpy) Write(p []byte) (int, error) { return f.buf.Write(p) }
func (f *flushSpy) Flush()                      { f.flushed = true }

type errWriter struct{ err error }

func (e errWriter) Write(_ []byte) (int, error) { return 0, e.err }
