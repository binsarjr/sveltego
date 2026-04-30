package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Server is a minimal sveltego LSP server. It speaks LSP framed JSON-RPC over
// a reader/writer pair (typically stdin/stdout) and dispatches a small set of
// methods. The hover/definition/references handlers return empty results until
// the gopls proxy lands; the scaffold guarantees the initialize/shutdown
// handshake completes correctly so editors can attach.
type Server struct {
	reader *bufio.Reader
	writer io.Writer
	logger io.Writer

	mu          sync.Mutex
	initialized atomic.Bool
	shutdown    atomic.Bool
}

// New builds a Server that reads from r, writes responses to w, and logs
// non-protocol diagnostics to logs.
func New(r io.Reader, w, logs io.Writer) *Server {
	return &Server{
		reader: bufio.NewReader(r),
		writer: w,
		logger: logs,
	}
}

// Serve runs the read/dispatch loop until the client requests shutdown+exit,
// the underlying reader returns io.EOF, or ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := ReadMessage(s.reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			var rpcErr *RPCError
			if errors.As(err, &rpcErr) {
				s.logf("parse error: %s", rpcErr.Message)
				continue
			}
			return fmt.Errorf("read: %w", err)
		}
		s.dispatch(msg)
		if msg.Method == "exit" {
			return nil
		}
	}
}

func (s *Server) dispatch(msg *Message) {
	switch msg.Method {
	case "initialize":
		s.handle(msg, s.handleInitialize)
	case "initialized":
		// notification — no response
	case "shutdown":
		s.handle(msg, s.handleShutdown)
	case "exit":
		// notification — Serve loop will exit
	case "textDocument/hover":
		s.handle(msg, s.handleHover)
	case "textDocument/definition":
		s.handle(msg, s.handleDefinition)
	case "textDocument/references":
		s.handle(msg, s.handleReferences)
	case "textDocument/didOpen", "textDocument/didChange",
		"textDocument/didClose", "textDocument/didSave":
		// scaffold accepts but ignores text sync notifications
	default:
		if msg.ID != nil {
			s.respondError(msg.ID, ErrMethodNotFound, "method not implemented: "+msg.Method)
		}
	}
}

type handlerFn func(params json.RawMessage) (any, *RPCError)

func (s *Server) handle(msg *Message, fn handlerFn) {
	defer func() {
		rec := recover()
		if rec == nil {
			return
		}
		s.logf("handler panic: method=%s id=%s panic=%v\n%s",
			msg.Method, idString(msg.ID), rec, debug.Stack())
		if msg.ID != nil {
			s.respondError(msg.ID, ErrInternal, fmt.Sprintf("internal error: %v", rec))
		}
	}()
	if msg.ID == nil {
		// Notification dispatched to a request handler — best-effort run, drop result.
		_, _ = fn(msg.Params)
		return
	}
	result, rpcErr := fn(msg.Params)
	if rpcErr != nil {
		s.respondError(msg.ID, rpcErr.Code, rpcErr.Message)
		return
	}
	s.respondResult(msg.ID, result)
}

func idString(id *json.RawMessage) string {
	if id == nil {
		return "<nil>"
	}
	return string(*id)
}

func (s *Server) respondResult(id *json.RawMessage, result any) {
	body, err := json.Marshal(result)
	if err != nil {
		s.respondError(id, ErrInternal, err.Error())
		return
	}
	s.write(&Message{ID: id, Result: body})
}

func (s *Server) respondError(id *json.RawMessage, code int, message string) {
	s.write(&Message{ID: id, Error: &RPCError{Code: code, Message: message}})
}

func (s *Server) write(msg *Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := WriteMessage(s.writer, msg); err != nil {
		s.logf("write: %v", err)
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.logger == nil {
		return
	}
	fmt.Fprintf(s.logger, "sveltego-lsp: "+format+"\n", args...)
}
