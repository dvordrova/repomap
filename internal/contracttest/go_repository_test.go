package contracttest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	goFixtureRootPackage             = "example.com/repomap/cumulative-go-fixture"
	goFixtureAppPackage              = goFixtureRootPackage + "/cmd/app"
	goFixturePublishedExamplePackage = "example.com/repomap/cumulative-go-published-example"
)

type goFixtureAuthorities struct {
	target       analysistarget.Target
	origins      []gofacts.PackageOrigin
	direct       surfacediscovery.DirectCallIndex
	external     surfacediscovery.ExternalCallIndex
	core         gocoreobject.Index
	dynamic      godynamichandoff.Index
	tests        []gofacts.TestSource
	dependencies *dependencies.Catalog
}

func TestCumulativeGoRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	repositoryPath, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, repositoryPath)
	authorities := analyzeGoFixture(t, repositoryPath, repository, goFixtureAppPackage, "cumulative-go-contract")
	assertUnusedPrivateMethodHasNoDanglingDirectNode(t, authorities)
	producerResultID := assertGoRetainedProducerReceiverAuthority(t, authorities)

	input, err := goadapter.BuildInput(
		repository,
		authorities.target,
		authorities.origins,
		authorities.direct,
		authorities.external,
		authorities.core,
		authorities.dynamic,
		authorities.tests,
	)
	if err != nil {
		t.Fatalf("build Go ProgramIndex: %v", err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertSharedArtifact(t, input, index)
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "go" || index.Target.Selector == "" || index.Target.Name != goFixtureAppPackage {
		t.Fatalf("Go ProgramIndex target = %#v", index.Target)
	}
	if !programIndexHasObject(index, programindex.ObjectMethod, "recreateStore") {
		t.Fatal("Go ProgramIndex omitted unused private method recreateStore")
	}
	assertGoNeutralBoundaryPatterns(t, index)
	assertGoSourceValues(t, repository, index)
	assertGoRuntimeRegistrations(t, index)
	assertGoListenAddresses(t, index)
	assertGoLocalHTTPNameFacts(t, index)
	assertGoExternalEventAndStoragePatterns(t, index)
	assertGoChainedCallAndCallbackTraversal(t, index)
	assertGoRetainedCallbackSourceArgument(t, index)
	assertGoAliasedCallbackSourceArguments(t, index)
	chain := programIndexObjectNamed(t, index, programindex.ObjectFunction, "registerChainedCallbacks", "cmd/app/main.go")
	assertChainedCallbackArguments(t, index, chain.ID, "Map", programindex.ResolutionExact)
	assertGoRetainedProducerReceiverProjection(t, index, producerResultID)
	assertGoInterfaceFieldEvidence(t, authorities, index)
	assertGoSharedHandoffFlows(t, index)
	assertGoCallableReceiverFields(t, authorities, index)
	assertGoInterfaceObjectTransfer(t, authorities, index)
	assertGoInterfaceDeclarations(t, authorities, index)
	assertGoResponseFieldDeclarations(t, repository, authorities, index)

	publishedAuthorities := analyzeGoFixture(
		t,
		repositoryPath,
		repository,
		goFixturePublishedExamplePackage,
		"cumulative-go-published-contract",
	)
	publishedIndex, err := goadapter.Build(
		repository,
		publishedAuthorities.target,
		publishedAuthorities.origins,
		publishedAuthorities.direct,
		publishedAuthorities.external,
		publishedAuthorities.core,
		publishedAuthorities.dynamic,
		publishedAuthorities.tests,
	)
	if err != nil {
		t.Fatalf("build nested-module Go ProgramIndex: %v", err)
	}
	assertProgramIndexRoundTrip(t, publishedIndex)
	assertPublishedRootImportRemainsExternal(t, publishedAuthorities, publishedIndex)
	library := analyzeGoFixture(t, repositoryPath, repository, goFixtureRootPackage, "cumulative-go-library-tests")
	libraryIndex, err := goadapter.Build(repository, library.target, library.origins, library.direct, library.external, library.core, library.dynamic, library.tests)
	if err != nil {
		t.Fatal(err)
	}
	assertProgramIndexRoundTrip(t, libraryIndex)
	assertGoRepeatedAliasedImports(t, library, libraryIndex)
	assertGoTestDeclarationProjection(t, repository, libraryIndex)
	assertGoTestSourceBuildSelection(t, repositoryPath, repository)
}

func assertGoRepeatedAliasedImports(t *testing.T, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "ReadAliasedImports", "root.go")
	callee := programIndexObjectNamed(t, index, programindex.ObjectFunction, "Get", "internal/localstore/store.go")
	count := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != caller.ID {
			continue
		}
		count++
		if !sameSingleID(relation.ToIDs, callee.ID) || relation.Resolution != programindex.ResolutionExact ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 ||
			relation.WitnessesObserved != 3 || relation.WitnessesOmitted != 0 || len(relation.Witnesses) != 3 {
			t.Fatalf("aliased Go imports changed the target or witness coverage: %+v", relation)
		}
		columns := make(map[int]bool)
		for _, witness := range relation.Witnesses {
			if witness.Location == nil || witness.Location.Path != "root.go" || witness.Location.Line != 21 || witness.Location.Column <= 0 {
				t.Fatalf("aliased Go call lost its exact location: %+v", witness)
			}
			columns[witness.Location.Column] = true
		}
		if len(columns) != 3 {
			t.Fatalf("three same-line Go calls collapsed: %+v", relation.Witnesses)
		}
	}
	if count != 1 {
		t.Fatalf("aliased Go call relations = %d, want one relation with three distinct witnesses", count)
	}
	catalog := authorities.dependencies
	if catalog == nil || catalog.Coverage.State != dependencies.CoverageComplete || len(catalog.Coverage.Omissions) != 0 {
		t.Fatalf("aliased Go imports invented missing dependency evidence: %+v", catalog)
	}
	rootImporter := ""
	for _, importer := range catalog.Importers {
		if importer.PackagePath == goFixtureRootPackage {
			rootImporter = importer.Ref
		}
	}
	count = 0
	for _, dependency := range catalog.Dependencies {
		if dependency.PackagePath != goFixtureRootPackage+"/internal/localstore" {
			continue
		}
		for _, importer := range dependency.ImporterRefs {
			if importer == rootImporter {
				count++
			}
		}
	}
	if rootImporter == "" || count != 1 {
		t.Fatalf("three Go aliases must retain one exact package dependency, found %d", count)
	}
}

