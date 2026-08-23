package programindex

import (
	"reflect"
	"strings"
	"testing"
)

func TestNewRejectsUnmeasuredAdapterCoverage(t *testing.T) {
	input := shapeInput()
	if _, err := New(input); err == nil || !strings.Contains(err.Error(), "coverage was not measured") {
		t.Fatalf("unmeasured coverage error = %v", err)
	}
}

func TestNewRejectsUnmeasuredObjectVisibility(t *testing.T) {
	input := representativeInput()
	input.Objects[0].Visibility = ""
	_, err := New(input)
	if err == nil || !strings.Contains(err.Error(), "visibility") {
		t.Fatalf("New error = %v, want explicit visibility rejection", err)
	}
}

func TestNewRejectsUnmeasuredRelationCoverage(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax"}},
	}}
	input.Coverage = CoverageInput{
		Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations),
	}
	if _, err := New(input); err == nil || !strings.Contains(err.Error(), "invalid relation coverage") {
		t.Fatalf("New error = %v, want explicit relation coverage rejection", err)
	}
}

func TestNewCanonicalizesResolvesAndSeals(t *testing.T) {
	input := representativeInput()
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.HasPrefix(index.Target.ID, "program-target-") || len(index.SHA256) != 64 {
		t.Fatalf("unsealed identities: target=%q sha=%q", index.Target.ID, index.SHA256)
	}
	if got, want := index.Target.Sources, []TargetSource{
		{FileRef: "manifest", Path: "project.toml"},
		{FileRef: "root-a", Path: "src/api.lang"},
		{FileRef: "root-b", Path: "src/worker.lang"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target sources = %#v, want %#v", got, want)
	}
	seed := objectWithSourceRef(t, index, "object-method")
	if got, want := index.Target.Seeds, []TargetSeed{{
		ObjectID: seed.ID, Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target seeds = %#v, want %#v", got, want)
	}
	for position := 1; position < len(index.Objects); position++ {
		if index.Objects[position-1].ID >= index.Objects[position].ID {
			t.Fatalf("objects are not canonical: %#v", index.Objects)
		}
	}
	for position := 1; position < len(index.Relations); position++ {
		if index.Relations[position-1].ID >= index.Relations[position].ID {
			t.Fatalf("relations are not canonical: %#v", index.Relations)
		}
	}

	method := objectWithSourceRef(t, index, "object-method")
	owner := objectWithSourceRef(t, index, "object-worker")
	pkg := objectWithSourceRef(t, index, "object-package")
	if method.OwnerID != owner.ID || method.ContainerID != pkg.ID || method.Signature != "run(context) -> error" {
		t.Fatalf("method ownership was not resolved: %#v", method)
	}
	unknownVisibility := objectWithSourceRef(t, index, "object-runner")
	if unknownVisibility.Visibility != VisibilityUnknown {
		t.Fatalf("visibility = %q, want explicit unknown", unknownVisibility.Visibility)
	}
	if got, want := index.Coverage, (Coverage{
		ObjectsObserved: 8, ObjectsIndexed: 6, ObjectsOmitted: 2,
		RelationsObserved: 5, RelationsIndexed: 3, RelationsOmitted: 2,
		ExactRelations: 1, AlternativeRelations: 1, UnresolvedRelations: 1,
		TargetsObserved: 6, TargetsIndexed: 3, TargetsOmitted: 3,
		WitnessesObserved: 4, WitnessesIndexed: 3, WitnessesOmitted: 1,
	}); got != want {
		t.Fatalf("coverage = %#v, want %#v", got, want)
	}

	reordered := representativeInput()
	reordered.Target.Sources[0], reordered.Target.Sources[2] = reordered.Target.Sources[2], reordered.Target.Sources[0]
	reordered.Target.Seeds = append(reordered.Target.Seeds, reordered.Target.Seeds[0])
	reordered.Objects[0], reordered.Objects[5] = reordered.Objects[5], reordered.Objects[0]
	reordered.Relations[0], reordered.Relations[2] = reordered.Relations[2], reordered.Relations[0]
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if reorderedIndex.SHA256 != index.SHA256 || !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatalf("input order changed canonical index:\nfirst=%#v\nsecond=%#v", index, reorderedIndex)
	}

	changedProducer := representativeInput()
	changedProducer.ScenarioSHA256 = strings.Repeat("d", 64)
	changedProducer.SourceSHA256 = strings.Repeat("e", 64)
	changedIndex, err := newMeasuredProgramIndex(changedProducer)
	if err != nil {
		t.Fatalf("New changed producer: %v", err)
	}
	if changedIndex.Target.ID != index.Target.ID {
		t.Fatalf("scenario/source SHA changed target local identity: %q != %q", changedIndex.Target.ID, index.Target.ID)
	}
	for position := range index.Objects {
		if changedIndex.Objects[position].ID != index.Objects[position].ID {
			t.Fatalf("scenario/source SHA changed object local identity")
		}
	}
	for position := range index.Relations {
		if changedIndex.Relations[position].ID != index.Relations[position].ID {
			t.Fatalf("scenario/source SHA changed relation local identity")
		}
	}
	if changedIndex.SHA256 == index.SHA256 {
		t.Fatal("producer SHA change did not change the complete-index seal")
	}

	renamed := representativeInput()
	renamed.Target.Name = "a better presentation label"
	renamedIndex, err := newMeasuredProgramIndex(renamed)
	if err != nil {
		t.Fatalf("New renamed target: %v", err)
	}
	if renamedIndex.Target.ID != index.Target.ID {
		t.Fatal("presentation name changed semantic target identity")
	}
	for position := range index.Objects {
		if renamedIndex.Objects[position].ID != index.Objects[position].ID {
			t.Fatal("presentation name changed object identity")
		}
	}
	if renamedIndex.SHA256 == index.SHA256 {
		t.Fatal("presentation name change did not change complete artifact bytes")
	}

	snapshot := index.Snapshot()
	snapshot.Target.Sources[0].Path = "changed.py"
	snapshot.Target.Seeds[0].ObjectID = "changed"
	snapshot.Target.Seeds[0].Location.Path = "changed.py"
	snapshot.Objects[0].Location = &Location{Path: "changed.py", Line: 1, Column: 1}
	snapshot.Relations[0].ToIDs = append(snapshot.Relations[0].ToIDs, "changed")
	snapshot.Relations[0].Witnesses[0].SourceExpression = "changed.call"
	snapshot.Relations[0].Witnesses[0].Location.Path = "changed.py"
	if index.Target.Sources[0].Path == "changed.py" || index.Target.Seeds[0].ObjectID == "changed" ||
		index.Target.Seeds[0].Location.Path == "changed.py" ||
		index.Objects[0].Location != nil && index.Objects[0].Location.Path == "changed.py" ||
		index.Relations[0].Witnesses[0].Location.Path == "changed.py" {
		t.Fatal("Snapshot aliases index storage")
	}
}

func TestWitnessSourceExpressionIsBoundedTypedDigestMaterial(t *testing.T) {
	input := representativeInput()
	input.Relations[1].Witnesses[0].SourceExpression = "runtime.schedule"
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := ""
	for _, relation := range index.Relations {
		if relation.SourceRef == "relation-exact" {
			got = relation.Witnesses[0].SourceExpression
		}
	}
	if got != "runtime.schedule" {
		t.Fatalf("source expression = %q", got)
	}

	changed := representativeInput()
	changed.Relations[1].Witnesses[0].SourceExpression = "scheduler.enqueue"
	changedIndex, err := newMeasuredProgramIndex(changed)
	if err != nil {
		t.Fatalf("New changed expression: %v", err)
	}
	if changedIndex.SHA256 == index.SHA256 {
		t.Fatal("source expression did not affect ProgramIndex seal")
	}

	invalid := representativeInput()
	invalid.Relations[1].Witnesses[0].SourceExpression = strings.Repeat("x", MaxTextBytes+1)
	if _, err := newMeasuredProgramIndex(invalid); err == nil || !strings.Contains(err.Error(), "invalid witness") {
		t.Fatalf("over-bound source expression error = %v", err)
	}
}

func TestCodecIsStrictAndValidatesSeal(t *testing.T) {
	index, err := newMeasuredProgramIndex(representativeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatalf("codec changed index:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}
	if ArtifactFilename != "program-index.json" {
		t.Fatalf("ArtifactFilename = %q", ArtifactFilename)
	}

	if _, err := Decode(append(append([]byte(nil), encoded...), []byte(` {}`)...)); err == nil {
		t.Fatal("Decode accepted trailing JSON")
	}
	unknown := append([]byte(`{"unknown":true,`), encoded[1:]...)
	if _, err := Decode(unknown); err == nil {
		t.Fatal("Decode accepted an unknown field")
	}
	tampered := []byte(strings.Replace(string(encoded), "runtime.schedule", "runtime.changed", 1))
	if _, err := Decode(tampered); err == nil {
		t.Fatal("Decode accepted content with a stale seal")
	}
}

func TestNewRejectsInvalidResolutionShapes(t *testing.T) {
	tests := []struct {
		name     string
		relation RelationInput
		want     string
	}{
		{
			name: "exact has two targets",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a", "target-b"}, Resolution: ResolutionExact,
				Witnesses: []Witness{{Kind: "syntax"}}},
			want: "invalid exact relation",
		},
		{
			name: "exact has no witness",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a"}, Resolution: ResolutionExact},
			want: "invalid exact relation",
		},
		{
			name: "alternatives has no target",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{}, Resolution: ResolutionAlternatives,
				Witnesses: []Witness{{Kind: "flow_candidates"}}},
			want: "invalid alternatives relation",
		},
		{
			name: "unresolved retains target",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a"}, Resolution: ResolutionUnresolved,
				Witnesses: []Witness{{Kind: "dynamic_name", Detail: "computed callee"}}},
			want: "invalid unresolved relation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			input.Relations = []RelationInput{test.relation}
			_, err := newMeasuredProgramIndex(input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewAcceptsOneObservedAlternativeWithoutRuntimeExactness(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "possible-call", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionAlternatives,
		TargetsObserved: 1, Witnesses: []Witness{{Kind: "syntax_candidate"}}, WitnessesObserved: 1,
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(index.Relations) != 1 || index.Relations[0].Resolution != ResolutionAlternatives ||
		len(index.Relations[0].ToIDs) != 1 || index.Relations[0].TargetsOmitted != 0 {
		t.Fatalf("possible relation = %#v", index.Relations)
	}
}

func TestNewRejectsDuplicateRelationIdentityAndBounds(t *testing.T) {
	input := shapeInput()
	relation := RelationInput{
		SourceRef: "same-local-relation", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax"}},
	}
	input.Relations = []RelationInput{relation, relation}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "duplicate relation identity") {
		t.Fatalf("duplicate relation error = %v", err)
	}

	input = shapeInput()
	input.Target.Name = strings.Repeat("x", MaxTextBytes+1)
	if _, err := newMeasuredProgramIndex(input); err == nil {
		t.Fatal("New accepted an over-bound scalar")
	}

	input = shapeInput()
	tooManyTargets := make([]string, MaxTargetsPerRelation+1)
	for position := range tooManyTargets {
		tooManyTargets[position] = "target-" + strings.Repeat("x", position+1)
	}
	input.Relations = []RelationInput{{
		SourceRef: "wide-relation", Kind: RelationCalls, FromRef: "caller", ToRefs: tooManyTargets,
		Resolution: ResolutionAlternatives, Witnesses: []Witness{{Kind: "syntax"}},
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "relation bound exceeded") {
		t.Fatalf("wide relation error = %v", err)
	}
}

func TestTargetSeedsAreExactIdentityBoundLocalObjects(t *testing.T) {
	input := representativeInput()
	first, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "object-worker", Kind: SeedBoundObject,
		Location: &Location{Path: "src/worker.lang", Line: 4, Column: 1},
	}}
	second, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New changed seed: %v", err)
	}
	if first.Target.ID == second.Target.ID || first.SHA256 == second.SHA256 {
		t.Fatal("changing the exact launch seed did not change target identity and index seal")
	}

	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "missing-object", Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "target seed") {
		t.Fatalf("missing target seed error = %v", err)
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "object-external", Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "not a local program object") {
		t.Fatalf("external target seed error = %v", err)
	}

	tampered := first.Snapshot()
	tampered.Target.Seeds[0].ObjectID = "program-object-missing"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a dangling target seed")
	}
}

