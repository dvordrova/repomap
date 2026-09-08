package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestQuestionMapLinksKeepExactMembershipAndOverlappingOwners(t *testing.T) {
	location := programindex.Location{Path: "store.go", Line: 20, Column: 1}
	library := groupindex.Index{Target: programindex.Target{ID: "library"}, Groups: []groupindex.Group{
		{ID: "storage", Title: "Storage", MemberSubjectIDs: []string{"write"}},
		{ID: "transactions", Title: "Transactions", MemberSubjectIDs: []string{"write"}},
		{ID: "same-name", Title: "Storage", MemberSubjectIDs: []string{"another"}},
	}}
	executable := groupindex.Index{Target: programindex.Target{ID: "application"}, Operations: []groupindex.Operation{{ID: "save", SubjectID: "write", Name: "save", Location: location}}}
	b := pageBuilder{indexes: []groupindex.Index{library, executable}, byProgram: map[string]*pageSection{
		"library":     {ID: "lib", ShortLabel: "store (library)", Map: &pageMap{}},
		"application": {ID: "app", ShortLabel: "store (executable)", Map: &pageMap{}},
	}}
	stop := atlas.QuestionStop{SubjectID: "write", Path: location.Path, Line: location.Line, Column: location.Column}
	got := b.questionStepMapLinks(stop)
	want := []pageQuestionMapLink{
		{Label: "store (library) / Storage", Href: "#" + groupAnchorID("lib", "storage"), NodeID: mapNodeID("storage")},
		{Label: "store (library) / Transactions", Href: "#" + groupAnchorID("lib", "transactions"), NodeID: mapNodeID("transactions")},
		{Label: "store (executable) / save", Href: "#" + operationNodeID("app", "save"), NodeID: operationNodeID("app", "save")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %+v, want %+v", got, want)
	}
	stop.SubjectID = "unknown"
	if got := b.questionStepMapLinks(stop); len(got) != 0 {
		t.Fatalf("same source position invented membership: %+v", got)
	}
	stop.SubjectID = ""
	if got := b.questionStepMapLinks(stop); len(got) != 0 {
		t.Fatalf("source-only stop invented membership: %+v", got)
	}
	stop.SubjectID = "save" // An observed boundary is also the operation ID.
	if got := b.questionStepMapLinks(stop); !reflect.DeepEqual(got, want[2:]) {
		t.Fatalf("boundary lost its existing operation: %+v", got)
	}
	stop.SubjectID, stop.Kind = "exec-boundary", "boundary"
	b.indexes[0].Subjects = []groupindex.Subject{{ID: "write", Object: &groupindex.ObjectFacts{Location: &location}}}
	wantFiles := []pageQuestionMapLink{
		{Label: "store (library) / file in Storage", Href: want[0].Href, NodeID: want[0].NodeID},
		{Label: "store (library) / file in Transactions", Href: want[1].Href, NodeID: want[1].NodeID},
	}
	if got := b.questionStepMapLinks(stop); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("boundary file ownership lost or presented as exact execution: %+v", got)
	}
}

