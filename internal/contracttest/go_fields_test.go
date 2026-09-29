package contracttest

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// goFixtureSite is where within first appears inside needle in a file of the
// Go fixture, as SSA and the corpus count lines and byte columns.
func goFixtureSite(t *testing.T, repositoryPath, path, needle, within string) (int, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryPath, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	offset := strings.Index(text, needle)
	inner := strings.Index(needle, within)
	if offset < 0 || inner < 0 {
		t.Fatalf("%s has no %q in %q", path, within, needle)
	}
	offset += inner
	return strings.Count(text[:offset], "\n") + 1, offset - strings.LastIndex(text[:offset], "\n")
}

// server_state.go's fields are read and written where its functions name
// them, and gathered by field whatever value each function reaches it from,
// as the C fixture's kvd: the package variable's shutdown flag has one
// writer, the signal handler, and one reader, stateBeforeSleep; the snapshot
// file's name is written by StartStateServer and loadStateConfig and read by
// saveStateSnapshot; an entry's value is reached through a local pointer
// (stateEntry.value) and through the package variable's array
// (serverState.db.value); a client's words through a parameter
// (stateClient.argv).
func assertGoFieldAccesses(t *testing.T, repositoryPath string, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const file = "internal/storefixture/server_state.go"
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	site := func(needle, within string) string {
		line, column := goFixtureSite(t, repositoryPath, file, needle, within)
		return strconv.Itoa(line) + ":" + strconv.Itoa(column)
	}
	accesses := map[string][]string{}
	for _, relation := range index.Relations {
		if relation.FieldPath == "" {
			continue
		}
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil ||
			len(relation.Witnesses) != 1 || relation.Witnesses[0].Detail != map[programindex.RelationKind]string{programindex.RelationReads: "read of ", programindex.RelationWrites: "write of "}[relation.Kind]+relation.FieldPath {
			t.Fatalf("field access is not one exact site with its path: %+v", relation)
		}
		field := objects[relation.ToIDs[0]]
		if relation.Location.Path != file {
			continue
		}
		key := objects[field.OwnerID].Name + "." + field.Name
		accesses[key] = append(accesses[key], string(relation.Kind)+" "+relation.FieldPath+" by "+objects[relation.FromID].Name+
			"@"+strconv.Itoa(relation.Location.Line)+":"+strconv.Itoa(relation.Location.Column))
	}
	for _, sites := range accesses {
		slices.Sort(sites)
	}
	want := map[string][]string{
		"stateServer.shutdown": {
			"reads serverState.shutdown by stateBeforeSleep@" + site("return serverState.shutdown", "shutdown"),
			"writes serverState.shutdown by onStateSignal@" + site("serverState.shutdown = true", "shutdown"),
		},
		"stateServer.dbfile": {
			"reads serverState.dbfile by saveStateSnapshot@" + site("out := serverState.dbfile", "dbfile"),
			"writes serverState.dbfile by StartStateServer@" + site(`serverState.dbfile = "dump.kv"`, "dbfile"),
			"writes serverState.dbfile by loadStateConfig@" + site("serverState.dbfile = words[1]", "dbfile"),
		},
		"stateEntry.value": {
			"reads serverState.db.value by saveStateSnapshot@" + site("serverState.db[j].value", "value"),
			"reads stateEntry.value by getState@" + site("return e.value", "value"),
			"writes stateEntry.value by setState@" + site("e.value = c.argv[2]", "value"),
		},
	}
	for field, sites := range want {
		if !reflect.DeepEqual(accesses[field], sites) {
			t.Errorf("%s:\n have %v\n want %v", field, accesses[field], sites)
		}
	}
	// ++ and += write; a nested struct is passed through to its field; a
	// loop bound and a parameter's slice read.
	for field, access := range map[string]string{
		"stateServer.dirty":  "writes serverState.dirty by setState@" + site("serverState.dirty++", "dirty"),
		"stateServer.dbSize": "writes serverState.dbSize by setState@" + site("serverState.dbSize++", "dbSize"),
		"stateStats.hits":    "writes serverState.stats.hits by setState@" + site("serverState.stats.hits += 1", "hits"),
		"stateClient.argv":   "reads stateClient.argv by setState@" + site("e := stateFind(c.argv[1])", "argv"),
		"stateEntry.key":     "reads serverState.db.key by stateFind@" + site("if serverState.db[j].key == key", "key"),
	} {
		if !slices.Contains(accesses[field], access) {
			t.Errorf("%s has no %q: %v", field, access, accesses[field])
		}
	}
	if len(accesses["stateServer.stats"]) != 0 {
		t.Errorf("a struct field passed through to its own field is no access: %v", accesses["stateServer.stats"])
	}
	for _, access := range accesses["stateServer.db"] {
		if strings.Contains(access, " by stateFind@") && !strings.HasSuffix(access, "@"+site("return &serverState.db[j]", "db")) {
			t.Errorf("stateFind reads serverState.db only where it takes an entry's address: %s", access)
		}
	}
	// A parameter's field is read by the parameter's type (tool_cli.go).
	found := false
	for _, relation := range index.Relations {
		if relation.FieldPath == "toolCommand.name" && relation.Kind == programindex.RelationReads && objects[relation.FromID].Name == "Run" {
			found = true
		}
	}
	if !found {
		t.Error("toolCommand.Run does not read toolCommand.name")
	}

	// Each access reaches GroupsIndex at its site with its path.
	categorized, err := programindex.Enrich(index, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := groupindex.Build(categorized, groupindex.Proposals{})
	if err != nil {
		t.Fatal(err)
	}
	pending := map[string]programindex.Relation{}
	for _, relation := range index.Relations {
		if relation.FieldPath != "" {
			pending[relation.ID] = relation
		}
	}
	for _, edge := range groups.StructuralEdges {
		if original, ok := pending[edge.RelationID]; ok && edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == original.Kind &&
			edge.FieldPath == original.FieldPath && *edge.Location == *original.Location {
			delete(pending, edge.RelationID)
		}
	}
	if len(pending) != 0 {
		t.Fatalf("GroupsIndex lost %d of the Go fixture's field accesses", len(pending))
	}

	// Places keeps them on the declaration, keyed by the struct type's place.
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	record := cSymbolPlace(t, graph, file, "stateServer")
	line, column := goFixtureSite(t, repositoryPath, file, "serverState.shutdown = true", "shutdown")
	var fields []string
	for _, field := range cSymbolPlace(t, graph, file, "onStateSignal").Symbol.Fields {
		fields = append(fields, field.Kind+" "+field.Path+" "+field.Field+"@"+strconv.Itoa(field.LineNo)+":"+strconv.Itoa(field.Column)+" "+strconv.FormatBool(field.TypeID == record.ID))
	}
	if want := []string{"writes serverState.shutdown shutdown@" + strconv.Itoa(line) + ":" + strconv.Itoa(column) + " true"}; !reflect.DeepEqual(fields, want) {
		t.Fatalf("onStateSignal's fields %v, want %v", fields, want)
	}
}

// A callable stored into the field of an outside value the code holds is
// handed to that field, named with its declared type as a C table row names
// its record's field: fs.Usage = c.Usage hands the usage printer to
// flag.FlagSet.Usage (func()), not to flag.FlagSet, and srv.Handler = mux the
// status handler to net/http.Server.Handler. A composite literal still
// constructs its type (assertGoConstructRegistrations).
func assertGoOutsideFieldStores(t *testing.T, repositoryPath string, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const file, server = "internal/storefixture/tool_cli.go", "internal/storefixture/server_state.go"
	signatures := map[string]string{}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol {
			signatures[object.Name] = object.Signature
		}
	}
	if signatures["flag.FlagSet.Usage"] != "func()" || signatures["net/http.Server.Handler"] != "http.Handler" {
		t.Fatalf("outside fields declared %q and %q", signatures["flag.FlagSet.Usage"], signatures["net/http.Server.Handler"])
	}
	result, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var registrations []string
	for _, fact := range result.OfKind(facts.KindRegistration) {
		if fact.Anchor == nil || !strings.HasPrefix(fact.Text, "flag.FlagSet") && !strings.HasPrefix(fact.Text, "net/http.Server") {
			continue
		}
		registrations = append(registrations, fact.Text+" "+fact.Key+" "+fact.Symbol+"@"+fact.Anchor.Path+":"+strconv.Itoa(fact.Anchor.Line)+":"+strconv.Itoa(fact.Anchor.Column))
	}
	slices.Sort(registrations)
	usageLine, usageColumn := goFixtureSite(t, repositoryPath, file, "fs.Usage = c.Usage", "Usage")
	handlerLine, handlerColumn := goFixtureSite(t, repositoryPath, server, "srv.Handler = mux", "Handler")
	want := []string{
		"flag.FlagSet.Usage Usage Usage@" + file + ":" + strconv.Itoa(usageLine) + ":" + strconv.Itoa(usageColumn),
		"net/http.Server.Handler Handler stateStatus@" + server + ":" + strconv.Itoa(handlerLine) + ":" + strconv.Itoa(handlerColumn),
	}
	if !reflect.DeepEqual(registrations, want) {
		t.Fatalf("stores into flag.FlagSet and net/http.Server\n have %v\n want %v", registrations, want)
	}

	// The reading's outside symbol is the field, with its declared type.
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: result})
	if err != nil {
		t.Fatal(err)
	}
	var apis []string
	for _, call := range cSymbolPlace(t, graph, file, "toolCommand.Run").Symbol.Calls {
		if call.API != nil && call.API.Receiver == "FlagSet" {
			apis = append(apis, call.API.Package+"."+call.API.Receiver+"."+call.API.Name+" "+call.API.Signature)
		}
	}
	if !slices.Contains(apis, "flag.FlagSet.Usage func()") {
		t.Fatalf("Run's outside calls %v name no flag.FlagSet.Usage func()", apis)
	}
}
