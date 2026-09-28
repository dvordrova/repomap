package lines

import (
	_ "embed"
	"fmt"
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	StageAPI     = "atlas_api"
	StagePublish = "atlas_publish"
)

// The closed options of the api questions that are not a boundary kind: a
// call that serves is the program's listening side, and none is the
// explicit answer that the symbol is no entry, starts no serving or talks to
// no other running program.
const (
	APIServes     = "serves"
	APIMiddleware = "middleware"
	APINone       = "none"
)

//go:embed prompts/api.md
var apiPrompt string

//go:embed prompts/entry_options.md
var entryOptionsText string

//go:embed prompts/api_publishes_options.md
var apiPublishesOptionsText string

//go:embed prompts/api_talks_options.md
var apiTalksOptionsText string

//go:embed prompts/publish.md
var publishPrompt string

// The criteria of every option of the api questions, read once from their
// embedded Markdown. Every question that asks what something of the
// repository becomes on our map reads one file: an option means the same
// in each of them.
var (
	entryOptions        = mustOptions("prompts/entry_options.md", entryOptionsText, entryOptionNames())
	apiPublishesOptions = mustOptions("prompts/api_publishes_options.md", apiPublishesOptionsText, []string{APIServes, APINone})
	apiTalksOptions     = mustOptions("prompts/api_talks_options.md", apiTalksOptionsText, TalksOptions())
)

// entryOptionNames are every option an entry question may offer: the entry
// kinds, middleware and none.
func entryOptionNames() []string {
	return append(atlas.EntryKinds(), APIMiddleware, APINone)
}

// EntryCriteria are the criteria of the named entry options, read from the
// one criteria file every entry question shares. A name the file does not
// define is a defect of the question asking it.
func EntryCriteria(names ...string) map[string]llm.Criteria {
	criteria := make(map[string]llm.Criteria, len(names))
	for _, name := range names {
		option, ok := entryOptions[name]
		if !ok {
			panic(fmt.Sprintf("lines: prompts/entry_options.md has no option %q", name))
		}
		criteria[name] = option
	}
	return criteria
}

// API reads the external symbols the repository calls, one question per
// symbol and cell, each a closed choice whose every option, none among them,
// carries its criteria. A symbol handed a repository callable is asked what
// the callable becomes and whether the call starts serving it; every other
// symbol is asked what the call does with other running programs. Only the
// decisions the boundaries read are asked.
//
// The categorizer (Jev) answers them. Measured on the saved requests of
// redis 1.3.6 (129 symbols), xk6-dns (47) and microblog (117), 3 draws each
// (2026-09-27), as wrong answers / symbols whose answer changed. The text
// model's optional cells, with no option for talking to nothing and none
// that fits accept: 7/1, 18/12 and 9/2 (inet_aton talks sdk, accept talks
// client_request). These options and criteria in the text model's prompt:
// 2/1 (select serves), 0/0 and 12/0. Jev with them: 0/0, 3/2 and 6/2, with
// 3 and 3 answers left explicitly unanswered under table.ClassifierMargin;
// inet_aton, accept, listen, bind, connect and gethostbyname were right in
// every draw of four rounds. With the one entry criteria file (2026-09-28),
// the saved binds questions of Redis, litestream, python-tutorial-game and
// pykrx (37 symbols, 5 draws against 5 control draws) kept every entry but
// two, each for a stated reason: a flag set's usage printer is printed text
// (none) and an errgroup's goroutine does one piece of work and ends (none).
func API(handed bool) table.Definition {
	def := table.Definition{Stage: StageAPI, Contract: "repomap.atlas.api.v7", System: apiPrompt, Classifier: true, Memoize: true}
	if handed {
		def.Contract += ".handed"
		// The two decisions are independent: a near-tie on one leaves the
		// other standing.
		def.Columns = []table.Column{
			{Name: "binds", Kind: table.Choice, Options: entryOptionNames(), Criteria: EntryCriteria(entryOptionNames()...), Item: "outside_symbol", Alone: true,
				Ask: "What does the repository's callable handed to `outside_symbol` become on our map?"},
			{Name: "publishes", Kind: table.Choice, Options: []string{APIServes, APINone}, Criteria: apiPublishesOptions, Item: "outside_symbol", Alone: true,
				Ask: "Does this call to `outside_symbol` make what the repository hands it reachable by other programs?"},
		}
		return def
	}
	def.Columns = []table.Column{
		{Name: "talks", Kind: table.Choice, Options: TalksOptions(), Criteria: apiTalksOptions, Item: "outside_symbol",
			Ask: "What does a call to `outside_symbol` do with other running programs?"},
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

// TalksOptions are what a call that hands nothing over can do with other
// running programs: serve as the program's listening side, one of the
// outgoing kinds, or none. There is no "other" to fall into.
func TalksOptions() []string {
	return []string{APIServes, atlas.BoundaryClientRequest, atlas.BoundaryDB, atlas.BoundaryQueueProducer, atlas.BoundaryQueueConsumer, atlas.BoundarySDK, APINone}
}

// mustOptions reads the criteria of exactly the named options. The text is
// embedded, so a malformed file is a defect every test of this package meets.
func mustOptions(file, text string, names []string) map[string]llm.Criteria {
	options, err := parseOptionCriteria(text)
	if err != nil {
		panic(fmt.Sprintf("lines: %s: %v", file, err))
	}
	for _, name := range names {
		if _, ok := options[name]; !ok {
			panic(fmt.Sprintf("lines: %s has no option %q", file, name))
		}
	}
	for name := range options {
		if !slices.Contains(names, name) {
			panic(fmt.Sprintf("lines: %s lists option %q the question does not ask", file, name))
		}
	}
	return options
}
