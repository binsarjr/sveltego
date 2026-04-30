package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/mcp"
)

// TestEndToEndAgainstRepo runs the server against the real sveltego
// repo layout to confirm the docs walk and kit parse succeed against
// production data.
func TestEndToEndAgainstRepo(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cfg := mcp.Config{Root: root}.WithDefaults()
	srv := mcp.New(cfg)

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"lookup_api","arguments":{"symbol":"Redirect"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scaffold_route","arguments":{"path":"about","kind":"page"}}}`,
	}, "\n") + "\n")

	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 responses, got %d:\n%s", len(lines), out.String())
	}

	var lookup struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &lookup); err != nil {
		t.Fatalf("decode lookup_api response: %v\n%s", err, lines[1])
	}
	if lookup.Error != nil {
		t.Fatalf("lookup_api error: %s", lookup.Error.Message)
	}
	if lookup.Result.IsError {
		t.Fatalf("lookup_api tool-error: %s", lookup.Result.Content[0].Text)
	}
	body := lookup.Result.Content[0].Text
	if !strings.Contains(body, "func Redirect") {
		t.Errorf("expected Redirect signature in output:\n%s", body)
	}
	if !strings.Contains(body, "redirect") {
		t.Errorf("expected godoc text in output:\n%s", body)
	}

	var scaffold struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &scaffold); err != nil {
		t.Fatalf("decode scaffold_route response: %v\n%s", err, lines[2])
	}
	scaffoldBody := scaffold.Result.Content[0].Text
	if !strings.Contains(scaffoldBody, "//go:build sveltego") {
		t.Errorf("expected build constraint in scaffold output:\n%s", scaffoldBody)
	}
	if !strings.Contains(scaffoldBody, "src/routes/about/page.server.go") {
		t.Errorf("expected page.server.go path in scaffold output:\n%s", scaffoldBody)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}
