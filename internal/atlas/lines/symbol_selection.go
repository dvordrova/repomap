package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

//go:embed prompts/symbol_selection.md
var symbolSelectionPrompt string

// SymbolSelection separates discovery from the prose displayed in the atlas.
// Its complete evidence is unchanged by directory/file presentation choices.
func SymbolSelection(types bool) table.Definition {
	def := table.Definition{
		Stage: StageSymbols, Contract: "repomap.atlas.symbol-selection.v2",
		System: symbolSelectionPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "key_symbol", Kind: table.Choice, Options: []string{"yes", "no"}, Note: "a declaration a newcomer needs to understand this file"},
		},
	}
	if types {
		def.Contract += ".types"
		return def
	}
	def.Columns = append(def.Columns,
		table.Column{Name: "activation", Kind: table.Choice, Options: []string{"none", "unassessed", "command", "request", "interaction", "scheduled", "continuous"}, Note: "supported external activation; unassessed when the evidence cannot establish a role"},
		table.Column{Name: "outbound", Kind: table.Sequence, OptionsFrom: "call_options", LimitFrom: "call_count", Note: "observed service, database or transport calls; none when no supplied call supports this"},
	)
	return def
}
