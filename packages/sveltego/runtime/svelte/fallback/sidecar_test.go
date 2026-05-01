package fallback

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSidecarScript writes a tiny Node script that mimics the real
// ssr-serve sidecar's startup contract: bind localhost on port 0 and
// emit `SVELTEGO_SSR_FALLBACK_LISTEN port=N` on stderr. The /render
// handler echoes the request body so tests can assert end-to-end
// dispatch.
func fakeSidecarScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pkg := []byte(`{"name":"fake-sidecar","type":"module","private":true}`)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), pkg, 0o600); err != nil {
		t.Fatal(err)
	}
	script := []byte(`
import { createServer } from "node:http";
const server = createServer((req, res) => {
  if (req.method === "POST" && req.url === "/render") {
    let chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      res.writeHead(200, {"content-type": "application/json"});
      res.end(JSON.stringify({ body: "<echo>" + Buffer.concat(chunks).length + "</echo>", head: "" }));
    });
    return;
  }
  res.writeHead(404); res.end("nope");
});
server.listen(0, "127.0.0.1", () => {
  process.stderr.write("SVELTEGO_SSR_FALLBACK_LISTEN port=" + server.address().port + "\n");
});
`)
	if err := os.WriteFile(filepath.Join(dir, "index.mjs"), script, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func skipIfNoNode(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping sidecar test")
	}
	return p
}

func TestSidecarStartAndRender(t *testing.T) {
	t.Parallel()
	nodePath := skipIfNoNode(t)
	dir := fakeSidecarScript(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	side, err := Start(ctx, SidecarOptions{
		NodePath:    nodePath,
		SidecarDir:  dir,
		ProjectRoot: dir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer side.Stop()
	if !strings.HasPrefix(side.Endpoint(), "http://127.0.0.1:") {
		t.Fatalf("unexpected endpoint: %q", side.Endpoint())
	}

	c := NewClient(ClientOptions{Endpoint: side.Endpoint(), CacheSize: 4, TTL: time.Minute})
	resp, err := c.Render(ctx, RenderRequest{Route: "/x", Source: "src/routes/x/_page.svelte", Data: map[string]any{"hello": "world"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(resp.Body, "<echo>") {
		t.Fatalf("expected echo body, got %q", resp.Body)
	}
}

func TestSidecarMissingDir(t *testing.T) {
	t.Parallel()
	nodePath := skipIfNoNode(t)
	_, err := Start(context.Background(), SidecarOptions{
		NodePath:    nodePath,
		SidecarDir:  filepath.Join(t.TempDir(), "missing"),
		ProjectRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected missing-dir error")
	}
	if !strings.Contains(err.Error(), "sidecar entry") {
		t.Fatalf("error missing entry path: %v", err)
	}
}
