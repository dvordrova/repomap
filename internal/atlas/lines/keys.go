package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageKeys = "atlas_keys"

// MaxKeysPerPart bounds the declarations shown as a part's keys.
const MaxKeysPerPart = 5

//go:embed prompts/keys.md
var keysPrompt string

// Keys asks, inside one part, which of its declarations a reader needs to
// understand what the part does. The part is the context of every row; the
// file a declaration happens to stand in is only its address.
func Keys() table.Definition {
	return table.Definition{
		Stage: StageKeys, Contract: "repomap.atlas.keys.v1", System: keysPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "key", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when a reader needs this declaration to understand what the part does"},
		},
	}
}
