package reading

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// freqtrade's database: every statement is built by a call that names
// nothing it reaches (select, text: decided none) and sent through an
// object an engine made, `Trade.session.execute(select(...).filter(...))`
// and `connection.execute(text(...))` in `with engine.begin() as
// connection`. Each row reaches what create_engine's decided argument
// reaches, so all of them end where its walk ends: one destination, and
// so does `read_sql(..., con=Trade.session.bind)`. A statement handed to
// nothing, or only to a logger, stays its own, and so does one sent
// through an object no outside call made. Objects made by one call given
// different words stay apart (connect("primary"), connect("replica")),
// and a field of what an outside call answered another kind made (a
// configuration a file read gives) is not followed.
func TestRowsSentThroughOneEngineReachWhatTheEngineReaches(t *testing.T) {
	at := func(path string, line, column int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: path, Line: line, Column: column}
	}
	result := func(path string, line, column int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "call_result", Anchor: at(path, line, column)}
	}
	outside := func(symbol, receiver, name string, line, column int, arguments ...atlas.SourceArgument) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: symbol, Line: line, Column: column, SourceArguments: arguments, API: &atlas.CallAPI{Package: "sqlalchemy", Receiver: receiver, Name: name}}
	}
	unresolved := func(name string, line, column int, receiver *sourcevalue.Value, arguments ...atlas.SourceArgument) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "calls", Name: name, Line: line, Column: column, ReceiverValue: receiver, SourceArguments: arguments, Resolution: "unresolved"}
	}
	positional := func(value *sourcevalue.Value) atlas.SourceArgument {
		return atlas.SourceArgument{Position: 1, Origin: value}
	}
	symbol := func(id, path string, line int, name string, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Column: 1}, Calls: calls}}
	}
	engine := outside("sqlalchemy.create_engine", "", "create_engine", 77, 18,
		positional(&sourcevalue.Value{Kind: "parameter", Text: "db_url", Position: 1, Owner: at("models.py", 48, 1)}))
	maker := outside("sqlalchemy.orm.sessionmaker", "orm", "sessionmaker", 87, 9,
		atlas.SourceArgument{Keyword: "autoflush", Origin: &sourcevalue.Value{Kind: "literal", Text: "False"}},
		atlas.SourceArgument{Keyword: "bind", Origin: result("models.py", 77, 18)})
	scoped := outside("sqlalchemy.orm.scoped_session", "orm", "scoped_session", 86, 21, positional(result("models.py", 87, 9)))
	migrateCall := atlas.SymbolCall{Kind: "calls", Name: "check_migrate", Line: 99, Column: 5, CalleeIDs: []string{"migrate"}, SourceArguments: []atlas.SourceArgument{positional(result("models.py", 77, 18))}}
	initDB := symbol("init", "models.py", 48, "init_db", engine, maker, scoped, migrateCall)
	bot := symbol("bot", "bot.py", 100, "Bot.__init__", atlas.SymbolCall{Kind: "calls", Name: "init_db", Line: 101, Column: 5, CalleeIDs: []string{"init"},
		SourceArguments: []atlas.SourceArgument{positional(&sourcevalue.Value{Kind: "literal", Text: "sqlite:///trades.db"})}})
	// Trade.session reads what its one store gave it: scoped_session's result.
	selectRow := outside("sqlalchemy.select", "", "select", 1578, 17)
	filter := outside("sqlalchemy.select.filter", "select", "filter", 1578, 46)
	filter.ReceiverValue = result("trade.py", 1578, 17)
	execute := unresolved("execute", 1577, 34, result("models.py", 86, 21), positional(result("trade.py", 1578, 46)))
	lonely := outside("sqlalchemy.select", "", "select", 1590, 9)
	logged := atlas.SymbolCall{Kind: "invokes_external", Name: "logging.getLogger.debug", Line: 1591, Column: 9, API: &atlas.CallAPI{Package: "logging", Receiver: "getLogger", Name: "debug"},
		ReceiverValue: result("trade.py", 1560, 10), SourceArguments: []atlas.SourceArgument{positional(result("trade.py", 1590, 9))}}
	logger := atlas.SymbolCall{Kind: "invokes_external", Name: "logging.getLogger", Line: 1560, Column: 10, API: &atlas.CallAPI{Package: "logging", Name: "getLogger"}}
	bind := &sourcevalue.Value{Kind: "field", Text: "bind", Parts: []sourcevalue.Value{*result("models.py", 86, 21)}}
	history := atlas.SymbolCall{Kind: "invokes_external", Name: "pandas.read_sql", Line: 794, Column: 19, API: &atlas.CallAPI{Package: "pandas", Name: "read_sql"},
		SourceArguments: []atlas.SourceArgument{positional(&sourcevalue.Value{Kind: "literal", Text: "wallet_history"}), {Keyword: "con", Origin: bind}}}
	settings := atlas.SymbolCall{Kind: "invokes_external", Name: "json.load", Line: 1500, Column: 12, API: &atlas.CallAPI{Package: "json", Name: "load"}}
	hook := atlas.SymbolCall{Kind: "invokes_external", Name: "requests.post", Line: 1501, Column: 5, API: &atlas.CallAPI{Package: "requests", Name: "post"},
		SourceArguments: []atlas.SourceArgument{positional(&sourcevalue.Value{Kind: "field", Text: "hook", Parts: []sourcevalue.Value{*result("trade.py", 1500, 12)}})}}
	count := symbol("count", "trade.py", 1570, "Trade.get_open_trade_count", selectRow, filter, execute, lonely, logger, logged, history, settings, hook)
	primary := outside("sqlalchemy.connect", "", "connect", 10, 12)
	primary.Values = []string{"primary"}
	replica := outside("sqlalchemy.connect", "", "connect", 11, 12)
	replica.Values = []string{"replica"}
	toPrimary := outside("sqlalchemy.text", "", "text", 12, 20)
	toReplica := outside("sqlalchemy.text", "", "text", 13, 20)
	pools := symbol("pools", "pools.py", 9, "copy_rows", primary, replica, toPrimary, toReplica,
		unresolved("execute", 12, 5, result("pools.py", 10, 12), positional(result("pools.py", 12, 20))),
		unresolved("execute", 13, 5, result("pools.py", 11, 12), positional(result("pools.py", 13, 20))))
	begin := unresolved("begin", 185, 17, &sourcevalue.Value{Kind: "parameter", Text: "engine", Position: 1, Owner: at("migrations.py", 412, 1)})
	text := outside("sqlalchemy.text", "", "text", 186, 28)
	connection := &sourcevalue.Value{Kind: "entered", Text: "connection", Parts: []sourcevalue.Value{*result("migrations.py", 185, 17)}}
	sent := unresolved("execute", 186, 20, connection, positional(result("migrations.py", 186, 28)))
	opaque := outside("sqlalchemy.text", "", "text", 190, 28)
	elsewhere := unresolved("execute", 190, 20, &sourcevalue.Value{Kind: "unknown", Text: "other"}, positional(result("migrations.py", 190, 28)))
	migrate := symbol("migrate", "migrations.py", 412, "check_migrate", begin, text, sent, opaque, elsewhere)
	choices := DestinationChoices{Arguments: map[string]ArgumentChoice{
		"sqlalchemy.create_engine": {Position: 1}, "sqlalchemy.select": {None: true}, "sqlalchemy.text": {None: true},
	}}
	choices.Arguments["pandas.read_sql"] = ArgumentChoice{Keyword: "con"}
	choices.Arguments["json.load"] = ArgumentChoice{Position: 1}
	choices.Arguments["requests.post"] = ArgumentChoice{Position: 1}
	reader := NewDestinationReader([]atlas.Place{initDB, bot, count, migrate, pools}, choices)
	reader.talks = map[string]string{"sqlalchemy.select": "db", "sqlalchemy.select.filter": "db", "sqlalchemy.text": "db", "pandas.read_sql": "db",
		"sqlalchemy.connect": "db", "json.load": "file", "requests.post": "client_request"}
	ends := func(uses []atlas.DestinationUse) []string {
		var result []string
		for _, use := range uses {
			result = append(result, destinationEnd(use))
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	want := ends(reader.Read(initDB, engine))
	if len(want) != 1 || want[0] != "value\x00sqlite:///trades.db\x00bot.py:101:5" {
		t.Fatalf("the engine's walk ends at %q", want)
	}
	if got := ends(reader.Exchange(count, selectRow, "db")); !slices.Equal(got, want) {
		t.Fatalf("a select sent through Trade.session ends at %q, want %q", got, want)
	}
	if got := ends(reader.Exchange(migrate, text, "db")); !slices.Equal(got, want) {
		t.Fatalf("a text sent through the connection an engine began ends at %q, want %q", got, want)
	}
	if got := reader.Exchange(count, lonely, "db"); got != nil {
		t.Fatalf("a statement handed to nothing but a logger reaches %+v", got)
	}
	if got := reader.Exchange(migrate, opaque, "db"); got != nil {
		t.Fatalf("a statement sent through an unknown object reaches %+v", got)
	}
	toFirst, toSecond := ends(reader.Exchange(pools, toPrimary, "db")), ends(reader.Exchange(pools, toReplica, "db"))
	if len(toFirst) != 1 || len(toSecond) != 1 || toFirst[0] == toSecond[0] {
		t.Fatalf("objects made by one call given other words end at %q and %q", toFirst, toSecond)
	}
	if got := ends(reader.Read(count, history)); len(got) != 1 || !strings.HasPrefix(got[0], "unresolved\x00") || !strings.Contains(got[0], ".bind") {
		t.Fatalf("a walk reads no object unless asked to: %q", got)
	}
	reader.objects = true
	if got := ends(reader.Read(count, history)); !slices.Equal(got, want) {
		t.Fatalf("read_sql on Trade.session.bind ends at %q, want %q", got, want)
	}
	if got := ends(reader.Read(count, hook)); len(got) != 1 || !strings.HasPrefix(got[0], "unresolved\x00") || !strings.Contains(got[0], ".hook") {
		t.Fatalf("a field of a configuration a file read made ends at %q", got)
	}
}

// One exchange, one destination: `inspector.get_columns("trades")` on
// what `inspect(engine)` returned continues inspect's exchange, so its
// destination ends where inspect's walk ends, while its own walk, its
// address and its chain, stays the table it names.
func TestACallOnAnExchangesResultEndsWhereTheExchangeEnds(t *testing.T) {
	at := &sourcevalue.Anchor{Path: "migrations.py", Line: 416, Column: 17}
	inspect := atlas.SymbolCall{Kind: "invokes_external", Name: "sqlalchemy.inspect", Line: 416, Column: 17, API: &atlas.CallAPI{Package: "sqlalchemy", Name: "inspect"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "sqlite:///trades.db"}}}}
	columns := atlas.SymbolCall{Kind: "invokes_external", Name: "sqlalchemy.inspect.get_columns", Line: 418, Column: 29, API: &atlas.CallAPI{Package: "sqlalchemy", Receiver: "inspect", Name: "get_columns"},
		ReceiverValue: &sourcevalue.Value{Kind: "call_result", Anchor: at}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "trades"}}}}
	place := atlas.Place{ID: "check", Kind: atlas.PlaceSymbol, Path: "migrations.py", LineNo: 412, TargetIDs: []string{"t1"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "check_migrate", Column: 1}, Calls: []atlas.SymbolCall{inspect, columns}}}
	walk := NewDestinationReader([]atlas.Place{place}, DestinationChoices{Arguments: map[string]ArgumentChoice{
		"sqlalchemy.inspect": {Position: 1}, "sqlalchemy.inspect.get_columns": {Position: 1}}})
	r := &reader{api: map[string]apiRole{"sqlalchemy.inspect": {talks: atlas.BoundaryDB}, "sqlalchemy.inspect.get_columns": {talks: atlas.BoundaryDB}}}
	own := walk.Read(place, columns)
	through := r.exchangeThrough(walk, place, columns, atlas.BoundaryDB)
	if len(own) != 1 || own[0].Address != "trades" {
		t.Fatalf("the call's own walk = %+v", own)
	}
	if len(through) != 1 || through[0].Address != "sqlite:///trades.db" || through[0].Steps[0].Line != 418 {
		t.Fatalf("the exchange ends at %+v", through)
	}
}
