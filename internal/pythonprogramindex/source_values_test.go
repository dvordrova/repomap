package pythonprogramindex

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestCumulativePythonSourceValuesRetainSharedHelperAndCapturedOwner(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/destinations.py"))
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	owners := map[sourcevalue.Anchor]string{}
	callSites := map[sourcevalue.Anchor]bool{}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		owners[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}] = place.Symbol.Decl.Name
		for _, call := range place.Symbol.Calls {
			if call.Line > 0 {
				callSites[sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}] = true
			}
		}
	}
	var visit func(atlas.Place, *sourcevalue.Value)
	visit = func(place atlas.Place, value *sourcevalue.Value) {
		if value == nil {
			return
		}
		if err := sourcevalue.Validate(value); err != nil {
			t.Fatal(err)
		}
		seen[value.Kind] = true
		if value.Initializer != nil {
			seen["field_initializer"] = true
			if strings.Contains(place.Symbol.Decl.Name, "MutableAdapter") && value.Text == "url" {
				if value.Initializer.Kind != "field_value" || value.Initializer.Parts[0].Kind != "unknown" {
					t.Fatalf("later field write kept initial address: %+v", value)
				}
				seen["mutable_field_unknown"] = true
			}
			visit(place, value.Initializer)
		}
		if value.Kind == "parameter" {
			if owners[*value.Owner] == "" {
				t.Fatalf("parameter owner is not a native declaration: %+v", value)
			}
			if place.Symbol.Decl.Name == "refresh" && value.Text == "base" {
				if owners[*value.Owner] != "captured_exchange" {
					t.Fatalf("capture changed owner: %+v", value)
				}
				seen["capture"] = true
			}
		}
		if value.Kind == "call_result" && !callSites[*value.Anchor] {
			t.Fatalf("producer has no original argument row: %+v", value)
		}
		if value.Kind == "literal" && value.Text == "https://시세.example/prices%20latest" {
			seen["raw_unicode_url"] = true
		}
		if value.Kind == "literal" && value.Text == "/시세" {
			seen["suffix"] = true
		}
		for i := range value.Parts {
			visit(place, &value.Parts[i])
		}
	}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			visit(place, call.ReceiverValue)
			visit(place, call.ResultValue)
			for _, argument := range call.SourceArguments {
				// A name rebound in the statement list of its first binding
				// holds the second value for the reads after it: the
				// replacement, never the literal it replaced.
				if place.Symbol.Decl.Name == "reassigned_address" && argument.Position == 2 && call.Name == "dispatch" {
					if argument.Origin.Kind != "parameter" || argument.Origin.Text != "replacement" {
						t.Fatalf("a rebound name read %+v, want the replacement", argument.Origin)
					}
					seen["rebound_in_place"] = true
				}
				visit(place, argument.Origin)
			}
		}
	}
	for _, name := range []string{"parameter", "literal", "concat", "call_result", "field", "index", "alternatives", "capture", "suffix", "raw_unicode_url", "rebound_in_place", "receiver", "record", "field_value", "field_initializer", "mutable_field_unknown", "entered"} {
		if !seen[name] {
			t.Errorf("source observation missing: %s", name)
		}
	}
	// The preset's decided argument: requests.get's first names its URL.
	reader := reading.NewDestinationReader(graph.Places, reading.DestinationChoices{Arguments: map[string]reading.ArgumentChoice{"requests.get": {Position: 1}}})
	var direct, config, constructor bool
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil || call.API.Package != "requests" || call.API.Name != "get" {
				continue
			}
			uses := reader.Read(place, call)
			switch place.Symbol.Decl.Name {
			case "direct_price_feed":
				direct = len(uses) == 1 && uses[0].Address == "https://시세.example/prices%20latest" && uses[0].Frontier == ""
			case "dispatch":
				for _, use := range uses {
					config = config || strings.Contains(use.Frontier, "notifications") && strings.Contains(use.Frontier, "url")
				}
			case "LiteralAdapter.refresh":
				constructor = len(uses) == 1 && uses[0].Address == "https://exchange.example/candles" && uses[0].Frontier == "" && len(uses[0].Steps) >= 2
				if !constructor {
					t.Errorf("native Python constructor chain: %+v", uses)
				}
			}
		}
	}
	if !direct || !config || !constructor {
		t.Fatalf("native destination reader: direct=%v config=%v constructor=%v", direct, config, constructor)
	}
}