func TestMapConceptsUseExactTypeInterpretations(t *testing.T) {
	b := pageBuilder{subjects: map[string]subjectRef{}}
	for _, name := range []string{"Ticket", "Receipt"} {
		b.subjects[name] = subjectRef{subject: groupindex.Subject{Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectType, Location: &programindex.Location{Path: "queue/types.go", Line: 4}}, Interpretation: &groupindex.Interpretation{Key: true, Line: "Exact retained explanation for " + name}}}
	}
	b.subjects["helper"] = subjectRef{subject: groupindex.Subject{Object: &groupindex.ObjectFacts{Name: "Ticket", Kind: programindex.ObjectFunction}, Interpretation: &groupindex.Interpretation{Key: true, Line: "This is a function."}}}
	group := groupindex.Group{Title: "Ticket management", MemberSubjectIDs: []string{"Ticket", "helper", "Receipt", "unknown"}}
	var got []pageMapConcept
	if err := json.Unmarshal([]byte(b.groupConcepts(group)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "Receipt" || got[0].Name != "Ticket" || got[0].Explanation != "Exact retained explanation for Ticket" || got[0].Source.Path != "queue/types.go" || got[0].Source.Line != 4 {
		t.Fatalf("concept membership or source changed: %+v", got)
	}
	if b.groupConcepts(groupindex.Group{MemberSubjectIDs: []string{"helper"}}) != "" {
		t.Fatal("a familiar function name became a type definition")
	}
}

func TestConceptColumnsKeepSameLineDeclarationsAndCrossTargetMembership(t *testing.T) {
	b := pageBuilder{data: &ReportData{}, subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"types.ts": "source-id"}}}
	for id, column := range map[string]int{"application-left": 7, "application-right": 41, "library-left": 7} {
		b.subjects[id] = subjectRef{subject: groupindex.Subject{ID: id,
			Object:         &groupindex.ObjectFacts{Name: "Value", Kind: programindex.ObjectType, Location: &programindex.Location{Path: "types.ts", Line: 4, Column: column}},
			Interpretation: &groupindex.Interpretation{Key: true, Line: "A value used by the program."}}}
	}
	application := b.groupConcepts(groupindex.Group{MemberSubjectIDs: []string{"application-left", "application-right"}})
	library := b.groupConcepts(groupindex.Group{MemberSubjectIDs: []string{"library-left"}})
	var a, shared []pageMapConcept
	if err := json.Unmarshal([]byte(application), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(library), &shared); err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || len(shared) != 1 || a[0].Column != 7 || a[1].Column != 41 || a[0].Kind != "type" ||
		a[0].Source.Text != "types.ts:4:7" || a[1].Source.Open != "types.ts:4:41" || !reflect.DeepEqual(a[0], shared[0]) {
		t.Fatalf("map concept lost exact native position or acquired target identity: %+v / %+v", a, shared)
	}
	selected, err := b.questionStep(atlas.QuestionRoute{Stops: []atlas.QuestionStop{{Path: "types.ts", Line: 4, Column: 41, Name: "Value"}}},
		atlas.QuestionStep{Path: "types.ts", Line: 4, Column: 41, StopIndexes: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	view := &pageView{Sections: []*pageSection{
		{ID: "application", ShortLabel: "App", Kind: "application", Map: &pageMap{Nodes: []pageMapNode{{ID: "app-values", FullTitle: "Values", Concepts: application}}}},
		{ID: "library", ShortLabel: "Library", Kind: "library", Map: &pageMap{Nodes: []pageMapNode{{ID: "lib-values", FullTitle: "Values", Concepts: library}}}},
	}, Questions: []*pageQuestion{{ID: "q-one", Question: "What does Value mean?", Answers: []pageAnswerPart{{Checks: []pageQuestionStep{selected}}}}}}
	b.learn(view)
	if len(view.LearnConcepts) != 2 || len(view.Glossary) != 2 || view.LearnConcepts[0].ID == view.LearnConcepts[1].ID {
		t.Fatalf("same name, line and explanation collapsed two declarations: %+v", view.LearnConcepts)
	}
	for _, concept := range view.LearnConcepts {
		switch concept.Column {
		case 7:
			if len(concept.Places) != 2 || concept.Places[0].Href != "#app-values" || concept.Places[1].Href != "#lib-values" || concept.Context != "types.ts:4:7" {
				t.Fatalf("one native declaration was split by its target owners: %+v", concept)
			}
		case 41:
			if len(concept.Places) != 1 || concept.Context != "types.ts:4:41" {
				t.Fatalf("a neighbouring declaration gained the library membership: %+v", concept)
			}
		default:
			t.Fatal("unknown source column")
		}
	}
	terms := view.Questions[0].Answers[0].Terms
	if selected.Column != 41 || len(terms) != 1 || terms[0].Column != 41 {
		t.Fatalf("the answer's exact source selected its same-line neighbour: %+v", terms)
	}
}

func TestOperationMapFollowsInvocationsButNotImportsOrSuppliedCallables(t *testing.T) {
	section := &pageSection{ID: "client", ShortLabel: "Client"}
	server := &pageSection{ID: "server", ShortLabel: "Server"}
	location := programindex.Location{Path: "server/handle.go", Line: 3, Column: 1}
	index := groupindex.Index{Target: programindex.Target{ID: "client"}, Groups: []groupindex.Group{
		{ID: "entry", Title: "Entry", MemberSubjectIDs: []string{"start"}},
		{ID: "worker", Title: "Worker", MemberSubjectIDs: []string{"work"}},
		{ID: "unrelated", Title: "Import only", MemberSubjectIDs: []string{"imported"}},
		{ID: "callback", Title: "Supplied only", MemberSubjectIDs: []string{"supplied"}},
		{ID: "behind-callback", Title: "Work behind supplied callable", MemberSubjectIDs: []string{"later"}},
	}, Operations: []groupindex.Operation{{ID: "restore", SubjectID: "start", GroupID: "entry", Name: "restore", Kind: "command", Source: "model", Location: programindex.Location{Path: "client/run.go", Line: 4, Column: 1}}}, StructuralEdges: []groupindex.StructuralEdge{
		{FromSubjectID: "start", ToSubjectID: "work", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "work", ToSubjectID: "start", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "start", ToSubjectID: "imported", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationImports, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "work", ToSubjectID: "supplied", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationPassesCallback, Resolution: programindex.ResolutionExact},
		{FromSubjectID: "supplied", ToSubjectID: "later", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact},
	}, Connections: []groupindex.Connection{{ID: "match", From: groupindex.Endpoint{TargetID: "client", GroupID: "worker"}, To: groupindex.Endpoint{TargetID: "server", GroupID: "handler"}, FromSubjectID: "work", SourceKind: "integration", Label: "sends restore request", ToLocation: &location}}}
	peer := groupindex.Index{Target: programindex.Target{ID: "server"}, Operations: []groupindex.Operation{{ID: "handle", GroupID: "handler", Name: "restore request", Location: location}}}
	builder := pageBuilder{indexes: []groupindex.Index{index, peer}, byProgram: map[string]*pageSection{"client": section, "server": server}}
	builder.subjects = make(map[string]subjectRef)
	for n, name := range []string{"start", "work", "imported", "supplied", "later"} {
		builder.subjects[name] = subjectRef{subject: groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name, Location: &programindex.Location{Path: "client/run.go", Line: n + 1, Column: 1}}}}
	}
	got := builder.buildOperationMap(section, &index)
	var witnesses map[string][]struct {
		Name     string `json:"name"`
		Possible bool   `json:"possible"`
	}
	if err := json.Unmarshal([]byte(got.Nodes[0].CallPaths), &witnesses); err != nil {
		t.Fatal(err)
	}
	workerPath := witnesses[mapNodeID("worker")]
	if len(workerPath) != 2 || workerPath[0].Name != "start" || workerPath[1].Name != "work" || workerPath[1].Possible {
		t.Fatalf("lost call witness: %+v", workerPath)
	}
	for _, group := range []string{"unrelated", "callback", "behind-callback"} {
		if strings.Contains(got.Nodes[0].Neighbours, mapNodeID(group)) {
			t.Fatal("an import or a supplied callable became an execution path")
		}
	}
	matched := false
	for _, node := range got.Nodes {
		if node.Href == "#"+operationNodeID("server", "handle") {
			matched = true
		}
	}
	if !matched {
		t.Fatal("matched destination does not open its operation")
	}
	for _, edge := range got.Edges {
		if edge.Operations != got.Nodes[0].ID {
			t.Fatalf("lost operation ownership: %+v", edge)
		}
	}
	if len(got.Edges) != 4 {
		t.Fatalf("want attachment, cycle of calls and integration, got %+v", got.Edges)
	}
	if again := builder.buildOperationMap(section, &index); !reflect.DeepEqual(got, again) {
		t.Fatal("map changes between identical renders")
	}
	// The same remote endpoint can appear on several maps on one HTML page.
	// Its DOM identity belongs to that view, while its destination stays fixed.
	second := *section
	second.ID = "another-view"
	other := builder.buildOperationMap(&second, &index)
	for _, first := range got.Nodes {
		if first.Href != "#"+operationNodeID("server", "handle") {
			continue
		}
		for _, next := range other.Nodes {
			if next.Href == first.Href && next.ID == first.ID {
				t.Fatal("remote nodes share a DOM ID across maps")
			}
		}
	}
	if got.MarkerID == other.MarkerID {
		t.Fatal("maps share an arrow marker ID")
	}
	index.StructuralEdges = append(index.StructuralEdges, groupindex.StructuralEdge{FromSubjectID: "work", ToSubjectID: "supplied", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives})
	invoked := builder.buildOperationMap(section, &index)
	if err := json.Unmarshal([]byte(invoked.Nodes[0].CallPaths), &witnesses); err != nil {
		t.Fatal(err)
	}
	callbackPath := witnesses[mapNodeID("behind-callback")]
	if len(callbackPath) != 4 || !callbackPath[2].Possible {
		t.Fatalf("possible dispatch lost from witness: %+v", callbackPath)
	}
	if !strings.Contains(invoked.Nodes[0].Neighbours, mapNodeID("behind-callback")) {
		t.Fatal("a real invocation of the supplied callable lost its downstream path")
	}
}

