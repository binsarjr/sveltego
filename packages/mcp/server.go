package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ProtocolVersion is the MCP protocol revision implemented by this server.
const ProtocolVersion = "2024-11-05"

// ServerName is the name reported in the initialize response.
const ServerName = "sveltego-mcp"

// ServerVersion is the implementation version reported on initialize.
const ServerVersion = "0.1.0"

// Server is a minimal MCP server speaking JSON-RPC 2.0 over a transport.
// Tools are registered by name; resources are advertised but not yet
// readable.
type Server struct {
	cfg   Config
	tools map[string]Tool

	mu         sync.Mutex
	out        *json.Encoder
	outWriter  io.Writer
	cachedDocs *DocsIndex
}

// Tool describes a single MCP tool: schema for clients and a handler
// invoked when the tool is called.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     ToolHandler
}

// ToolHandler is invoked when a client calls a registered tool. It
// receives the raw JSON arguments object and must return either text
// content or an error.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolResult, error)

// ToolResult is the value returned by a tool handler. Text is rendered
// as a single text content block; IsError marks the result as a tool
// error rather than a protocol error so the client surfaces it to the
// model rather than retrying.
type ToolResult struct {
	Text    string
	IsError bool
}

// New constructs a server with the supplied configuration. Built-in
// tools are registered immediately so ListTools is non-empty after
// construction.
func New(cfg Config) *Server {
	s := &Server{
		cfg:   cfg,
		tools: map[string]Tool{},
	}
	for _, t := range s.builtinTools() {
		s.tools[t.Name] = t
	}
	return s
}

// Tools returns the registered tool definitions sorted by name. Used by
// tests and the list_tools response.
func (s *Server) Tools() []Tool {
	out := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t)
	}
	sortToolsByName(out)
	return out
}

// Call invokes the named tool with the supplied JSON arguments. Used
// by tests to exercise handlers without round-tripping through stdio.
func (s *Server) Call(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
	t, ok := s.tools[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool: %q", name)
	}
	return t.Handler(ctx, args)
}

// ServeStdio reads newline-delimited JSON-RPC requests from r and
// writes responses to w until r returns EOF or ctx is cancelled.
// Concurrent requests are not supported; each request is handled
// before the next is read.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	enc := json.NewEncoder(w)
	s.mu.Lock()
	s.out = enc
	s.outWriter = w
	s.mu.Unlock()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		s.handleLine(ctx, line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read stdin: %w", err)
	}
	return nil
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeError(nil, codeParseError, "parse error: "+err.Error(), nil)
		return
	}
	if req.JSONRPC != "2.0" {
		s.writeError(req.ID, codeInvalidRequest, "jsonrpc must be \"2.0\"", nil)
		return
	}

	resp, err := s.dispatch(ctx, req)
	if err != nil {
		var rpcErr *rpcError
		if errors.As(err, &rpcErr) {
			s.writeError(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
			return
		}
		s.writeError(req.ID, codeInternalError, err.Error(), nil)
		return
	}
	if req.ID == nil {
		return
	}
	s.writeResult(req.ID, resp)
}

func (s *Server) writeResult(id json.RawMessage, result any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.out == nil {
		return
	}
	_ = s.out.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) writeError(id json.RawMessage, code int, message string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.out == nil {
		return
	}
	_ = s.out.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}
