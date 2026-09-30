package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

const StageCore = "atlas_core"

// The role a part plays in its program. The program exists for its domain
// parts. An example part is code the program ships for its users to read,
// copy or start from, which it does not run itself: no core. Whether a part
// is made only of test code is a fact of its files, never a role the model
// gives it.
const (
	PartDomain    = "domain"
	PartInterface = "interface"
	PartWiring    = "wiring"
	PartSupport   = "support"
	PartExample   = "example"
)

//go:embed prompts/core.md
var corePrompt string

//go:embed prompts/core_options.md
var coreOptionsText string

// coreOptions are the criteria of the option the task alone does not
// settle: example code, whose names and purpose alone had read as the
// program's domain (freqtrade's sample strategies had carried the core
// mark).
var coreOptions = mustOptions("prompts/core_options.md", coreOptionsText, []string{PartExample})

// Core asks the one role each part plays. What the program exists for is
// then the code's to read off: its domain parts.
func Core() table.Definition {
	criteria := make(map[string]llm.Criteria, len(coreOptions))
	for name, value := range coreOptions {
		criteria[name] = value
	}
	return table.Definition{
		Stage: StageCore, Contract: "repomap.atlas.core.v6", System: corePrompt,
		// Measured on 50 saved parts of repomap: Jev chose DeepSeek's role for
		// 45; the other five were borderline (two UI presentation parts it
		// called interface where DeepSeek said domain). The example option,
		// re-asked over the saved core requests of freqtrade, redis,
		// litestream and othello (99 parts): freqtrade's Strategy templates
		// example at 1.00; no other part example; three parts that had been
		// near-ties (leads 0.09-0.15) moved between their two close roles.
		Classifier: true,
		Columns: []table.Column{
			{Name: "role", Kind: table.Choice, Options: []string{PartDomain, PartInterface, PartWiring, PartSupport, PartExample}, Criteria: criteria, Note: "the one role this part plays in the program"},
		},
	}
}
