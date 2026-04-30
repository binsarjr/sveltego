// Build tag `integration` gates this end-to-end test that boots the
// real dev server, edits a .svelte file, and asserts that the next HTTP
// request returns the updated SSR HTML. It runs codegen and a child
// `go build` per change cycle so it's slow (5–15s) and unfit for the
// inner dev loop. Run with:
//
//	go test -tags=integration -run TestDevHotSwap_Svelte ./internal/devserver/...
//go:build integration

package devserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// stageProject copies testdata/example into a fresh temp dir and rewrites
// go.mod with a replace directive pointing at the real sveltego module.
func stageProject(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	src := filepath.Join(wd, "testdata", "example")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("fixture missing at %s: %v", src, err)
	}
	sveltego, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatalf("abs sveltego: %v", err)
	}

	dst := t.TempDir()
	err = filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(target, ".template") {
			raw = []byte(strings.ReplaceAll(string(raw), "__SVELTEGO__", sveltego))
			target = strings.TrimSuffix(target, ".template")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	return dst
}

// freePort asks the kernel for an available TCP port. The port is
// released before return so the caller has a small window to bind it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestDevHotSwap_Svelte(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	root := stageProject(t)
	port := freePort(t)
	goPort := freePort(t)
	vitePort := freePort(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())

	var (
		runErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = Run(ctx, Options{
			ProjectRoot: root,
			Port:        port,
			GoPort:      goPort,
			VitePort:    vitePort,
			NoClient:    true, // skip Vite to avoid Node dependency in CI
			Logger:      logger,
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		if runErr != nil && !strings.Contains(runErr.Error(), "context canceled") {
			t.Logf("run returned: %v", runErr)
		}
	})

	if err := waitHTTP(fmt.Sprintf("http://127.0.0.1:%d/", port), 15*time.Second); err != nil {
		t.Fatalf("dev server never became ready: %v", err)
	}

	body, err := fetch(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !strings.Contains(body, "marker:original") {
		t.Fatalf("first fetch missing marker:original; body:\n%s", body)
	}

	page := filepath.Join(root, "src", "routes", "+page.svelte")
	if err := os.WriteFile(page, []byte("<h1>marker:edited</h1>\n"), 0o644); err != nil {
		t.Fatalf("rewrite page: %v", err)
	}

	if err := waitForBody(fmt.Sprintf("http://127.0.0.1:%d/", port), "marker:edited", 10*time.Second); err != nil {
		t.Fatalf("svelte hot-swap never propagated: %v", err)
	}
}

func TestDevHotSwap_Go(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	root := stageProject(t)
	port := freePort(t)
	goPort := freePort(t)
	vitePort := freePort(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())

	var (
		runErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = Run(ctx, Options{
			ProjectRoot: root,
			Port:        port,
			GoPort:      goPort,
			VitePort:    vitePort,
			NoClient:    true,
			Logger:      logger,
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		if runErr != nil && !strings.Contains(runErr.Error(), "context canceled") {
			t.Logf("run returned: %v", runErr)
		}
	})

	if err := waitHTTP(fmt.Sprintf("http://127.0.0.1:%d/", port), 15*time.Second); err != nil {
		t.Fatalf("dev server never became ready: %v", err)
	}

	// Touch main.go to trigger a Go restart. We rewrite the log message
	// so the rebuild path runs end-to-end (not just no-op on identical bytes).
	mainGo := filepath.Join(root, "cmd", "app", "main.go")
	raw, err := os.ReadFile(mainGo)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	updated := strings.Replace(string(raw), "devexample: listening on", "devexample [v2]: listening on", 1)
	if err := os.WriteFile(mainGo, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite main: %v", err)
	}

	// After rebuild the proxy should be reachable again.
	if err := waitHTTP(fmt.Sprintf("http://127.0.0.1:%d/", port), 30*time.Second); err != nil {
		t.Fatalf("dev server never recovered after go change: %v", err)
	}
	body, err := fetch(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("post-restart fetch: %v", err)
	}
	if !strings.Contains(body, "marker:original") {
		t.Fatalf("post-restart body missing marker:original:\n%s", body)
	}
}

// waitHTTP polls until url returns a 2xx response or the timeout elapses.
func waitHTTP(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for {
		resp, err := client.Get(url) //nolint:noctx // dev test polling
		if err == nil {
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err == nil {
				return fmt.Errorf("status %d", resp.StatusCode)
			}
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// fetch returns the body of url as a string.
func fetch(url string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url) //nolint:noctx // dev test
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// waitForBody polls url until the response body contains substr or the
// timeout elapses.
func waitForBody(url, substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		body, err := fetch(url)
		if err == nil && strings.Contains(body, substr) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("body never contained %q", substr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
