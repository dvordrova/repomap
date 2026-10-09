package contracttest

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/pythondependencies"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

const pythonFixtureSelector = "python:.:script:repomap-fixture"

func TestCumulativePythonNamespaceDependencyAuthority(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.ProjectDir == "workspace" && candidate.Kind == pythontarget.KindLibrary {
			target = candidate
		}
	}
	if len(target.Packages) != 1 || !target.Packages[0].Namespace || target.Packages[0].Name != "fixture_shared" {
		t.Fatalf("native namespace inventory = %#v", target.Packages)
	}
	input, err := sharedPythonFixtureInput(t, repository, target)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	assertProgramIndexRoundTrip(t, index)
	adaptertest.AssertSharedArtifact(t, input, index)
	var namespace programindex.Object
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectPackage && object.Name == "fixture_shared" {
			namespace = object
		}
		if object.Kind == programindex.ObjectExternalSymbol &&
			(object.Name == "fixture_shared.local.missing.unavailable" || object.Name == "fixture_shared.runtime_member") {
			t.Fatalf("invented a namespace member or ordinary-package child: %#v", object)
		}
	}
	if namespace.ID == "" || namespace.Directory != target.Packages[0].Dir || namespace.Location != nil {
		t.Fatalf("namespace lost its native directory or acquired a source anchor: %#v", namespace)
	}
	const source = "workspace/src/fixture_shared/local/consumer.py"
	consumer := programIndexObjectNamed(t, index, programindex.ObjectModule, "fixture_shared.local.consumer", source)
	assertExactPythonImportBoundary(t, index, consumer.ID, namespace.ID, "fixture_shared.runtime_member")
	load := programIndexExternalObjectNamed(t, index, "fixture_shared.module_loading.load")
	extensions := programIndexExternalObjectNamed(t, index, "fixture_shared.extensions")
	for _, want := range []struct{ detail, kind, toID string }{
		{"fixture_shared", "import", namespace.ID},
		{"fixture_shared.module_loading.load", pythondependencies.WitnessExternalFromImport, load.ID},
		{"fixture_shared.extensions", pythondependencies.WitnessExternalImport, extensions.ID},
	} {
		found := false
		for _, relation := range index.Relations {
			if relation.Kind != programindex.RelationImports || relation.FromID != consumer.ID ||
				relation.Resolution != programindex.ResolutionExact || !sameSingleID(relation.ToIDs, want.toID) ||
				relation.Location == nil || relation.Location.Path != source {
				continue
			}
			for _, witness := range relation.Witnesses {
				if witness.Detail == want.detail && witness.Kind == want.kind {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("namespace import lost its source authority: %#v", want)
		}
	}
	reader := programIndexObjectNamed(t, index, programindex.ObjectFunction, "read", source)
	foundCall := false
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationInvokesExternal && relation.FromID == reader.ID && sameSingleID(relation.ToIDs, load.ID) {
			if relation.Resolution != programindex.ResolutionExact || relation.Location == nil || relation.Location.Path != source {
				t.Fatalf("namespace import call lost its original qualified observation: %#v", relation)
			}
			foundCall = true
		}
	}
	if !foundCall {
		t.Fatal("namespace from-import did not retain the later call binding")
	}
	deps, err := pythondependencies.Build(index)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps.Coverage.Omissions) != 1 || deps.Coverage.State != dependencies.CoveragePartial ||
		deps.Coverage.Omissions[0].PackagePath != "fixture_shared.local.missing.unavailable" ||
		deps.Coverage.Omissions[0].Reason != dependencies.OmissionDependencyIdentityMissing {
		t.Fatalf("only the ordinary package's missing child should remain unresolved: %#v", deps.Coverage)
	}
	want := map[string]dependencies.Kind{
		"fixture_shared":                dependencies.KindWorkspace,
		"fixture_shared.module_loading": dependencies.KindExternal,
		"fixture_shared.extensions":     dependencies.KindExternal,
	}
	for _, dependency := range deps.Dependencies {
		if dependency.Kind != want[dependency.PackagePath] {
			t.Fatalf("namespace dependency acquired an unsupported owner: %#v", dependency)
		}
		if dependency.PackagePath == "fixture_shared" && dependency.RepositoryPath != namespace.Directory {
			t.Fatalf("namespace dependency lost its exact directory: %#v", dependency)
		}
		if dependency.Kind == dependencies.KindExternal && dependency.RepositoryPath != "" {
			t.Fatalf("external namespace portion acquired a local path: %#v", dependency)
		}
		delete(want, dependency.PackagePath)
	}
	if len(want) != 0 {
		t.Fatalf("namespace dependencies missing: %#v", want)
	}
}

// A Python library's entries are its API: the public functions of its public
// modules and the public methods of their public classes. scoring.py's
// Scoreboard.show is one; its _reset, the function nested in show, and
// _formats.py's score_text (its module's name begins with an underscore)
// are not, nor is exports.py's format_score, which __all__ leaves out.
func TestCumulativePythonLibraryExportsItsAPI(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.ProjectDir == "." && candidate.Kind == pythontarget.KindLibrary {
			target = candidate
		}
	}
	input, err := sharedPythonFixtureInput(t, repository, target)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	exported := map[string]bool{}
	for _, export := range index.Target.Exports {
		object := programIndexObjectByID(index, export.ObjectID)
		name := object.Name
		if object.Kind == programindex.ObjectMethod {
			name = programIndexObjectByID(index, object.OwnerID).Name + "." + name
		}
		exported[export.Location.Path+" "+name] = true
	}
	if index.Target.ExportBasis != programindex.ExportsVisibility {
		t.Fatalf("library export basis %q", index.Target.ExportBasis)
	}
	for name, want := range map[string]bool{
		"src/fixture_app/scoring.py Scoreboard.show":   true,
		"src/fixture_app/exports.py render_level":      true,
		"src/fixture_app/scoring.py Scoreboard._reset": false,
		"src/fixture_app/scoring.py padded":            false,
		"src/fixture_app/_formats.py score_text":       false,
		"src/fixture_app/exports.py format_score":      false,
	} {
		if exported[name] != want {
			t.Fatalf("%s exported %v, want %v", name, exported[name], want)
		}
	}
	for name := range exported {
		if len(name) > 6 && name[:6] == "tests/" {
			t.Fatalf("a test is the library's API: %s", name)
		}
	}
}