func assertGoResponseFieldDeclarations(t *testing.T, repository *corpus.Corpus, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	const path = "internal/storefixture/level_responses.go"
	want := map[string]struct {
		name, signature string
		line, column    int
	}{
		"GetLevelsInfoResponse":      {"Count", `Count int "json:\"count\""`, 5, 2},
		"OtherLevelsInfoResponse":    {"Count", `Count string "json:\"count_label\""`, 10, 2},
		"EmbeddedLevelsInfoResponse": {"GetLevelsInfoResponse", "GetLevelsInfoResponse " + goFixtureRootPackage + "/internal/storefixture.GetLevelsInfoResponse", 15, 2},
	}
	for _, declaration := range authorities.core.Types {
		if expected, ok := want[declaration.Name]; ok {
			if len(declaration.Fields) != 1 {
				t.Fatalf("Go native type %s fields = %+v", declaration.Name, declaration.Fields)
			}
			field := declaration.Fields[0]
			if field.ID == "" || field.Name != expected.name || field.Signature != expected.signature ||
				field.Location != (gocoreobject.Location{Path: path, Line: expected.line, Column: expected.column}) {
				t.Fatalf("Go native type %s field = %+v", declaration.Name, field)
			}
		}
		if declaration.Name == "LevelsInfoAlias" && len(declaration.Fields) != 0 {
			t.Fatalf("Go alias acquired a false field declaration: %+v", declaration)
		}
	}
	objects := make(map[string]programindex.Object)
	owners := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Location != nil && object.Location.Path == path && object.Kind == programindex.ObjectType {
			owners[object.Name] = object
		}
	}
	fields := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		owner := objects[object.OwnerID]
		if owner.Name == "LevelsInfoAlias" {
			t.Fatalf("Go alias acquired a false owner: %+v", object)
		}
		if expected, ok := want[owner.Name]; ok {
			if fields[owner.Name].ID != "" || object.Kind != programindex.ObjectVariable || object.ContainerID != owner.ID ||
				object.Name != expected.name || object.Signature != expected.signature || object.Location == nil ||
				object.Location.Path != path || object.Location.Line != expected.line || object.Location.Column != expected.column {
				t.Fatalf("Go type %s projected field = %+v", owner.Name, object)
			}
			fields[owner.Name] = object
		}
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertRegistrationArgument(t, graph, "cmd/app/main.go", "getLevel", map[int]string{21: "/api/levels", 115: "/api/embedded", 117: "/api/overridden-lookalike"})
	seen := make(map[string]bool)
	for _, chunk := range lines.QuestionRows(graph) {
		for _, field := range chunk.Row.Fields {
			if field.Name != "evidence" {
				continue
			}
			for _, evidence := range field.Value.([]map[string]any) {
				name, _ := evidence["name"].(string)
				expected, ok := want[name]
				if !ok {
					continue
				}
				anchor := chunk.Anchors[evidence["ref"].(string)]
				members, _ := evidence["owned_declarations"].([]map[string]any)
				if anchor.SubjectID != owners[name].ID || fields[name].ID == "" || len(members) != 1 ||
					members[0]["path"] != path || members[0]["line"] != expected.line || members[0]["name"] != expected.name ||
					!strings.HasPrefix(members[0]["signature"].(string), expected.name+" ") {
					t.Fatalf("Go type %s lost exact field evidence before question selection: %+v", name, evidence)
				}
				if name != "EmbeddedLevelsInfoResponse" && members[0]["signature"] != expected.signature {
					t.Fatalf("Go type %s lost field type or JSON tag: %+v", name, members)
				}
				seen[name] = true
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("Go type %s did not reach the ordinary question evidence", name)
		}
	}
}

func assertGoInterfaceDeclarations(t *testing.T, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	owners := map[string]string{}
	for _, object := range index.Objects {
		owners[object.ID] = object.Name
	}
	methods := map[string]bool{"Cancel": false, "Status": false}
	ids := map[string]bool{}
	for _, object := range index.Objects {
		if owners[object.OwnerID] != "TicketContract" {
			if (owners[object.OwnerID] == "EmbeddedTicket" || owners[object.OwnerID] == "TicketAlias") && object.Kind == programindex.ObjectMethod {
				t.Fatalf("embedded method acquired a false owner: %+v", object)
			}
			continue
		}
		if _, ok := methods[object.Name]; !ok || object.Kind != programindex.ObjectMethod || object.Location == nil || object.Location.Path != "internal/storefixture/fixtures.go" || !strings.Contains(object.Signature, "id T") {
			t.Fatalf("incorrect interface declaration: %+v", object)
		}
		methods[object.Name] = true
		ids[object.ID] = true
	}
	for name, found := range methods {
		if !found {
			t.Fatalf("interface method %s missing", name)
		}
	}
	for _, declaration := range authorities.core.Callables {
		if strings.Contains(declaration.Receiver, ".TicketContract[") && declaration.DirectCallNodeID != "" {
			t.Fatalf("interface declaration invented a body: %+v", declaration)
		}
	}
	for _, relation := range index.Relations {
		if ids[relation.FromID] && relation.Kind != programindex.RelationContains {
			t.Fatalf("interface declaration invented runtime relations: %+v", relation)
		}
	}
}

