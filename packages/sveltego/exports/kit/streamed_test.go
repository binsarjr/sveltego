package kit_test

import (
	"context"
	"errors"
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
