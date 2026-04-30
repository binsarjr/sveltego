package kit_test

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
)

func TestStreamResolvesValue(t *testing.T) {
	t.Parallel()
	s := kit.Stream(func() (int, error) { return 42, nil })
	got, err := s.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait err = %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
	if !s.IsResolved() {
		t.Fatalf("IsResolved = false after successful Wait")
	}
}

func TestStreamPropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	s := kit.Stream(func() (string, error) { return "", want })
	_, err := s.Wait(context.Background(), time.Second)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestStreamTimeout(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	s := kit.Stream(func() (int, error) {
		<-release
		return 1, nil
	})
	defer close(release)
	_, err := s.Wait(context.Background(), 5*time.Millisecond)
	if !errors.Is(err, kit.ErrStreamTimeout) {
		t.Fatalf("err = %v, want ErrStreamTimeout", err)
	}
}

func TestStreamContextCancel(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	s := kit.Stream(func() (int, error) {
		<-release
		return 1, nil
	})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Wait(ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStreamNoTimeoutHonorsContext(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	s := kit.Stream(func() (int, error) {
		<-release
		return 1, nil
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := s.Wait(ctx, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestStreamUniqueIDs(t *testing.T) {
	t.Parallel()
	const n = 64
	ids := make(map[uint64]struct{}, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := kit.Stream(func() (int, error) { return 0, nil })
			mu.Lock()
			ids[s.ID()] = struct{}{}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(ids) != n {
		t.Fatalf("got %d unique IDs, want %d", len(ids), n)
	}
}

func TestStreamConcurrentReaders(t *testing.T) {
	t.Parallel()
	s := kit.Stream(func() (int, error) {
		time.Sleep(5 * time.Millisecond)
		return 7, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := s.Wait(context.Background(), time.Second)
			if err != nil || got != 7 {
				t.Errorf("got=%d err=%v want 7,nil", got, err)
			}
		}()
	}
	wg.Wait()
}

func TestStreamCtx_PropagatesCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan error, 1)
	s := kit.StreamCtx(ctx, func(c context.Context) (int, error) {
		<-c.Done()
		exited <- c.Err()
		return 0, c.Err()
	})
	cancel()
	select {
	case err := <-exited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("producer ctx.Err = %v, want Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("producer did not exit within 1s after parent cancel")
	}
	got, err := s.Wait(context.Background(), time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait err = %v, want Canceled", err)
	}
	if got != 0 {
		t.Fatalf("Wait value = %d, want 0", got)
	}
}

func TestStreamCtx_CancelMethod(t *testing.T) {
	t.Parallel()
	exited := make(chan struct{})
	s := kit.StreamCtx(context.Background(), func(c context.Context) (int, error) {
		<-c.Done()
		close(exited)
		return 0, c.Err()
	})
	s.Cancel()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("producer did not exit within 1s after Cancel()")
	}
	// Cancel must be idempotent.
	s.Cancel()
	s.Cancel()
}

func TestStreamCtx_NilCancelOnNilStreamed(t *testing.T) {
	t.Parallel()
	var s *kit.Streamed[int]
	s.Cancel() // must not panic
}

// TestStreamCtx_NoLeakAfterRequestCancel simulates the streaming
// pipeline contract: a slow producer is started inside an HTTP handler,
// the client disconnects (request ctx cancels), and the goroutine
// receives the cancellation through StreamCtx and exits. Verifies the
// goroutine count returns to baseline.
func TestStreamCtx_NoLeakAfterRequestCancel(t *testing.T) {
	t.Parallel()

	producerStarted := make(chan struct{})
	producerExited := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_ = kit.StreamCtx(r.Context(), func(c context.Context) (int, error) {
			close(producerStarted)
			<-c.Done()
			close(producerExited)
			return 0, c.Err()
		})
		// Block until r.Context() fires so the producer outlives the
		// handler entry but exits via ctx cancel, mirroring the leak
		// scenario from issue #211.
		<-r.Context().Done()
	}))
	defer srv.Close()

	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	reqDone := make(chan struct{})
	go func() {
		defer close(reqDone)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		if err != nil {
			return
		}
		resp, err := srv.Client().Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()

	select {
	case <-producerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("producer never started")
	}

	cancel()

	select {
	case <-producerExited:
	case <-time.After(2 * time.Second):
		t.Fatal("producer goroutine did not exit after request cancellation")
	}
	<-reqDone

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: baseline=%d current=%d", baseline, runtime.NumGoroutine())
}

// --- StreamedChan tests ---

// TestStreamedChan_ThreeValues verifies that all three values pushed into the
// channel reach the resolved result (last one wins, matching the drain contract).
func TestStreamedChan_ThreeValues(t *testing.T) {
	t.Parallel()
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	s := kit.StreamedChan(context.Background(), ch)
	got, err := s.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait err = %v", err)
	}
	if got != 3 {
		t.Fatalf("got %d, want 3 (last value)", got)
	}
}

// TestStreamedChan_CtxCancelMidStream verifies that cancelling the request
// context stops draining and surfaces context.Canceled.
func TestStreamedChan_CtxCancelMidStream(t *testing.T) {
	t.Parallel()

	// Unbuffered so the drain blocks after reading the first value.
	ch := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())

	s := kit.StreamedChan(ctx, ch)

	// Send first value, then cancel before sending more.
	ch <- 10
	cancel()

	_, err := s.Wait(context.Background(), 2*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	// Drain any leftover read attempt by closing the channel so the goroutine
	// doesn't leak across test boundaries.
	close(ch)
}

// TestStreamedChan_EmptyChannel resolves with zero and nil when the channel
// is closed before any value arrives.
func TestStreamedChan_EmptyChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan string)
	close(ch)

	s := kit.StreamedChan(context.Background(), ch)
	got, err := s.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait err = %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

// --- StreamedSeq tests ---

// intSeq returns an iter.Seq2[int, error] that yields the given values with
// nil errors, making it easy to drive StreamedSeq in tests.
func intSeq(vals ...int) iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		for _, v := range vals {
			if !yield(v, nil) {
				return
			}
		}
	}
}