func TestCumulativePythonCallbackAliasesRetainArgumentAuthority(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{pythonFixtureTarget(t, catalog)})
	if err != nil {
		t.Fatalf("build callback alias fixture: %v", err)
	}
	index := indexes[0]
	const sourcePath = "src/fixture_app/models.py"
	chain := programIndexObjectNamed(t, index, programindex.ObjectFunction, "register_chained_callbacks", sourcePath)
	assertChainedCallbackArguments(t, index, chain.ID, "map", programindex.ResolutionExact)
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "register_callback_aliases", sourcePath)
	arguments := make(map[string]programindex.PatternArgument)
	for _, relation := range index.Relations {
		if relation.FromID != caller.ID {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				arguments[argument.ID] = argument
			}
		}
	}
	var lambdaCount, keywordCount int
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationPassesCallback || relation.FromID != caller.ID {
			continue
		}
		argument, ok := arguments[relation.SourceArgumentID]
		if !ok || len(relation.ToIDs) != 1 || !sameSingleID(argument.ObjectIDs, relation.ToIDs[0]) ||
			argument.Resolution != relation.Resolution || argument.ObjectsObserved != 1 ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			t.Fatalf("callback alias lost its argument authority: relation=%#v argument=%#v", relation, argument)
		}
		// handler = handle_delivery runs only under `if replace_handler`, so
		// deliver_callback(handler) may pass the caller's value instead: the
		// last assignment gives no callback.
		target := programIndexObjectByID(index, relation.ToIDs[0])
		if target.Kind != programindex.ObjectLambda || relation.Resolution != programindex.ResolutionExact {
			t.Fatalf("callback acquired unsupported authority: target=%#v relation=%#v", target, relation)
		}
		lambdaCount++
		if argument.Keyword == "callback" {
			keywordCount++
		}
	}
	if lambdaCount != 3 || keywordCount != 1 {
		t.Fatalf("callback aliases: lambda=%d keyword=%d", lambdaCount, keywordCount)
	}
	// The argument keeps the name's own variable, not the function the
	// branch may have stored in it.
	reassigned := false
	for _, relation := range index.Relations {
		if relation.FromID != caller.ID || relation.Location == nil || relation.Location.Line != 41 {
			continue
		}
		for _, pattern := range relation.Patterns {
			argument := pythonPatternArgument(t, pattern, 1)
			if pattern.Selector != "deliver_callback" || len(argument.ObjectIDs) != 1 ||
				programIndexObjectByID(index, argument.ObjectIDs[0]).Kind != programindex.ObjectVariable {
				t.Fatalf("a name reassigned under a branch passed its last function: %#v", argument)
			}
			reassigned = true
		}
	}
	if !reassigned {
		t.Fatal("register_callback_aliases lost its deliver_callback(handler) call")
	}
	for _, name := range []string{"register_unknown_callback", "register_overwritten_callback"} {
		owner := programIndexObjectNamed(t, index, programindex.ObjectFunction, name, sourcePath)
		found := false
		for _, relation := range index.Relations {
			if relation.FromID != owner.ID {
				continue
			}
			if relation.Kind == programindex.RelationPassesCallback {
				t.Fatalf("%s invented a callback from an unknown value: %#v", name, relation)
			}
			for _, pattern := range relation.Patterns {
				if pattern.Selector != "deliver_callback" {
					continue
				}
				argument := pythonPatternArgument(t, pattern, 1)
				if len(argument.ObjectIDs) != 1 || programIndexObjectByID(index, argument.ObjectIDs[0]).Kind != programindex.ObjectVariable ||
					argument.Resolution != programindex.ResolutionExact {
					t.Fatalf("%s lost the original variable authority: %#v", name, argument)
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("%s lost its call argument", name)
		}
	}
	// Lambdas inside store targets (assignment, annotated assignment and for
	// targets) belong to the function and are passed like any other lambda.
	marker := programIndexObjectNamed(t, index, programindex.ObjectFunction, "mark_exit_rows", sourcePath)
	storeLambdas := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectLambda && object.ContainerID == marker.ID {
			storeLambdas[object.ID] = object
		}
	}
	callbackLines := make(map[int]bool)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationPassesCallback || relation.FromID != marker.ID {
			continue
		}
		if len(relation.ToIDs) != 1 {
			t.Fatalf("store-target callback has %d targets: %#v", len(relation.ToIDs), relation)
		}
		target, ok := storeLambdas[relation.ToIDs[0]]
		if !ok || relation.Resolution != programindex.ResolutionExact ||
			relation.SourceArgumentID == "" || relation.Location == nil ||
			target.Location == nil || relation.Location.Line != target.Location.Line {
			t.Fatalf("store-target lambda lost its callback: %#v", relation)
		}
		callbackLines[relation.Location.Line] = true
	}
	if len(storeLambdas) != 3 || len(callbackLines) != 3 {
		t.Fatalf("store-target lambdas: declared=%d callbacks=%v", len(storeLambdas), callbackLines)
	}
	// Annotations and defaults run where the function is defined: the module
	// passes the Depends lambda and declares the Annotated check, while the
	// lambda default belongs to the function that writes it.
	module := programIndexObjectNamed(t, index, programindex.ObjectModule, "fixture_app.models", sourcePath)
	limit := programIndexObjectNamed(t, index, programindex.ObjectFunction, "level_limit", sourcePath)
	sorter := programIndexObjectNamed(t, index, programindex.ObjectFunction, "row_sorter", sourcePath)
	headerLambdas := make(map[string]string)
	sorterLambdas := 0
	for _, object := range index.Objects {
		if object.Kind != programindex.ObjectLambda || object.Location == nil || object.Location.Path != sourcePath {
			continue
		}
		if object.Location.Line == limit.Location.Line && object.ContainerID == module.ID {
			headerLambdas[object.ID] = object.Signature
		}
		if object.ContainerID == sorter.ID {
			sorterLambdas++
		}
	}
	headerCallbacks := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationPassesCallback || len(relation.ToIDs) != 1 || headerLambdas[relation.ToIDs[0]] == "" {
			continue
		}
		if relation.FromID != module.ID || relation.Resolution != programindex.ResolutionExact ||
			relation.SourceArgumentID == "" || headerLambdas[relation.ToIDs[0]] != "lambda" {
			t.Fatalf("annotation lambda lost its Depends callback: %#v", relation)
		}
		headerCallbacks++
	}
	if len(headerLambdas) != 2 || headerCallbacks != 1 || sorterLambdas != 2 {
		t.Fatalf("header lambdas: annotations=%v callbacks=%d row_sorter=%d", headerLambdas, headerCallbacks, sorterLambdas)
	}
	// A decorator's arguments and a default run where the class or function
	// is defined: the module for a class decorator, the class for a method's
	// decorator and default. The decoration stays the decorated declaration's.
	routes := programIndexObjectNamed(t, index, programindex.ObjectType, "LevelRoutes", sourcePath)
	names := map[string]string{module.ID: "module"}
	for _, object := range index.Objects {
		if names[object.ID] == "" && object.Location != nil && object.Location.Path == sourcePath {
			names[object.ID] = object.Name
		}
	}
	var definitionTime []string
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != sourcePath || len(relation.ToIDs) != 1 ||
			relation.Location.Line < routes.Location.Line-1 || relation.Location.Line > routes.EndLine {
			continue
		}
		if target := names[relation.ToIDs[0]]; target == "routed" || target == "route_path" {
			definitionTime = append(definitionTime, fmt.Sprintf("%+d %s %s from %s",
				relation.Location.Line-routes.Location.Line, relation.Kind, target, names[relation.FromID]))
		}
	}
	sort.Strings(definitionTime)
	want := []string{
		"+1 calls route_path from LevelRoutes",
		"+1 decorates routed from load_level",
		"+2 calls route_path from LevelRoutes",
		"+3 calls route_path from load_level",
		"-1 calls route_path from module",
		"-1 decorates routed from LevelRoutes",
	}
	if !reflect.DeepEqual(definitionTime, want) {
		t.Fatalf("definition-time calls:\n have %q\n want %q", definitionTime, want)
	}
}

