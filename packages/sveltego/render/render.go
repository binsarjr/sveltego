package render

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// jsonEncoder pools a json.Encoder bound to a settable target. Encoders
// retain their writer permanently, so we wrap *Writer in an adapter
// whose target swaps per Encode call. Reusing the encoder skips per-call
// allocation of the encoder's internal state.
type jsonEncoder struct {
	target *Writer
	enc    *json.Encoder
}

func (j *jsonEncoder) Write(p []byte) (int, error) {
	j.target.buf = append(j.target.buf, p...)
	return len(p), nil
}

var jsonPool = sync.Pool{
	New: func() any {
		j := &jsonEncoder{}
		j.enc = json.NewEncoder(j)
		j.enc.SetEscapeHTML(true)
		return j
	},
}

const defaultBufCap = 4096

// Writer accumulates HTML into an internal buffer. Designed for per-request
// reuse via Acquire and Release; do not share a Writer across goroutines.
type Writer struct {
	buf []byte
}

var pool = sync.Pool{
	New: func() any {
		return &Writer{buf: make([]byte, 0, defaultBufCap)}
	},
}

// New returns a Writer with default buffer capacity. Most callers should
// use Acquire instead so the underlying buffer is recycled.
func New() *Writer {
	return &Writer{buf: make([]byte, 0, defaultBufCap)}
}

// Acquire fetches a Writer from a sync.Pool, ready for fresh use. The
// caller owns the Writer until Release.
func Acquire() *Writer {
	w, _ := pool.Get().(*Writer)
	return w
}

// Release returns w to the pool. The buffer is reset; callers must drop
// every reference to w (and to slices it returned) before calling Release.
func Release(w *Writer) {
	if w == nil {
		return
	}
	w.Reset()
	pool.Put(w)
}

// WriteString appends a pre-trusted literal. Codegen emits this for source
// template constants; no escaping is performed.
func (w *Writer) WriteString(s string) {
	w.buf = append(w.buf, s...)
}

// WriteRaw appends a pre-trusted dynamic string. Identical to WriteString
// in behavior, but documents intent: codegen emits this for {@html expr}
// so the unsafe path is greppable in audits.
func (w *Writer) WriteRaw(s string) {
	w.buf = append(w.buf, s...)
}

// WriteRawBytes appends a pre-trusted []byte. Equivalent to WriteRaw but
// avoids the []byte->string conversion at call sites that already hold
// bytes (e.g. the head splice on the render hot path).
func (w *Writer) WriteRawBytes(p []byte) {
	w.buf = append(w.buf, p...)
}

// WriteEscape appends v HTML-escaped for text context. Numeric and bool
// values bypass fmt to avoid allocations on the hot path. nil emits an
// empty string. A value that implements both error and fmt.Stringer is
// rendered via Error(); types that need Stringer precedence should not
// also satisfy error.
func (w *Writer) WriteEscape(v any) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		w.buf = appendEscapeText(w.buf, x)
	case []byte:
		w.buf = appendEscapeText(w.buf, string(x))
	case int:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int8:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int16:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int32:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int64:
		w.buf = strconv.AppendInt(w.buf, x, 10)
	case uint:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint8:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint16:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint32:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint64:
		w.buf = strconv.AppendUint(w.buf, x, 10)
	case float32:
		w.buf = strconv.AppendFloat(w.buf, float64(x), 'g', -1, 32)
	case float64:
		w.buf = strconv.AppendFloat(w.buf, x, 'g', -1, 64)
	case bool:
		w.buf = strconv.AppendBool(w.buf, x)
	case error:
		w.buf = appendEscapeText(w.buf, x.Error())
	case fmt.Stringer:
		w.buf = appendEscapeText(w.buf, x.String())
	default:
		w.buf = appendEscapeTextBytes(w.buf, fmt.Appendf(nil, "%v", v))
	}
}

// WriteEscapeAttr appends v escaped for an attribute value. Codegen wraps
// attribute values in double quotes outside this call. Numeric and bool values
// bypass fmt. nil emits an empty string. A value that implements both error
// and fmt.Stringer is rendered via Error(); types that need Stringer
// precedence should not also satisfy error.
func (w *Writer) WriteEscapeAttr(v any) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		w.buf = appendEscapeAttr(w.buf, x)
	case []byte:
		w.buf = appendEscapeAttr(w.buf, string(x))
	case int:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int8:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int16:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int32:
		w.buf = strconv.AppendInt(w.buf, int64(x), 10)
	case int64:
		w.buf = strconv.AppendInt(w.buf, x, 10)
	case uint:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint8:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint16:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint32:
		w.buf = strconv.AppendUint(w.buf, uint64(x), 10)
	case uint64:
		w.buf = strconv.AppendUint(w.buf, x, 10)
	case float32:
		w.buf = strconv.AppendFloat(w.buf, float64(x), 'g', -1, 32)
	case float64:
		w.buf = strconv.AppendFloat(w.buf, x, 'g', -1, 64)
	case bool:
		w.buf = strconv.AppendBool(w.buf, x)
	case error:
		w.buf = appendEscapeAttr(w.buf, x.Error())
	case fmt.Stringer:
		w.buf = appendEscapeAttr(w.buf, x.String())
	default:
		w.buf = appendEscapeAttrBytes(w.buf, fmt.Appendf(nil, "%v", v))
	}
}

// WriteJSON appends v as JSON-encoded HTML-safe payload, suitable for
// embedding inside a <script> tag. The encoder's default HTML escaping
// rewrites <, >, &, U+2028, U+2029 inside strings to \u escapes.
func (w *Writer) WriteJSON(v any) error {
	j, _ := jsonPool.Get().(*jsonEncoder)
	j.target = w
	err := j.enc.Encode(v)
	j.target = nil
	jsonPool.Put(j)
	if err != nil {
		return fmt.Errorf("render: encode json: %w", err)
	}
	if n := len(w.buf); n > 0 && w.buf[n-1] == '\n' {
		w.buf = w.buf[:n-1]
	}
	return nil
}

// Bytes returns the accumulated buffer. The returned slice aliases the
// Writer's buffer until the next mutating call or Release; callers must
// not mutate it.
func (w *Writer) Bytes() []byte {
	return w.buf
}

// Len returns the current buffer length in bytes.
func (w *Writer) Len() int {
	return len(w.buf)
}

// maxPooledBufCap is the largest buffer capacity kept when a Writer returns
// to the pool. Buffers grown beyond this by a single large render are dropped
// and replaced with a fresh default-capacity buffer so one rogue request
// cannot permanently bloat the pool's steady-state footprint.
const maxPooledBufCap = 64 * 1024

// Reset truncates the buffer to zero length. If the buffer's capacity exceeds
// maxPooledBufCap the backing array is released and replaced with a new
// default-sized one; this prevents a single large render from permanently
// bloating the pool.
func (w *Writer) Reset() {
	if cap(w.buf) > maxPooledBufCap {
		w.buf = make([]byte, 0, defaultBufCap)
		return
	}
	w.buf = w.buf[:0]
}