// TestStreamedSeq_LastValueWins confirms that StreamedSeq resolves with the
// final yielded element (last write wins, matching the drain contract).
func TestStreamedSeq_LastValueWins(t *testing.T) {
	t.Parallel()
	s := kit.StreamedSeq(context.Background(), intSeq(10, 20, 30))
	got, err := s.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait err = %v", err)
	}
	if got != 30 {
		t.Fatalf("got %d, want 30", got)
	}
}

// TestStreamedSeq_ErrorTerminates verifies that the first non-nil error from
// the iterator becomes the resolved error and stops further iteration.
func TestStreamedSeq_ErrorTerminates(t *testing.T) {
	t.Parallel()
	boom := errors.New("seq error")
	seq := func(yield func(int, error) bool) {
		yield(1, nil)
		yield(2, boom) // should stop here
		yield(3, nil)  // must not be reached
	}
	s := kit.StreamedSeq(context.Background(), seq)
	_, err := s.Wait(context.Background(), time.Second)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// TestStreamedSeq_CtxCancelStopsIteration confirms that a pre-cancelled
// context causes StreamedSeq to stop after the first yield and surface
// context.Canceled. Using an already-cancelled context makes the test
// deterministic: the Done channel is closed before the drain goroutine
// ever calls yield, so the first yield's cancellation check fires
// immediately.
func TestStreamedSeq_CtxCancelStopsIteration(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so Done is already closed

	reached := 0
	seq := func(yield func(int, error) bool) {
		for i := range 10 {
			reached++
			if !yield(i, nil) {
				return
			}
		}
	}

	s := kit.StreamedSeq(ctx, seq)
	_, err := s.Wait(context.Background(), 2*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	// The drain must have stopped before processing all 10 values.
	if reached >= 10 {
		t.Fatalf("seq was not stopped early: reached %d iterations", reached)
	}
}

// TestStreamedSeq_EmptySeq resolves with zero and nil for an empty iterator.
func TestStreamedSeq_EmptySeq(t *testing.T) {
	t.Parallel()
	empty := func(yield func(int, error) bool) {}
	s := kit.StreamedSeq(context.Background(), iter.Seq2[int, error](empty))
	got, err := s.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait err = %v", err)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