func assertChainedCallbackArguments(t *testing.T, index programindex.Index, callerID, selector string, resolution programindex.Resolution) {
	t.Helper()
	arguments := make(map[string]programindex.PatternArgument)
	calls := 0
	for _, relation := range index.Relations {
		if relation.FromID != callerID {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != selector {
				continue
			}
			calls++
			if relation.PatternsObserved != len(relation.Patterns) || relation.PatternsOmitted != 0 {
				t.Fatalf("chained calls merged their original argument patterns: %#v", relation)
			}
			for _, argument := range pattern.Arguments {
				arguments[argument.ID] = argument
			}
		}
	}
	seenArguments, seenCallbacks := make(map[string]bool), make(map[string]bool)
	for _, relation := range index.Relations {
		if relation.FromID != callerID || relation.Kind != programindex.RelationPassesCallback {
			continue
		}
		argument, ok := arguments[relation.SourceArgumentID]
		if !ok || len(relation.ToIDs) != 1 || !sameSingleID(argument.ObjectIDs, relation.ToIDs[0]) ||
			relation.Resolution != resolution || argument.Resolution != resolution ||
			argument.ObjectsObserved != 1 || relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			t.Fatalf("chained callback lost its own source argument: relation=%#v argument=%#v", relation, argument)
		}
		seenArguments[argument.ID], seenCallbacks[relation.ToIDs[0]] = true, true
	}
	if calls != 2 || len(seenArguments) != 2 || len(seenCallbacks) != 2 {
		t.Fatalf("chained callbacks: calls=%d arguments=%d callbacks=%d", calls, len(seenArguments), len(seenCallbacks))
	}
}

func TestCumulativePythonRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	repositoryPath, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatalf("discover cumulative Python fixture: %v", err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("validate cumulative Python target catalog: %v", err)
	}
	target := pythonFixtureTarget(t, catalog)
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatalf("build Python ProgramIndex: %v", err)
	}
	if len(indexes) != 1 {
		t.Fatalf("Python ProgramIndex count = %d, want one exact target", len(indexes))
	}
	index := indexes[0]
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "python" || index.Target.Selector != pythonFixtureSelector {
		t.Fatalf("Python ProgramIndex target = %#v", index.Target)
	}
	// pyproject.toml's [project.scripts] installs repomap-fixture.
	if !slices.Equal(index.Target.Executables, []string{"repomap-fixture"}) {
		t.Fatalf("Python executables = %v", index.Target.Executables)
	}
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedCallable {
		t.Fatalf("Python script target seeds = %#v, want one exact callable seed", index.Target.Seeds)
	}
	seed := programIndexObjectByID(index, index.Target.Seeds[0].ObjectID)
	if seed.ID == "" || seed.Kind != programindex.ObjectFunction || seed.Name != "main" {
		t.Fatalf("Python script seed object = %#v, want exact main function", seed)
	}
	if len(index.Target.Exports) != 0 {
		t.Fatalf("a script exports %d callables", len(index.Target.Exports))
	}
	assertCumulativePythonSemanticFacts(t, index)
	assertPythonLocalHTTPNameFacts(t, index)
	assertPythonHTTPRegistrations(t, repository, index)
	adaptertest.AssertQueryOccurrenceOwners(t, repositoryPath, repository, index, "src/fixture_app/data_sources.py")
	adaptertest.AssertSQLQueryFacts(t, index, "src/fixture_app/data_sources.py", map[string]string{"SELECT id FROM direct_rows": "direct_rows"})
	adaptertest.AssertSQLQueryFacts(t, index, "src/fixture_app/sql_literals.py", nil, "create %s dir")
	assertPythonRepeatedImportAliases(t, index)
	assertPythonStoredCallbacks(t, index)
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatalf("build Python atlas: %v", err)
	}
	adaptertest.AssertExecutionScope(t, index, graph, "src/fixture_app/events.py", 13, programindex.ObjectModule)
	// A module-level assignment's call is the module body's, the equivalent
	// of a Go package-level variable's initializer (GO).
	adaptertest.AssertExecutionScope(t, index, graph, "src/fixture_app/cli.py", 44, programindex.ObjectModule)
	adaptertest.AssertCallControls(t, index, graph, "src/fixture_app/events.py", "process_pending_jobs", map[int][]adaptertest.Control{
		33: nil,
		35: {{Line: 34, Kind: "while body with constant true condition"}},
		40: {{Line: 39, Kind: "for body"}},
		47: nil,
		54: {{Line: 53, Kind: "async for body"}},
	})
	adaptertest.AssertRegistrationArgument(t, graph, "src/fixture_app/events.py", "handle_order", map[int]string{13: "orders.created", 21: "orders.direct"})
	for _, want := range []struct {
		name      string
		signature string
		line      int
	}{
		{name: "GetLevelsInfoResponse", signature: "count: int", line: 2},
		{name: "OtherResponse", signature: "count: str", line: 6},
	} {
		t.Run(want.name+" owns its count field", func(t *testing.T) {
			owner := programIndexObjectNamed(t, index, programindex.ObjectType, want.name, "src/fixture_app/models.py")
			if owner.Signature != "class "+want.name {
				t.Fatalf("Python type lost its written class kind: %+v", owner)
			}
			var field programindex.Object
			for _, object := range index.Objects {
				if object.Name == "count" && object.OwnerID == owner.ID {
					if field.ID != "" {
						t.Fatalf("%s has duplicate count declarations", want.name)
					}
					field = object
				}
			}
			if field.ID == "" || field.Kind != programindex.ObjectVariable || field.ContainerID != owner.ID ||
				field.Signature != want.signature || field.Location == nil ||
				field.Location.Path != "src/fixture_app/models.py" || field.Location.Line != want.line || field.Location.Column != 5 {
				t.Fatalf("%s lost its count declaration, syntax or exact location: %#v", want.name, field)
			}
			for _, place := range graph.Places {
				if place.Symbol == nil || place.Symbol.Decl.ObjectID != index.Target.ID+"."+owner.ID {
					continue
				}
				members := place.Symbol.Members
				if len(members) != 1 || members[0].Decl.ObjectID != index.Target.ID+"."+field.ID ||
					members[0].Decl.Signature != want.signature || members[0].Path != field.Location.Path ||
					members[0].Decl.LineNo != want.line || members[0].Decl.Column != 5 {
					t.Fatalf("%s atlas membership differs from its native field: %#v", want.name, members)
				}
			}
			for _, chunk := range lines.QuestionRows(graph) {
				for ref, anchor := range chunk.Anchors {
					if anchor.SubjectID != index.Target.ID+"."+owner.ID {
						continue
					}
					for _, evidence := range chunk.Row.Fields {
						if evidence.Name != "evidence" {
							continue
						}
						for _, row := range evidence.Value.([]map[string]any) {
							if row["ref"] != ref {
								continue
							}
							if row["signature"] != "class "+want.name {
								t.Fatalf("Python question lost its class kind: %+v", row)
							}
							members, ok := row["owned_declarations"].([]map[string]any)
							if !ok || len(members) != 1 || members[0]["name"] != "count" ||
								members[0]["signature"] != want.signature || members[0]["path"] != field.Location.Path || members[0]["line"] != want.line {
								t.Fatalf("%s question evidence lost or mixed its count declaration: %#v", want.name, row)
							}
							return
						}
					}
				}
			}
			t.Fatalf("%s did not reach question evidence", want.name)
		})
	}

	fixturePackage := programIndexObjectNamed(
		t, index, programindex.ObjectPackage, "fixture_app", "src/fixture_app/__init__.py",
	)
	testsPackage := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "tests.__init__", "tests/__init__.py",
	)
	consumer := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "tests.test_facade", "tests/test_facade.py",
	)
	assertExactPythonImportBoundary(
		t, index, consumer.ID, fixturePackage.ID, "fixture_app.main",
	)
	assertExactPythonImportBoundary(
		t, index, consumer.ID, testsPackage.ID, "tests.runtime",
	)

	dependencyCatalog, err := pythondependencies.Build(index)
	if err != nil {
		t.Fatalf("build cumulative Python dependency catalog: %v", err)
	}
	if dependencyCatalog.Coverage.State != dependencies.CoverageComplete ||
		len(dependencyCatalog.Coverage.Omissions) != 0 {
		t.Fatalf("Python dependency coverage = %#v, want complete", dependencyCatalog.Coverage)
	}
}

func assertPythonRepeatedImportAliases(t *testing.T, index programindex.Index) {
	t.Helper()
	const path = "src/fixture_app/models.py"
	module := programIndexObjectNamed(t, index, programindex.ObjectModule, "fixture_app.models", path)
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "parse_alias_inputs", path)
	loads := programIndexExternalObjectNamed(t, index, "json.loads")
	jsonModule := programIndexExternalObjectNamed(t, index, "json")
	imports, calls := 0, 0
	callLines := make(map[int]bool)
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path {
			continue
		}
		if relation.Kind == programindex.RelationImports && relation.FromID == module.ID &&
			(sameSingleID(relation.ToIDs, loads.ID) || sameSingleID(relation.ToIDs, jsonModule.ID)) {
			imports++
			if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
				(relation.ToIDs[0] != loads.ID && relation.ToIDs[0] != jsonModule.ID) ||
				relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 ||
				relation.WitnessesObserved != 1 || relation.WitnessesOmitted != 0 || len(relation.Witnesses) != 1 {
				t.Fatalf("repeated import aliases became missing evidence: %#v", relation)
			}
		}
		if relation.Kind == programindex.RelationInvokesExternal && relation.FromID == caller.ID {
			calls++
			callLines[relation.Location.Line] = true
			if !sameSingleID(relation.ToIDs, loads.ID) || relation.Resolution != programindex.ResolutionExact ||
				relation.WitnessesObserved != 1 || relation.WitnessesOmitted != 0 || len(relation.Witnesses) != 1 {
				t.Fatalf("import alias lost its independently anchored call: %#v", relation)
			}
		}
	}
	if imports != 3 || calls != 6 || len(callLines) != 6 {
		t.Fatalf("import aliases have %d import sites and %d calls at %d locations; want 3, 6, 6", imports, calls, len(callLines))
	}
	if index.Coverage.WitnessesOmitted != 0 {
		t.Fatalf("duplicate import aliases made the index incomplete: %#v", index.Coverage)
	}
}