func assertGoTestDeclarationProjection(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	want := map[string]bool{"TestPublishedRoot": false, "testExpectedRoot": false, "TestPublishedAPI": false, "TestRootFromTestOnlyPackage": false, "TestPrivateHelper": false, "expected": false, "observe": false, "testObserver": false}
	testIDs := make(map[string]bool)
	names := make(map[string]string)
	for _, object := range index.Objects {
		if object.Location == nil || !strings.HasSuffix(object.Location.Path, "_test.go") {
			continue
		}
		if _, ok := want[object.Name]; !ok {
			t.Fatalf("unexpected test declaration: %+v", object)
		}
		want[object.Name] = true
		if object.Visibility != programindex.VisibilityInternal || len(object.SymbolLinkIdentities) != 0 {
			t.Fatalf("test became public runtime API: %+v", object)
		}
		testIDs[object.ID] = true
		names[object.ID] = object.Name
	}
	for _, seed := range index.Target.Seeds {
		if testIDs[seed.ObjectID] {
			t.Fatal("test declaration became a runtime seed")
		}
	}
	for _, relation := range index.Relations {
		if testIDs[relation.FromID] && relation.Kind != programindex.RelationContains {
			t.Fatal("parsed test gained inferred call authority")
		}
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	for _, chunk := range lines.QuestionRows(graph) {
		for _, anchor := range chunk.Anchors {
			if testIDs[anchor.SubjectID] && strings.HasSuffix(anchor.Path, "_test.go") && anchor.Line > 0 {
				anchors[names[anchor.SubjectID]] = true
			}
		}
	}
	for name, present := range want {
		if !present || !anchors[name] {
			t.Fatalf("test %s did not reach the same indexed question path: object=%v anchor=%v", name, present, anchors[name])
		}
	}
}

func assertGoTestSourceBuildSelection(t *testing.T, root string, repository *corpus.Corpus) {
	t.Helper()
	var paths []string
	for _, entry := range repository.Entries() {
		paths = append(paths, entry.Path)
	}
	options := gofacts.LoadOptions{GoTarget: runtime.GOOS + "/" + runtime.GOARCH, BuildTags: []string{"repomap_optional_tests"}}
	loaded, err := gofacts.LoadWithOptions(t.Context(), root, paths, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.TestSources) != 5 {
		t.Fatalf("build-selected optional test source missing: %+v", loaded.TestSources)
	}
	// Compiler discovery can see files outside the supplied corpus; those must
	// not gain source evidence merely because go list names them.
	var narrowed []string
	for _, path := range paths {
		if path != "root_external_test.go" {
			narrowed = append(narrowed, path)
		}
	}
	loaded, err = gofacts.LoadWithOptions(t.Context(), root, narrowed, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.TestSources) != 4 {
		t.Fatalf("test source escaped corpus selection: %+v", loaded.TestSources)
	}
	for _, source := range loaded.TestSources {
		if source.Path == "root_external_test.go" {
			t.Fatal("excluded test source was read")
		}
	}
	// Preserve the known file when parsing a broken test fails, and keep its
	// unavailable declarations distinct from a successfully scanned empty file.
	path := filepath.Join(root, "root_test.go")
	if err := os.WriteFile(path, []byte("package cumulativegofixture\n\nfunc broken("), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = gofacts.LoadWithOptions(t.Context(), root, paths, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range loaded.TestSources {
		if source.Path == "root_test.go" {
			if source.DeclarationsScanned || len(source.Declarations) != 0 || len(loaded.Warnings) == 0 {
				t.Fatalf("broken source became parsed evidence: %+v", source)
			}
			return
		}
	}
	t.Fatal("broken test file disappeared from inventory")
}

func assertGoInterfaceObjectTransfer(t *testing.T, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	names := make(map[string]string)
	for _, function := range authorities.dynamic.Functions {
		names[function.ID] = function.Symbol
	}
	seenExact, seenAlternatives := false, false
	for _, handoff := range authorities.dynamic.Handoffs {
		if handoff.Kind != godynamichandoff.CallbackTransfer || !strings.HasSuffix(names[handoff.CallerID], ".RegisterServices") {
			continue
		}
		if handoff.Slot.Parameter != 2 || handoff.Slot.Method != "Apply" || !strings.HasSuffix(handoff.Slot.DeclaredType, ".serviceContract") || !strings.HasSuffix(names[handoff.StaticTarget.FunctionID], ".installService") {
			t.Fatalf("lost object-transfer slot: %+v", handoff)
		}
		switch handoff.Resolution {
		case godynamichandoff.ResolutionExact:
			seenExact = len(handoff.Candidates) == 1 && strings.HasSuffix(strings.NewReplacer("(", "", ")", "", "*", "").Replace(names[handoff.Candidates[0].FunctionID]), ".serviceLayer.Apply")
		case godynamichandoff.ResolutionAlternatives:
			seenAlternatives = len(handoff.Candidates) == 2 && handoff.CandidatesOmitted == 0
		default:
			t.Fatalf("unknown object gained callable authority: %+v", handoff)
		}
		for _, candidate := range handoff.Candidates {
			if strings.Contains(names[candidate.FunctionID], "unregisteredServiceLayer") || strings.Contains(names[candidate.FunctionID], "InternalHelper") {
				t.Fatal("compatible type or extra method gained transfer authority")
			}
		}
	}
	if !seenExact || !seenAlternatives {
		t.Fatalf("interface object transfers exact=%v alternatives=%v", seenExact, seenAlternatives)
	}
	projected := 0
	for _, relation := range index.Relations {
		for _, witness := range relation.Witnesses {
			if !strings.Contains(witness.Detail, ".installService;") {
				continue
			}
			projected++
			if relation.Kind != programindex.RelationPassesCallback || relation.SourceArgumentID != "" || !strings.Contains(witness.Detail, "method Apply") || witness.Location == nil || witness.Location.Path != "internal/storefixture/fixtures.go" {
				t.Fatalf("lost neutral registration projection: %+v", relation)
			}
			for id := range names {
				if strings.Contains(witness.Detail, id) {
					t.Fatal("provider-readable binding contains native object identity")
				}
			}
		}
	}
	if projected != 2 {
		t.Fatalf("got %d projected object transfers", projected)
	}
}

func assertGoCallableReceiverFields(t *testing.T, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	names := make(map[string]string)
	for _, function := range authorities.dynamic.Functions {
		names[function.ID] = function.Symbol
	}
	seen := 0
	wantWitnesses := 0
	for _, handoff := range authorities.dynamic.Handoffs {
		if handoff.Kind != godynamichandoff.CallableBinding || !strings.HasSuffix(names[handoff.CallerID], ".BuildActionPair") {
			continue
		}
		seen++
		if len(handoff.Candidates) != 1 || handoff.Resolution != godynamichandoff.ResolutionExact {
			t.Fatalf("lost callback: %+v", handoff)
		}
		want := map[string]bool{"Label = \"inspect\"": true, "Summary = \"Inspect saved state\"": true}
		if strings.HasSuffix(names[handoff.Candidates[0].FunctionID], ".restoreAction") {
			want = map[string]bool{"Label = \"restore [file]\"": true, "Label = \"recover [file]\"": true, "Summary = \"Restore a saved state\"": true, "Enabled = true": true, "Count = 3": true, "Extra = \"fixture\"": true}
		}
		wantWitnesses += len(want)
		for _, field := range handoff.ReceiverFields {
			key := field.Field + " = " + field.Literal
			if !want[key] {
				t.Fatalf("another receiver's field or duplicate store: %s in %+v", key, handoff)
			}
			delete(want, key)
			if field.Location.Path != "internal/storefixture/fixtures.go" || field.Location.Line < 1 || field.Location.Column < 1 {
				t.Fatalf("field has no source: %+v", field)
			}
		}
		if len(want) != 0 {
			t.Fatalf("missing callable receiver fields: %v; got %+v", want, handoff)
		}
	}
	if seen != 2 {
		t.Fatalf("got %d registration objects, want two", seen)
	}
	projected := 0
	for _, relation := range index.Relations {
		for _, witness := range relation.Witnesses {
			if witness.Kind != "callable_receiver_field" || witness.Location == nil || witness.Location.Path != "internal/storefixture/fixtures.go" {
				continue
			}
			projected++
			if relation.Kind != programindex.RelationPassesCallback || relation.Invocation != "callable_binding:field" || witness.Detail == "" {
				t.Fatalf("receiver field became an execution edge: %+v", relation)
			}
		}
	}
	if projected != wantWitnesses {
		t.Fatalf("projected %d field stores, want %d", projected, wantWitnesses)
	}
}

func assertGoInterfaceFieldEvidence(t *testing.T, authorities goFixtureAuthorities, index programindex.Index) {
	t.Helper()
	names := make(map[string]string)
	for _, function := range authorities.dynamic.Functions {
		names[function.ID] = strings.NewReplacer("(", "", ")", "", "*", "").Replace(function.Symbol)
	}
	seen := make(map[string]bool)
	want := map[string][]string{"fieldFacade.Put": {"storedEngine.Put", "alternateEngine.Put"}, "outerFacade.Put": {"fieldFacade.Put"}, "unrelatedFacade.Put": {"neverStoredEngine.Put"}, "unknownFacade.Put": {}}
	for _, handoff := range authorities.dynamic.Handoffs {
		if handoff.Kind != godynamichandoff.InterfaceInvoke {
			continue
		}
		name := names[handoff.CallerID]
		for caller, expected := range want {
			if name != caller && !strings.HasSuffix(name, "."+caller) {
				continue
			}
			seen[caller] = true
			if handoff.Slot.Field == "" || handoff.Slot.ContainerType == "" || len(handoff.Candidates) != len(expected) || handoff.CandidatesOmitted < 1 {
				t.Fatalf("field receiver evidence for %s: %+v", caller, handoff)
			}
			if len(expected) == 0 {
				if handoff.Resolution != godynamichandoff.ResolutionUnresolved {
					t.Fatal("unknown field was resolved")
				}
				continue
			}
			if handoff.Resolution != godynamichandoff.ResolutionAlternatives {
				t.Fatal("field observation became an exact instance call")
			}
			for _, expectedName := range expected {
				found := false
				for _, candidate := range handoff.Candidates {
					if names[candidate.FunctionID] != expectedName && !strings.HasSuffix(names[candidate.FunctionID], "."+expectedName) {
						continue
					}
					found = candidate.Evidence == godynamichandoff.EvidenceInterfaceFieldAssignment && len(candidate.Assignments) > 0
					for _, at := range candidate.Assignments {
						if (at.Path != "internal/storefixture/fixtures.go" && at.Path != "internal/storefixture/handoff_flow.go") || at.Line < 1 || at.Column < 1 {
							t.Fatalf("unanchored receiver assignment: %+v", at)
						}
					}
				}
				if !found {
					t.Fatalf("%s lost observed receiver %s: %+v", caller, expectedName, handoff)
				}
			}
		}
	}
	for caller := range want {
		if !seen[caller] {
			t.Fatalf("no interface invocation for %s; functions: %v", caller, names)
		}
	}
	projected := 0
	for _, relation := range index.Relations {
		for _, witness := range relation.Witnesses {
			if witness.Kind != "interface_field_assignment" {
				continue
			}
			projected++
			if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionAlternatives || relation.TargetsOmitted < 1 || witness.Location == nil ||
				(witness.Location.Path != "internal/storefixture/fixtures.go" && witness.Location.Path != "internal/storefixture/handoff_flow.go") {
				t.Fatalf("ProgramIndex lost possible field assignment evidence: %+v", relation)
			}
		}
	}
	if projected == 0 {
		t.Fatal("ProgramIndex dropped interface field assignment witnesses")
	}
}

func assertGoSharedHandoffFlows(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	wantUnknown := map[string]int{"SharedCallbackFlow": 4, "CyclicCallbackFlow": 1}
	seen := make(map[string]bool)
	assignments := make(map[int]bool)
	for _, relation := range index.Relations {
		if unknown, ok := wantUnknown[objects[relation.FromID].Name]; ok && relation.Kind == programindex.RelationCalls {
			if relation.Resolution != programindex.ResolutionAlternatives || relation.TargetsOmitted != unknown || relation.TargetsObserved != unknown+1 ||
				len(relation.ToIDs) != 1 || objects[relation.ToIDs[0]].Name != "flowAction" {
				t.Fatalf("shared/cyclic callback changed projected authority: %+v", relation)
			}
			seen[objects[relation.FromID].Name] = true
		}
		for _, witness := range relation.Witnesses {
			if witness.Kind == "interface_field_assignment" && witness.Location != nil && witness.Location.Path == "internal/storefixture/handoff_flow.go" {
				assignments[witness.Location.Line] = true
			}
		}
	}
	for name := range wantUnknown {
		if !seen[name] {
			t.Errorf("missing callback projection for %s", name)
		}
	}
	if len(assignments) != 2 || !assignments[40] || !assignments[41] {
		t.Fatalf("shared value lost exact store locations: %v", assignments)
	}
}

func assertGoAliasedCallbackSourceArguments(t *testing.T, index programindex.Index) {
	t.Helper()
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "registerAliasedCallbacks", "cmd/app/main.go")
	arguments := make(map[string]programindex.PatternArgument)
	for _, relation := range index.Relations {
		if relation.FromID == caller.ID {
			for _, pattern := range relation.Patterns {
				for _, argument := range pattern.Arguments {
					arguments[argument.ID] = argument
				}
			}
		}
	}
	var named, literal int
	for _, relation := range index.Relations {
		if relation.FromID != caller.ID || relation.Kind != programindex.RelationPassesCallback {
			continue
		}
		argument, found := arguments[relation.SourceArgumentID]
		if !found || len(relation.ToIDs) != 1 || !sameSingleID(argument.ObjectIDs, relation.ToIDs[0]) ||
			relation.Resolution != programindex.ResolutionExact || argument.Resolution != relation.Resolution ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 || argument.ObjectsObserved != 1 {
			t.Fatalf("Go callable alias lost its argument authority: relation=%#v argument=%#v", relation, argument)
		}
		target := programIndexObjectByID(index, relation.ToIDs[0])
		switch {
		case target.Name == "namedCallback" && target.Kind == programindex.ObjectFunction:
			named++
		case target.Kind == programindex.ObjectFunction && target.Name == "registerAliasedCallbacks$1":
			literal++
		default:
			t.Fatalf("Go callable alias resolved to unexpected object: %#v", target)
		}
	}
	if named != 1 || literal != 1 {
		t.Fatalf("Go callback aliases: named=%d literal=%d", named, literal)
	}
}

func assertGoRetainedCallbackSourceArgument(t *testing.T, index programindex.Index) {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const callLine = 77
	arguments := make(map[string]programindex.PatternArgument)
	var transfers []programindex.Relation
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationCalls {
			for _, pattern := range relation.Patterns {
				if pattern.Location == nil || pattern.Location.Path != sourcePath || pattern.Location.Line != callLine {
					continue
				}
				for _, argument := range pattern.Arguments {
					arguments[argument.ID] = argument
				}
			}
		}
		if relation.Kind == programindex.RelationPassesCallback && relation.Location != nil && relation.Location.Path == sourcePath && relation.Location.Line == callLine {
			transfers = append(transfers, relation)
		}
	}
	if len(transfers) != 1 {
		t.Fatalf("callback transfers at returned closure = %d", len(transfers))
	}
	for _, relation := range transfers {
		argument, found := arguments[relation.SourceArgumentID]
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			relation.Invocation != "callback_transfer:synchronous" || !found || len(argument.ObjectIDs) != 1 || argument.ObjectIDs[0] != relation.ToIDs[0] ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			t.Fatalf("callback transfer lost its retained owning argument: %#v", relation)
		}
	}
}

