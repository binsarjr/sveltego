package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (s *Server) builtinTools() []Tool {
	return []Tool{
		{
			Name:        "search_docs",
			Description: "Full-text search across sveltego documentation. Returns top matches with title and snippet.",
			InputSchema: schemaSearchDocs,
			Handler:     s.handleSearchDocs,
		},
		{
			Name:        "get_doc_page",
			Description: "Read the full markdown of a documentation page by its slug (path relative to the docs root, with or without .md).",
			InputSchema: schemaGetDocPage,
			Handler:     s.handleGetDocPage,
		},
		{
			Name:        "lookup_api",
			Description: "Look up the Go signature and godoc for a kit.* runtime symbol (e.g. \"Cookies\", \"Redirect\", \"NewCookies\").",
			InputSchema: schemaLookupAPI,
			Handler:     s.handleLookupAPI,
		},
		{
			Name:        "get_example",
			Description: "Return the source files of a named playground app under playgrounds/. Output is capped at 100KB.",
			InputSchema: schemaGetExample,
			Handler:     s.handleGetExample,
		},
		{
			Name:        "validate_template",
			Description: "Parse a Svelte template snippet and return diagnostics. Stub: full parser integration is a follow-up.",
			InputSchema: schemaValidateTemplate,
			Handler:     s.handleValidateTemplate,
		},
		{
			Name:        "scaffold_route",
			Description: "Return boilerplate text for a +page.svelte / page.server.go / server.go / +error.svelte under src/routes/<path>.",
			InputSchema: schemaScaffoldRoute,
			Handler:     s.handleScaffoldRoute,
		},
	}
}

var (
	schemaSearchDocs = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "case-insensitive substring to search for"},
    "limit": {"type": "integer", "default": 5, "minimum": 1, "maximum": 50}
  },
  "required": ["query"]
}`)

	schemaGetDocPage = json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": {"type": "string", "description": "page slug, e.g. \"contributing/golden-tests\""}
  },
  "required": ["slug"]
}`)

	schemaLookupAPI = json.RawMessage(`{
  "type": "object",
  "properties": {
    "symbol": {"type": "string", "description": "exported symbol name in the kit package, e.g. \"Cookies\""}
  },
  "required": ["symbol"]
}`)

	schemaGetExample = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "playground directory name, e.g. \"basic\""}
  },
  "required": ["name"]
}`)

	schemaValidateTemplate = json.RawMessage(`{
  "type": "object",
  "properties": {
    "source": {"type": "string", "description": ".svelte source to validate"}
  },
  "required": ["source"]
}`)

	schemaScaffoldRoute = json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "route path beneath src/routes, e.g. \"about\" or \"post/[id]\""},
    "kind": {
      "type": "string",
      "enum": ["page", "layout", "server", "error"],
      "description": "page = +page.svelte + page.server.go; layout = +layout.svelte + layout.server.go; server = server.go REST endpoint; error = +error.svelte"
    }
  },
  "required": ["path", "kind"]
}`)
)

type searchDocsArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (s *Server) handleSearchDocs(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a searchDocsArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if strings.TrimSpace(a.Query) == "" {
		return ToolResult{}, errors.New("query is required")
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 5
	}
	idx, err := s.docsIndex()
	if err != nil {
		return ToolResult{}, err
	}
	hits := idx.Search(a.Query, limit)
	if len(hits) == 0 {
		return ToolResult{Text: fmt.Sprintf("no matches for %q in %d indexed pages", a.Query, idx.Len())}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q:\n\n", len(hits), a.Query)
	for _, h := range hits {
		fmt.Fprintf(&b, "## %s\n%s\n\n%s\n\n---\n\n", h.Slug, h.Title, h.Snippet)
	}
	return ToolResult{Text: strings.TrimRight(b.String(), "-\n ")}, nil
}

type getDocPageArgs struct {
	Slug string `json:"slug"`
}

func (s *Server) handleGetDocPage(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a getDocPageArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if strings.TrimSpace(a.Slug) == "" {
		return ToolResult{}, errors.New("slug is required")
	}
	body, err := readDocPage(s.cfg.DocsDir, a.Slug)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Text: body}, nil
}

type lookupAPIArgs struct {
	Symbol string `json:"symbol"`
}

func (s *Server) handleLookupAPI(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a lookupAPIArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if strings.TrimSpace(a.Symbol) == "" {
		return ToolResult{}, errors.New("symbol is required")
	}
	entry, err := lookupKitSymbol(s.cfg.KitDir, a.Symbol)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Text: entry}, nil
}

type getExampleArgs struct {
	Name string `json:"name"`
}

func (s *Server) handleGetExample(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a getExampleArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if strings.TrimSpace(a.Name) == "" {
		return ToolResult{}, errors.New("name is required")
	}
	body, err := readExample(s.cfg.PlaygroundsDir, a.Name)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Text: body}, nil
}

type validateTemplateArgs struct {
	Source string `json:"source"`
}

func (s *Server) handleValidateTemplate(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a validateTemplateArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if a.Source == "" {
		return ToolResult{}, errors.New("source is required")
	}
	return ToolResult{
		Text:    "validate_template is not yet wired to the parser; tracked as a follow-up issue. The Svelte template parser lives under packages/sveltego/internal/parser and is unimportable from this module by Go's internal-package rule. The follow-up will expose a thin re-export under packages/sveltego/exports or shell out to the sveltego CLI.",
		IsError: false,
	}, nil
}

type scaffoldRouteArgs struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (s *Server) handleScaffoldRoute(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var a scaffoldRouteArgs
	if err := unmarshalArgs(raw, &a); err != nil {
		return ToolResult{}, err
	}
	if strings.TrimSpace(a.Path) == "" {
		return ToolResult{}, errors.New("path is required")
	}
	if strings.TrimSpace(a.Kind) == "" {
		return ToolResult{}, errors.New("kind is required (page|layout|server|error)")
	}
	out, err := scaffold(a.Path, a.Kind)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Text: out}, nil
}

func unmarshalArgs(raw json.RawMessage, into any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
