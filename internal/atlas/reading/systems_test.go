package reading

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A package's calls may choose different systems (control review B7:
// casdoor's three xorm.NewEngine and two sql.Open calls, given different
// drivers, reached atlas_systems as one call of each). The question sees
// every call of each symbol given different values, calls given the same
// once (sql.Open(name, url) knows no more than sql.Open(driver, dsn)); an
// answer
// naming several systems lists the package under each, and a call through
// it is never named after the package's first system: the destination
// question names each call by what it is given.
func TestAPackageWhoseCallsChooseSeveralSystemsIsNamedByEach(t *testing.T) {
	const xorm = "github.com/xorm-io/xorm"
	call := func(pkg, name string, line, column int, values ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: column, API: &atlas.CallAPI{Package: pkg, Name: name}, Values: values}
	}
	symbol := func(id, path string, line int, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: "symbol:" + id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, Parent: "file:" + path, TargetIDs: []string{"server"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + id, Name: id}, Calls: calls}}
	}
	// A data source a call of the repository's supplies is evidence of its
	// own, as a driver literal is.
	refined := call("database/sql", "Open", 24, 17)
	refined.SourceArguments = []atlas.SourceArgument{{Position: 2, Origin: &sourcevalue.Value{Kind: "call_result", Text: "refineForPostgres", Anchor: &sourcevalue.Anchor{Path: "store/ping.go", Line: 24, Column: 37}}}}
	graph := []atlas.Place{
		symbol("OpenMain", "store/main.go", 4, call(xorm, "NewEngine", 5, 17, "mysql")),
		symbol("OpenReports", "store/reports.go", 8, call(xorm, "NewEngine", 9, 17, "postgres")),
		// Given what OpenMain's is given: the same evidence, sent once.
		symbol("OpenCopy", "store/copy.go", 12, call(xorm, "NewEngine", 13, 17, "mysql")),
		symbol("Ping", "store/ping.go", 20, call("database/sql", "Open", 21, 13, "mysql"), call("database/sql", "Open", 22, 16), call("database/sql", "Open", 23, 16), refined),
	}
	sources := map[string]string{
		"store/main.go":    strings.Repeat("\n", 4) + `	engine, err := xorm.NewEngine("mysql", dsn)` + "\n",
		"store/reports.go": strings.Repeat("\n", 8) + `	engine, err := xorm.NewEngine("postgres", dsn)` + "\n",
		"store/copy.go":    strings.Repeat("\n", 12) + `	engine, err := xorm.NewEngine("mysql", dsn)` + "\n",
		"store/ping.go":    strings.Repeat("\n", 20) + `	db, err := sql.Open("mysql", dsn)` + "\n" + `	other, err := sql.Open(driver, dsn)` + "\n" + `	third, err := sql.Open(name, url)` + "\n" + `	fourth, err := sql.Open(driver, refineForPostgres(dsn))` + "\n",
	}
	var mu sync.Mutex
	sent := map[string]string{}
	destinations := map[string]string{}
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == lines.StageSystems {
			for i, source := range input["rows"].([]any) {
				row := source.(map[string]any)
				pkg := row["package"].(string)
				sent[pkg] = string(mustJSON(row["calls"]))
				rows[i]["system"] = map[string]string{xorm: "MySQL; PostgreSQL", "database/sql": "none"}[pkg]
			}
			return
		}
		named := false
		for _, column := range input["fill"].([]any) {
			named = named || column.(map[string]any)["name"] == "destination"
		}
		for i, source := range input["rows"].([]any) {
			if !named {
				rows[i]["line"], rows[i]["address"] = "sends", "unknown"
				continue
			}
			text := string(mustJSON(source))
			system := "MySQL"
			if strings.Contains(text, "postgres") {
				system = "PostgreSQL"
			}
			destinations[text] = system
			rows[i]["destination"] = destinationRef(input, system)
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{{ID: "server", Dependencies: []Dependency{{Package: xorm, Module: xorm, Version: "v1.1.6"}}}}
	r.opts.ReadSource = func(path string) ([]byte, error) { return []byte(sources[path]), nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{xorm + ".NewEngine": {talks: atlas.BoundaryDB}, "database/sql.Open": {talks: atlas.BoundaryDB}}
	r.arguments = map[string]ArgumentChoice{xorm + ".NewEngine": {Position: 2}, "database/sql.Open": {Position: 2}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := `[{"calls":["xorm.NewEngine(\"mysql\", dsn)","xorm.NewEngine(\"postgres\", dsn)"],"symbol":"NewEngine"}]`; sent[xorm] != want {
		t.Fatalf("xorm's calls were sent as %s, want %s", sent[xorm], want)
	}
	if want := `[{"calls":["sql.Open(\"mysql\", dsn)","sql.Open(driver, dsn)","sql.Open(driver, refineForPostgres(dsn))"],"symbol":"Open"}]`; sent["database/sql"] != want {
		t.Fatalf("database/sql's calls were sent as %s, want %s", sent["database/sql"], want)
	}
	got := map[string]string{}
	for _, state := range r.boundaries {
		got[state.place.Path] = state.destinationOf("server")
	}
	want := map[string]string{"store/main.go": "MySQL", "store/reports.go": "PostgreSQL", "store/copy.go": "MySQL"}
	for path, system := range want {
		if got[path] != system {
			t.Fatalf("destinations = %v, want %v (asked %v)", got, want, destinations)
		}
	}
	if len(destinations) == 0 {
		t.Fatal("a package of several systems named its calls in code, unasked")
	}
}

func TestSystemNamesReadsEveryNameOnce(t *testing.T) {
	for cell, want := range map[string][]string{
		"MySQL":                         {"MySQL"},
		"MySQL; PostgreSQL":             {"MySQL", "PostgreSQL"},
		"MySQL;postgreSQL; none; mysql": {"MySQL", "postgreSQL"},
		"none":                          nil,
		"None.":                         nil,
		" ; ":                           nil,
	} {
		if got := lines.SystemNames(cell); !slices.Equal(got, want) {
			t.Errorf("SystemNames(%q) = %q, want %q", cell, got, want)
		}
	}
}