func TestOperationMapUsesOneDestinationForOverlappingComponentViews(t *testing.T) {
	location := programindex.Location{Path: "server/handler.go", Line: 10, Column: 6}
	client := groupindex.Index{Target: programindex.Target{ID: "client"}, Groups: []groupindex.Group{{ID: "entry", Title: "Entry", MemberSubjectIDs: []string{"call"}}},
		Operations: []groupindex.Operation{{ID: "start", SubjectID: "call", GroupID: "entry", Name: "start", Kind: "command", Location: programindex.Location{Path: "cli.go", Line: 1}}}}
	indexes := []groupindex.Index{client}
	sections := map[string]*pageSection{"client": {ID: "client", ShortLabel: "Client"}}
	for _, view := range []struct{ id, name, kind string }{{"lib", "service", "library"}, {"exe", "service", "executable"}, {"other", "other service", "executable"}} {
		indexes = append(indexes, groupindex.Index{Target: programindex.Target{ID: view.id, Name: view.name, Kind: view.kind, Language: "go"},
			Operations: []groupindex.Operation{{ID: "receive", Name: "receive", GroupID: view.id + "-handlers", Location: location}}})
		sections[view.id] = &pageSection{ID: view.id, ShortLabel: view.name}
		client.Connections = append(client.Connections, groupindex.Connection{ID: view.id, SourceKind: "integration", FromSubjectID: "call",
			From: groupindex.Endpoint{TargetID: "client", GroupID: "entry"}, To: groupindex.Endpoint{TargetID: view.id, GroupID: view.id + "-handlers"}, ToLocation: &location})
	}
	builder := pageBuilder{indexes: indexes, byProgram: sections}
	got := builder.buildOperationMap(sections["client"], &client)
	var remote []string
	for _, node := range got.Nodes {
		if node.Remote {
			remote = append(remote, node.Href)
		}
	}
	if len(remote) != 2 || strings.Contains(strings.Join(remote, " "), "#lib-") || !strings.Contains(strings.Join(remote, " "), "#exe-") || !strings.Contains(strings.Join(remote, " "), "#other-") {
		t.Fatalf("duplicate source views or lost a different component: %v", remote)
	}
	if len(client.Connections) != 3 {
		t.Fatal("presentation changed the source connections")
	}
}

