package activitypath

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

type fixedJSONProvider struct {
	response []byte
}

func (provider *fixedJSONProvider) State() []byte {
	return []byte(`{"provider":"activity-path-test"}`)
}

func (provider *fixedJSONProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (provider *fixedJSONProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{
		Response: provider.response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func TestBuildClassifiesRoutesAndRoundTripsArtifact(t *testing.T) {
	cases := []struct {
		name      string
		relations []programindex.RelationInput
		status    Status
		frontier  FrontierReason
		distance  int
		possible  int
		callbacks int
	}{
		{
			name: "exact call", status: StatusExact, distance: 1,
			relations: []programindex.RelationInput{testRelation(
				"root-caller", programindex.RelationCalls, "root", []string{"caller"}, programindex.ResolutionExact, 3,
			)},
		},
		{
			name: "alternative call", status: StatusPossible, distance: 1, possible: 1,
			relations: []programindex.RelationInput{testRelation(
				"root-caller", programindex.RelationCalls, "root", []string{"caller"}, programindex.ResolutionAlternatives, 3,
			)},
		},
		{
			name: "callback handoff", status: StatusPossible, distance: 1, possible: 1, callbacks: 1,
			relations: []programindex.RelationInput{testRelation(
				"root-caller", programindex.RelationPassesCallback, "root", []string{"caller"}, programindex.ResolutionExact, 3,
			)},
		},
		{
			name: "reachable unresolved call has no caller authority", status: StatusUnconnected,
			relations: []programindex.RelationInput{testRelation(
				"root-unknown", programindex.RelationCalls, "root", nil, programindex.ResolutionUnresolved, 3,
			)},
		},
		{
			name: "reachable decorator joint", status: StatusFrontier, frontier: FrontierDecoratorBoundary,
			relations: []programindex.RelationInput{testRelation(
				"root-decorator", programindex.RelationDecorates, "root", []string{"caller"}, programindex.ResolutionExact, 3,
			)},
		},
		{
			name: "unrelated unresolved component stays closed", status: StatusUnconnected,
			relations: []programindex.RelationInput{testRelation(
				"x-unknown", programindex.RelationCalls, "x", nil, programindex.ResolutionUnresolved, 3,
			)},
		},
		{
			name: "caller ancestor outgoing boundary cannot create an incoming path", status: StatusUnconnected,
			relations: []programindex.RelationInput{
				testRelation("x-caller", programindex.RelationCalls, "x", []string{"caller"}, programindex.ResolutionExact, 3),
				testRelation("x-unknown", programindex.RelationCalls, "x", nil, programindex.ResolutionUnresolved, 4),
			},
		},
		{name: "closed disconnected", status: StatusUnconnected},
		{
			name: "shortest path beats longer exact path", status: StatusPossible, distance: 1, possible: 1,
			relations: []programindex.RelationInput{
				testRelation("root-caller-possible", programindex.RelationCalls, "root", []string{"caller"}, programindex.ResolutionAlternatives, 3),
				testRelation("root-x", programindex.RelationCalls, "root", []string{"x"}, programindex.ResolutionExact, 4),
				testRelation("x-caller", programindex.RelationCalls, "x", []string{"caller"}, programindex.ResolutionExact, 5),
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			index, activities, integrations, uses := testInputs(t, test.relations)
			result, err := Build(index, activities, integrations, uses)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if len(result.Routes) != 1 || len(result.Outcomes) != 1 {
				t.Fatalf("routes/outcomes = %d/%d", len(result.Routes), len(result.Outcomes))
			}
			route := result.Routes[0]
			if route.Status != test.status ||
				route.Distance != test.distance || route.PossibleSteps != test.possible ||
				route.CallbackHandoffs != test.callbacks {
				t.Fatalf("route = %#v", route)
			}
			if test.frontier == "" && len(route.Frontier) != 0 ||
				test.frontier != "" && !reflect.DeepEqual(route.Frontier, []FrontierReason{test.frontier}) {
				t.Fatalf("frontier = %#v, want %q", route.Frontier, test.frontier)
			}
			encoded, err := Encode(result)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			decoded, err := Decode(encoded, index, activities, integrations, uses)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(decoded, result) {
				t.Fatal("artifact round trip changed the result")
			}
		})
	}
}

func TestRouteFrontierIsBoundToExactCallerRelationAcrossDisconnectedRegions(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/jobs.py", Line: line, Column: 1}
	}
	relations := []programindex.RelationInput{
		testRelation("root-reached", programindex.RelationCalls, "root", []string{"reached"}, programindex.ResolutionExact, 3),
		testRelation("reached-decorator", programindex.RelationDecorates, "reached", []string{"related-caller"}, programindex.ResolutionExact, 4),
		testRelation("reached-unknown", programindex.RelationCalls, "reached", nil, programindex.ResolutionUnresolved, 5),
		testRelation("other-caller", programindex.RelationCalls, "other-root", []string{"unrelated-caller"}, programindex.ResolutionExact, 12),
		testRelation("other-unknown", programindex.RelationCalls, "other-root", nil, programindex.ResolutionUnresolved, 13),
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "library", Name: "app", Selector: "library:app",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "app/jobs.py"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "root", Kind: programindex.SeedCallable, Location: location(2)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "app.jobs", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "root", Kind: programindex.ObjectFunction, Name: "root", Visibility: programindex.VisibilityPublic, ContainerRef: "module", Location: location(2)},
			{SourceRef: "reached", Kind: programindex.ObjectFunction, Name: "reached", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(3)},
			{SourceRef: "related-caller", Kind: programindex.ObjectFunction, Name: "related", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(6)},
			{SourceRef: "other-root", Kind: programindex.ObjectFunction, Name: "otherRoot", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(10)},
			{SourceRef: "unrelated-caller", Kind: programindex.ObjectFunction, Name: "unrelated", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(14)},
		},
		Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 6, RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	byName := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
		byName[object.Name] = object
	}
	prepared := inputs{index: index, activitiesSHA: strings.Repeat("c", 64), objects: objects}
	graph, err := compileGraph(prepared)
	if err != nil {
		t.Fatalf("compileGraph: %v", err)
	}
	paths := search([]programindex.Object{byName["root"]}, graph.allAdjacency)
	frontiers := graph.compileFrontierIndex(paths)

	related, err := buildRoute(prepared, paths, frontiers, byName["related"])
	if err != nil {
		t.Fatalf("related route: %v", err)
	}
	if related.Status != StatusFrontier ||
		!reflect.DeepEqual(related.Frontier, []FrontierReason{FrontierDecoratorBoundary}) {
		t.Fatalf("related route = %#v", related)
	}
	unrelated, err := buildRoute(prepared, paths, frontiers, byName["unrelated"])
	if err != nil {
		t.Fatalf("unrelated route: %v", err)
	}
	if unrelated.Status != StatusUnconnected || len(unrelated.Frontier) != 0 {
		t.Fatalf("unrelated route inherited another region's frontier: %#v", unrelated)
	}
}