func assertCumulativePythonSemanticFacts(t *testing.T, index programindex.Index) {
	t.Helper()
	main := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "main", "src/fixture_app/cli.py",
	)
	app := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "app", "src/fixture_app/cli.py",
	)
	getLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "get_level", "src/fixture_app/cli.py",
	)
	dynamicLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "dynamic_level", "src/fixture_app/cli.py",
	)
	dynamicPath := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "dynamic_path", "src/fixture_app/cli.py",
	)
	reassignedLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "reassigned_level", "src/fixture_app/cli.py",
	)
	retrieveLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "retrieve_level", "src/fixture_app/levels.py",
	)
	fetchLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "fetch_level", "src/fixture_app/levels.py",
	)
	eventsModule := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "fixture_app.events", "src/fixture_app/events.py",
	)
	consumer := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "consumer", "src/fixture_app/events.py",
	)
	handleOrder := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "handle_order", "src/fixture_app/events.py",
	)
	subscribeDynamic := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "subscribe_dynamic", "src/fixture_app/events.py",
	)
	subscribeDirect := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "subscribe_direct", "src/fixture_app/events.py",
	)
	runtimeConsumer := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "runtime_consumer", "src/fixture_app/events.py",
	)
	topic := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "topic", "src/fixture_app/events.py",
	)
	callback := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "callback", "src/fixture_app/events.py",
	)
	bindDuplicateCallbacks := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "bind_duplicate_callbacks", "src/fixture_app/events.py",
	)

	fastAPI := programIndexExternalObjectNamed(t, index, "fastapi.FastAPI")
	uvicornRun := programIndexExternalObjectNamed(t, index, "uvicorn.run")
	httpxGet := programIndexExternalObjectNamed(t, index, "httpx.get")
	kafkaConsumer := programIndexExternalObjectNamed(t, index, "kafka.KafkaConsumer")
	for _, external := range []programindex.Object{fastAPI, uvicornRun, httpxGet, kafkaConsumer} {
		if external.External == nil || external.External.AuthorityKind != programindex.ExternalAuthorityPackage {
			t.Fatalf("cumulative Python package authority = %#v", external)
		}
	}

	route := pythonRelation(
		t, index, programindex.RelationDecorates, getLevel.ID, "", programindex.ResolutionUnresolved,
	)
	routePattern := singlePythonPattern(t, route)
	if routePattern.Form != programindex.PatternDecoratorCall || routePattern.Selector != "get" ||
		routePattern.ReceiverID != app.ID ||
		routePattern.ReceiverOriginResolution != programindex.ResolutionExact ||
		!sameSingleID(routePattern.ReceiverOriginIDs, fastAPI.ID) ||
		routePattern.ReceiverOriginsObserved != 1 || routePattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python HTTP decorator pattern = %#v", routePattern)
	}
	routePath := pythonPatternArgument(t, routePattern, 1)
	if routePath.Kind != programindex.PatternLiteralString ||
		routePath.Value != "/api/level/{level_id}" || routePath.ObjectsObserved != 0 ||
		len(routePath.ObjectIDs) != 0 {
		t.Fatalf("cumulative Python HTTP route path = %#v", routePath)
	}

	dynamicRoute := pythonRelation(
		t, index, programindex.RelationDecorates, dynamicLevel.ID, "", programindex.ResolutionUnresolved,
	)
	dynamicRoutePattern := singlePythonPattern(t, dynamicRoute)
	dynamicRoutePath := pythonPatternArgument(t, dynamicRoutePattern, 1)
	if dynamicRoutePattern.Selector != "get" || dynamicRoutePath.Kind != programindex.PatternDynamic ||
		dynamicRoutePath.Resolution != programindex.ResolutionExact ||
		!sameSingleID(dynamicRoutePath.ObjectIDs, dynamicPath.ID) ||
		dynamicRoutePath.ValueCandidatesObserved != 1 || dynamicRoutePath.ValueCandidatesOmitted != 0 ||
		len(dynamicRoutePath.ValueCandidates) != 1 {
		t.Fatalf("cumulative Python initializer-backed route = %#v", dynamicRoutePattern)
	}
	dynamicValue := dynamicRoutePath.ValueCandidates[0]
	if dynamicValue.Kind != programindex.PatternLiteralString || dynamicValue.Value != "/api/dynamic" ||
		dynamicValue.Resolution != programindex.PatternValuePossible ||
		dynamicValue.SourceKind != programindex.PatternValueSourceInitializer ||
		!sameSingleID(dynamicValue.SourceObjectIDs, dynamicPath.ID) ||
		dynamicValue.SourceObjectsObserved != 1 || dynamicValue.SourceObjectsOmitted != 0 {
		t.Fatalf("cumulative Python initializer value = %#v", dynamicValue)
	}

	reassignedRoute := pythonRelation(
		t, index, programindex.RelationDecorates, reassignedLevel.ID, "", programindex.ResolutionUnresolved,
	)
	reassignedRoutePath := pythonPatternArgument(t, singlePythonPattern(t, reassignedRoute), 1)
	if reassignedRoutePath.Kind != programindex.PatternDynamic ||
		reassignedRoutePath.ValueCandidatesObserved != 0 || reassignedRoutePath.ValueCandidatesOmitted != 0 ||
		len(reassignedRoutePath.ValueCandidates) != 0 || len(reassignedRoutePath.ObjectIDs) != 1 {
		t.Fatalf("cumulative Python reassignment did not fail closed = %#v", reassignedRoutePath)
	}
	reassignedSource := programIndexObjectByID(index, reassignedRoutePath.ObjectIDs[0])
	if reassignedSource.ID == "" || reassignedSource.Kind != programindex.ObjectVariable ||
		reassignedSource.Name != "reassigned_path" || reassignedSource.Location == nil ||
		reassignedSource.Location.Path != "src/fixture_app/cli.py" || reassignedSource.Location.Line != 57 {
		t.Fatalf("cumulative Python reassigned source = %#v", reassignedSource)
	}

	bootstrap := pythonRelation(
		t, index, programindex.RelationInvokesExternal, main.ID, uvicornRun.ID,
		programindex.ResolutionExact,
	)
	bootstrapPattern := singlePythonPattern(t, bootstrap)
	bootstrapApp := pythonPatternArgument(t, bootstrapPattern, 1)
	if bootstrapPattern.Form != programindex.PatternCall || bootstrapPattern.Selector != "run" ||
		bootstrapApp.Kind != programindex.PatternDynamic ||
		bootstrapApp.Resolution != programindex.ResolutionExact ||
		!sameSingleID(bootstrapApp.ObjectIDs, app.ID) || bootstrapApp.ObjectsObserved != 1 ||
		bootstrapApp.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python server bootstrap pattern = %#v", bootstrapPattern)
	}

	assertPythonRelation(
		t, index, programindex.RelationCalls, getLevel.ID, retrieveLevel.ID,
		programindex.ResolutionExact,
	)
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, getLevel.ID, fetchLevel.ID,
		programindex.ResolutionExact,
	)
	assertPythonRelation(
		t, index, programindex.RelationCalls, handleOrder.ID, retrieveLevel.ID,
		programindex.ResolutionExact,
	)
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, handleOrder.ID, fetchLevel.ID,
		programindex.ResolutionExact,
	)

	// retrieve_level calls its parameter loader, and every call into it
	// hands fetch_level there: the call runs fetch_level, a function value.
	loaderCall := pythonRelation(
		t, index, programindex.RelationCalls, retrieveLevel.ID, fetchLevel.ID, programindex.ResolutionExact,
	)
	loaderPattern := singlePythonPattern(t, loaderCall)
	if loaderPattern.Selector != "loader" || loaderCall.Dispatch != programindex.DispatchFunctionValue ||
		loaderCall.TargetsObserved != 1 || loaderCall.TargetsOmitted != 0 {
		t.Fatalf("cumulative Python callback invocation through a parameter = %#v", loaderCall)
	}

	outbound := pythonRelation(
		t, index, programindex.RelationInvokesExternal, fetchLevel.ID, httpxGet.ID,
		programindex.ResolutionExact,
	)
	outboundPattern := singlePythonPattern(t, outbound)
	outboundURL := pythonPatternArgument(t, outboundPattern, 1)
	if outboundPattern.Selector != "get" || outboundURL.Kind != programindex.PatternStringTemplate ||
		len(outboundURL.Parts) != 2 ||
		outboundURL.Parts[0].Kind != programindex.PatternPartLiteral ||
		outboundURL.Parts[0].Text != "https://catalog.example/levels/" ||
		outboundURL.Parts[1].Kind != programindex.PatternPartHole {
		t.Fatalf("cumulative Python outbound HTTP pattern = %#v", outboundPattern)
	}

	// A method on a value produced by an outside constructor is that
	// class's method.
	kafkaSubscribe := programIndexExternalObjectNamed(t, index, "kafka.KafkaConsumer.subscribe")
	subscription := pythonRelation(
		t, index, programindex.RelationInvokesExternal, eventsModule.ID, kafkaSubscribe.ID, programindex.ResolutionExact,
	)
	subscriptionPattern := singlePythonPattern(t, subscription)
	if subscriptionPattern.Selector != "subscribe" || subscriptionPattern.ReceiverID != consumer.ID ||
		subscriptionPattern.ReceiverOriginResolution != programindex.ResolutionExact ||
		!sameSingleID(subscriptionPattern.ReceiverOriginIDs, kafkaConsumer.ID) ||
		subscriptionPattern.ReceiverOriginsObserved != 1 ||
		subscriptionPattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python consumer subscription pattern = %#v", subscriptionPattern)
	}
	subscriptionTopic := pythonPatternArgument(t, subscriptionPattern, 1)
	subscriptionHandler := pythonPatternArgument(t, subscriptionPattern, 2)
	if subscriptionTopic.Kind != programindex.PatternLiteralString ||
		subscriptionTopic.Value != "orders.created" ||
		subscriptionHandler.Kind != programindex.PatternDynamic ||
		subscriptionHandler.Resolution != programindex.ResolutionExact ||
		!sameSingleID(subscriptionHandler.ObjectIDs, handleOrder.ID) ||
		subscriptionHandler.ObjectsObserved != 1 || subscriptionHandler.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python consumer subscription arguments = %#v", subscriptionPattern.Arguments)
	}
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, eventsModule.ID, handleOrder.ID,
		programindex.ResolutionExact,
	)

	dynamicSubscription := pythonRelation(
		t, index, programindex.RelationCalls, subscribeDynamic.ID, "", programindex.ResolutionUnresolved,
	)
	dynamicPattern := singlePythonPattern(t, dynamicSubscription)
	if dynamicPattern.Selector != "subscribe" || dynamicPattern.ReceiverID != runtimeConsumer.ID ||
		len(dynamicPattern.ReceiverOriginIDs) != 0 || dynamicPattern.ReceiverOriginsObserved != 0 ||
		dynamicPattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python dynamic subscription receiver = %#v", dynamicPattern)
	}
	dynamicTopic := pythonPatternArgument(t, dynamicPattern, 1)
	dynamicCallback := pythonPatternArgument(t, dynamicPattern, 2)
	if dynamicTopic.Kind != programindex.PatternDynamic ||
		dynamicTopic.Resolution != programindex.ResolutionExact ||
		!sameSingleID(dynamicTopic.ObjectIDs, topic.ID) || dynamicTopic.ObjectsObserved != 1 ||
		dynamicTopic.ObjectsOmitted != 0 ||
		dynamicCallback.Kind != programindex.PatternDynamic ||
		dynamicCallback.Resolution != programindex.ResolutionExact ||
		!sameSingleID(dynamicCallback.ObjectIDs, callback.ID) || dynamicCallback.ObjectsObserved != 1 ||
		dynamicCallback.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python dynamic subscription arguments = %#v", dynamicPattern.Arguments)
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == subscribeDynamic.ID {
			t.Fatalf("dynamic callback parameter gained callable authority: %#v", relation)
		}
	}

	directFactory := pythonRelation(
		t, index, programindex.RelationInvokesExternal, subscribeDirect.ID, kafkaConsumer.ID,
		programindex.ResolutionExact,
	)
	directFactoryPattern := singlePythonPattern(t, directFactory)
	if directFactoryPattern.Selector != "KafkaConsumer" || directFactoryPattern.ResultID == "" ||
		directFactoryPattern.Location == nil ||
		directFactoryPattern.Location.Path != "src/fixture_app/events.py" ||
		directFactoryPattern.Location.Line != 21 || directFactoryPattern.Location.Column != 5 {
		t.Fatalf("direct Python factory pattern = %#v", directFactoryPattern)
	}
	directResult := programIndexObjectByID(index, directFactoryPattern.ResultID)
	if directResult.ID == "" || directResult.Kind != programindex.ObjectVariable ||
		directResult.Name != "call result" || directResult.Location == nil ||
		directResult.Location.Path != "src/fixture_app/events.py" ||
		directResult.Location.Line != 21 || directResult.Location.Column != 5 {
		t.Fatalf("direct Python factory result object = %#v", directResult)
	}
	// A result another call acts on, or takes as an argument, is an object
	// placed where its call expression starts: the direct factory (21:5), the
	// first map of the chained lambdas (models.py:60:12), the route_path calls
	// LevelRoutes' decorators take (models.py:236:9, 238:13), and the chains of
	// subscribe_chained and chained_text_calls. KafkaConsumer() and the first
	// subscribe of line 64 both start at 64:5. Patterns and call_result
	// anchors use the attribute name instead, so calls stay apart there.
	callResults := map[string]int{}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectVariable && object.Name == "call result" && object.Location != nil &&
			(object.Location.Path == "src/fixture_app/events.py" || object.Location.Path == "src/fixture_app/models.py") {
			callResults[fmt.Sprintf("%s:%d:%d", object.Location.Path, object.Location.Line, object.Location.Column)]++
		}
	}
	wantResults := map[string]int{"src/fixture_app/events.py:21:5": 1, "src/fixture_app/models.py:60:12": 1,
		"src/fixture_app/models.py:236:9": 1, "src/fixture_app/models.py:238:13": 1,
		"src/fixture_app/events.py:64:5": 2, "src/fixture_app/events.py:68:27": 1, "src/fixture_app/events.py:69:16": 1}
	if !reflect.DeepEqual(callResults, wantResults) {
		t.Fatalf("Python callback source call-result objects = %v, want %v", callResults, wantResults)
	}

	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python decorator registration",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationDecorates, FromID: getLevel.ID, Resolution: programindex.ResolutionUnresolved,
			Path: "src/fixture_app/cli.py", Line: 17,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, WitnessesOmitted: 0,
			PatternsObserved: 1, PatternsOmitted: 0,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternDecoratorCall, Selector: "get", ReceiverID: app.ID,
				Path: "src/fixture_app/cli.py", Line: 17,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{fastAPI.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
				},
				Observed: 1, Arguments: []adaptertest.Argument{{
					Position: 1, Kind: programindex.PatternLiteralString, Value: "/api/level/{level_id}",
				}},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python callback registration",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: eventsModule.ID, ToIDs: []string{kafkaSubscribe.ID}, Resolution: programindex.ResolutionExact,
			Path: "src/fixture_app/events.py", Line: 13,
			TargetsObserved: 1, TargetsOmitted: 0, WitnessesObserved: 1, WitnessesOmitted: 0,
			PatternsObserved: 1, PatternsOmitted: 0,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: consumer.ID,
				Path: "src/fixture_app/events.py", Line: 13,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{kafkaConsumer.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
				},
				Observed: 2, Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternLiteralString, Value: "orders.created"},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{{
			ArgumentPosition: 2,
			Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: eventsModule.ID, ToIDs: []string{handleOrder.ID},
				Resolution: programindex.ResolutionExact, Path: "src/fixture_app/events.py", Line: 13,
				TargetsObserved: 1, WitnessesObserved: 1,
			},
		}},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python template output call",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: fetchLevel.ID, ToIDs: []string{httpxGet.ID},
			Resolution: programindex.ResolutionExact,
			Path:       "src/fixture_app/levels.py", Line: 5,
			TargetsObserved: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "get", Observed: 1,
				Path: "src/fixture_app/levels.py", Line: 5,
				Arguments: []adaptertest.Argument{{
					Position: 1, Kind: programindex.PatternStringTemplate,
					Parts: []programindex.PatternPart{
						{Kind: programindex.PatternPartLiteral, Text: "https://catalog.example/levels/"},
						{Kind: programindex.PatternPartHole},
					},
				}},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python dynamic callback frontier",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationCalls, FromID: subscribeDynamic.ID, Resolution: programindex.ResolutionUnresolved,
			Path: "src/fixture_app/events.py", Line: 17,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: runtimeConsumer.ID, Observed: 2,
				Path: "src/fixture_app/events.py", Line: 17,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{topic.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{callback.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
				},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python direct-result callback registration",
		// A call on an outside call's direct result is that symbol's member.
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: subscribeDirect.ID,
			ToIDs:      []string{programIndexExternalObjectNamed(t, index, "kafka.KafkaConsumer.subscribe").ID},
			Resolution: programindex.ResolutionExact,
			Path:       "src/fixture_app/events.py", Line: 21,
			TargetsObserved: 1, TargetsOmitted: 0, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: directResult.ID,
				// The registration addresses the terminal selector; its receiver
				// remains the factory's result at column 5 on this same line.
				Path: "src/fixture_app/events.py", Line: 21, Column: 21,
				Observed: 2,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternLiteralString, Value: "orders.direct"},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{{
			ArgumentPosition: 2,
			Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: subscribeDirect.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact,
				Path: "src/fixture_app/events.py", Line: 21,
				TargetsObserved: 1, WitnessesObserved: 1,
			},
		}},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python duplicate callback arguments",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: bindDuplicateCallbacks.ID,
			ToIDs: []string{programIndexExternalObjectNamed(t, index, "kafka.KafkaConsumer.bind_pair").ID}, Resolution: programindex.ResolutionExact,
			Path: "src/fixture_app/events.py", Line: 25,
			TargetsObserved: 1, TargetsOmitted: 0, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "bind_pair", ReceiverID: consumer.ID,
				Path: "src/fixture_app/events.py", Line: 25,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{kafkaConsumer.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
				},
				Observed: 2,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{
			{ArgumentPosition: 1, Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: bindDuplicateCallbacks.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact,
				Path: "src/fixture_app/events.py", Line: 25, TargetsObserved: 1, WitnessesObserved: 1,
			}},
			{ArgumentPosition: 2, Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: bindDuplicateCallbacks.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionExact,
				Path: "src/fixture_app/events.py", Line: 25, TargetsObserved: 1, WitnessesObserved: 1,
			}},
		},
		RequireComplete: true,
	})
}