func TestZoneFramesDoNotCoverOtherZonesOrLooseNodes(t *testing.T) {
	blocks := []mapBlock{
		{container: &groupindex.Container{ID: "integration", Title: "Integration testing"}, groups: make([]groupindex.Group, 3)},
		{container: &groupindex.Container{ID: "validation", Title: "Validation and reporting"}, groups: make([]groupindex.Group, 2)},
		{groups: make([]groupindex.Group, 1)},
		{container: &groupindex.Container{ID: "next", Title: "Another column"}, groups: make([]groupindex.Group, 3)},
	}
	placed := placeLaneBlocks([][]mapBlock{blocks})
	frames := make(map[string]*pageMapFrame)
	var loose []pageMapNode
	for _, entry := range placed.entries {
		node := pageMapNode{X: mapPadding + float64(entry.column)*(mapNodeWidth+mapColumnGap), Y: mapPadding + entry.y, Width: mapNodeWidth, Height: mapNodeHeight}
		if entry.container != nil {
			growFrame(frames, entry.container, node)
		} else {
			loose = append(loose, node)
		}
	}
	sealed := sealFrames(frames)
	for i, frame := range sealed {
		for _, other := range sealed[i+1:] {
			if frame.X < other.X+other.Width && other.X < frame.X+frame.Width && frame.Y < other.Y+other.Height && other.Y < frame.Y+frame.Height {
				t.Fatalf("zone frames overlap: %+v and %+v", frame, other)
			}
		}
		for _, node := range loose {
			if frame.X < node.X+node.Width && node.X < frame.X+frame.Width && frame.Y < node.Y+node.Height && node.Y < frame.Y+frame.Height {
				t.Fatalf("zone covers unrelated node: %+v / %+v", frame, node)
			}
		}
	}
}