func TestPossibleRouteTieBreakMinimizesUncertaintyBeforeEdgeOrder(t *testing.T) {
	relations := []programindex.RelationInput{
		testRelation("root-x", programindex.RelationCalls, "root", []string{"x"}, programindex.ResolutionAlternatives, 3),
		testRelation("x-caller", programindex.RelationCalls, "x", []string{"caller"}, programindex.ResolutionAlternatives, 4),
		testRelation("root-y", programindex.RelationCalls, "root", []string{"y"}, programindex.ResolutionExact, 5),
		testRelation("y-caller", programindex.RelationCalls, "y", []string{"caller"}, programindex.ResolutionAlternatives, 6),
	}
	index, activities, integrations, uses := testInputs(t, relations)
	result, err := Build(index, activities, integrations, uses)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	route := result.Routes[0]
	if route.Status != StatusPossible || route.Distance != 2 || route.PossibleSteps != 1 ||
		len(route.Nodes) != 3 || route.Nodes[1].Name != "y" {
		t.Fatalf("tie-broken route = %#v", route)
	}
}

func TestBuildAcceptsSeededModuleAsZeroHopActivity(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/jobs.py", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "script", Name: "jobs", Selector: "script:app/jobs.py",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "app/jobs.py"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "module", Kind: programindex.SeedMainGuard, Location: location(2)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "app.jobs", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: "acme.send", Visibility: programindex.VisibilityUnknown},
		},
		Relations: []programindex.RelationInput{externalRelation("module")},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 2, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	activities := selectActivities(t, index)
	integrations := selectedIntegration(t)
	uses := selectUses(t, index, integrations)
	result, err := Build(index, activities, integrations, uses)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].Status != StatusExact ||
		result.Routes[0].Distance != 0 || result.Routes[0].Activity == nil ||
		result.Routes[0].Activity.Kind != programindex.ObjectModule {
		t.Fatalf("module route = %#v", result.Routes)
	}
}

