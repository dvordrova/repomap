package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageOperations = "atlas_operations"

//go:embed prompts/operations.md
var operationsPrompt string

func Operations() table.Definition {
	return table.Definition{Stage: StageOperations, Contract: "repomap.atlas.operations.v20", System: withVocabulary(operationsPrompt), Independent: true, Memoize: true, Columns: []table.Column{
		{Name: "entry", Kind: table.Choice, Options: []string{"self", "none"}, Note: "self requires evidence of the task this declaration fulfils and its independent activation; process entry, asynchronous launch or staying alive alone is insufficient; starting or dispatching the host runtime is none; a task may delegate work to helpers; callers are evidence, never an operation assignment"},
		{Name: "activation", Kind: table.Choice, Options: []string{"command", "request", "interaction", "scheduled", "continuous"}, When: map[string]string{"entry": "self"}},
		// Asked only when the row carries registered name refs: a row without
		// them has one reading, label, and the decoder takes it unasked.
		{Name: "name_kind", Kind: table.Choice, Options: []string{"http", "label"}, When: map[string]string{"entry": "self"}, WhenOptionsFrom: "registered_name_options", Missing: "label", Unasked: "label", Note: "http selects an original registered path; label describes other work or a handler without an observed literal path; rows without registered_name_options are label"},
		{Name: "http_method", Kind: table.Choice, Options: []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH", "ANY"}, When: map[string]string{"entry": "self", "name_kind": "http"}},
		{Name: "http_path", Kind: table.Choice, OptionsFrom: "registered_name_options", When: map[string]string{"entry": "self", "name_kind": "http"}, Note: "one original registration ref, never copied or invented path text"},
		{Name: "name", Kind: table.Text, MaxRunes: 60, When: map[string]string{"entry": "self", "name_kind": "label"}, EmptyFrom: "description"},
		{Name: "description", Kind: table.Text, MaxRunes: 180, When: map[string]string{"entry": "self"}},
	}}
}
