package devserver

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestWatcherDetectsSvelteChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	page := filepath.Join(root, "src", "routes", "+page.svelte")
	if err := os.WriteFile(page, []byte("<h1>v1</h1>"), 0o644); err != nil {
		t.Fatalf("seed page: %v", err)
	}

	w, err := NewWatcher(root, quietLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()

	// Give the watcher a moment to register, then write.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(page, []byte("<h1>v2</h1>"), 0o644); err != nil {
		t.Fatalf("rewrite page: %v", err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != ChangeSvelte {
			t.Errorf("kind = %v, want ChangeSvelte", ev.Kind)
		}
		if len(ev.Paths) == 0 {
			t.Errorf("paths empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event in 2s")
	}

	cancel()
	<-done
}

func TestWatcherClassifiesGoChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	server := filepath.Join(root, "src", "routes", "page.server.go")
	if err := os.WriteFile(server, []byte("//go:build sveltego\npackage routes\n"), 0o644); err != nil {
		t.Fatalf("seed server: %v", err)
	}

	w, err := NewWatcher(root, quietLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go w.Run(ctx) //nolint:errcheck // test cancels ctx

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(server, []byte("//go:build sveltego\npackage routes\n// edit\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != ChangeGo {
			t.Errorf("kind = %v, want ChangeGo", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event in 2s")
	}
}

func TestWatcherIgnoresGenChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir gen: %v", err)
	}

	w, err := NewWatcher(root, quietLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Run(ctx) //nolint:errcheck

	gen := filepath.Join(root, ".gen", "manifest.gen.go")
	if err := os.WriteFile(gen, []byte("package gen\n"), 0o644); err != nil {
		t.Fatalf("write gen: %v", err)
	}

	select {
	case ev := <-w.Events():
		t.Errorf("unexpected event for .gen change: %+v", ev)
	case <-time.After(300 * time.Millisecond):
		// expected: no event
	}
}

func TestWatcherCoalescesBurst(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	page := filepath.Join(root, "src", "routes", "+page.svelte")
	if err := os.WriteFile(page, []byte("v1"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w, err := NewWatcher(root, quietLogger())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Run(ctx) //nolint:errcheck

	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(page, []byte{byte('a' + i)}, 0o644)
		time.Sleep(10 * time.Millisecond)
	}

	// Should receive exactly one batch within 500ms.
	select {
	case <-w.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("no event in 2s")
	}
	// And no second event in the next 200ms.
	select {
	case ev := <-w.Events():
		t.Errorf("unexpected second event: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}