func testInputs(
	t *testing.T,
	traversal []programindex.RelationInput,
) (programindex.Index, activityentrypoint.Result, integrationdependency.Result, integrationusage.Result) {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/jobs.py", Line: line, Column: 1}
	}
	relations := append([]programindex.RelationInput(nil), traversal...)
	relations = append(relations, externalRelation("caller"))
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "library", Name: "app", Selector: "library:app",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "app/jobs.py"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "root", Kind: programindex.SeedCallable, Location: location(2)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "app.jobs", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "root", Kind: programindex.ObjectFunction, Name: "root", Visibility: programindex.VisibilityPublic, ContainerRef: "module", Location: location(2)},
			{SourceRef: "x", Kind: programindex.ObjectFunction, Name: "x", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(4)},
			{SourceRef: "y", Kind: programindex.ObjectFunction, Name: "y", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(6)},
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "publish", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(10)},
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: "acme.send", Visibility: programindex.VisibilityUnknown},
		},
		Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 6, RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	integrations := selectedIntegration(t)
	return index, selectActivities(t, index), integrations, selectUses(t, index, integrations)
}

func testRelation(
	ref string,
	kind programindex.RelationKind,
	from string,
	to []string,
	resolution programindex.Resolution,
	line int,
) programindex.RelationInput {
	location := &programindex.Location{Path: "app/jobs.py", Line: line, Column: 3}
	return programindex.RelationInput{
		SourceRef: ref, Kind: kind, FromRef: from, ToRefs: to, Resolution: resolution,
		Location: location, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "test_relation", Location: location}}, WitnessesObserved: 1,
	}
}

func externalRelation(from string) programindex.RelationInput {
	location := &programindex.Location{Path: "app/jobs.py", Line: 20, Column: 3}
	return programindex.RelationInput{
		SourceRef: "external-call", Kind: programindex.RelationInvokesExternal,
		FromRef: from, ToRefs: []string{"external"}, Resolution: programindex.ResolutionAlternatives,
		Invocation: "awaited", Location: location, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{
			Kind: "callsite_candidate", Detail: "human callsite evidence",
			SourceExpression: "client.send", Location: location,
		}},
		WitnessesObserved: 1,
	}
}

func selectActivities(t *testing.T, index programindex.Index) activityentrypoint.Result {
	t.Helper()
	result, err := activityentrypoint.Run(
		context.Background(), llm.Executor{Enabled: false},
		&fixedJSONProvider{response: []byte(`{"activity_refs":["a1"]}`)}, index,
	)
	if err != nil {
		t.Fatalf("activityentrypoint.Run: %v", err)
	}
	return result
}

func selectedIntegration(t *testing.T) integrationdependency.Result {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "python", Name: "app.jobs", ModulePath: "app",
		PackagePath: "app.jobs", RepositoryPath: "app",
	})
	if err != nil {
		t.Fatalf("dependencies.SealImporter: %v", err)
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{importer}, []dependencies.Dependency{{
		Language: "python", Kind: dependencies.KindExternal, Name: "acme", ModulePath: "acme",
		PackagePath: "acme", ImporterRefs: []string{importer.Ref},
	}}, nil)
	if err != nil {
		t.Fatalf("dependencies.BuildWithOmissions: %v", err)
	}
	result := integrationdependency.Result{
		Version: integrationdependency.Version, DependencyCatalogSHA256: strings.Repeat("c", 64),
		Dependencies: []integrationdependency.SelectedDependency{{
			Dependency: catalog.Dependencies[0], Importers: []dependencies.Importer{importer},
		}},
		Coverage: integrationdependency.Coverage{Observed: 1, Advertised: 1, ModelCalled: true},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("integration dependency result: %v", err)
	}
	return result
}

func selectUses(
	t *testing.T,
	index programindex.Index,
	integrations integrationdependency.Result,
) integrationusage.Result {
	t.Helper()
	result, err := integrationusage.Run(
		context.Background(), llm.Executor{Enabled: false},
		&fixedJSONProvider{response: []byte(`{"uses":[{"operation_ref":"o1","label":"Send event","mechanism":"unknown"}]}`)},
		index, integrations,
	)
	if err != nil {
		t.Fatalf("integrationusage.Run: %v", err)
	}
	return result
}
