package groupindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Redis's anet.c shape: the connect is written in a generic helper that
// two thin wrappers call, and the programs reach it from their own parts.
// Its reader sees syncWithMaster and cliConnect, never only the wrapper
// one hop inside anet.c; a wrapper nothing calls, a test and code the
// program never runs name nobody, a seed inside the part is a way in, and
// a cycle inside the part ends.
func TestOutboundCallIsReachedFromTheFirstCallersOutsideItsPart(t *testing.T) {
	type object struct {
		ref, name, path string
		line            int
		unreachable     bool
	}
	objects := []object{
		{"generic", "anetGenericConnect", "anet.c", 10, false},
		{"connect", "anetConnect", "anet.c", 30, false},
		{"nonblock", "anetNonBlockConnect", "anet.c", 40, false},
		{"retry", "anetRetry", "anet.c", 50, false},
		{"netmain", "anetMain", "anet.c", 60, false},
		{"sync", "syncWithMaster", "replication.c", 5, false},
		{"cli", "cliConnect", "cli.c", 5, false},
		{"stale", "staleConnect", "cli.c", 20, true},
		{"test", "testConnect", "anet_test.c", 5, false},
	}
	calls := [][2]string{
		{"connect", "generic"}, {"nonblock", "generic"}, {"retry", "connect"}, {"connect", "retry"},
		{"netmain", "connect"}, {"sync", "connect"}, {"cli", "connect"}, {"cli", "connect"},
		{"stale", "connect"}, {"test", "connect"},
	}
	var inputs []programindex.ObjectInput
	for _, item := range objects {
		inputs = append(inputs, programindex.ObjectInput{SourceRef: item.ref, Kind: programindex.ObjectFunction, Name: item.name, Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: item.path, Line: item.line, Column: 1}, EndLine: item.line + 8, Unreachable: item.unreachable})
	}
	lineOf := map[string]object{}
	for _, item := range objects {
		lineOf[item.ref] = item
	}
	var relations []programindex.RelationInput
	for position, call := range calls {
		from := lineOf[call[0]]
		site := &programindex.Location{Path: from.path, Line: from.line + 1 + position%2, Column: 3}
		relations = append(relations, programindex.RelationInput{SourceRef: "r" + string(rune('a'+position)), Kind: programindex.RelationCalls, FromRef: call[0], ToRefs: []string{call[1]},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: site,
			Witnesses: []programindex.Witness{{Kind: "call", Location: site}}, WitnessesObserved: 1})
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "c", Kind: "executable", Name: "server", Selector: "c:server", TestSources: []string{"anet_test.c"},
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "anet.c"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "netmain", Kind: programindex.SeedCallable, Location: &programindex.Location{Path: "anet.c", Line: 60, Column: 1}}},
		},
		Objects: inputs, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(inputs), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := map[string]string{}
	for _, object := range program.Objects {
		id[object.Name] = object.ID
	}
	members := func(names ...string) []string {
		var ids []string
		for _, name := range names {
			ids = append(ids, id[name])
		}
		return ids
	}
	file := func(path string) []atlas.File { return []atlas.File{{Path: path, Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}} }
	target := atlas.Target{ID: program.Target.ID, Name: program.Target.Name, Language: "c", Kind: "executable", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{
			{ID: "net", Dir: ".", Title: "Networking", Side: atlas.SideOut, MemberIDs: members("anetGenericConnect", "anetConnect", "anetNonBlockConnect", "anetRetry", "anetMain"), Files: file("anet.c")},
			{ID: "replication", Dir: ".", Title: "Replication", Side: atlas.SideMid, MemberIDs: members("syncWithMaster"), Files: file("replication.c")},
			{ID: "cli", Dir: ".", Title: "Command line", Side: atlas.SideIn, MemberIDs: members("cliConnect", "staleConnect"), Files: file("cli.c")},
			{ID: "tests", Dir: ".", Title: "Tests", Side: atlas.SideMid, ForTests: true, MemberIDs: members("testConnect"), Files: file("anet_test.c")},
		},
		Boundaries: []atlas.Boundary{{ID: "b1", ObjectID: id["anetGenericConnect"], BoxID: "net", Direction: atlas.DirectionOut, Kind: atlas.BoundaryClientRequest,
			Destination: "TCP endpoint", External: "socket.h.connect", Source: "model", Path: "anet.c", LineNo: 14, Column: 9, Values: []string{}, Line: "Connects to a TCP endpoint."}},
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "redis", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	if len(index.Outbound) != 1 {
		t.Fatalf("outbound = %+v", index.Outbound)
	}
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	titles := map[string]string{}
	for _, group := range index.Groups {
		titles[group.ID] = group.Title
	}
	var got []string
	for _, caller := range index.Outbound[0].ReachedFrom {
		site := "no site"
		if caller.Location != nil {
			site = caller.Location.Path
		}
		got = append(got, titles[caller.GroupID]+": "+names[caller.SubjectID]+" at "+site)
	}
	slices.Sort(got)
	// cliConnect calls anetConnect from two places: two sites, one caller.
	want := []string{"Command line: cliConnect at cli.c", "Command line: cliConnect at cli.c", "Networking: anetMain at no site", "Replication: syncWithMaster at replication.c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the connect is reached from %v, want %v", got, want)
	}
	raw, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decode(raw, program)
	if err != nil || !reflect.DeepEqual(restored.Outbound, index.Outbound) {
		t.Fatalf("the saved callers changed: %+v, %v", restored.Outbound, err)
	}
	copied := index.Snapshot()
	sited := slices.IndexFunc(index.Outbound[0].ReachedFrom, func(caller OutboundCaller) bool { return caller.Location != nil })
	copied.Outbound[0].ReachedFrom[sited].Location.Line = 999
	if index.Outbound[0].ReachedFrom[sited].Location.Line == 999 {
		t.Fatal("a snapshot shares the callers' sites")
	}
	broken := index.Snapshot()
	broken.Outbound[0].ReachedFrom = slices.Clone(broken.Outbound[0].ReachedFrom)
	slices.Reverse(broken.Outbound[0].ReachedFrom)
	if err := broken.validateOutbound(subjectsOf(broken), groupsOf(broken)); err == nil {
		t.Fatal("callers out of canonical order were accepted")
	}
}

func subjectsOf(index Index) map[string]Subject {
	result := map[string]Subject{}
	for _, subject := range index.Subjects {
		result[subject.ID] = subject
	}
	return result
}

func groupsOf(index Index) map[string]struct{} {
	result := map[string]struct{}{}
	for _, group := range index.Groups {
		result[group.ID] = struct{}{}
	}
	return result
}
