package report

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramViewObjectPreservesTypedExternalAuthority(t *testing.T) {
	object := programindex.Object{
		ID: "program-object-typed", SourceRef: "external", Kind: programindex.ObjectExternalSymbol,
		Name: "net/http.Client.Do", Visibility: programindex.VisibilityPublic,
		External: &programindex.ExternalSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Do"},
	}
	view := programViewObject(object)
	if view.External == nil || *view.External != *object.External {
		t.Fatalf("external authority = %#v, want %#v", view.External, object.External)
	}
	object.External.Name = "mutated"
	if view.External.Name != "Do" {
		t.Fatal("program view retained an alias to ProgramIndex external authority")
	}
}

func TestProgramViewResolvesSeedsAndKeepsClosedBoundedRelations(t *testing.T) {
	index := programViewIndexFixture(t)
	full, err := NewProgramView(index)
	if err != nil {
		t.Fatalf("NewProgramView: %v", err)
	}
	if err := full.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if full.TargetID != index.Target.ID || full.IndexSHA256 != index.SHA256 ||
		full.IndexCoverage != index.Coverage {
		t.Fatalf("index binding was not preserved: %#v", full)
	}
	if got, want := full.Projection, (ProgramViewProjectionCounts{
		Seeds:     ProgramViewCollectionCounts{Eligible: 1, Shown: 1, Omitted: 0},
		Objects:   ProgramViewCollectionCounts{Eligible: 6, Shown: 6, Omitted: 0},
		Relations: ProgramViewCollectionCounts{Eligible: 2, Shown: 2, Omitted: 0},
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("full projection counts = %#v, want %#v", got, want)
	}

	seedObject := programViewFixtureObject(t, index, "method-run")
	ownerObject := programViewFixtureObject(t, index, "type-worker")
	moduleObject := programViewFixtureObject(t, index, "module-app")
	seed := full.Seeds[0]
	if seed.ObjectID != seedObject.ID || seed.Kind != programindex.SeedCallable ||
		seed.Name != seedObject.Name || seed.ObjectKind != seedObject.Kind ||
		seed.Signature != seedObject.Signature || seed.Visibility != seedObject.Visibility ||
		seed.OwnerID != ownerObject.ID || seed.ContainerID != moduleObject.ID ||
		!equalProgramViewLocations(seed.LaunchLocation, index.Target.Seeds[0].Location) ||
		!equalProgramViewLocations(seed.DeclarationLocation, seedObject.Location) {
		t.Fatalf("resolved seed = %#v", seed)
	}

	limited, err := projectProgramView(index, programViewLimits{
		Seeds: 4, Objects: 4, Relations: 4, TextBytes: maxProgramViewTextBytes,
	})
	if err != nil {
		t.Fatalf("bounded projection: %v", err)
	}
	wantClosure := map[string]bool{
		seedObject.ID:   true,
		ownerObject.ID:  true,
		moduleObject.ID: true,
		programViewFixtureObject(t, index, "package-app").ID: true,
	}
	if len(limited.Objects) != len(wantClosure) {
		t.Fatalf("bounded objects = %#v", limited.Objects)
	}
	for _, object := range limited.Objects {
		if !wantClosure[object.ID] {
			t.Fatalf("bounded projection selected object outside seed closure: %#v", object)
		}
	}
	if got, want := limited.Projection.Objects, (ProgramViewCollectionCounts{Eligible: 6, Shown: 4, Omitted: 2}); got != want {
		t.Fatalf("bounded object counts = %#v, want %#v", got, want)
	}
	if got, want := limited.Projection.Relations, (ProgramViewCollectionCounts{Eligible: 2, Shown: 1, Omitted: 1}); got != want {
		t.Fatalf("bounded relation counts = %#v, want %#v", got, want)
	}
	if len(limited.Relations) != 1 || limited.Relations[0].Resolution != programindex.ResolutionUnresolved ||
		limited.Relations[0].FromID != seedObject.ID || len(limited.Relations[0].ToIDs) != 0 ||
		limited.Relations[0].TargetsObserved != 2 || limited.Relations[0].TargetsIndexed != 0 ||
		limited.Relations[0].TargetsOmitted != 2 || limited.Relations[0].WitnessesObserved != 1 ||
		limited.Relations[0].WitnessesIndexed != 1 || limited.Relations[0].WitnessesOmitted != 0 ||
		len(limited.Relations[0].Witnesses) != 1 ||
		limited.Relations[0].Witnesses[0].Detail != "runtime handler name" ||
		limited.Relations[0].WitnessesProjectionOmitted != 0 {
		t.Fatalf("unresolved relation projection = %#v", limited.Relations)
	}

	if _, err := projectProgramView(index, programViewLimits{
		Seeds: 4, Objects: 3, Relations: 4, TextBytes: maxProgramViewTextBytes,
	}); err == nil || !strings.Contains(err.Error(), "target seeds require") {
		t.Fatalf("mandatory closure error = %v", err)
	}

	tampered := *limited
	tampered.Relations = append([]ProgramViewRelation(nil), limited.Relations...)
	tampered.Relations[0].ToIDs = []string{seedObject.ID}
	tampered.Relations[0].TargetsIndexed = 1
	tampered.Relations[0].TargetsOmitted = 1
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "unresolved resolution") {
		t.Fatalf("tampered unresolved relation error = %v", err)
	}
}

