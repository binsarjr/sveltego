package render

import (
	"io"
	"net/http"
)

// FlushTo writes any buffered content to dst, calls dst.Flush when dst
// implements http.Flusher, and resets the underlying buffer. Used by the
// streaming render path between the shell and each chunk so the client
// sees progressive output instead of one consolidated body. Returns the
// io.Writer error (if any) without flushing on failure.
func (w *Writer) FlushTo(dst io.Writer) error {
	if w == nil || dst == nil {
		return nil
	}
	if len(w.buf) > 0 {
		if _, err := dst.Write(w.buf); err != nil {
			return err
		}
		w.buf = w.buf[:0]
	}
	if f, ok := dst.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
