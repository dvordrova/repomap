package reading

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// An ORM statement is built in a chain of calls, each answered `db`
// (freqtrade's select(func.count(Trade.id)).filter(...), and
// select(func.sum(...).label("cost"))): one exchange with the database is
// one boundary, at the call that begins the statement. A call on what a
// call of the same kind returned continues its exchange (.filter on
// select's statement), and a call whose result a call of the same kind is
// handed whole is part of that call's (func.count in select, and
// func.sum with the .label made on it). A call of another kind stays its
// own boundary, and so does a value handed only as a field of a result.
func TestOneExchangeWithASystemIsOneBoundary(t *testing.T) {
	site := func(line, column int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: "main.c", Line: line, Column: column}
	}
	call := func(symbol string, line, column int) atlas.SymbolCall {
		dot := strings.LastIndex(symbol, ".")
		return atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: symbol, Line: line, Column: column,
			API: &atlas.CallAPI{Package: symbol[:dot], Name: symbol[dot+1:]}}
	}
	result := func(line, column int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "call_result", Anchor: site(line, column)}
	}
	count := call("sqlalchemy.count", 2, 20)
	selectCount := call("sqlalchemy.select", 2, 10)
	selectCount.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: result(2, 20)}}
	filter := call("sqlalchemy.filter", 2, 40)
	filter.ReceiverValue = result(2, 10)
	sum := call("sqlalchemy.sum", 4, 20)
	label := call("sqlalchemy.label", 4, 40)
	label.ReceiverValue = result(4, 20)
	selectSum := call("sqlalchemy.select", 4, 10)
	selectSum.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: result(4, 40)}}
	// A request whose body is a field of the statement's result is another
	// exchange, of another kind.
	post := call("requests.post", 6, 10)
	post.SourceArguments = []atlas.SourceArgument{{Position: 2, Origin: &sourcevalue.Value{Kind: "field", Text: "rowcount", Parts: []sourcevalue.Value{*result(4, 10)}}}}
	get := call("requests.get", 7, 10)
	nested := call("requests.post", 7, 30)
	nested.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "field", Text: "url", Parts: []sourcevalue.Value{*result(7, 10)}}}}
	places := apiGraph(count, selectCount, filter, sum, label, selectSum, post, get, nested)
	r := apiReader(t, t.TempDir(), places, nil)
	for _, symbol := range []string{"sqlalchemy.count", "sqlalchemy.select", "sqlalchemy.filter", "sqlalchemy.sum", "sqlalchemy.label"} {
		r.api[symbol] = apiRole{talks: atlas.BoundaryDB}
	}
	r.api["requests.post"] = apiRole{talks: atlas.BoundaryClientRequest}
	r.api["requests.get"] = apiRole{talks: atlas.BoundaryClientRequest}
	r.bindInterpretedBoundaries()
	var got []string
	for _, state := range r.boundaries {
		got = append(got, state.apiSymbol+" "+state.kind)
	}
	sort.Strings(got)
	want := []string{"requests.get client_request", "requests.post client_request", "requests.post client_request", "sqlalchemy.select db", "sqlalchemy.select db"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("boundaries = %q\nwant %q", got, want)
	}
}