func TestProgramViewRelationLimitKeepsSeedNeighborhood(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/main.py", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64),
		SourceSHA256:   strings.Repeat("d", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "app/main.py"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "seed", Kind: programindex.SeedCallable, Location: location(1),
			}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "app", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "seed", Kind: programindex.ObjectFunction, Name: "main", Visibility: programindex.VisibilityPublic, ContainerRef: "module", Location: location(1)},
			{SourceRef: "near", Kind: programindex.ObjectFunction, Name: "near", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(5)},
			{SourceRef: "far-from", Kind: programindex.ObjectFunction, Name: "farFrom", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(10)},
			{SourceRef: "far-to", Kind: programindex.ObjectFunction, Name: "farTo", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(12)},
		},
		Relations: []programindex.RelationInput{
			{SourceRef: "far-call", Kind: programindex.RelationCalls, FromRef: "far-from", ToRefs: []string{"far-to"}, Resolution: programindex.ResolutionExact, Location: location(11), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "fixture"}}, WitnessesObserved: 1},
			{SourceRef: "seed-call", Kind: programindex.RelationCalls, FromRef: "seed", ToRefs: []string{"near"}, Resolution: programindex.ResolutionExact, Location: location(2), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "fixture"}}, WitnessesObserved: 1},
		},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 5, RelationsObserved: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := projectProgramView(index, programViewLimits{
		Seeds: 2, Objects: 8, Relations: 1, TextBytes: maxProgramViewTextBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	seed := programViewFixtureObject(t, index, "seed")
	if len(view.Relations) != 1 || view.Relations[0].FromID != seed.ID {
		t.Fatalf("relation limit did not retain the seed neighborhood: %#v", view.Relations)
	}
}

func programViewIndexFixture(t *testing.T) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/main.py", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "app/main.py"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "method-run", Kind: programindex.SeedCallable, Location: location(10),
			}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "package-app", Kind: programindex.ObjectPackage, Name: "app", Visibility: programindex.VisibilityPublic, Location: location(1)},
			{SourceRef: "module-app", Kind: programindex.ObjectModule, Name: "app.main", Visibility: programindex.VisibilityPublic, ContainerRef: "package-app", Location: location(1)},
			{SourceRef: "type-worker", Kind: programindex.ObjectType, Name: "Worker", Visibility: programindex.VisibilityInternal, ContainerRef: "module-app", Location: location(5)},
			{SourceRef: "method-run", Kind: programindex.ObjectMethod, Name: "run", Signature: "run()", Visibility: programindex.VisibilityPublic, OwnerRef: "type-worker", ContainerRef: "module-app", Location: location(10)},
			{SourceRef: "function-public", Kind: programindex.ObjectFunction, Name: "serve", Signature: "serve()", Visibility: programindex.VisibilityPublic, ContainerRef: "module-app", Location: location(20)},
			{SourceRef: "function-internal", Kind: programindex.ObjectFunction, Name: "helper", Signature: "helper()", Visibility: programindex.VisibilityInternal, ContainerRef: "module-app", Location: location(30)},
		},
		Relations: []programindex.RelationInput{
			{SourceRef: "call-public", Kind: programindex.RelationCalls, FromRef: "method-run", ToRefs: []string{"function-public"}, Resolution: programindex.ResolutionExact, Invocation: "direct", Location: location(12), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "python_call", Location: location(12)}}, WitnessesObserved: 1},
			{SourceRef: "dynamic-call", Kind: programindex.RelationCalls, FromRef: "method-run", Resolution: programindex.ResolutionUnresolved, Invocation: "runtime selected", Location: location(14), TargetsObserved: 2, Witnesses: []programindex.Witness{{Kind: "dynamic_name", Detail: "runtime handler name", Location: location(14)}}, WitnessesObserved: 1},
		},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 6, RelationsObserved: 2,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func programViewFixtureObject(t *testing.T, index programindex.Index, sourceRef string) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.SourceRef == sourceRef {
			return object
		}
	}
	t.Fatalf("fixture object %q not found", sourceRef)
	return programindex.Object{}
}