func TestTargetSourcesPreservePairsRejectConflictsAndCanonicalize(t *testing.T) {
	firstInput := shapeInput()
	firstInput.Target.Sources = []TargetSource{
		{FileRef: "support", Path: "src/support.lang"},
		{FileRef: "root", Path: "root.lang"},
	}
	first, err := newMeasuredProgramIndex(firstInput)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	secondInput := shapeInput()
	secondInput.Target.Sources = []TargetSource{
		{FileRef: "root", Path: "root.lang"},
		{FileRef: "support", Path: "src/support.lang"},
	}
	second, err := newMeasuredProgramIndex(secondInput)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(first.Target, second.Target) || first.SHA256 != second.SHA256 {
		t.Fatalf("source input order changed sealed index:\nfirst=%#v\nsecond=%#v", first.Target, second.Target)
	}
	if got, want := first.Target.Sources, []TargetSource{
		{FileRef: "root", Path: "root.lang"},
		{FileRef: "support", Path: "src/support.lang"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical sources = %#v, want paired sources %#v", got, want)
	}

	for _, test := range []struct {
		name    string
		sources []TargetSource
		want    string
	}{
		{
			name: "one ref names two paths",
			sources: []TargetSource{
				{FileRef: "root", Path: "root.lang"},
				{FileRef: "root", Path: "other.lang"},
			},
			want: "conflicting paths",
		},
		{
			name: "one path names two refs",
			sources: []TargetSource{
				{FileRef: "root", Path: "root.lang"},
				{FileRef: "other", Path: "root.lang"},
			},
			want: "conflicting file refs",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			input.Target.Sources = test.sources
			if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStructuredTargetSeedPreservesLaunchLocationAndRejectsIncompatibleObject(t *testing.T) {
	input := shapeInput()
	input.Objects = []ObjectInput{
		{SourceRef: "module", Kind: ObjectModule, Name: "tool", Visibility: VisibilityPublic,
			Location: &Location{Path: "root.lang", Line: 1, Column: 1}},
		{SourceRef: "function", Kind: ObjectFunction, Name: "declaredFirst", Visibility: VisibilityPublic,
			Location: &Location{Path: "root.lang", Line: 1, Column: 1}},
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "module", Kind: SeedMainGuard,
		Location: &Location{Path: "root.lang", Line: 7, Column: 1},
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := index.Target.Seeds[0].Location.Line, 7; got != want {
		t.Fatalf("main-guard launch line = %d, want %d", got, want)
	}
	module := objectWithSourceRef(t, index, "module")
	if module.Location == nil || module.Location.Line != 1 {
		t.Fatalf("module declaration location = %#v, want line 1", module.Location)
	}

	changedLaunch := input
	changedLaunch.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "module", Kind: SeedMainGuard,
		Location: &Location{Path: "root.lang", Line: 8, Column: 1},
	}}
	changed, err := newMeasuredProgramIndex(changedLaunch)
	if err != nil {
		t.Fatalf("New changed launch: %v", err)
	}
	if changed.Target.ID == index.Target.ID {
		t.Fatal("distinct main-guard launch lines share one target identity")
	}

	for _, seed := range []TargetSeedInput{
		{ObjectRef: "function", Kind: SeedMainGuard, Location: &Location{Path: "root.lang", Line: 7, Column: 1}},
		{ObjectRef: "function", Kind: SeedCallable, Location: &Location{Path: "root.lang", Line: 7, Column: 1}},
	} {
		invalid := input
		invalid.Target.Seeds = []TargetSeedInput{seed}
		if _, err := newMeasuredProgramIndex(invalid); err == nil || !strings.Contains(err.Error(), "incompatible with object") {
			t.Fatalf("incompatible seed %#v error = %v", seed, err)
		}
	}
}

func TestPythonTargetSelectorDistinguishesOtherwiseIdenticalViews(t *testing.T) {
	common := shapeInput()
	common.Target.Language = "python"
	common.Target.Name = "same display"
	common.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "root-callable", Kind: SeedCallable,
		Location: &Location{Path: "root.lang", Line: 1, Column: 1},
	}}
	common.Objects = []ObjectInput{{
		SourceRef: "root-callable", Kind: ObjectFunction, Name: "main", Visibility: VisibilityPublic,
		Location: &Location{Path: "root.lang", Line: 1, Column: 1},
	}}
	firstInput := common
	firstInput.Target.Selector = "python:.:script:first"
	first, err := newMeasuredProgramIndex(firstInput)
	if err != nil {
		t.Fatalf("New first view: %v", err)
	}

	secondInput := common
	secondInput.Target.Selector = "python:.:script:second"
	second, err := newMeasuredProgramIndex(secondInput)
	if err != nil {
		t.Fatalf("New second view: %v", err)
	}
	if first.Target.ID == second.Target.ID {
		t.Fatalf("Python views with selectors %q and %q share target ID %q",
			first.Target.Selector, second.Target.Selector, first.Target.ID)
	}
}

func TestValidateRejectsTampering(t *testing.T) {
	index, err := newMeasuredProgramIndex(representativeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tampered := index.Snapshot()
	method := objectPositionWithSourceRef(t, tampered, "object-method")
	tampered.Objects[method].Signature = "run()"
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered signature Validate error = %v", err)
	}

	tampered = index.Snapshot()
	tampered.Relations[0].FromID = "program-object-unknown"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a dangling relation source")
	}

	tampered = index.Snapshot()
	tampered.Coverage.RelationsObserved++
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted tampered coverage")
	}
}

