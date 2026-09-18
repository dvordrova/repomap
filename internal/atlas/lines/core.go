package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageCore = "atlas_core"

//go:embed prompts/core.md
var corePrompt string

// Core asks, for each part of a program, whether the program exists for it
// and whether it exists only for the program's tests. An empty cell is no.
func Core() table.Definition {
	return table.Definition{
		Stage: StageCore, Contract: "repomap.atlas.core.v3", System: corePrompt, Independent: true,
		Columns: []table.Column{
			{Name: "core", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when the program exists for what this part does: without it the program has no reason to run"},
			{Name: "for_tests", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this part exists only so the program's tests can run"},
		},
	}
}
