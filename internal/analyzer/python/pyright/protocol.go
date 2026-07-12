package pyright

import "encoding/json"

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type sourceRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          sourceRange      `json:"range"`
	SelectionRange sourceRange      `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type callHierarchyItem struct {
	Name           string          `json:"name"`
	Detail         string          `json:"detail,omitempty"`
	Kind           int             `json:"kind"`
	URI            string          `json:"uri"`
	Range          sourceRange     `json:"range"`
	SelectionRange sourceRange     `json:"selectionRange"`
	Data           json.RawMessage `json:"data,omitempty"`
}

type incomingCall struct {
	From       callHierarchyItem `json:"from"`
	FromRanges []sourceRange     `json:"fromRanges"`
}

type outgoingCall struct {
	To         callHierarchyItem `json:"to"`
	FromRanges []sourceRange     `json:"fromRanges"`
}

type lspLocation struct {
	URI   string      `json:"uri"`
	Range sourceRange `json:"range"`
}

type initializeResult struct {
	Capabilities struct {
		DocumentSymbolProvider any `json:"documentSymbolProvider,omitempty"`
		CallHierarchyProvider  any `json:"callHierarchyProvider,omitempty"`
		ReferencesProvider     any `json:"referencesProvider,omitempty"`
	} `json:"capabilities"`
	ServerInfo struct {
		Name    string `json:"name,omitempty"`
		Version string `json:"version,omitempty"`
	} `json:"serverInfo,omitempty"`
}
