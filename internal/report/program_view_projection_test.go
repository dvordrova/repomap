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

func TestProgramViewRetainsEveryCollectionBeyondFormerThresholds(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app/main.py", Line: line, Column: 1}
	}
	objects := make([]programindex.ObjectInput, MaxProgramViewObjects+1)
	for position := range objects {
		ref := fmt.Sprintf("object-%05d", position)
		objects[position] = programindex.ObjectInput{
			SourceRef: ref, Kind: programindex.ObjectFunction, Name: ref,
			Visibility: programindex.VisibilityPublic, Location: location(position + 1),
		}
	}
	seeds := make([]programindex.TargetSeedInput, MaxProgramViewSeeds+1)
	for position := range seeds {
		seeds[position] = programindex.TargetSeedInput{
			ObjectRef: fmt.Sprintf("object-%05d", position),
			Kind:      programindex.SeedCallable,
			Location:  location(position + 1),
		}
	}
	relations := make([]programindex.RelationInput, MaxProgramViewRelations+1)
	for position := range relations {
		witnessCount := 1
		if position == 0 {
			witnessCount = MaxProgramViewWitnessesPerRelation + 1
		}
		witnesses := make([]programindex.Witness, witnessCount)
		for witnessPosition := range witnesses {
			witnesses[witnessPosition] = programindex.Witness{
				Kind:     fmt.Sprintf("fixture-%02d", witnessPosition),
				Location: location(position + 1),
			}
		}
		relations[position] = programindex.RelationInput{
			SourceRef: fmt.Sprintf("relation-%05d", position),
			Kind:      programindex.RelationCalls, FromRef: "object-00000", ToRefs: []string{"object-00001"},
			Resolution: programindex.ResolutionExact, Location: location(position + 1),
			TargetsObserved: 1, Witnesses: witnesses, WitnessesObserved: len(witnesses),
		}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "app/main.py"}},
			AnchorFileRef: "f1", Seeds: seeds,
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("ProgramIndex beyond former ProgramView thresholds: %v", err)
	}
	view, err := NewProgramView(index)
	if err != nil {
		t.Fatalf("NewProgramView: %v", err)
	}
	if got, want := len(view.Seeds), MaxProgramViewSeeds+1; got != want {
		t.Fatalf("retained seeds = %d, want %d", got, want)
	}
	if got, want := len(view.Objects), MaxProgramViewObjects+1; got != want {
		t.Fatalf("retained objects = %d, want %d", got, want)
	}
	if got, want := len(view.Relations), MaxProgramViewRelations+1; got != want {
		t.Fatalf("retained relations = %d, want %d", got, want)
	}
	maxWitnesses := 0
	for _, relation := range view.Relations {
		if len(relation.Witnesses) > maxWitnesses {
			maxWitnesses = len(relation.Witnesses)
		}
		if relation.WitnessesProjectionOmitted != 0 {
			t.Fatalf("relation %q reports projection omissions: %#v", relation.ID, relation)
		}
	}
	if got, want := maxWitnesses, MaxProgramViewWitnessesPerRelation+1; got != want {
		t.Fatalf("largest retained witness collection = %d, want %d", got, want)
	}
	for name, counts := range map[string]ProgramViewCollectionCounts{
		"seeds": view.Projection.Seeds, "objects": view.Projection.Objects, "relations": view.Projection.Relations,
	} {
		if counts.Eligible != counts.Shown || counts.Omitted != 0 {
			t.Fatalf("%s projection is not exhaustive: %#v", name, counts)
		}
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("complete view validation: %v", err)
	}

	warnings := ProgramViewScaleWarnings(*view)
	warningByKind := make(map[ProgramViewScaleWarningKind]ProgramViewScaleWarning, len(warnings))
	for _, warning := range warnings {
		warningByKind[warning.Kind] = warning
	}
	for kind, want := range map[ProgramViewScaleWarningKind]int{
		ProgramViewScaleWarningSeeds:     MaxProgramViewSeeds + 1,
		ProgramViewScaleWarningObjects:   MaxProgramViewObjects + 1,
		ProgramViewScaleWarningRelations: MaxProgramViewRelations + 1,
		ProgramViewScaleWarningWitnesses: MaxProgramViewWitnessesPerRelation + 1,
	} {
		if got := warningByKind[kind].MaximumRetained; got != want {
			t.Fatalf("warning %q maximum = %d, want %d; all warnings %#v", kind, got, want, warnings)
		}
	}
}

func TestProgramViewRetainsAggregateTextBeyondFormerThreshold(t *testing.T) {
	location := &programindex.Location{Path: "app/main.py", Line: 1, Column: 1}
	largeSignature := strings.Repeat("x", maxProgramViewTextBytes+1)
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("3", 64), SourceSHA256: strings.Repeat("4", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: "app", Selector: "app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: location.Path}},
			AnchorFileRef: "f1", Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "main", Kind: programindex.SeedCallable, Location: location,
			}},
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "main", Kind: programindex.ObjectFunction, Name: "main",
			Signature: largeSignature, Visibility: programindex.VisibilityPublic, Location: location,
		}},
		Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 1, RelationsObserved: 0,
		},
	})
	if err != nil {
		t.Fatalf("ProgramIndex with large retained signature: %v", err)
	}
	view, err := NewProgramView(index)
	if err != nil {
		t.Fatalf("NewProgramView beyond former text threshold: %v", err)
	}
	if view.Objects[0].Signature != largeSignature || view.Seeds[0].Signature != largeSignature {
		t.Fatal("complete ProgramView did not retain the large signature")
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("complete large ProgramView validation: %v", err)
	}
	warnings := ProgramViewScaleWarnings(*view)
	found := false
	for _, warning := range warnings {
		if warning.Kind == ProgramViewScaleWarningText && warning.MaximumRetained > maxProgramViewTextBytes {
			found = true
		}
	}
	if !found {
		t.Fatalf("aggregate text warning missing from %#v", warnings)
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
