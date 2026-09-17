package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	StageAPI     = "atlas_api"
	StagePublish = "atlas_publish"
)

//go:embed prompts/api.md
var apiPrompt string

//go:embed prompts/publish.md
var publishPrompt string

// API reads the external symbols the repository calls, one row per symbol:
// what a callable handed to it becomes, whether it publishes what its holder
// holds, and what other system it talks to. Every cell is optional; a symbol
// that does none of it gets no cell.
func API() table.Definition {
	return table.Definition{
		Stage: StageAPI, Contract: "repomap.atlas.api.v1", System: apiPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "binds", Kind: table.Choice, Options: atlas.IncomingBoundaryKinds(), Optional: true, Note: "what a repository callable handed to this symbol becomes; leave out when it runs the callable in place"},
			{Name: "publishes", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when the call makes what its holder holds reachable from outside"},
			{Name: "talks", Kind: table.Choice, Options: atlas.OutgoingBoundaryKinds(), Optional: true, Note: "the kind of other running system this symbol sends to, reads from or creates a client of"},
		},
	}
}

// Publish asks which holder a publishing call serves when the code could not
// follow the value back to one: the rows are the calls, the window's holders
// are the values that hold callables, with what they hold.
func Publish() table.Definition {
	return table.Definition{
		Stage: StagePublish, Contract: "repomap.atlas.publish.v1", System: publishPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "holder", Kind: table.Choice, OptionsFrom: "holder_options", Optional: true, Note: "the h* holder this call publishes; leave out when none of them"},
		},
	}
}