// Python already puts each call at its attribute name, so chained and nested
// calls on one line keep their own facts and boundary places; this is the
// equivalent of the TypeScript chained-call check.
func TestCumulativePythonChainedCallsKeepTheirOwnPositions(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	input, err := sharedPythonFixtureInput(t, repository, pythonFixtureTarget(t, catalog))
	if err != nil {
		t.Fatal(err)
	}
	// KafkaConsumer().subscribe("orders.chained", handle_order).subscribe("orders.chained", record_order)
	// normalized = "/".join(path.split("/"))  -- join takes no literal, so no fact
	// repeated = name.replace("/", "-").replace("/", "-")
	// head = path.split("/")[0].split("/")
	// A call on a call's result continues the outside symbol that call
	// names (kafka.KafkaConsumer.subscribe.subscribe); the parser knows no
	// type for the untyped parameters, so their calls name none. Each is
	// its own fact at its own attribute name.
	adaptertest.AssertCallSiteBoundaries(t, repository, input, "src/fixture_app/events.py", []adaptertest.CallSite{
		{Line: 64, Column: 21, Key: "subscribe", Text: "kafka.KafkaConsumer.subscribe", Path: "orders.chained", Symbol: "handle_order"},
		{Line: 64, Column: 63, Key: "subscribe", Text: "kafka.KafkaConsumer.subscribe.subscribe", Path: "orders.chained", Symbol: "record_order"},
		{Line: 68, Column: 32, Key: "split", Path: "/"},
		{Line: 69, Column: 21, Key: "replace", Path: "/"},
		{Line: 69, Column: 39, Key: "replace", Path: "/"},
		{Line: 70, Column: 17, Key: "split", Path: "/"},
		{Line: 70, Column: 31, Key: "split", Path: "/"},
	})
}