func assertGoRetainedProducerReceiverAuthority(
	t *testing.T,
	authorities goFixtureAuthorities,
) string {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const consumerLine = 69
	resultID := ""
	for _, family := range authorities.external.Families {
		if family.Target.PackagePath != "net/http" || family.Target.Receiver != "HandlerFunc" ||
			family.Target.Name != "ServeHTTP" {
			continue
		}
		for _, pattern := range family.Patterns {
			if pattern.Callsite.Path != sourcePath || pattern.Callsite.Line != consumerLine {
				continue
			}
			if len(pattern.ReceiverResultIDs) != 1 || pattern.ReceiversObserved != 1 ||
				pattern.ReceiversOmitted != 0 {
				t.Fatalf("cumulative producer consumer authority = %#v", pattern)
			}
			resultID = pattern.ReceiverResultIDs[0]
		}
	}
	if resultID == "" {
		t.Fatal("cumulative Go fixture omitted the program-wide chained external consumer")
	}
	for _, edge := range authorities.direct.Edges {
		for _, pattern := range edge.Patterns {
			if pattern.ResultID == resultID {
				return resultID
			}
		}
	}
	t.Fatal("declaration-wide direct graph lost the producer outside main reachability")
	return ""
}

func assertGoRetainedProducerReceiverProjection(
	t *testing.T,
	index programindex.Index,
	resultID string,
) {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const consumerLine = 69
	objects := make(map[string]programindex.Object, len(index.Objects))
	receiverID := ""
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.SourceRef == resultID {
			receiverID = object.ID
		}
	}
	if receiverID == "" {
		t.Fatal("retained producer lost its call-result object")
	}
	found := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
			continue
		}
		target := objects[relation.ToIDs[0]]
		if target.External == nil || target.External.PackagePath != "net/http" ||
			target.External.Receiver != "HandlerFunc" || target.External.Name != "ServeHTTP" {
			continue
		}
		if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
			t.Fatalf("net/http receiver authority = %#v", target.External)
		}
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil || pattern.Location.Path != sourcePath ||
				pattern.Location.Line != consumerLine {
				continue
			}
			found = true
			if pattern.ReceiverID != receiverID || len(pattern.ReceiverOriginIDs) != 0 ||
				pattern.ReceiverOriginResolution != "" ||
				pattern.ReceiverOriginsObserved != 0 || pattern.ReceiverOriginsOmitted != 0 {
				t.Fatalf("retained producer receiver provenance = %#v", pattern)
			}
		}
	}
	if !found {
		t.Fatal("ProgramIndex omitted the chained external consumer with a retained local producer")
	}
}

