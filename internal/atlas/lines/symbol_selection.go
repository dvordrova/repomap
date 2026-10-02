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
		// Judged on cmd/repomap: three blind judges over 60 rows where Jev
		// and DeepSeek disagreed sided with Jev at a 0.8 cutoff on 50 and
		// with DeepSeek on 32; at 0.6 Jev was right on 28. Both models mark
		// helpers as keys; the judges agreed with 4 of 10 shared yes rows.
		Classifier: true, YesAt: 0.8,
		System: symbolSelectionPrompt, Memoize: true,
		Columns: []table.Column{
			{Name: "key_symbol", Kind: table.Choice, Options: []string{"yes", "no"}, Note: "a declaration a newcomer should look at first"},
		},
	}
	if types {
		def.Contract += ".types"
		return def
	}
	// Symbol rows carry calls; their prompt defines every rendered value.
	// A row over the categorizer's envelope is asked packed.
	def.System = withVocabulary(symbolSelectionPrompt)
	def.Pack = PackSymbolRow
	return def
}
