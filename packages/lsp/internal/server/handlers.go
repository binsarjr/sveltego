package server

import (
	"encoding/json"
)

// serverInfo is the static name/version pair returned from initialize.
var serverInfo = Info{Name: "sveltego-lsp", Version: "0.0.0-scaffold"}

func (s *Server) handleInitialize(params json.RawMessage) (any, *RPCError) {
	var p InitializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{Code: ErrInvalidParams, Message: err.Error()}
		}
	}
	s.initialized.Store(true)
	return InitializeResult{
		Capabilities: Capabilities{
			TextDocumentSync:   1, // full sync
			HoverProvider:      true,
			DefinitionProvider: true,
			ReferencesProvider: true,
		},
		ServerInfo: serverInfo,
	}, nil
}

func (s *Server) handleShutdown(_ json.RawMessage) (any, *RPCError) {
	s.shutdown.Store(true)
	return nil, nil
}

// handleHover returns a stub markup body. The gopls proxy follow-up will
// translate the position through the source map and forward the request.
func (s *Server) handleHover(params json.RawMessage) (any, *RPCError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: err.Error()}
	}
	return Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: "_sveltego-lsp scaffold_: hover not yet wired through gopls.",
		},
	}, nil
}

// handleDefinition returns an empty Location list until the gopls proxy lands.
func (s *Server) handleDefinition(params json.RawMessage) (any, *RPCError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: err.Error()}
	}
	return []Location{}, nil
}

// handleReferences returns an empty Location list until the gopls proxy lands.
func (s *Server) handleReferences(params json.RawMessage) (any, *RPCError) {
	var p ReferenceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: err.Error()}
	}
	return []Location{}, nil
}
