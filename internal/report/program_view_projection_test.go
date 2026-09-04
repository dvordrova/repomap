package report

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramViewObjectPreservesTypedExternalAuthority(t *testing.T) {
	if ProgramViewVersion != 5 {
		t.Fatalf("ProgramView version = %d, want 5", ProgramViewVersion)
	}
	object := programindex.Object{
		ID: "program-object-typed", SourceRef: "external", Kind: programindex.ObjectExternalSymbol,
		Name: "net/http.Client.Do", Visibility: programindex.VisibilityPublic,
		External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPlatform, PackagePath: "net/http", Receiver: "Client", Name: "Do"},
	}
	view := programViewObject(object)
	if view.External == nil || *view.External != *object.External {
		t.Fatalf("external authority = %#v, want %#v", view.External, object.External)
	}
	object.External.Name = "mutated"
	if view.External.Name != "Do" {
		t.Fatal("program view retained an alias to ProgramIndex external authority")
	}
	missingKind := view
	missingKind.External = cloneProgramViewExternal(view.External)
	missingKind.External.AuthorityKind = ""
	if err := validateProgramViewObject(missingKind); err == nil {
		t.Fatal("program view accepted missing external authority kind")
	}
	unknownKind := view
	unknownKind.External = cloneProgramViewExternal(view.External)
	unknownKind.External.AuthorityKind = "registry"
	if err := validateProgramViewObject(unknownKind); err == nil {
		t.Fatal("program view accepted unknown external authority kind")
	}
}

func TestProgramViewResolvesSeedsAndKeepsCompleteRelations(t *testing.T) {
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

	unresolvedPosition := -1
	for position, relation := range full.Relations {
		if relation.Resolution == programindex.ResolutionUnresolved {
			unresolvedPosition = position
			break
		}
	}
	if unresolvedPosition < 0 {
		t.Fatalf("complete projection has no unresolved fixture relation: %#v", full.Relations)
	}
	tampered := *full
	tampered.Relations = append([]ProgramViewRelation(nil), full.Relations...)
	tampered.Relations[unresolvedPosition].ToIDs = []string{seedObject.ID}
	tampered.Relations[unresolvedPosition].TargetsIndexed = 1
	tampered.Relations[unresolvedPosition].TargetsOmitted = 1
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "unresolved resolution") {
		t.Fatalf("tampered unresolved relation error = %v", err)
	}
}

func TestProgramViewRetainsFactsBeyondAdvisoryPerValueThresholds(t *testing.T) {
	longText := strings.Repeat("x", programindex.MaxTextBytes+1)
	longPath := strings.Repeat("p", programindex.MaxTextBytes+1) + ".py"
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: longPath, Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{{
		SourceRef: "caller", Kind: programindex.ObjectFunction, Name: longText,
		Signature: longText, Visibility: programindex.VisibilityPublic, Location: location(1),
	}}
	targetRefs := make([]string, 0, programindex.MaxTargetsPerRelation+1)
	for position := 0; position <= programindex.MaxTargetsPerRelation; position++ {
		ref := fmt.Sprintf("target-%03d", position)
		targetRefs = append(targetRefs, ref)
		objects = append(objects, programindex.ObjectInput{
			SourceRef: ref, Kind: programindex.ObjectFunction, Name: ref,
			Visibility: programindex.VisibilityInternal, Location: location(position + 2),
		})
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("e", 64), SourceSHA256: strings.Repeat("f", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: longPath}},
			AnchorFileRef: "f1", Seeds: []programindex.TargetSeedInput{},
		},
		Objects: objects,
		Relations: []programindex.RelationInput{{
			SourceRef: "wide-call", Kind: programindex.RelationCalls, FromRef: "caller",
			ToRefs: targetRefs, Resolution: programindex.ResolutionAlternatives,
			Invocation: longText, Location: location(1), TargetsObserved: len(targetRefs),
			Witnesses: []programindex.Witness{{
				Kind: "call", SourceExpression: longText, Location: location(1),
			}},
			WitnessesObserved: 1,
		}},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: 1,
		},
	})
	if err != nil {
		t.Fatalf("ProgramIndex beyond advisory thresholds: %v", err)
	}
	view, err := NewProgramView(index)
	if err != nil {
		t.Fatalf("NewProgramView beyond advisory thresholds: %v", err)
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("Validate beyond advisory thresholds: %v", err)
	}
	if len(view.Relations) != 1 || len(view.Relations[0].ToIDs) != len(targetRefs) ||
		view.Relations[0].Invocation != longText || view.Relations[0].Location.Path != longPath ||
		view.Relations[0].Witnesses[0].SourceExpression != longText {
		t.Fatalf("wide exact projection lost facts: %#v", view.Relations)
	}
	foundLongObject := false
	for _, object := range view.Objects {
		if object.SourceRef == "caller" {
			foundLongObject = object.Name == longText && object.Signature == longText && object.Location.Path == longPath
		}
	}
	if !foundLongObject {
		t.Fatal("exact long ProgramIndex object was not retained losslessly")
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
