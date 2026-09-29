package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

//go:embed prompts/api_start.md
var apiStartPrompt string

// StartOptions are what a repository function one statement starts to run
// on its own can become: work another program sends it (a goroutine per
// accepted connection), a periodic job, a worker for the program's whole
// life, a consumer of a broker's messages, or none (one piece of work that
// ends). What a person gives the program, writes in its configuration or
// clicks is never started by the program itself, and a started function
// runs around no handler.
func StartOptions() []string {
	return []string{atlas.BoundaryRequest, atlas.BoundaryScheduled, atlas.BoundaryContinuous, atlas.BoundaryQueueConsumer, APINone}
}

// APIStart asks, of one statement that starts a repository function to run
// on its own (a registration with a shared invocation word: Go's `go f()`,
// a coroutine handed to asyncio.create_task), what the started function
// becomes on our map (repomap.atlas.starts.v1). One row is one statement:
// litestream starts 20 functions with `go`, a ticker monitor at one and a
// signal waiter at another, and one answer read from one example would
// decide them all. The item is the statement as written, a closure's body
// included, the declaration it is written in with its signature, the
// started function with its signature, what that function calls by name,
// and the started call's literals. Each statement is remembered by what its
// item shows, never by its line; the criteria are the one entry criteria
// file's.
func APIStart() table.Definition {
	return table.Definition{Stage: StageAPI, Contract: "repomap.atlas.starts.v1", System: apiStartPrompt, Classifier: true, Memoize: true,
		Columns: []table.Column{{Name: "starts", Kind: table.Choice, Options: StartOptions(), Criteria: EntryCriteria(StartOptions()...), Item: "started_call",
			Ask: "What does the function this statement starts on its own become on our map?"}}}
}
