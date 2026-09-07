package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageOperations = "atlas_operations"

//go:embed prompts/operations.md
var operationsPrompt string

func Operations() table.Definition {
	return table.Definition{Stage: StageOperations, Contract: "repomap.atlas.operations.v12", System: operationsPrompt, Window: 8, Independent: true, Columns: []table.Column{
		{Name: "entry", Kind: table.Choice, OptionsFrom: "entry_options", Note: "self when this declaration is directly activated; u* for an upstream operation; none without an operation"},
		{Name: "activation", Kind: table.Choice, Options: []string{"none", "command", "request", "interaction", "scheduled", "continuous"}},
		{Name: "name", Kind: table.Text, MaxRunes: 60},
		{Name: "description", Kind: table.Text, MaxRunes: 180},
	}}
}