func TestMapTitleWrapsAndOnlyCutsWhatCannotFit(t *testing.T) {
	for _, test := range []struct {
		title string
		want  []string
	}{
		{"Domain models", []string{"Domain models"}},
		{"Application bootstrap and routing", []string{"Application bootstrap", "and routing"}},
		{"Simulation rendering and animation", []string{"Simulation rendering", "and animation"}},
		{"", []string{""}},
		{
			"Averyveryverylongsinglewordthatcannotfit",
			[]string{"Averyveryverylongsingl…"},
		},
	} {
		got := mapTitle(test.title)
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("mapTitle(%q) = %#v, want %#v", test.title, got, test.want)
		}
		for _, line := range got {
			if len([]rune(line)) > mapTitleBudget {
				t.Fatalf("mapTitle(%q) line %q is %d runes, over the %d budget",
					test.title, line, len([]rune(line)), mapTitleBudget)
			}
		}
		if len(got) > mapTitleLines {
			t.Fatalf("mapTitle(%q) used %d lines", test.title, len(got))
		}
	}
}

// TestMapTitleMarksWhatItDropped keeps a cut visible: a reader must be able to
// tell a whole name from a shortened one.
func TestMapTitleMarksWhatItDropped(t *testing.T) {
	long := "Level drawing and rendering for the simulation canvas"
	got := mapTitle(long)
	if len(got) != mapTitleLines {
		t.Fatalf("mapTitle(%q) = %#v", long, got)
	}
	if !strings.HasSuffix(got[len(got)-1], "…") {
		t.Fatalf("a shortened title does not say so: %#v", got)
	}
}

// TestAddressValueSeparatesAPortFromATime keeps "where to reach them" from
// listing something that merely has the shape of an address.
func TestAddressValueSeparatesAPortFromATime(t *testing.T) {
	for _, value := range []string{
		":8080", "localhost:8080", "0.0.0.0:3000", "http://localhost:8080",
		"https://example.test:443/path", "127.0.0.1:65535",
	} {
		if !addressValue(value) {
			t.Fatalf("addressValue(%q) = false, want true", value)
		}
	}
	for _, value := range []string{
		"", "12:30", "8080", "localhost", "x:0", "x:65536", "a b:80", "1:2",
	} {
		if addressValue(value) {
			t.Fatalf("addressValue(%q) = true, want false", value)
		}
	}
}

// Hierarchy is independent of activation: an unclassified part must remain
// discoverable by structure, while native operation ownership stays intact.
func TestMapStructureRetainsAreasAndGroupsWithoutOperations(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "target"}, Groups: []groupindex.Group{
		{ID: "store", Title: "Storage", Summary: "Persists records", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"record"}},
		{ID: "buffer", Title: "Buffer", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"buffered"}},
		{ID: "loose", Title: "Other responsibility", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"other"}},
	}, Containers: []groupindex.Container{{ID: "storage-area", Title: "Persistence", Summary: "Stores data", GroupIDs: []string{"store", "buffer"}}},
		Connections: []groupindex.Connection{{ID: "write", From: groupindex.Endpoint{TargetID: "target", GroupID: "buffer"}, To: groupindex.Endpoint{TargetID: "target", GroupID: "store"}, Label: "writes", Summary: "Writes buffered records"}}}
	section := &pageSection{ID: "section", programTargetID: "target"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{"target": section}}
	got := builder.buildMap(section)
	if !got.Explorer || got.Operations {
		t.Fatal("structure depends on an operation")
	}
	var area *pageMapNode
	present := map[string]bool{}
	for i := range got.Nodes {
		n := &got.Nodes[i]
		present[n.ID] = true
		if n.Branch == "area" {
			area = n
		}
	}
	if area == nil || area.FullTitle != "Persistence" || area.Summary != "Stores data" || area.Children != mapNodeID("store")+" "+mapNodeID("buffer") {
		t.Fatalf("lost retained area: %+v", area)
	}
	for _, id := range []string{"store", "buffer", "loose"} {
		if !present[mapNodeID(id)] {
			t.Fatalf("group %s disappeared", id)
		}
	}
	var semantic []pageMapEdge
	for _, e := range got.Edges {
		if e.Scope == "structure" {
			semantic = append(semantic, e)
		}
	}
	if len(semantic) != 1 || semantic[0].Summary != "Writes buffered records" || !semantic[0].Possible {
		t.Fatalf("lost semantic evidence: %+v", semantic)
	}
	if again := builder.buildMap(section); !reflect.DeepEqual(got, again) {
		t.Fatal("structure projection is nondeterministic")
	}
}