func assertGoChainedCallAndCallbackTraversal(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectIDs := make(map[string]string)
	for _, object := range index.Objects {
		objects[object.ID] = object
		objectIDs[object.Name] = object.ID
	}
	registerID := objectIDs["registerProductRoutes"]
	handleID := ""
	for _, object := range index.Objects {
		if object.Name == "HandleFunc" && objects[object.OwnerID].Name == "fixtureRouter" {
			handleID = object.ID
		}
	}
	methodsID := objectIDs["Methods"]
	if registerID == "" || handleID == "" || methodsID == "" {
		t.Fatalf("cumulative chained-call objects = register:%q handle:%q methods:%q", registerID, handleID, methodsID)
	}
	var handlePatterns, methodsPatterns []programindex.RelationPattern
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != registerID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		switch relation.ToIDs[0] {
		case handleID:
			handlePatterns = append(handlePatterns, relation.Patterns...)
		case methodsID:
			methodsPatterns = append(methodsPatterns, relation.Patterns...)
		}
	}
	if len(handlePatterns) != 5 || len(methodsPatterns) != 5 {
		t.Fatalf("cumulative chained patterns = HandleFunc:%d Methods:%d", len(handlePatterns), len(methodsPatterns))
	}
	handlerByPath := map[string]string{
		"/products": objectIDs["listProductsHandler"],
		"/product":  objectIDs["createProductHandler"],
	}
	resultByLine := make(map[int]string, len(handlePatterns))
	handlerArgumentIDs := make(map[string]string, len(handlePatterns))
	for _, pattern := range handlePatterns {
		if pattern.ResultID == "" || len(pattern.Arguments) != 2 ||
			pattern.Arguments[0].Kind != programindex.PatternLiteralString ||
			pattern.Arguments[1].Resolution != programindex.ResolutionExact ||
			len(pattern.Arguments[1].ObjectIDs) != 1 {
			t.Fatalf("cumulative HandleFunc result/callback pattern = %#v", pattern)
		}
		result := objects[pattern.ResultID]
		if result.Kind != programindex.ObjectVariable || result.Location == nil {
			t.Fatalf("cumulative HandleFunc result object = %#v", result)
		}
		resultByLine[result.Location.Line] = pattern.ResultID
		handlerArgumentIDs[pattern.Arguments[1].ObjectIDs[0]] = pattern.Arguments[1].ID
		if want := handlerByPath[pattern.Arguments[0].Value]; want != "" && pattern.Arguments[1].ObjectIDs[0] != want {
			t.Fatalf("cumulative HandleFunc handler for %q = %q, want %q", pattern.Arguments[0].Value, pattern.Arguments[1].ObjectIDs[0], want)
		}
	}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true}
	for _, pattern := range methodsPatterns {
		if pattern.ReceiverID == "" || len(pattern.Arguments) != 1 ||
			pattern.Arguments[0].Kind != programindex.PatternLiteralString || !methods[pattern.Arguments[0].Value] {
			t.Fatalf("cumulative Methods receiver/literal pattern = %#v", pattern)
		}
		result := objects[pattern.ReceiverID]
		if result.Location == nil || resultByLine[result.Location.Line] != pattern.ReceiverID {
			t.Fatalf("cumulative Methods receiver has no same-line HandleFunc result = %#v", pattern)
		}
	}
	callbackJoins := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationPassesCallback || relation.FromID != registerID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		argumentID, expected := handlerArgumentIDs[relation.ToIDs[0]]
		if !expected {
			continue
		}
		callbackJoins++
		if relation.SourceArgumentID != argumentID {
			t.Fatalf("cumulative callback source argument = %q, want %q: %#v", relation.SourceArgumentID, argumentID, relation)
		}
	}
	if callbackJoins != len(handlerArgumentIDs) {
		t.Fatalf("cumulative callback source joins = %d, want %d", callbackJoins, len(handlerArgumentIDs))
	}
	for _, pair := range [][2]string{
		{"listProductsHandler", "listProducts"},
		{"createProductHandler", "createProduct"},
		{"getProductHandler", "getProduct"},
		{"updateProductHandler", "updateProduct"},
		{"deleteProductHandler", "deleteProduct"},
	} {
		fromID, toID := objectIDs[pair[0]], objectIDs[pair[1]]
		found := false
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationCalls && relation.FromID == fromID &&
				relation.Resolution == programindex.ResolutionExact && len(relation.ToIDs) == 1 && relation.ToIDs[0] == toID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cumulative callback body call %s -> %s is absent", pair[0], pair[1])
		}
	}
}