func pythonFixtureTarget(t *testing.T, catalog pythontarget.Catalog) pythontarget.Target {
	t.Helper()
	for _, target := range catalog.Entries {
		if target.Selector == pythonFixtureSelector {
			return target
		}
	}
	t.Fatalf("Python fixture target %q is absent from catalog %#v", pythonFixtureSelector, catalog.Entries)
	return pythontarget.Target{}
}

func programIndexObjectByID(index programindex.Index, id string) programindex.Object {
	for _, object := range index.Objects {
		if object.ID == id {
			return object
		}
	}
	return programindex.Object{}
}

func programIndexObjectNamed(
	t *testing.T,
	index programindex.Index,
	kind programindex.ObjectKind,
	name string,
	path string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name && object.Location != nil &&
			object.Location.Path == path {
			return object
		}
	}
	t.Fatalf("Python ProgramIndex has no %s %q at %q", kind, name, path)
	return programindex.Object{}
}

func assertExactPythonImportBoundary(
	t *testing.T,
	index programindex.Index,
	fromID string,
	toID string,
	detail string,
) {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationImports || relation.FromID != fromID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			relation.ToIDs[0] != toID {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Kind == "from_import_module_boundary" && witness.Detail == detail {
				return
			}
		}
	}
	t.Fatalf("Python ProgramIndex has no exact module boundary %q from %q to %q", detail, fromID, toID)
}

