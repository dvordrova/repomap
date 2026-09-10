package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageOperations = "atlas_operations"

//go:embed prompts/operations.md
var operationsPrompt string

func Operations() table.Definition {
	return table.Definition{Stage: StageOperations, Contract: "repomap.atlas.operations.v14", System: operationsPrompt, Independent: true, Memoize: true, Columns: []table.Column{
		{Name: "entry", Kind: table.Choice, OptionsFrom: "entry_options", Note: "self when this declaration is directly activated; u* for an upstream operation; none without an operation"},
		{Name: "activation", Kind: table.Choice, Options: []string{"command", "request", "interaction", "scheduled", "continuous"}, When: map[string]string{"entry": "self"}},
		{Name: "name", Kind: table.Text, MaxRunes: 60, When: map[string]string{"entry": "self"}},
		{Name: "description", Kind: table.Text, MaxRunes: 180, When: map[string]string{"entry": "self"}},
	}}
}