func assertGoNeutralBoundaryPatterns(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectIDs := make(map[string]string)
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Kind == programindex.ObjectFunction {
			objectIDs[object.Name] = object.ID
		}
	}
	handlerID := objectIDs["getLevel"]
	if handlerID == "" {
		t.Fatal("cumulative Go fixture omitted exact HTTP handler object")
	}
	relationFor := func(name, caller string) programindex.Relation {
		t.Helper()
		source := programIndexObjectNamed(t, index, programindex.ObjectFunction, caller, "cmd/app/main.go")
		for _, relation := range index.Relations {
			if relation.FromID != source.ID || relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
				continue
			}
			target := objects[relation.ToIDs[0]]
			if target.External != nil && target.External.PackagePath == "net/http" &&
				target.External.Name == name {
				if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
					t.Fatalf("net/http.%s authority = %#v", name, target.External)
				}
				return relation
			}
		}
		t.Fatalf("cumulative Go fixture omitted %s → net/http.%s", caller, name)
		return programindex.Relation{}
	}
	client := relationFor("Get", "fetchLevels")
	if len(client.Patterns) != 1 || client.Patterns[0].Selector != "Get" ||
		len(client.Patterns[0].Arguments) != 1 ||
		client.Patterns[0].Arguments[0].Kind != programindex.PatternLiteralString ||
		client.Patterns[0].Arguments[0].Value != "/api/levels" {
		t.Fatalf("cumulative Go client pattern = %#v", client)
	}
	route := relationFor("HandleFunc", "registerLevelRoute")
	if len(route.Patterns) != 1 || len(route.Patterns[0].Arguments) != 2 ||
		route.Patterns[0].Arguments[0].Value != "/api/levels" ||
		route.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
		len(route.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
		route.Patterns[0].Arguments[1].ObjectIDs[0] != handlerID {
		t.Fatalf("cumulative Go route pattern = %#v", route)
	}
	bootstrap := relationFor("ListenAndServe", "registerLevelRoute")
	if len(bootstrap.Patterns) != 1 || len(bootstrap.Patterns[0].Arguments) != 2 ||
		bootstrap.Patterns[0].Arguments[0].Value != ":8080" ||
		bootstrap.Patterns[0].Arguments[1].Kind != programindex.PatternDynamic {
		t.Fatalf("cumulative Go bootstrap frontier pattern = %#v", bootstrap)
	}

	registerID := objectIDs["registerLevelConsumer"]
	subscribeID := objectIDs["Subscribe"]
	consumerID := objectIDs["consumeLevel"]
	if registerID == "" || subscribeID == "" || consumerID == "" {
		t.Fatalf("cumulative Go consumer objects = register:%q subscribe:%q consumer:%q",
			registerID, subscribeID, consumerID)
	}
	var subscribeCalls, callbackTransfers []programindex.Relation
	for _, relation := range index.Relations {
		if relation.FromID != registerID || relation.Resolution != programindex.ResolutionExact ||
			len(relation.ToIDs) != 1 {
			continue
		}
		switch {
		case relation.Kind == programindex.RelationCalls && relation.ToIDs[0] == subscribeID:
			subscribeCalls = append(subscribeCalls, relation)
		case relation.Kind == programindex.RelationPassesCallback && relation.ToIDs[0] == consumerID:
			callbackTransfers = append(callbackTransfers, relation)
		}
	}
	if len(subscribeCalls) != 1 {
		t.Fatalf("cumulative Go Subscribe calls = %#v", subscribeCalls)
	}
	subscribe := subscribeCalls[0]
	if subscribe.PatternsObserved != 1 || len(subscribe.Patterns) != 1 ||
		subscribe.Patterns[0].Selector != "Subscribe" || len(subscribe.Patterns[0].Arguments) != 2 ||
		subscribe.Patterns[0].Arguments[0].Kind != programindex.PatternLiteralString ||
		subscribe.Patterns[0].Arguments[0].Value != "levels.requested" ||
		subscribe.Patterns[0].Arguments[1].Kind != programindex.PatternDynamic ||
		subscribe.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
		len(subscribe.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
		subscribe.Patterns[0].Arguments[1].ObjectIDs[0] != consumerID {
		t.Fatalf("cumulative Go Subscribe neutral pattern = %#v", subscribe)
	}
	if len(callbackTransfers) != 1 || callbackTransfers[0].Invocation != "callback_transfer:synchronous" ||
		callbackTransfers[0].SourceArgumentID != subscribe.Patterns[0].Arguments[1].ID ||
		callbackTransfers[0].TargetsObserved != 1 || callbackTransfers[0].TargetsOmitted != 0 {
		t.Fatalf("cumulative Go callback transfer = %#v", callbackTransfers)
	}
}

func assertGoExternalEventAndStoragePatterns(
	t *testing.T,
	index programindex.Index,
) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	want := map[string]struct {
		packagePath string
		name        string
	}{
		"event":   {packagePath: "os/signal", name: "Notify"},
		"storage": {packagePath: "os", name: "Create"},
	}
	patternIDs := make(map[string]string, len(want))
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
			continue
		}
		target := objects[relation.ToIDs[0]]
		if target.External == nil {
			continue
		}
		for key, expected := range want {
			if target.External.PackagePath != expected.packagePath || target.External.Name != expected.name {
				continue
			}
			if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
				t.Fatalf("cumulative Go %s platform authority = %#v", key, target.External)
			}
			if patternIDs[key] != "" {
				t.Fatalf("cumulative Go fixture duplicated %s external relation", key)
			}
			if relation.Resolution != programindex.ResolutionExact || relation.TargetsObserved != 1 ||
				relation.TargetsOmitted != 0 || relation.WitnessesObserved != 1 ||
				relation.WitnessesOmitted != 0 || len(relation.Patterns) != 1 {
				t.Fatalf("cumulative Go %s external authority = %#v", key, relation)
			}
			pattern := relation.Patterns[0]
			if pattern.Selector != expected.name || pattern.Form != programindex.PatternCall ||
				pattern.Location == nil || pattern.Location.Path != "internal/storefixture/fixtures.go" ||
				pattern.ArgumentsOmitted != 0 || pattern.ArgumentsObserved != len(pattern.Arguments) {
				t.Fatalf("cumulative Go %s neutral pattern = %#v", key, pattern)
			}
			switch key {
			case "event":
				if len(pattern.Arguments) != 2 ||
					pattern.Arguments[0].Kind != programindex.PatternDynamic ||
					pattern.Arguments[1].Kind != programindex.PatternDynamic {
					t.Fatalf("cumulative Go signal registration arguments = %#v", pattern.Arguments)
				}
			case "storage":
				if pattern.ArgumentsObserved != 1 || pattern.ArgumentsOmitted != 0 ||
					len(pattern.Arguments) != 1 ||
					pattern.Arguments[0].Kind != programindex.PatternLiteralString ||
					pattern.Arguments[0].Value != "fixture-state.db" {
					t.Fatalf("cumulative Go storage arguments = %#v", pattern.Arguments)
				}
			}
			patternIDs[key] = pattern.ID
		}
	}
	for key := range want {
		if patternIDs[key] == "" {
			t.Fatalf("cumulative Go fixture omitted exact external %s pattern", key)
		}
	}
}