func programIndexExternalObjectNamed(
	t *testing.T,
	index programindex.Index,
	name string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && object.Name == name {
			return object
		}
	}
	t.Fatalf("Python ProgramIndex has no external symbol %q", name)
	return programindex.Object{}
}

func assertPythonRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	resolution programindex.Resolution,
) {
	t.Helper()
	_ = pythonRelation(t, index, kind, fromID, toID, resolution)
}

func pythonRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	resolution programindex.Resolution,
) programindex.Relation {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind != kind || relation.FromID != fromID || relation.Resolution != resolution {
			continue
		}
		if (toID == "" && len(relation.ToIDs) == 0) || sameSingleID(relation.ToIDs, toID) {
			return relation
		}
	}
	var seen []string
	for _, relation := range index.Relations {
		if relation.FromID == fromID {
			seen = append(seen, fmt.Sprintf("%s %s %v", relation.Kind, relation.Resolution, relation.ToIDs))
		}
	}
	t.Fatalf(
		"Python ProgramIndex has no %s relation from %q to %q with %s resolution; relations from it: %v",
		kind, fromID, toID, resolution, seen,
	)
	return programindex.Relation{}
}

func singlePythonPattern(t *testing.T, relation programindex.Relation) programindex.RelationPattern {
	t.Helper()
	if relation.PatternsObserved != 1 || relation.PatternsOmitted != 0 || len(relation.Patterns) != 1 {
		t.Fatalf("Python relation pattern coverage = %#v", relation)
	}
	return relation.Patterns[0]
}

func pythonPatternArgument(
	t *testing.T,
	pattern programindex.RelationPattern,
	position int,
) programindex.PatternArgument {
	t.Helper()
	for _, argument := range pattern.Arguments {
		if argument.Position == position {
			return argument
		}
	}
	t.Fatalf("Python pattern %q has no positional argument %d: %#v", pattern.ID, position, pattern.Arguments)
	return programindex.PatternArgument{}
}

func sameSingleID(values []string, want string) bool {
	return len(values) == 1 && values[0] == want
}
