// Package server implements the sveltego Language Server core.
package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a JSON-RPC 2.0 envelope. ID is *json.RawMessage so notifications
// (no ID), numeric IDs, and string IDs all round-trip.
type Message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// RPCError mirrors the JSON-RPC 2.0 error shape.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface so RPCError flows through errors.As.
func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// LSP error codes used by the server.
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// ReadMessage reads a single LSP frame: `Content-Length` header then JSON body.
func ReadMessage(r *bufio.Reader) (*Message, error) {
	length, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, &RPCError{Code: ErrParse, Message: err.Error()}
	}
	return &msg, nil
}

// WriteMessage writes a single LSP frame: header + JSON body.
func WriteMessage(w io.Writer, msg *Message) error {
	if msg.JSONRPC == "" {
		msg.JSONRPC = "2.0"
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func readHeader(r *bufio.Reader) (int, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return 0, fmt.Errorf("malformed header: %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(key), "content-length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0, fmt.Errorf("invalid content-length: %w", err)
			}
			length = n
		}
	}
	if length < 0 {
		return 0, errors.New("missing content-length header")
	}
	return length, nil
}
