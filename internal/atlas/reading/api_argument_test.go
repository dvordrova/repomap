package reading

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// argumentSource is main.go: a request built with its URL and sent, a file
// created by its path, and a URL parsed that no call sends.
const argumentSource = `func main(endpoint, path, raw string) {
	req, _ := http.NewRequest("GET", endpoint, nil)
	resp, _ := http.DefaultClient.Do(req)
	f, _ := os.Create(path)
	u, _ := url.Parse(raw)
}
`

// Which argument names what a call reaches is asked once per symbol whose
// calls reach something, and of the symbols whose results those arguments
// carry, round by round, never of a symbol no walk reaches: Do is asked
// with its request, and NewRequest, whose result that request is, with the
// call it is given to; os.Create, which reaches a file, is asked too and
// its file logged; url.Parse, which nothing sends, is not asked. The walk
// of Do's boundary then follows the request to the URL's parameter.
func TestWhichArgumentIsAskedOfWhatReachesAndOfWhatItsArgumentCarries(t *testing.T) {
	anchor := func(line int, mark string) *sourcevalue.Anchor {
		text := strings.Split(argumentSource, "\n")[line-1]
		return &sourcevalue.Anchor{Path: "main.go", Line: line, Column: strings.Index(text, mark) + 1}
	}
	owner := &sourcevalue.Anchor{Path: "main.go", Line: 1, Column: 1}
	call := func(pkg, receiver, name string, line int, mark string, arguments ...atlas.SourceArgument) atlas.SymbolCall {
		at := anchor(line, mark)
		return atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: name, Line: line, Column: at.Column, SourceArguments: arguments,
			API: &atlas.CallAPI{Package: pkg, Receiver: receiver, Name: name}}
	}
	parameter := func(position int, name string) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "parameter", Position: position, Text: name, Owner: owner}
	}
	newRequest := call("net/http", "", "NewRequest", 2, "http.NewRequest",
		atlas.SourceArgument{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "GET"}}, atlas.SourceArgument{Position: 2, Origin: parameter(1, "endpoint")})
	do := call("net/http", "*Client", "Do", 3, "http.DefaultClient.Do",
		atlas.SourceArgument{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Text: "http.NewRequest", Anchor: anchor(2, "http.NewRequest")}})
	create := call("os", "", "Create", 4, "os.Create", atlas.SourceArgument{Position: 1, Origin: parameter(2, "path")})
	parse := call("net/url", "", "Parse", 5, "url.Parse", atlas.SourceArgument{Position: 1, Origin: parameter(3, "raw")})
	places := apiGraph(newRequest, do, create, parse)
	places[1].Path, places[0].Path, places[1].Symbol.Decl.Column = "main.go", "main.go", 1
	talks := map[string]string{"net/http.Client.Do": atlas.BoundaryClientRequest, "os.Create": lines.APIFile}
	arguments := map[string]string{"net/http.Client.Do": "argument 1", "net/http.NewRequest": "argument 2", "os.Create": "argument 1"}
	var mu sync.Mutex
	asked := map[string]map[string]any{}
	questions := 0
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		symbol, _ := question.Item["symbol"].(string)
		switch column := key[strings.LastIndex(key, "|")+1:]; column {
		case "talks":
			if answer := talks[symbol]; answer != "" {
				return typesafetest.Choose(answer), true
			}
			return typesafetest.Choose(lines.APINone), true
		case "argument":
			mu.Lock()
			asked[symbol] = question.Item
			questions++
			mu.Unlock()
			return typesafetest.Choose(arguments[symbol]), true
		case "enters":
			return typesafetest.Choose(lines.APINone), true
		}
		return llm.Verdict{}, false
	}}
	r := apiReader(t, t.TempDir(), places, categorizer)
	r.opts.ReadSource = func(string) ([]byte, error) { return []byte(argumentSource), nil }
	if err := r.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range asked {
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"net/http.Client.Do", "net/http.NewRequest", "os.Create"}; !slices.Equal(names, want) {
		t.Fatalf("asked which argument of %v, want %v", names, want)
	}
	if given := asked["net/http.NewRequest"]["result_given_to"]; !slices.Equal(anyStrings(given), []string{"net/http.Client.Do (client_request)"}) {
		t.Fatalf("NewRequest was not asked with the call its result is given to: %v", asked["net/http.NewRequest"])
	}
	if usage := asked["os.Create"]["usage"]; usage != "os.Create(path)" {
		t.Fatalf("os.Create's usage = %v", usage)
	}
	want := map[string]ArgumentChoice{"net/http.Client.Do": {Position: 1}, "net/http.NewRequest": {Position: 2}, "os.Create": {Position: 1}}
	for name, choice := range want {
		if r.arguments[name] != choice {
			t.Fatalf("%s's argument = %+v, want %+v", name, r.arguments[name], choice)
		}
	}
	if !strings.Contains(r.tables.String(), "atlas_files: os.Create at main.go:4 reaches {path}") {
		t.Fatalf("the file os.Create reaches was not logged:\n%s", r.tables.String())
	}
	// The decisions are remembered per symbol: a second reading asks none.
	answered := questions
	again := apiReader(t, r.opts.Executor.RootDir, places, categorizer)
	again.opts.ReadSource = r.opts.ReadSource
	if err := again.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	if questions != answered || again.arguments["net/http.NewRequest"] != want["net/http.NewRequest"] {
		t.Fatalf("a warm reading asked again or lost a decision: %+v", again.arguments)
	}
	// The boundary Do makes follows its request to the URL it was built with.
	uses := NewDestinationReader(places, DestinationChoices{Arguments: r.arguments}).Read(places[1], do)
	if len(uses) != 1 || uses[0].Frontier != "endpoint" || len(uses[0].Steps) != 2 {
		t.Fatalf("Do's walk = %+v", uses)
	}
}

func anyStrings(value any) []string {
	var result []string
	switch list := value.(type) {
	case []string:
		return list
	case []any:
		for _, item := range list {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
	}
	return result
}