func representativeInput() Input {
	return Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: TargetInput{
			Language: "neutral-test", Kind: "executable", Name: "example", Selector: "fixture",
			Sources: []TargetSource{
				{FileRef: "root-b", Path: "src/worker.lang"},
				{FileRef: "manifest", Path: "project.toml"},
				{FileRef: "root-a", Path: "src/api.lang"},
				{FileRef: "root-a", Path: "src/api.lang"},
			},
			AnchorFileRef: "manifest",
			Seeds: []TargetSeedInput{{
				ObjectRef: "object-method", Kind: SeedCallable,
				Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
			}},
		},
		Objects: []ObjectInput{
			{SourceRef: "object-method", Kind: ObjectMethod, Name: "run", Visibility: VisibilityPublic,
				Signature: "run(context) -> error", OwnerRef: "object-worker", ContainerRef: "object-package",
				Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3}},
			{SourceRef: "object-external", Kind: ObjectExternalSymbol, Name: "runtime.schedule", Visibility: VisibilityPublic},
			{SourceRef: "object-impl-b", Kind: ObjectType, Name: "WorkerB", Visibility: VisibilityInternal,
				ContainerRef: "object-package", Location: &Location{Path: "src/b.lang", Line: 4, Column: 1}},
			{SourceRef: "object-package", Kind: ObjectPackage, Name: "example", Visibility: VisibilityPublic,
				Location: &Location{Path: "src/package.lang", Line: 1, Column: 1}},
			{SourceRef: "object-runner", Kind: ObjectType, Name: "Runner", Visibility: VisibilityUnknown,
				ContainerRef: "object-package", Location: &Location{Path: "src/api.lang", Line: 2, Column: 1}},
			{SourceRef: "object-worker", Kind: ObjectType, Name: "WorkerA", Visibility: VisibilityInternal,
				ContainerRef: "object-package", Location: &Location{Path: "src/worker.lang", Line: 4, Column: 1}},
		},
		Relations: []RelationInput{
			{
				SourceRef: "relation-unresolved", Kind: RelationInvokesExternal, FromRef: "object-method",
				Resolution: ResolutionUnresolved, Invocation: "runtime selected", TargetsObserved: 3,
				WitnessesObserved: 2, Witnesses: []Witness{{Kind: "dynamic_name", Detail: "callee name is computed",
					Location: &Location{Path: "src/worker.lang", Line: 20, Column: 7}}},
			},
			{
				SourceRef: "relation-exact", Kind: RelationCalls, FromRef: "object-method", ToRefs: []string{"object-external"},
				Resolution: ResolutionExact, Invocation: "deferred", Location: &Location{Path: "src/worker.lang", Line: 14, Column: 5},
				Witnesses: []Witness{{Kind: "syntax_call", Location: &Location{Path: "src/worker.lang", Line: 14, Column: 5}}},
			},
			{
				SourceRef: "relation-alternatives", Kind: RelationImplements, FromRef: "object-runner",
				ToRefs: []string{"object-impl-b", "object-worker"}, Resolution: ResolutionAlternatives,
				Witnesses: []Witness{{Kind: "compatible_declaration", Detail: "both declarations satisfy the local contract",
					Location: &Location{Path: "src/api.lang", Line: 2, Column: 1}}},
			},
		},
		Coverage: CoverageInput{Measured: true, ObjectsObserved: 8, RelationsObserved: 5},
	}
}

