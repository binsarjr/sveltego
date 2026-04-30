package kit_test

import (
	"context"
	"errors"
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
