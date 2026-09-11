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
		Stage: StageSymbols, Contract: "repomap.atlas.symbol-selection.v5",
		System: symbolSelectionPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "key_symbol", Kind: table.Choice, Options: []string{"yes", "no"}, Note: "a declaration a newcomer needs to understand this file"},
		},
	}
	if types {
		def.Contract += ".types"
		return def
	}
	// The candidate cell is a gate: the operations table reviews every yes
	// with its own evidence and decides the activation kind itself.
	def.Columns = append(def.Columns,
		table.Column{Name: "operation_candidate", Kind: table.Choice, Options: []string{"yes", "no"}, Missing: "no", Note: "yes when the observations support an externally activated operation (command, request, interaction, scheduled or continuous work); the operations table decides which"},
		table.Column{Name: "outbound", Kind: table.Sequence, OptionsFrom: "call_options", WhenOptionsFrom: "call_options", Note: "evidence for boundary review: communication or an explicitly configured remote client/exporter; none when unsupported"},
	)
	return def
}
