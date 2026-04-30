package mcp

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDocsIndexConcurrentLoadIsCached(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "intro.md"), "# Intro\n\nhello\n")
	mustWrite(t, filepath.Join(dir, "guide", "hooks.md"), "# Hooks\n\nbody\n")

	srv := New(Config{DocsDir: dir})

	const goroutines = 64

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		first atomic.Pointer[DocsIndex]
		mism  atomic.Int32
		errs  atomic.Int32
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			idx, err := srv.docsIndex()
			if err != nil {
				errs.Add(1)
				return
			}
			if first.CompareAndSwap(nil, idx) {
				return
			}
			if first.Load() != idx {
				mism.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if errs.Load() != 0 {
		t.Fatalf("docsIndex returned %d errors", errs.Load())
	}
	if mism.Load() != 0 {
		t.Fatalf("docsIndex returned %d distinct pointers; want all calls to share the cached index", mism.Load())
	}
	if got := first.Load(); got == nil || got.Len() != 2 {
		t.Fatalf("loaded index = %+v, want 2 pages", got)
	}
}

func TestDocsIndexCachesErrorOnce(t *testing.T) {
	t.Parallel()

	srv := New(Config{DocsDir: t.TempDir()})

	idx1, err1 := srv.docsIndex()
	if err1 != nil {
		t.Fatalf("first load: %v", err1)
	}
	idx2, err2 := srv.docsIndex()
	if err2 != nil {
		t.Fatalf("second load: %v", err2)
	}
	if idx1 != idx2 {
		t.Fatalf("docsIndex returned different pointers on repeat call: %p vs %p", idx1, idx2)
	}
}
