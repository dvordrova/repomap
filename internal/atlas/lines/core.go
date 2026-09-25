package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageCore = "atlas_core"

// The role a part plays in its program. The program exists for its domain
// parts. Whether a part is made only of test code is a fact of its files,
// never a role the model gives it.
const (
	PartDomain    = "domain"
	PartInterface = "interface"
	PartWiring    = "wiring"
	PartSupport   = "support"
)

//go:embed prompts/core.md
var corePrompt string

// Core asks the one role each part plays. What the program exists for is
// then the code's to read off: its domain parts.
func Core() table.Definition {
	return table.Definition{
		Stage: StageCore, Contract: "repomap.atlas.core.v5", System: corePrompt,
		// Measured on 50 saved parts of repomap: Jev chose DeepSeek's role for
		// 45; the other five were borderline (two UI presentation parts it
		// called interface where DeepSeek said domain).
		Classifier: true,
		Columns: []table.Column{
			{Name: "role", Kind: table.Choice, Options: []string{PartDomain, PartInterface, PartWiring, PartSupport}, Note: "the one role this part plays in the program"},
		},
	}
}
