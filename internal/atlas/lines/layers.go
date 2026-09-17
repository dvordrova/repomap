package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageLayers = "atlas_layers"

//go:embed prompts/layers.md
var layersPrompt string

// LayerRoles is what a declaration on a chain does with what passes through.
var LayerRoles = []string{"access", "adapter", "logic", "passthrough"}

// Layers asks, for each declaration on a chain from an entry to a call that
// leaves the program, what it does with what passes through — from its
// source.
func Layers() table.Definition {
	return table.Definition{
		Stage: StageLayers, Contract: "repomap.atlas.layers.v1", System: layersPrompt, Independent: true,
		Columns: []table.Column{{Name: "role", Kind: table.Choice, Options: LayerRoles, Note: "what this declaration does with what passes through it, read from its source"}},
	}
}
