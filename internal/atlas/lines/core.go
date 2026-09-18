package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageCore = "atlas_core"

// The role a part plays in its program. The program exists for its domain
// parts; a tests part exists only for the program's tests.
const (
	PartDomain    = "domain"
	PartInterface = "interface"
	PartWiring    = "wiring"
	PartSupport   = "support"
	PartTests     = "tests"
)

//go:embed prompts/core.md
var corePrompt string

// Core asks the one role each part plays. What the program exists for is
// then the code's to read off: its domain parts.
func Core() table.Definition {
	return table.Definition{
		Stage: StageCore, Contract: "repomap.atlas.core.v4", System: corePrompt, Independent: true,
		Columns: []table.Column{
			{Name: "role", Kind: table.Choice, Options: []string{PartDomain, PartInterface, PartWiring, PartSupport, PartTests}, Note: "the one role this part plays in the program"},
		},
	}
}
