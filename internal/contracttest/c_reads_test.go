package contracttest

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// cReads lists a program's reads as reader -> variable@file:line, one entry
// per read site, sorted; every read is exact and names a file-scope
// variable.
func cReads(t *testing.T, index programindex.Index) map[string][]string {
	t.Helper()
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	reads := map[string][]string{}
	for _, relation := range index.Relations {
		// A field's reads name the field (c_fields_test.go).
		if relation.Kind != programindex.RelationReads || relation.FieldPath != "" {
			continue
		}
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil {
			t.Fatalf("read without its variable or site: %+v", relation)
		}
		variable := objects[relation.ToIDs[0]]
		if variable.Kind != programindex.ObjectVariable || objects[variable.ContainerID].Kind != programindex.ObjectModule {
			t.Fatalf("a read names no file-scope variable: %+v", variable)
		}
		reader := objects[relation.FromID].Name
		reads[reader] = append(reads[reader], variable.Name+"@"+variable.Location.Path+":"+strconv.Itoa(relation.Location.Line))
	}
	for _, sites := range reads {
		slices.Sort(sites)
	}
	return reads
}

// A function reads each file-scope variable or table it names, as Redis's
// findFuncName reads staticsymbols.h's symsTable: kvd's printSymbols reads
// staticsyms.h's, and each program's lookupCommand its own command table.
// The variable itself as the destination of = is written, not read.
func TestCFixtureReadsFileScopeVariables(t *testing.T) {
	fixture := loadCFixture(t)
	site := func(path, needle, within string) string {
		line, _ := fixture.at(t, path, needle, within)
		return strconv.Itoa(line)
	}
	server, client, dump := buildCIndex(t, fixture, "c:kvd"), buildCIndex(t, fixture, "c:kvcli"), buildCIndex(t, fixture, "c:tools/dump.c")
	symbols := "symsTable@staticsyms.h:"
	lookup := func(path string) []string {
		first, second := site(path, "cmdTable[j].name != NULL", ""), site(path, "strcasecmp(name, cmdTable[j].name)", "")
		return []string{"cmdTable@" + path + ":" + first, "cmdTable@" + path + ":" + second, "cmdTable@" + path + ":" + second}
	}
	serverReads := cReads(t, server)
	for reader, want := range map[string][]string{
		"printSymbols":  {symbols + site("kvd.c", "symsTable[j].name != NULL", ""), symbols + site("kvd.c", `printf("%s %#lx\n", symsTable`, ""), symbols + site("kvd.c", `printf("%s %#lx\n", symsTable`, "")},
		"lookupCommand": lookup("kvd.c"),
		// server.shutdown = 1 writes a member of server, reached through it.
		"onSignal": {"server@kvd.c:" + site("kvd.c", "server.shutdown = 1;", "")},
	} {
		if !reflect.DeepEqual(serverReads[reader], want) {
			t.Errorf("kvd's %s reads %v, want %v", reader, serverReads[reader], want)
		}
	}
	if clientReads := cReads(t, client); !reflect.DeepEqual(clientReads["lookupCommand"], lookup("kvcli.c")) {
		t.Errorf("kvcli's lookupCommand reads %v", clientReads["lookupCommand"])
	}
	// progname = argv[0] writes progname; the error message reads it.
	if dumpReads := cReads(t, dump); !reflect.DeepEqual(dumpReads["main"], []string{"progname@tools/dump.c:" + site("tools/dump.c", `fprintf(stderr, "%s: ", progname);`, "")}) {
		t.Errorf("the dump tool's main reads %v", dumpReads["main"])
	}

	// Each read reaches GroupsIndex as a structural edge at its own site.
	categorized, err := programindex.Enrich(server, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := groupindex.Build(categorized, groupindex.Proposals{})
	if err != nil {
		t.Fatal(err)
	}
	reads := map[string]programindex.Relation{}
	for _, relation := range server.Relations {
		if relation.Kind == programindex.RelationReads {
			reads[relation.ID] = relation
		}
	}
	for _, edge := range groups.StructuralEdges {
		if original, ok := reads[edge.RelationID]; ok && edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == programindex.RelationReads &&
			edge.Location != nil && *edge.Location == *original.Location {
			delete(reads, edge.RelationID)
		}
	}
	if len(reads) != 0 {
		t.Fatalf("GroupsIndex lost %d of kvd's reads", len(reads))
	}
}