func shapeInput() Input {
	return Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: TargetInput{Language: "neutral-test", Kind: "executable", Name: "shape",
			Selector: "shape", Sources: []TargetSource{{FileRef: "root", Path: "root.lang"}}, AnchorFileRef: "root"},
		Objects: []ObjectInput{
			{SourceRef: "caller", Kind: ObjectFunction, Name: "caller", Visibility: VisibilityInternal},
			{SourceRef: "target-a", Kind: ObjectFunction, Name: "a", Visibility: VisibilityInternal},
			{SourceRef: "target-b", Kind: ObjectFunction, Name: "b", Visibility: VisibilityInternal},
		},
	}
}

func newMeasuredProgramIndex(input Input) (Index, error) {
	for position := range input.Relations {
		if input.Relations[position].TargetsObserved == 0 {
			input.Relations[position].TargetsObserved = len(input.Relations[position].ToRefs)
			if input.Relations[position].TargetsObserved == 0 {
				input.Relations[position].TargetsObserved = 1
			}
		}
		if input.Relations[position].WitnessesObserved == 0 {
			input.Relations[position].WitnessesObserved = len(input.Relations[position].Witnesses)
			if input.Relations[position].WitnessesObserved == 0 {
				input.Relations[position].WitnessesObserved = 1
			}
		}
	}
	if !input.Coverage.Measured {
		input.Coverage = CoverageInput{
			Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations),
		}
	}
	return New(input)
}

func objectWithSourceRef(t *testing.T, index Index, ref string) Object {
	t.Helper()
	return index.Objects[objectPositionWithSourceRef(t, index, ref)]
}

func objectPositionWithSourceRef(t *testing.T, index Index, ref string) int {
	t.Helper()
	for position, object := range index.Objects {
		if object.SourceRef == ref {
			return position
		}
	}
	t.Fatalf("object source ref %q not found", ref)
	return -1
}
