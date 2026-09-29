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
	StageProgram = "atlas_program"
)

// The closed options of the api questions that are not a boundary kind: a
// call that serves is the program's listening side, and none is the
// explicit answer that the symbol is no entry, starts no serving or talks to
// no other running program.
const (
	APIServes     = "serves"
	APIMiddleware = "middleware"
	APINone       = "none"
	// APIFile is the talks answer that a call reaches a file or a directory
	// by its path: no other running program, and no outside system.
	APIFile = "file"
)

//go:embed prompts/api.md
var apiPrompt string

//go:embed prompts/entry_options.md
var entryOptionsText string

//go:embed prompts/api_call.md
var apiCallPrompt string

//go:embed prompts/inputs.md
var inputsPrompt string

//go:embed prompts/api_talks_options.md
var apiTalksOptionsText string

//go:embed prompts/publish.md
var publishPrompt string

//go:embed prompts/program.md
var programPrompt string

//go:embed prompts/program_options.md
var programOptionsText string

// The criteria of every option of the api questions, read once from their
// embedded Markdown. Every question that asks what something of the
// repository becomes on our map reads one file: an option means the same
// in each of them.
var (
	entryOptions    = mustOptions("prompts/entry_options.md", entryOptionsText, entryOptionNames())
	apiTalksOptions = mustOptions("prompts/api_talks_options.md", apiTalksOptionsText, TalksOptions())
	programOptions  = mustOptions("prompts/program_options.md", programOptionsText, []string{programWord, ProgramNotNamed})
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
// carries its criteria. Every symbol is asked what a call to it does with
// other running programs or with files (talks); a symbol handed a repository
// callable is also asked what the callable becomes (binds). Only the
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
//
// A handed symbol was asked only whether its call starts serving what it
// is handed until 2026-09-29: ssh.Dial, handed a configuration the
// repository built with a callback in it, was never asked what it talks
// to. The talks choice holds serving among its options, so a handed symbol
// the code calls is asked it in that question's place, and the count of
// questions stays the same. A handed symbol no call names (the field a
// table's rows store a callable in) has no call for talks to decide and is
// asked binds alone.
//
// binds and talks say which of the two decisions a table asks; at least
// one of them.
func API(binds, talks bool) table.Definition {
	def := table.Definition{Stage: StageAPI, Contract: "repomap.atlas.api.v9", System: apiPrompt, Classifier: true, Memoize: true}
	talking := table.Column{Name: "talks", Kind: table.Choice, Options: TalksOptions(), Criteria: apiTalksOptions, Item: "outside_symbol",
		Ask: "What does a call to `outside_symbol` do with other running programs or with files?"}
	if !binds {
		def.Columns = []table.Column{talking}
		return def
	}
	// The two decisions are independent: a near-tie on one leaves the
	// other standing.
	def.Contract += ".handed"
	def.Columns = []table.Column{{Name: "binds", Kind: table.Choice, Options: entryOptionNames(), Criteria: EntryCriteria(entryOptionNames()...), Item: "outside_symbol", Alone: true,
		Ask: "What does the repository's callable handed to `outside_symbol` become on our map?"}}
	if talks {
		talking.Alone = true
		def.Columns = append(def.Columns, talking)
		return def
	}
	def.Contract += ".uncalled"
	return def
}

// APICall asks, of one call outside tests that gives an outside symbol a
// word, what the words that call is given become on our map
// (repomap.atlas.enters.v1): flag.Bool("verbose", …) declares an option,
// printf("%s\n", …) prints text, and one string comparison checks argv at
// one call and the program's own names at another, so the answer belongs to
// the call, not to its symbol. The item is the symbol and its declared
// type, the call as written, the declaration it is written in, every word
// it is given, where each of its arguments comes from and what a call to
// the symbol does with other running programs when that is decided. Each
// call is remembered on its own. The symbol's talks answer is decided
// first: a call whose symbol talks to another program, or serves, is not
// asked, since its words are that program's (one is not both). No outcome
// is offered in both: taking messages from a queue is talks's, so enters
// offers no queue_consumer and no middleware.
func APICall() table.Definition {
	return table.Definition{Stage: StageAPI, Contract: "repomap.atlas.enters.v1", System: apiCallPrompt, Classifier: true, Memoize: true,
		Columns: []table.Column{{Name: "enters", Kind: table.Choice, Options: EntersOptions(), Criteria: EntryCriteria(EntersOptions()...), Item: "outside_call",
			Ask: "What does the word or words this call is given become on our map?"}}}
}

// FieldOptions are what the key a field's tag names can become: a setting
// of the program's configuration file, or none (a key of data it parses or
// sends).
func FieldOptions() []string {
	return []string{atlas.BoundarySetting, APINone}
}

// EntersOptions are what the words a call is given can become: an entry
// kind, except the queue consumer talks decides, or none.
func EntersOptions() []string {
	var options []string
	for _, kind := range atlas.EntryKinds() {
		if kind != atlas.BoundaryQueueConsumer {
			options = append(options, kind)
		}
	}
	return append(options, APINone)
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

// TalksOptions are what a call can do with other running programs or with
// files: serve as the program's listening side, one of the outgoing kinds,
// start another program, reach a file by its path, or none. There is no
// "other" to fall into.
func TalksOptions() []string {
	return []string{APIServes, atlas.BoundaryClientRequest, atlas.BoundaryDB, atlas.BoundaryQueueProducer, atlas.BoundaryQueueConsumer, atlas.BoundarySDK, atlas.BoundaryRunsProgram, APIFile, APINone}
}

// ReachesByArgument reports a talks answer whose call reaches something an
// argument or its receiver names: an outgoing kind a destination is read
// for, or a file. Serving has its address among its words and a started
// program its own question.
func ReachesByArgument(talks string) bool {
	switch talks {
	case atlas.BoundaryClientRequest, atlas.BoundaryDB, atlas.BoundaryQueueProducer, atlas.BoundaryQueueConsumer, atlas.BoundarySDK, APIFile:
		return true
	}
	return false
}

// ProgramNotNamed is the program answer that no word a call is given names
// the program it starts: the program comes from a value the code computes.
const ProgramNotNamed = "not_named"

// programWord names the criteria every word a program question offers
// carries: the words are the call's own, so one text says what choosing
// any of them means.
const programWord = "word"

// Program asks, of each call that starts another program, which of the
// words it is given names that program, or that none does
// (repomap.atlas.program.v1). One row is one call: a symbol such as
// exec.Command starts git at one site and make at another, so the answer
// belongs to the call, not to the symbol. The options are the call's own
// words (ProgramRow), every one of them, as written.
func Program() table.Definition {
	word := programOptions[programWord]
	return table.Definition{Stage: StageProgram, Contract: "repomap.atlas.program.v1", System: programPrompt, Classifier: true, Memoize: true,
		Columns: []table.Column{{Name: "program", Kind: table.Choice, Options: []string{ProgramNotNamed}, OptionsFrom: "words",
			Criteria: map[string]llm.Criteria{ProgramNotNamed: programOptions[ProgramNotNamed]}, EachCriteria: &word, Item: "outside_symbol",
			Ask: "Which word the call gives `outside_symbol` names the program it starts?"}}}
}

// ProgramRow is one call that starts another program: the outside symbol,
// the call as written and every word it is given that can stand on one
// line, each once, in the order the call writes them, as request-local
// refs. It returns the words by ref, which restore the answer as written.
func ProgramRow(id, symbol, usage string, values []string) (table.Row, map[string]string) {
	fields := []table.Field{{Name: "symbol", Value: symbol}}
	if usage != "" {
		fields = append(fields, table.Field{Name: "usage", Value: usage})
	}
	byRef := make(map[string]string)
	var words []map[string]any
	seen := make(map[string]bool)
	for _, word := range NameableWords(values) {
		if seen[word] {
			continue
		}
		seen[word] = true
		ref := fmt.Sprintf("w%d", len(words)+1)
		byRef[ref] = word
		words = append(words, map[string]any{"ref": ref, "title": word})
	}
	if len(words) > 0 {
		fields = append(fields, table.Field{Name: "words", Value: words})
	}
	return table.Row{ID: id, Fields: fields}, byRef
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

// StageInputs asks what the repository's own registrations and tables
// are: a callable a repository function keeps for later, and a table of
// names (pass 2, C).
const StageInputs = "atlas_inputs"

// Inputs asks one closed question of each candidate of a form
// (repomap.atlas.inputs.v1.<form>): "stored", a callable the repository
// hands to its own function that keeps it (the entry kinds, middleware or
// none, as the handed-callable question offers); "table", a table of names
// the repository declares (the entry kinds but the queue consumer, or
// none); "field", a field of a repository structure whose tag names a key
// (setting or none). One row per candidate; an undecided answer makes no
// entry.
func Inputs(form string) table.Definition {
	def := table.Definition{Stage: StageInputs, Contract: "repomap.atlas.inputs.v1." + form, System: inputsPrompt, Classifier: true, Memoize: true}
	switch form {
	case "stored":
		def.Columns = []table.Column{{Name: "becomes", Kind: table.Choice, Options: entryOptionNames(), Criteria: EntryCriteria(entryOptionNames()...), Item: "candidate",
			Ask: "What does the repository's callable in `candidate`, which the repository's own function keeps for later, become on our map?"}}
	case "table":
		def.Columns = []table.Column{{Name: "becomes", Kind: table.Choice, Options: EntersOptions(), Criteria: EntryCriteria(EntersOptions()...), Item: "candidate",
			Ask: "What do the rows of the table in `candidate` become on our map?"}}
	case "field":
		def.Columns = []table.Column{{Name: "becomes", Kind: table.Choice, Options: FieldOptions(), Criteria: EntryCriteria(FieldOptions()...), Item: "candidate",
			Ask: "What does the key the tag of the field in `candidate` names become on our map?"}}
	default:
		panic("lines: no inputs form " + form)
	}
	return def
}
