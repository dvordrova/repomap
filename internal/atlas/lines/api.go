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
// symbol is asked what it does with the values it gets. Every cell is
// optional: a symbol that does none of it gets no cell.
func API(handed bool) table.Definition {
	def := table.Definition{Stage: StageAPI, Contract: "repomap.atlas.api.v3", System: apiPrompt}
	if handed {
		def.Contract += ".handed"
		def.Columns = []table.Column{
			{Name: "binds", Kind: table.Choice, Options: atlas.EntryKinds(), Optional: true, Note: "what the handed callable becomes: http_server a handler of requests on the given path; queue_consumer a handler of messages; scheduled work a timer runs; interaction a handler of a user's action; extension a hook registered with a host; command a command a runner activates. A symbol that runs the callable in place, wraps it, or only marks or transforms it binds nothing"},
			{Name: "middleware", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when the callable runs around or before the handlers rather than being an entry of its own"},
			{Name: "publishes", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this call starts serving: listens on an address, runs the application, connects the consumer"},
		}
		return def
	}
	def.Columns = []table.Column{
		{Name: "publishes", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this call starts serving: listens on an address, runs the application, connects the consumer"},
		{Name: "talks", Kind: table.Choice, Options: talksOptions(), Optional: true, Note: "the kind of other running system this call itself sends to, reads from or opens a connection to: http_client, db, queue_producer, queue_consumer, sdk. A call that builds or configures — returning the same type it was called on, setting a header, tuning a pool — and a call that reads a result already received talk to nothing"},
		{Name: "reads_input", Kind: table.Choice, Options: atlas.InputKinds(), Optional: true, Note: "the part of a received request this symbol reads: body, path, query or header"},
		{Name: "writes_output", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this symbol writes the response a received request gets"},
		{Name: "auth", Kind: table.Choice, Options: atlas.AuthKinds(), Optional: true, Note: "with credentials: verifies a token or a password, issues a token, hashes a secret"},
		{Name: "config", Kind: table.Choice, Options: atlas.ConfigKinds(), Optional: true, Note: "reads one configuration value, or loads configuration from a source"},
		{Name: "validates", Kind: table.Choice, Options: []string{"yes"}, Optional: true, Note: "yes when this symbol checks input against rules"},
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
	return []string{atlas.BoundaryHTTPClient, atlas.BoundaryDB, atlas.BoundaryQueueProducer, atlas.BoundaryQueueConsumer, atlas.BoundarySDK}
}