func TestMapStructureRetainsIncomingCrossTargetConnectionsAtDestination(t *testing.T) {
	backend := groupindex.Index{Target: programindex.Target{ID: "backend"}, Groups: []groupindex.Group{
		{ID: "core", Title: "Lesson service", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"handler"}},
	}}
	frontend := groupindex.Index{Target: programindex.Target{ID: "frontend"}}
	for i, name := range []string{"load", "run", "submit"} {
		frontend.Groups = append(frontend.Groups, groupindex.Group{ID: name, Title: name + " interaction", Lane: groupindex.LaneCore})
		frontend.Connections = append(frontend.Connections, groupindex.Connection{
			ID: name, SourceKind: "integration", SupportResolution: programindex.PatternValuePossible,
			From: groupindex.Endpoint{TargetID: "frontend", GroupID: name}, To: groupindex.Endpoint{TargetID: "backend", GroupID: "core"},
			Label: name + " HTTP request", Summary: "Retained interpretation for " + name,
			FromLocation: &programindex.Location{Path: "front/api.ts", Line: 11 + i, Column: 5 + i},
			ToLocation:   &programindex.Location{Path: "backend/app.py", Line: 21 + i, Column: 1},
		})
	}
	// A second source-owned relation incident to neither endpoint must not
	// create a remote island while rendering this backend.
	unrelated := groupindex.Index{Target: programindex.Target{ID: "unrelated"}, Groups: []groupindex.Group{{ID: "elsewhere", Title: "Elsewhere"}}}
	unrelated.Connections = []groupindex.Connection{{ID: "unrelated", From: groupindex.Endpoint{TargetID: "unrelated", GroupID: "elsewhere"}, To: groupindex.Endpoint{TargetID: "frontend", GroupID: "load"}, Label: "not incident to backend"}}
	section := &pageSection{ID: "backend-section", programTargetID: "backend"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{backend, frontend, unrelated}, byProgram: map[string]*pageSection{
		"backend": section, "frontend": {ID: "frontend-section", ShortLabel: "Front"}, "unrelated": {ID: "unrelated-section", ShortLabel: "Other"},
	}, links: pageLinks{sourceIDs: map[string]string{"front/api.ts": "front", "backend/app.py": "backend"}}}
	got := builder.buildMap(section)
	var relations []pageMapEdge
	for _, edge := range got.Edges {
		if edge.Scope == "structure" {
			relations = append(relations, edge)
		} else if edge.Scope == "operation" {
			t.Fatalf("matched integration became an operation path: %+v", edge)
		}
	}
	if len(relations) != len(frontend.Connections) {
		t.Fatalf("destination retained %d incoming relations, want %d: %+v", len(relations), len(frontend.Connections), relations)
	}
	for i, connection := range frontend.Connections {
		edge := relations[i]
		from := mapNodeID(section.ID + "-foreign-" + connection.From.GroupID)
		if edge.From != from || edge.To != mapNodeID("core") || edge.Label != connection.Label || edge.Summary != connection.Summary || !edge.Possible {
			t.Fatalf("incoming direction or interpretation changed: %+v", edge)
		}
		if want := builder.links.anchor(connection.FromLocation.Path, connection.FromLocation.Line, connection.FromLocation.Column); edge.FromSource != want {
			t.Fatalf("source anchor changed: %+v, want %+v", edge.FromSource, want)
		}
		if want := builder.links.anchor(connection.ToLocation.Path, connection.ToLocation.Line, connection.ToLocation.Column); edge.ToSource != want {
			t.Fatalf("destination anchor changed: %+v, want %+v", edge.ToSource, want)
		}
		found := false
		for _, node := range got.Nodes {
			if node.ID == from {
				found = node.Remote && node.Component == "frontend" && node.FullTitle == frontend.Groups[i].Title && node.Href == "#"+groupAnchorID("frontend-section", connection.From.GroupID)
			}
			if node.Component == "unrelated" {
				t.Fatalf("unrelated saved relation added a remote island: %+v", node)
			}
		}
		if !found {
			t.Fatalf("incoming source lost its original group stub: %s", from)
		}
	}
	if again := builder.buildMap(section); !reflect.DeepEqual(got, again) {
		t.Fatal("incoming cross-target projection is nondeterministic")
	}
}
