package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageOperations = "atlas_operations"

//go:embed prompts/operations.md
var operationsPrompt string

func Operations() table.Definition {
	return table.Definition{Stage: StageOperations, Contract: "repomap.atlas.operations.v16", System: operationsPrompt, Independent: true, Memoize: true, Columns: []table.Column{
		{Name: "entry", Kind: table.Choice, OptionsFrom: "entry_options", Note: "choose from this row's entry_options: self when directly activated, an advertised observed_callers ref for its upstream operation, or none without an operation"},
		{Name: "activation", Kind: table.Choice, Options: []string{"command", "request", "interaction", "scheduled", "continuous"}, When: map[string]string{"entry": "self"}},
		{Name: "name_kind", Kind: table.Choice, OptionsFrom: "name_kind_options", When: map[string]string{"entry": "self"}, Note: "http selects an original registered path; label describes other work or a handler without an observed literal path"},
		{Name: "http_method", Kind: table.Choice, Options: []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH", "ANY"}, When: map[string]string{"entry": "self", "name_kind": "http"}},
		{Name: "http_path", Kind: table.Choice, OptionsFrom: "registered_name_options", When: map[string]string{"entry": "self", "name_kind": "http"}, Note: "one original registration ref, never copied or invented path text"},
		{Name: "name", Kind: table.Text, MaxRunes: 60, When: map[string]string{"entry": "self", "name_kind": "label"}},
		{Name: "description", Kind: table.Text, MaxRunes: 180, When: map[string]string{"entry": "self"}},
	}}
}
