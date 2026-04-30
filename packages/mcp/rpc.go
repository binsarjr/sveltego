package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

func newInvalidParams(msg string) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: msg}
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) (any, error) {
	switch req.Method {
	case "initialize":
		return s.onInitialize(req.Params)
	case "initialized", "notifications/initialized":
		return nil, nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return s.onToolsList(), nil
	case "tools/call":
		return s.onToolsCall(ctx, req.Params)
	case "resources/list":
		return s.onResourcesList(), nil
	case "resources/read":
		return s.onResourcesRead(req.Params)
	default:
		return nil, &rpcError{
			Code:    codeMethodNotFound,
			Message: "method not found: " + req.Method,
		}
	}
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      serverInfo         `json:"serverInfo"`
	Capabilities    serverCapabilities `json:"capabilities"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type serverCapabilities struct {
	Tools     map[string]any `json:"tools,omitempty"`
	Resources map[string]any `json:"resources,omitempty"`
}

func (s *Server) onInitialize(_ json.RawMessage) (any, error) {
	return initializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: serverInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Capabilities: serverCapabilities{
			Tools:     map[string]any{"listChanged": false},
			Resources: map[string]any{"listChanged": false, "subscribe": false},
		},
	}, nil
}

type toolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDescriptor `json:"tools"`
}

func (s *Server) onToolsList() any {
	out := make([]toolDescriptor, 0, len(s.tools))
	for _, t := range s.Tools() {
		out = append(out, toolDescriptor{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return toolsListResult{Tools: out}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func (s *Server) onToolsCall(ctx context.Context, params json.RawMessage) (any, error) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newInvalidParams("invalid params: " + err.Error())
	}
	if p.Name == "" {
		return nil, newInvalidParams("missing tool name")
	}
	tool, ok := s.tools[p.Name]
	if !ok {
		return nil, newInvalidParams("unknown tool: " + p.Name)
	}
	res, err := tool.Handler(ctx, p.Arguments)
	if err != nil {
		return toolCallResult{
			Content: []toolContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil
	}
	return toolCallResult{
		Content: []toolContent{{Type: "text", Text: res.Text}},
		IsError: res.IsError,
	}, nil
}

type resourceDescriptor struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type resourcesListResult struct {
	Resources []resourceDescriptor `json:"resources"`
}

func (s *Server) onResourcesList() any {
	return resourcesListResult{Resources: []resourceDescriptor{}}
}

type resourcesReadParams struct {
	URI string `json:"uri"`
}

func (s *Server) onResourcesRead(params json.RawMessage) (any, error) {
	var p resourcesReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newInvalidParams("invalid params: " + err.Error())
	}
	return nil, &rpcError{
		Code:    codeMethodNotFound,
		Message: "resources/read not implemented yet (follow-up)",
	}
}

func sortToolsByName(ts []Tool) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
}
