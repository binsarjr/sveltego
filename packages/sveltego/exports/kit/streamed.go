package kit

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"time"
)

// DefaultStreamTimeout caps how long a Streamed value's goroutine is
// awaited during render before its placeholder resolves to an error.
const DefaultStreamTimeout = 30 * time.Second

// ErrStreamTimeout is reported when Streamed.Wait returns because its
// timeout elapsed before the producer goroutine completed.
var ErrStreamTimeout = errors.New("kit: stream timeout")

// ErrClientGone is returned by the chunk writer when a client disconnects
// mid-stream (broken pipe, closed connection, cancelled request). The
// pipeline logs it once at debug level and suppresses further writes;
// it never routes through HandleError because a disconnect is not a
// server-side fault.
var ErrClientGone = errors.New("kit: client disconnected")

// streamIDCounter assigns unique IDs to Streamed values within a process.
// Render path uses the ID as the data-stream attribute and as the first
// argument to __sveltego__resolve so the client patches the right slot.
var streamIDCounter atomic.Uint64

// Streamed is a future-style value Load may place inside its returned
// PageData. The render path emits a placeholder for the field, flushes
// the shell to the client, then waits for resolution before writing a
// patch script that hydrates the slot.
//
// Construct via StreamCtx (preferred) or Stream. The zero value is not
// usable; copying a Streamed after construction is unsupported because
// the producer goroutine writes through its pointer.
type Streamed[T any] struct {
	id     uint64
	done   chan struct{}
	cancel context.CancelFunc
	result T
	err    error
}

// StreamedAny is the type-erased view of a Streamed[T] used by the
// render pipeline to register streams via reflection without binding to
// a concrete type parameter. User code typically does not implement this
// interface; *Streamed[T] satisfies it implicitly.
type StreamedAny interface {
	StreamID() uint64
	WaitAny(ctx context.Context, timeout time.Duration) (any, error)
}

// Stream spawns fn in a goroutine and returns a Streamed[T] whose Wait
// resolves with fn's return values. The goroutine starts immediately so
// slow work overlaps with shell rendering.
//
// Deprecated: Use StreamCtx so the producer receives a cancellable context
// and exits promptly when the request goes away. Stream is implemented in
// terms of StreamCtx with a background parent, so Cancel still works on
// the returned value but the producer fn cannot observe cancellation directly.
func Stream[T any](fn func() (T, error)) *Streamed[T] {
	return StreamCtx(context.Background(), func(context.Context) (T, error) {
		return fn()
	})
}

// StreamCtx spawns fn in a goroutine bound to a child of ctx and returns
// a Streamed[T] whose Wait resolves with fn's return values. The child
// context is cancelled when fn returns, when the parent ctx is
// cancelled, or when Cancel is called on the Streamed; producers that
// honor ctx.Done() exit promptly in all three cases.
//
// The goroutine starts immediately so slow work overlaps with shell
// rendering. Callers that orphan the returned Streamed (never wait, never
// cancel) leak a goroutine until fn returns on its own; the streaming
// pipeline calls Cancel when the request context dies before the patch
// script is emitted, so production code does not need to do this
// manually.
//
// ctx must not be nil.
func StreamCtx[T any](ctx context.Context, fn func(context.Context) (T, error)) *Streamed[T] {
	derived, cancel := context.WithCancel(ctx)
	s := &Streamed[T]{
		id:     streamIDCounter.Add(1),
		done:   make(chan struct{}),
		cancel: cancel,
	}
	go func() {
		defer close(s.done)
		defer cancel()
		s.result, s.err = fn(derived)
	}()
	return s
}

// Cancel signals the producer goroutine to exit by cancelling the
// context passed to fn. Cancel is safe to call multiple times and from
// any goroutine; it returns immediately and does not wait for the
// producer to finish. The streaming pipeline calls Cancel when the
// request context is cancelled before the stream resolves so DB queries,
// HTTP fetches, and other ctx-aware work do not outlive the request.
func (s *Streamed[T]) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
}

