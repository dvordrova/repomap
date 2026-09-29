package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed prompts/api_argument.md
var apiArgumentPrompt string

//go:embed prompts/api_argument_options.md
var apiArgumentOptionsText string

// argumentEach names the criteria every argument, or the receiver, an
// argument question offers carries: the entries are the call's own, so one
// text says what choosing any of them means.
const argumentEach = "argument"

var apiArgumentOptions = mustOptions("prompts/api_argument_options.md", apiArgumentOptionsText, []string{argumentEach, APINone})

// APIArgument asks, of each outside symbol whose calls reach something by
// what they are given (ReachesByArgument), which argument of a call to it,
// or its receiver, names the address or the file the call reaches
// (repomap.atlas.argument.v1): http.Client.Do's request, fopen's path,
// s3's key, a statement's database handle. The code then follows that one
// argument at every call (DestinationReader); no list of packages says it.
// The item is the symbol, its declared type, one call as written, its talks
// answer and `arguments`, the call's arguments by position or keyword and
// its receiver, each with where the call's value comes from, as
// request-local refs; the options are those refs and none, every ref
// carrying one set of criteria. A symbol whose call's result another such
// call is given at its chosen argument (http.NewRequest's request, handed
// to Do) is asked the same question, with `result_given_to` naming those
// calls. Each symbol is remembered on its own (Memoize).
func APIArgument() table.Definition {
	each := apiArgumentOptions[argumentEach]
	return table.Definition{Stage: StageAPI, Contract: "repomap.atlas.argument.v1", System: apiArgumentPrompt, Classifier: true, Memoize: true,
		Columns: []table.Column{{Name: "argument", Kind: table.Choice, Options: []string{APINone}, OptionsFrom: "arguments",
			Criteria: map[string]llm.Criteria{APINone: apiArgumentOptions[APINone]}, EachCriteria: &each, Item: "outside_symbol",
			Ask: "Which argument of this call to `outside_symbol`, or its receiver, names the address or the file the call reaches?"}}}
}