func analyzeGoFixture(
	t *testing.T,
	repositoryPath string,
	repository *corpus.Corpus,
	packagePath string,
	_ string,
) goFixtureAuthorities {
	t.Helper()
	deferred, err := snapshot.BuildContext(t.Context(), snapshot.Options{
		RepoPath: repositoryPath, GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
		RepositoryCorpus: repository,
	})
	if err != nil {
		t.Fatalf("build cumulative Go fixture target catalog: %v", err)
	}
	targetRef := ""
	if deferred.TargetCatalog != nil {
		for _, entry := range deferred.TargetCatalog.Entries {
			if entry.Candidate.Target.PackagePath == packagePath || entry.Candidate.Target.Kind == analysistarget.KindModuleLibrary && entry.Candidate.Target.ModulePath == packagePath {
				targetRef = entry.Candidate.Target.Ref
				break
			}
		}
	}
	if targetRef == "" {
		t.Fatalf("cumulative Go fixture target %q is absent", packagePath)
	}
	scoped, err := snapshot.ScopeAnalysisTarget(deferred, targetRef)
	if err != nil {
		t.Fatalf("scope cumulative Go fixture target: %v", err)
	}
	if scoped.AnalysisTarget.Kind == analysistarget.KindExecutablePackage {
		assertGoFixtureTestSources(t, deferred, scoped)
	}
	input, err := goadapter.AnalysisInput(scoped.GoFacts, scoped.AnalysisTarget)
	if err != nil {
		t.Fatalf("bind cumulative Go fixture analysis input: %v", err)
	}
	options := surfacediscovery.DefaultOptions(repositoryPath, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureExternalCallIndex = true
	options.CaptureCoreObjectIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, input)
	if err != nil {
		t.Fatalf("analyze cumulative Go fixture: %v", err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil ||
		result.CoreObjectIndex == nil || result.DynamicHandoffIndex == nil {
		t.Fatalf("Go fixture analysis omitted producer authority: %#v", result)
	}
	authorities := goFixtureAuthorities{
		target:       scoped.AnalysisTarget.Snapshot(),
		origins:      append([]gofacts.PackageOrigin(nil), scoped.GoFacts.PackageOrigins...),
		direct:       result.DirectCallIndex.Snapshot(),
		external:     result.ExternalCallIndex.Snapshot(),
		core:         result.CoreObjectIndex.Snapshot(),
		dynamic:      result.DynamicHandoffIndex.Snapshot(),
		tests:        gofacts.CloneTestSources(scoped.GoFacts.TestSources),
		dependencies: scoped.GoFacts.Dependencies,
	}
	if authorities.target.Ref == "" || authorities.direct.SHA256 == "" ||
		authorities.external.SHA256 == "" || authorities.core.SHA256 == "" ||
		authorities.dynamic.SHA256 == "" {
		t.Fatalf("Go fixture analysis omitted producer authority: %#v", authorities)
	}
	return authorities
}

func assertGoFixtureTestSources(t *testing.T, deferred, executable snapshot.Snapshot) {
	t.Helper()
	if len(executable.GoFacts.TestSources) != 0 {
		t.Fatal("an executable imported its dependencies' tests into its own component")
	}
	var libraryRef string
	for _, entry := range deferred.TargetCatalog.Entries {
		target := entry.Candidate.Target
		if target.Kind == analysistarget.KindModuleLibrary && target.ModulePath == goFixtureRootPackage {
			libraryRef = target.Ref
		}
	}
	library, err := snapshot.ScopeAnalysisTarget(deferred, libraryRef)
	if err != nil {
		t.Fatal(err)
	}
	repository, scoped := deferred.GoFacts, library.GoFacts
	want := map[string]struct {
		name     string
		line     int
		external bool
	}{
		"root_test.go":                       {"TestPublishedRoot", 5, false},
		"root_external_test.go":              {"TestPublishedAPI", 9, true},
		"testonly/api_test.go":               {"TestRootFromTestOnlyPackage", 9, true},
		"internal/testhelper/helper_test.go": {"TestPrivateHelper", 5, false},
	}
	if len(repository.TestSources) != len(want) || len(scoped.TestSources) != len(want) {
		t.Fatalf("lost independent test source inventory: repository=%+v scoped=%+v", repository.TestSources, scoped.TestSources)
	}
	for _, source := range scoped.TestSources {
		expected, ok := want[source.Path]
		if !ok || !source.DeclarationsScanned || source.External != expected.external || source.ModulePath != goFixtureRootPackage || source.ModuleDir != "." {
			t.Fatalf("unexpected test source: %+v", source)
		}
		found := false
		for _, declaration := range source.Declarations {
			if declaration.Name == expected.name {
				found = declaration.Path == source.Path && declaration.Line == expected.line && declaration.Column == 6
			}
		}
		if !found {
			t.Fatalf("missing exact test declaration: %+v", source)
		}
		delete(want, source.Path)
	}
	for _, pkg := range repository.Packages {
		if strings.HasSuffix(pkg.CanonicalPath, "/testonly") {
			t.Fatal("test-only directory became an ordinary package")
		}
		for _, declaration := range pkg.Declarations {
			if strings.HasSuffix(declaration.Path, "_test.go") {
				t.Fatal("test declaration became ordinary package API")
			}
		}
	}
	before := repository.TestSources[0].Declarations[0].Name
	scoped.TestSources[0].Declarations[0].Name = "mutated"
	if repository.TestSources[0].Declarations[0].Name != before {
		t.Fatal("scoped test source declarations share mutable storage")
	}
	scoped.TestSources[0].Declarations[0].Name = before
}

func writePublishedGoFixtureModule(t *testing.T, repositoryPath string) {
	t.Helper()
	publishedRoot := filepath.Join(filepath.Dir(repositoryPath), "published-root")
	if err := os.MkdirAll(publishedRoot, 0o700); err != nil {
		t.Fatalf("create published Go fixture module: %v", err)
	}
	files := map[string]string{
		"go.mod": "module " + goFixtureRootPackage + "\n\ngo 1.22\n",
		"root.go": `package cumulativegofixture

func PublishedRoot() string {
	return "published root"
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(publishedRoot, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write published Go fixture module %s: %v", name, err)
		}
	}
}

func assertPublishedRootImportRemainsExternal(
	t *testing.T,
	authorities goFixtureAuthorities,
	index programindex.Index,
) {
	t.Helper()
	if authorities.target.PackagePath != goFixturePublishedExamplePackage ||
		authorities.target.ModuleDir != "_examples/published" {
		t.Fatalf("nested-module target = %#v", authorities.target)
	}
	for _, node := range authorities.direct.Nodes {
		if node.Package == goFixtureRootPackage {
			t.Fatalf("published root import became a repository direct-call node: %#v", node)
		}
	}
	foundExternalFamily := false
	for _, family := range authorities.external.Families {
		if family.Target.PackagePath == goFixtureRootPackage && family.Target.Name == "PublishedRoot" {
			foundExternalFamily = true
			break
		}
	}
	if !foundExternalFamily {
		t.Fatalf("published root import is absent from external-call authority: %#v", authorities.external.Families)
	}
	if index.Target.Language != "go" || index.Target.Name != goFixturePublishedExamplePackage ||
		index.Target.Selector != goFixturePublishedExamplePackage {
		t.Fatalf("nested-module ProgramIndex target = %#v", index.Target)
	}
	foundExternalObject := false
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectPackage && object.Name == goFixtureRootPackage {
			t.Fatalf("published root import became a local ProgramIndex package: %#v", object)
		}
		if object.Kind == programindex.ObjectExternalSymbol && object.External != nil &&
			object.External.PackagePath == goFixtureRootPackage && object.External.Name == "PublishedRoot" {
			if object.External.AuthorityKind != programindex.ExternalAuthorityPackage {
				t.Fatalf("published root import authority = %#v", object.External)
			}
			foundExternalObject = true
		}
	}
	if !foundExternalObject {
		t.Fatalf("published root import is absent from external ProgramIndex objects: %#v", index.Objects)
	}
}

func assertUnusedPrivateMethodHasNoDanglingDirectNode(t *testing.T, authorities goFixtureAuthorities) {
	t.Helper()
	var method gocoreobject.CallableDeclaration
	for _, callable := range authorities.core.Callables {
		if callable.Name == "recreateStore" && strings.HasSuffix(
			callable.Receiver, "/internal/storefixture.fileStoreTestBundle",
		) {
			method = callable
			break
		}
	}
	if method.ID == "" {
		t.Fatal("CoreObjectIndex omitted (*fileStoreTestBundle).recreateStore")
	}
	if method.DirectCallNodeID != "" {
		t.Fatalf("unused private method gained dangling direct-call node %q", method.DirectCallNodeID)
	}
	for _, node := range authorities.direct.Nodes {
		if node.Symbol.Name == "recreateStore" {
			t.Fatalf("unused private method unexpectedly entered DirectCallIndex: %#v", node)
		}
	}
}

func programIndexHasObject(index programindex.Index, kind programindex.ObjectKind, name string) bool {
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name {
			return true
		}
	}
	return false
}
