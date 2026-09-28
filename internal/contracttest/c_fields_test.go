package contracttest

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// kvd's fields are read and written where its functions name them, and
// gathered by field whatever value each function reaches it from: the
// global server's shutdown flag has one writer, the signal handler, and one
// reader, beforeSleep, as Redis's server.masterhost is written by
// loadServerConfig and slaveofCommand and read by the replication cron; the
// snapshot file's name is written by main and loadConfig and read by
// bgsaveCommand; an entry's value is reached through a local pointer
// (kvEntry.value) and through the global's array (server.db.value).
func TestCFixtureReadsAndWritesRecordFields(t *testing.T) {
	fixture := loadCFixture(t)
	server := buildCIndex(t, fixture, "c:kvd")
	objects := map[string]programindex.Object{}
	for _, object := range server.Objects {
		objects[object.ID] = object
	}
	site := func(needle, within string) string {
		line, column := fixture.at(t, "kvd.c", needle, within)
		return strconv.Itoa(line) + ":" + strconv.Itoa(column)
	}
	accesses := map[string][]string{}
	for _, relation := range server.Relations {
		if relation.FieldPath == "" {
			continue
		}
		field := objects[relation.ToIDs[0]]
		key := objects[field.OwnerID].Name + "." + field.Name
		accesses[key] = append(accesses[key], string(relation.Kind)+" "+relation.FieldPath+" by "+objects[relation.FromID].Name+
			"@"+relation.Location.Path+":"+strconv.Itoa(relation.Location.Line)+":"+strconv.Itoa(relation.Location.Column))
	}
	for _, sites := range accesses {
		slices.Sort(sites)
	}
	want := map[string][]string{
		"kvServer.shutdown": {
			"reads server.shutdown by beforeSleep@kvd.c:" + site("if (server.shutdown) loopStop(l);", "shutdown"),
			"writes server.shutdown by onSignal@kvd.c:" + site("server.shutdown = 1;", "shutdown"),
		},
		"kvServer.dbfile": {
			"reads server.dbfile by bgsaveCommand@kvd.c:" + site("saveSnapshot(server.dbfile)", "dbfile"),
			"writes server.dbfile by loadConfig@kvd.c:" + site("server.dbfile = strdup(argv[1])", "dbfile"),
			"writes server.dbfile by main@kvd.c:" + site(`server.dbfile = "dump.kv";`, "dbfile"),
		},
		"kvEntry.value": {
			"reads kvEntry.value by delCommand@kvd.c:" + site("free(e->value);\n        *e", "value"),
			"reads kvEntry.value by getCommand@kvd.c:" + site("addReplyBulk(c, e->value)", "value"),
			"reads kvEntry.value by setCommand@kvd.c:" + site("free(e->value);\n    }", "value"),
			"reads server.db.value by saveSnapshot@kvd.c:" + site("server.db[j].value);", "value"),
			"writes kvEntry.value by setCommand@kvd.c:" + site("e->value = strdup(c->argv[2])", "value"),
		},
	}
	for field, sites := range want {
		if !reflect.DeepEqual(accesses[field], sites) {
			t.Errorf("%s:\n have %v\n want %v", field, accesses[field], sites)
		}
	}
	// A counter ++ and += write; the loop bound reads; an array member
	// indexed on the way to an element's field is passed through.
	for _, access := range []string{
		"writes server.dirty by setCommand@kvd.c:" + site("server.dirty++;", "dirty"),
		"writes server.dirty by delCommand@kvd.c:" + site("server.dirty += deleted;", "dirty"),
		"reads server.dbSize by dbFind@kvd.c:" + site("for (j = 0; j < server.dbSize; j++)\n        if (strcmp", "dbSize"),
	} {
		key := "kvServer." + strings.TrimPrefix(strings.Fields(access)[1], "server.")
		if !slices.Contains(accesses[key], access) {
			t.Errorf("%s has no %q: %v", key, access, accesses[key])
		}
	}
	for _, access := range accesses["kvServer.db"] {
		if strings.Contains(access, " by dbFind@") && !strings.Contains(access, site("return &server.db[j]", "db")) {
			t.Errorf("dbFind reads server.db only where it takes an element's address: %s", access)
		}
	}

	// Each access reaches GroupsIndex at its site with its path.
	categorized, err := programindex.Enrich(server, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := groupindex.Build(categorized, groupindex.Proposals{})
	if err != nil {
		t.Fatal(err)
	}
	pending := map[string]programindex.Relation{}
	for _, relation := range server.Relations {
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
		t.Fatalf("GroupsIndex lost %d of kvd's field accesses", len(pending))
	}

	// Places keeps them on the declaration, keyed by the record type's place.
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: server, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	record := cSymbolPlace(t, graph, "kvd.h", "kvServer")
	line, column := fixture.at(t, "kvd.c", "server.shutdown = 1;", "shutdown")
	var fields []string
	for _, field := range cSymbolPlace(t, graph, "kvd.c", "onSignal").Symbol.Fields {
		fields = append(fields, field.Kind+" "+field.Path+" "+field.Field+"@"+strconv.Itoa(field.LineNo)+":"+strconv.Itoa(field.Column)+" "+strconv.FormatBool(field.TypeID == record.ID))
	}
	if want := []string{"writes server.shutdown shutdown@" + strconv.Itoa(line) + ":" + strconv.Itoa(column) + " true"}; !reflect.DeepEqual(fields, want) {
		t.Fatalf("onSignal's fields %v, want %v", fields, want)
	}
}
