package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

//go:embed prompts/api_dispatch.md
var apiDispatchPrompt string

// APIDispatch asks, of one value the repository's own code compares with
// several words (ProgramIndex Comparison: a switch's cases, an if/elif
// chain, a match or case form), what those words become on our map
// (repomap.atlas.dispatch.v1). One row is one comparison, asked once for
// all its cases: litestream's `switch cmd` compares cmd with its 14
// subcommands and is one question, not 14. The item is the value as
// written, where it comes from (originText), the declaration the
// comparison is written in with its signature, and each case's words in
// source order. The options and their criteria are the call question's
// (EntersOptions, the one entry criteria file); each comparison is
// remembered by what its item shows, never by its line.
func APIDispatch() table.Definition {
	return table.Definition{Stage: StageInputs, Contract: "repomap.atlas.dispatch.v1", System: apiDispatchPrompt, Classifier: true, Memoize: true,
		Columns: []table.Column{{Name: "enters", Kind: table.Choice, Options: EntersOptions(), Criteria: EntryCriteria(EntersOptions()...), Item: "comparison",
			Ask: "What do the words this value is compared with become on our map?"}}}
}