// destinations.py's ledger is reached through objects, the way freqtrade's
// persistence is: `Ledger.session = scoped_session(sessionmaker(bind=
// engine))` is the one store of a class attribute, written through the
// class's name, so `Ledger.session` reads that call's result wherever it is
// read, and `Archive.session`, stored from it, too; Journal also stores
// its session in its class body, so it reads none. `with engine.begin() as
// connection` binds what entering begin() gives, in both blocks, and a
// statement rebound in its block is the second statement where it is sent
// next, but not in a lambda run later. Every statement sent through one of
// those objects ends where create_engine's walk ends: one destination.
func TestCumulativePythonStatementsSentThroughObjectsEndAtTheirEngine(t *testing.T) {
	const path = "src/fixture_app/destinations.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(files[path], "\n")
	lineOf := func(text string) int {
		for i, line := range lines {
			if strings.TrimSpace(line) == text {
				return i + 1
			}
		}
		t.Fatalf("no line %q", text)
		return 0
	}
	type found struct {
		place atlas.Place
		call  atlas.SymbolCall
	}
	calls := map[string][]found{}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != path {
			continue
		}
		for _, call := range place.Symbol.Calls {
			calls[place.Symbol.Decl.Name+" "+call.Name] = append(calls[place.Symbol.Decl.Name+" "+call.Name], found{place, call})
		}
	}
	one := func(key string) found {
		if len(calls[key]) != 1 {
			t.Fatalf("calls %q: %+v", key, calls[key])
		}
		return calls[key][0]
	}
	session := lineOf("Ledger.session = scoped_session(sessionmaker(bind=engine))")
	for _, key := range []string{"count_entries execute", "archived_entries scalars"} {
		receiver := one(key).call.ReceiverValue
		if receiver == nil || receiver.Kind != "call_result" || receiver.Anchor == nil || receiver.Anchor.Line != session {
			t.Fatalf("%s is made on %+v, want what scoped_session returned at line %d", key, receiver, session)
		}
	}
	if receiver := one("journal_entries scalars").call.ReceiverValue; receiver == nil || receiver.Kind != "field" {
		t.Fatalf("a class attribute stored twice is read as %+v", receiver)
	}
	executes := calls["migrate_ledger execute"]
	if len(executes) != 3 {
		t.Fatalf("migrate_ledger executes %d times", len(executes))
	}
	for _, execute := range executes {
		if receiver := execute.call.ReceiverValue; receiver == nil || receiver.Kind != "entered" || receiver.Parts[0].Kind != "call_result" {
			t.Fatalf("a connection a with statement entered is %+v", receiver)
		}
	}
	second := lineOf(`statement = sqlalchemy.update(Archive).values(note="")`)
	sent := executes[2].call.SourceArguments[0].Origin
	if sent.Kind != "call_result" || sent.Anchor.Line != second {
		t.Fatalf("the statement rebound in its block is sent as %+v, want the one at line %d", sent, second)
	}
	// The lambda is no place of its own: its call is read in the index.
	deferred := lineOf("return lambda: Ledger.session.execute(statement)")
	lambdaRead := false
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path || relation.Location.Line != deferred {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != "execute" {
				continue
			}
			lambdaRead = true
			if len(pattern.Arguments) != 1 || pattern.Arguments[0].Origin == nil || pattern.Arguments[0].Origin.Kind != "unknown" {
				t.Fatalf("a lambda reads a rebound name as %+v", pattern.Arguments)
			}
		}
	}
	if !lambdaRead {
		t.Fatal("the lambda's call was not found")
	}
	none := reading.ArgumentChoice{None: true}
	reader := reading.NewDestinationReader(graph.Places, reading.DestinationChoices{
		Arguments: map[string]reading.ArgumentChoice{"sqlalchemy.create_engine": {Position: 1}, "sqlalchemy.select": none, "sqlalchemy.text": none, "sqlalchemy.update": none},
		Talks: map[string]string{"sqlalchemy.select": "db", "sqlalchemy.text": "db", "sqlalchemy.update": "db", "sqlalchemy.update.values": "db",
			"sqlalchemy.select.where": "db", "sqlalchemy.update.where": "db", "sqlalchemy.select.cte": "db"},
	})
	ends := func(uses []atlas.DestinationUse) []string {
		var result []string
		for _, use := range uses {
			if n := len(use.Steps); n > 0 {
				step := use.Steps[n-1]
				result = append(result, fmt.Sprintf("%s|%s|%s:%d", use.Address, use.Frontier, step.Path, step.Line))
			}
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	engine := one("open_ledger sqlalchemy.create_engine")
	want := ends(reader.Read(engine.place, engine.call))
	if len(want) != 1 || !strings.Contains(want[0], "|url|") {
		t.Fatalf("create_engine's walk ends at %q", want)
	}
	// The statement an if statement's arms bind is either after it.
	returned := one("filtered_entries ledger_query").call.ResultValue
	if returned == nil || returned.Kind != "alternatives" || len(returned.Parts) != 2 {
		t.Fatalf("ledger_query returns %+v, want the where and the statement before it", returned)
	}
	pruning := one("open_ledger_to_prune sqlalchemy.create_engine")
	pruned := ends(reader.Read(pruning.place, pruning.call))
	for _, key := range []string{"prune_ledger sqlalchemy.select", "prune_ledger sqlalchemy.update"} {
		if got := ends(reader.Exchange(one(key).place, one(key).call, "db")); !slices.Equal(got, pruned) {
			t.Fatalf("%s ends at %q, want %q", key, got, pruned)
		}
	}
	for _, key := range []string{"count_entries sqlalchemy.select", "archived_entries sqlalchemy.select", "migrate_ledger sqlalchemy.text", "migrate_ledger sqlalchemy.update",
		"ledger_query sqlalchemy.select", "ledger_totals sqlalchemy.select"} {
		if len(calls[key]) == 0 {
			t.Fatalf("no call %q", key)
		}
		for _, row := range calls[key] {
			if got := ends(reader.Exchange(row.place, row.call, "db")); !slices.Equal(got, want) {
				t.Fatalf("%s at line %d ends at %q, want %q", key, row.call.Line, got, want)
			}
		}
	}
	journal := one("journal_entries sqlalchemy.select")
	if got := reader.Exchange(journal.place, journal.call, "db"); got != nil {
		t.Fatalf("a statement sent through a session no one store gives reaches %+v", got)
	}
}
