package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func newSession(t *testing.T) (clientWrite *io.PipeWriter, clientRead *bufio.Reader, done <-chan error) {
	t.Helper()
	clientToServer, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	srv := New(clientToServer, serverToClientW, io.Discard)

	errs := make(chan error, 1)
	go func() {
		err := srv.Serve(context.Background())
		_ = serverToClientW.Close()
		errs <- err
	}()

	t.Cleanup(func() {
		_ = clientToServerW.Close()
		_ = clientToServer.Close()
		_ = serverToClientR.Close()
	})

	return clientToServerW, bufio.NewReader(serverToClientR), errs
}

func writeFrame(t *testing.T, w io.Writer, msg *Message) {
	t.Helper()
	if err := WriteMessage(w, msg); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func readFrame(t *testing.T, r *bufio.Reader) *Message {
	t.Helper()
	msg, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return msg
}

func rawID(t *testing.T, id int) *json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("marshal id: %v", err)
	}
	rm := json.RawMessage(raw)
	return &rm
}

func TestServer_InitializeShutdownExit(t *testing.T) {
	t.Parallel()
	clientW, clientR, done := newSession(t)

	writeFrame(t, clientW, &Message{
		ID:     rawID(t, 1),
		Method: "initialize",
		Params: json.RawMessage(`{"processId":42,"rootUri":"file:///tmp"}`),
	})

	resp := readFrame(t, clientR)
	if resp.Error != nil {
		t.Fatalf("initialize returned error: %+v", resp.Error)
	}
	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if !result.Capabilities.HoverProvider {
		t.Errorf("expected HoverProvider=true, got %+v", result.Capabilities)
	}
	if !result.Capabilities.DefinitionProvider {
		t.Errorf("expected DefinitionProvider=true, got %+v", result.Capabilities)
	}
	if !result.Capabilities.ReferencesProvider {
		t.Errorf("expected ReferencesProvider=true, got %+v", result.Capabilities)
	}
	if result.ServerInfo.Name != "sveltego-lsp" {
		t.Errorf("expected server name sveltego-lsp, got %q", result.ServerInfo.Name)
	}

	writeFrame(t, clientW, &Message{Method: "initialized", Params: json.RawMessage(`{}`)})

	writeFrame(t, clientW, &Message{ID: rawID(t, 2), Method: "shutdown"})
	shutdownResp := readFrame(t, clientR)
	if shutdownResp.Error != nil {
		t.Fatalf("shutdown returned error: %+v", shutdownResp.Error)
	}

	writeFrame(t, clientW, &Message{Method: "exit"})
	if err := <-done; err != nil {
		t.Fatalf("server exited with error: %v", err)
	}
}

func TestServer_HoverStub(t *testing.T) {
	t.Parallel()
	clientW, clientR, done := newSession(t)

	writeFrame(t, clientW, &Message{ID: rawID(t, 1), Method: "initialize"})
	_ = readFrame(t, clientR)

	writeFrame(t, clientW, &Message{
		ID:     rawID(t, 2),
		Method: "textDocument/hover",
		Params: json.RawMessage(`{"textDocument":{"uri":"file:///x.svelte"},"position":{"line":0,"character":0}}`),
	})
	resp := readFrame(t, clientR)
	if resp.Error != nil {
		t.Fatalf("hover error: %+v", resp.Error)
	}
	var hover Hover
	if err := json.Unmarshal(resp.Result, &hover); err != nil {
		t.Fatalf("decode hover: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "scaffold") {
		t.Errorf("hover stub missing scaffold marker: %q", hover.Contents.Value)
	}

	writeFrame(t, clientW, &Message{ID: rawID(t, 3), Method: "shutdown"})
	_ = readFrame(t, clientR)
	writeFrame(t, clientW, &Message{Method: "exit"})
	if err := <-done; err != nil {
		t.Fatalf("server exited with error: %v", err)
	}
}

func TestServer_UnknownMethod(t *testing.T) {
	t.Parallel()
	clientW, clientR, done := newSession(t)

	writeFrame(t, clientW, &Message{ID: rawID(t, 1), Method: "initialize"})
	_ = readFrame(t, clientR)

	writeFrame(t, clientW, &Message{ID: rawID(t, 9), Method: "textDocument/somethingNew"})
	resp := readFrame(t, clientR)
	if resp.Error == nil {
		t.Fatalf("expected method-not-found error, got result %s", string(resp.Result))
	}
	if resp.Error.Code != ErrMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrMethodNotFound, resp.Error.Code)
	}

	writeFrame(t, clientW, &Message{ID: rawID(t, 10), Method: "shutdown"})
	_ = readFrame(t, clientR)
	writeFrame(t, clientW, &Message{Method: "exit"})
	if err := <-done; err != nil {
		t.Fatalf("server exited with error: %v", err)
	}
}

func TestServer_FrameRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	msg := &Message{ID: rawID(t, 7), Method: "ping", Params: json.RawMessage(`{"k":1}`)}
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Method != "ping" {
		t.Errorf("method round-trip: got %q", got.Method)
	}
	if string(got.Params) != `{"k":1}` {
		t.Errorf("params round-trip: got %s", string(got.Params))
	}
}

func TestServer_ProxyDisabled(t *testing.T) {
	t.Parallel()
	if _, err := (DisabledProxy{}).Forward("textDocument/hover", nil); err == nil {
		t.Fatalf("expected ErrProxyDisabled, got nil")
	}
}
