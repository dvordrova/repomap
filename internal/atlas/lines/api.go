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

// API reads the external symbols the repository calls. A symbol handed a
// repository callable is asked what the callable becomes; every other
// symbol is asked whether it starts serving and what other running system
// it talks to. Every cell is optional: a symbol that does none of it gets no
// cell. Only cells the boundaries read are asked: a decision without a
// reader comes back as its own table, with an explicit none, when a reader
// for it lands.
func API(handed bool) table.Definition {
	def := table.Definition{Stage: StageAPI, Contract: "repomap.atlas.api.v5", System: apiPrompt}
	if handed {
		def.Contract += ".handed"
		def.Columns = []table.Column{
			{Name: "binds", Kind: table.Choice, Options: atlas.EntryKinds(), Optional: true, Note: "what the handed callable becomes: request a handler of what a client sends over a connection, whatever the protocol (a route, an RPC method, a command a client sends); command a command a person runs from a command line or task runner; interaction a handler of a person's action in a user interface; scheduled work a timer runs; continuous work that runs for as long as the program does, on a thread, task or loop of its own; queue_consumer a handler of messages taken from a queue; extension a hook a host program calls at its own points. A symbol that runs the callable in place, wraps it, or only marks or transforms it binds nothing, and neither does one that runs it only when the process is signalled or fails"},
			{Name: "middleware", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when the callable runs around or before the handlers rather than being an entry of its own"},
			{Name: "publishes", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this call starts serving: listens on an address, runs the application, connects the consumer"},
		}
		return def
	}
	def.Columns = []table.Column{
		{Name: "publishes", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this call starts serving: listens on an address, runs the application, connects the consumer"},
		{Name: "talks", Kind: table.Choice, Options: talksOptions(), Optional: true, Note: "the kind of other running system this call itself sends to, reads from or opens a connection to: client_request, db, queue_producer, queue_consumer, sdk. A call that builds or configures — returning the same type it was called on, setting a header, tuning a pool — and a call that reads a result already received talk to nothing"},
	}
	return def
}

// Publish asks which holder a publishing call serves when the code could not
// follow the value back to one: the rows are the calls, the window's holders
// are the values that hold callables, with what they hold.
func Publish() table.Definition {
	return table.Definition{
		Stage: StagePublish, Contract: "repomap.atlas.publish.v1", System: publishPrompt,
		Columns: []table.Column{
			{Name: "holder", Kind: table.Choice, OptionsFrom: "holder_options", Optional: true, Note: "the h* holder this call publishes; leave out when none of them"},
		},
	}
}

// talksOptions are the outgoing kinds a symbol can talk to. A symbol that
// talks to nothing named leaves the cell out; there is no "other" to fall into.
func talksOptions() []string {
	return []string{atlas.BoundaryClientRequest, atlas.BoundaryDB, atlas.BoundaryQueueProducer, atlas.BoundaryQueueConsumer, atlas.BoundarySDK}
}
