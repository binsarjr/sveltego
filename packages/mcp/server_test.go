package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInitializeAndListTools(t *testing.T) {
	t.Parallel()

	srv := New(Config{}.WithDefaults())

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n")

	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := splitLines(out.String())
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses (initialize + tools/list), got %d:\n%s", len(lines), out.String())
	}

	var initResp rpcResponse
	mustJSON(t, lines[0], &initResp)
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	if !bytes.Equal(initResp.ID, []byte("1")) {
		t.Errorf("initialize id = %s, want 1", initResp.ID)
	}

	resultJSON, err := json.Marshal(initResp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var ir initializeResult
	mustJSON(t, string(resultJSON), &ir)
	if ir.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", ir.ProtocolVersion, ProtocolVersion)
	}
	if ir.ServerInfo.Name != ServerName {
		t.Errorf("server name = %q, want %q", ir.ServerInfo.Name, ServerName)
	}

	var listResp rpcResponse
	mustJSON(t, lines[1], &listResp)
	listJSON, err := json.Marshal(listResp.Result)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	var lr toolsListResult
	mustJSON(t, string(listJSON), &lr)
	wantTools := map[string]bool{
		"search_docs":       true,
		"get_doc_page":      true,
		"lookup_api":        true,
		"get_example":       true,
		"validate_template": true,
		"scaffold_route":    true,
	}
	if len(lr.Tools) != len(wantTools) {
		t.Errorf("tool count = %d, want %d", len(lr.Tools), len(wantTools))
	}
	for _, td := range lr.Tools {
		if !wantTools[td.Name] {
			t.Errorf("unexpected tool %q", td.Name)
		}
		if td.Description == "" {
			t.Errorf("tool %q missing description", td.Name)
		}
		if len(td.InputSchema) == 0 {
			t.Errorf("tool %q missing input schema", td.Name)
		}
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"nope/never"}` + "\n")
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resp rpcResponse
	mustJSON(t, strings.TrimSpace(out.String()), &resp)
	if resp.Error == nil {
		t.Fatalf("expected error for unknown method, got %v", resp.Result)
	}
	if resp.Error.Code != codeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, codeMethodNotFound)
	}
}

func TestMalformedJSONReturnsParseError(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	in := strings.NewReader("{not json}\n")
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resp rpcResponse
	mustJSON(t, strings.TrimSpace(out.String()), &resp)
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("expected parse error, got %+v", resp)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	t.Parallel()

	srv := New(Config{})
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification should produce no response, got %q", out.String())
	}
}

func splitLines(s string) []string {
	out := []string{}
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func mustJSON(t *testing.T, body string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), into); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, body)
	}
}
