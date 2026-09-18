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
		Stage: StageSymbols, Contract: "repomap.atlas.symbol-selection.v9",
		System: symbolSelectionPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "key_symbol", Kind: table.Choice, Options: []string{"yes", "no"}, Note: "a declaration a newcomer should look at first"},
		},
	}
	if types {
		def.Contract += ".types"
		return def
	}
	// Symbol rows carry calls; their prompt defines every rendered value.
	def.System = withVocabulary(symbolSelectionPrompt)
	return def
}