// ID returns the unique identifier assigned at Stream time. The render
// path emits this as the placeholder's data-stream attribute.
func (s *Streamed[T]) ID() uint64 {
	if s == nil {
		return 0
	}
	return s.id
}

// StreamID exposes ID through the StreamedAny interface.
func (s *Streamed[T]) StreamID() uint64 {
	return s.ID()
}

// IsResolved reports whether the producer goroutine has finished.
func (s *Streamed[T]) IsResolved() bool {
	if s == nil {
		return false
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// Wait blocks until the producer goroutine completes, ctx is cancelled,
// or timeout elapses. A zero timeout disables the timeout branch and
// only honors ctx. The returned error is fn's error on success path,
// ctx.Err() on cancellation, or ErrStreamTimeout on timeout.
func (s *Streamed[T]) Wait(ctx context.Context, timeout time.Duration) (T, error) {
	var zero T
	if s == nil {
		return zero, errors.New("kit: nil Streamed")
	}
	if timeout <= 0 {
		select {
		case <-s.done:
			return s.result, s.err
		case <-ctxDone(ctx):
			return zero, ctx.Err()
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return s.result, s.err
	case <-ctxDone(ctx):
		return zero, ctx.Err()
	case <-timer.C:
		return zero, ErrStreamTimeout
	}
}

// WaitAny is the type-erased Wait used by the render pipeline. The
// returned interface boxes the concrete T so reflection can JSON-encode
// it without re-walking the Streamed wrapper.
func (s *Streamed[T]) WaitAny(ctx context.Context, timeout time.Duration) (any, error) {
	v, err := s.Wait(ctx, timeout)
	return v, err
}

// StreamedSeq drains seq in a goroutine and resolves with the last value
// yielded (and the first non-nil error encountered, if any). Iteration
// stops early when ctx is cancelled. The goroutine starts immediately so
// work overlaps with shell rendering.
//
// Each (v, nil) pair from seq overwrites the accumulated value; a non-nil
// error terminates the drain and becomes the resolved error. When no value
// was yielded before an error or cancellation, the zero value of T is
// returned.
//
// ctx must not be nil.
//
// Note: the current streaming pipeline resolves Streamed[T] as a single
// value. Full chunk-by-chunk SSE emission requires the hydration format
// defined in issue #35.
func StreamedSeq[T any](ctx context.Context, seq iter.Seq2[T, error]) *Streamed[T] {
	return StreamCtx(ctx, func(c context.Context) (T, error) {
		var (
			last     T
			firstErr error
		)
		// Call seq directly with a custom yield so we can return false to
		// stop iteration early without the "continued after false" panic that
		// a bare return inside for-range would cause.
		seq(func(v T, err error) bool {
			// Check cancellation before accepting the yielded value.
			select {
			case <-c.Done():
				firstErr = c.Err()
				return false // stop iteration
			default:
			}
			if err != nil {
				firstErr = err
				return false // stop iteration
			}
			last = v
			return true
		})
		return last, firstErr
	})
}

// StreamedChan drains ch in a goroutine and resolves with the last value
// received. Draining stops when ch is closed or ctx is cancelled.
// The goroutine starts immediately so work overlaps with shell rendering.
//
// Values are accumulated in order; the resolved result is the final
// received value. When the channel is closed before any value arrives,
// the zero value of T is returned with a nil error.
//
// ctx must not be nil.
//
// Note: the current streaming pipeline resolves Streamed[T] as a single
// value. Full chunk-by-chunk SSE emission requires the hydration format
// defined in issue #35.
func StreamedChan[T any](ctx context.Context, ch <-chan T) *Streamed[T] {
	return StreamCtx(ctx, func(c context.Context) (T, error) {
		var last T
		for {
			select {
			case <-c.Done():
				return last, c.Err()
			case v, ok := <-ch:
				if !ok {
					return last, nil
				}
				last = v
			}
		}
	})
}

// ctxDone returns ctx.Done() or a nil channel when ctx is nil. A nil
// channel blocks forever in select, which is the desired behavior for
// callers that pass a nil context.
func ctxDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}
