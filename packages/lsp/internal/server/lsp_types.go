package server

// Position is a zero-based line/character pair. character is UTF-16 code units
// per LSP, but the scaffold treats it opaquely until full hover lands.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a pair of Positions; end is exclusive.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location pairs a document URI with a Range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a text document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams is the standard `textDocument` + `position` shape.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// InitializeParams is the subset of LSP `initialize` params the scaffold needs.
type InitializeParams struct {
	ProcessID    int    `json:"processId,omitempty"`
	RootURI      string `json:"rootUri,omitempty"`
	ClientInfo   *Info  `json:"clientInfo,omitempty"`
	Capabilities any    `json:"capabilities,omitempty"`
}

// Info names the client (or server) and its version.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeResult announces server capabilities.
type InitializeResult struct {
	Capabilities Capabilities `json:"capabilities"`
	ServerInfo   Info         `json:"serverInfo"`
}

// Capabilities is the subset of LSP server capabilities the scaffold advertises.
type Capabilities struct {
	TextDocumentSync   int  `json:"textDocumentSync"`
	HoverProvider      bool `json:"hoverProvider"`
	DefinitionProvider bool `json:"definitionProvider"`
	ReferencesProvider bool `json:"referencesProvider"`
}

// MarkupContent is the standard hover body.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Hover is the response to `textDocument/hover`.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// ReferenceParams adds the "include declaration" flag on top of position params.
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ReferenceContext mirrors LSP `ReferenceContext`.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}
