package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/runtime/router"
)

func TestCronTask_fires(t *testing.T) {
	t.Parallel()

	var count atomic.Int64
	srv, err := New(Config{
		Routes: []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x")}},
		Shell:  testShell,
		Logger: quietLogger(),
		CronTasks: []kit.CronTask{
			{
				Name: "counter",
				Spec: "@every 50ms",
				Fn: func(_ context.Context) error {
					count.Add(1)
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Give the task enough time to fire at least 3 times.
	time.Sleep(250 * time.Millisecond)

	got := count.Load()
	if got < 3 {
		t.Fatalf("cron task fired %d times in 250ms, want ≥3", got)
	}
}

func TestCronTask_stopsOnShutdown(t *testing.T) {
	t.Parallel()

	var count atomic.Int64
	srv, err := New(Config{
		Routes: []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x")}},
		Shell:  testShell,
		Logger: quietLogger(),
		CronTasks: []kit.CronTask{
			{
				Spec: "@every 20ms",
				Fn: func(_ context.Context) error {
					count.Add(1)
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := srv.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Let it fire a few times.
	time.Sleep(100 * time.Millisecond)

	// Cancel via Shutdown (no http.Server bound, so Shutdown just cancels cron).
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	after := count.Load()
	// Give goroutines a short window to exit.
	time.Sleep(80 * time.Millisecond)
	final := count.Load()

	// Counter must not keep climbing after shutdown.
	if final > after+1 {
		t.Fatalf("cron kept running after shutdown: count went from %d to %d", after, final)
	}
}

func TestCronTask_badSpecSkipped(t *testing.T) {
	t.Parallel()

	// A task with a bad spec must not prevent the server from starting.
	srv, err := New(Config{
		Routes: []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x")}},
		Shell:  testShell,
		Logger: quietLogger(),
		CronTasks: []kit.CronTask{
			{Spec: "*/5 * * * *", Fn: func(_ context.Context) error { return nil }},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Init must succeed even though the task spec is invalid.
	if err := srv.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
}
