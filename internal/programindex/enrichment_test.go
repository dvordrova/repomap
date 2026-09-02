package programindex

import (
	"reflect"
	"strings"
	"testing"
)

const categorizationTestDocumentationSHA256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

func TestEnrichCanonicalizesOverlappingObjectAndPatternCategoriesAndReseals(t *testing.T) {
	base := categorizationTestIndex(t)
	objectID := objectWithSourceRef(t, base, "target-a").ID
	patternID := patternWithSourceRef(t, relationWithSourceRef(t, base, "relation"), "pattern").ID
	baseEncoded, err := Encode(base)
	if err != nil {
		t.Fatalf("Encode base: %v", err)
	}
	if strings.Contains(string(baseEncoded), `"categorization"`) {
		t.Fatalf("un-enriched artifact contains optional categorization: %s", baseEncoded)
	}

	accepted := []CategoryAssignment{
		{SubjectID: patternID, Categories: []Category{CategoryCore, CategoryInbound}},
		{SubjectID: objectID, Categories: []Category{
			CategoryDependency, CategoryBackgroundActivity, CategoryCore,
		}},
	}
	enriched, err := Enrich(base, categorizationTestDocumentationSHA256, accepted)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if enriched.Categorization == nil ||
		enriched.Categorization.BaseIndexSHA256 != base.SHA256 ||
		enriched.Categorization.ReducedDocumentationSHA256 != categorizationTestDocumentationSHA256 {
		t.Fatalf("categorization binding = %#v, base sha = %q", enriched.Categorization, base.SHA256)
	}
	if enriched.SHA256 == base.SHA256 || len(enriched.SHA256) != 64 {
		t.Fatalf("enriched seal = %q, base seal = %q", enriched.SHA256, base.SHA256)
	}
	if base.Categorization != nil {
		t.Fatal("Enrich mutated the base Index")
	}
	if !reflect.DeepEqual(enriched.Target, base.Target) ||
		!reflect.DeepEqual(enriched.Objects, base.Objects) ||
		!reflect.DeepEqual(enriched.Relations, base.Relations) ||
		enriched.Coverage != base.Coverage {
		t.Fatal("Enrich changed deterministic ProgramIndex authority")
	}

	wantBySubject := map[string][]Category{
		objectID:  {CategoryBackgroundActivity, CategoryCore, CategoryDependency},
		patternID: {CategoryCore, CategoryInbound},
	}
	if len(enriched.Categorization.Assignments) != len(wantBySubject) {
		t.Fatalf("assignments = %#v", enriched.Categorization.Assignments)
	}
	for position, assignment := range enriched.Categorization.Assignments {
		if position > 0 && enriched.Categorization.Assignments[position-1].SubjectID >= assignment.SubjectID {
			t.Fatalf("assignments are not canonical: %#v", enriched.Categorization.Assignments)
		}
		if want := wantBySubject[assignment.SubjectID]; !reflect.DeepEqual(assignment.Categories, want) {
			t.Fatalf("categories for %q = %#v, want %#v", assignment.SubjectID, assignment.Categories, want)
		}
	}
	if err := enriched.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	reordered := []CategoryAssignment{
		{SubjectID: objectID, Categories: []Category{
			CategoryCore, CategoryDependency, CategoryBackgroundActivity,
		}},
		{SubjectID: patternID, Categories: []Category{CategoryInbound, CategoryCore}},
	}
	reorderedIndex, err := Enrich(base, categorizationTestDocumentationSHA256, reordered)
	if err != nil {
		t.Fatalf("Enrich reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, enriched) {
		t.Fatalf("accepted order changed enriched Index:\nfirst=%#v\nsecond=%#v", enriched, reorderedIndex)
	}

	encoded, err := Encode(enriched)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, enriched) {
		t.Fatalf("codec changed categorization:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}
	unknownNestedField := []byte(strings.Replace(
		string(encoded), `"base_index_sha256":`, `"unexpected":true,"base_index_sha256":`, 1,
	))
	if _, err := Decode(unknownNestedField); err == nil {
		t.Fatal("Decode accepted an unknown categorization field")
	}

	snapshot := enriched.Snapshot()
	snapshot.Categorization.Assignments[0].SubjectID = "changed"
	snapshot.Categorization.Assignments[0].Categories[0] = CategoryInbound
	if enriched.Categorization.Assignments[0].SubjectID == "changed" ||
		enriched.Categorization.Assignments[0].Categories[0] != wantBySubject[enriched.Categorization.Assignments[0].SubjectID][0] {
		t.Fatal("Snapshot aliases categorization storage")
	}
}

func TestEnrichRejectsUnknownMalformedDuplicateAndConflictingAssignments(t *testing.T) {
	base := categorizationTestIndex(t)
	objectID := objectWithSourceRef(t, base, "target-a").ID
	tests := []struct {
		name        string
		accepted    []CategoryAssignment
		wantInError string
	}{
		{name: "empty subject", accepted: []CategoryAssignment{{Categories: []Category{CategoryCore}}}, wantInError: "unknown subject"},
		{name: "request local ref", accepted: []CategoryAssignment{{SubjectID: "g1", Categories: []Category{CategoryCore}}}, wantInError: "unknown subject"},
		{name: "unknown canonical-looking subject", accepted: []CategoryAssignment{{SubjectID: "program-object-" + strings.Repeat("f", 64), Categories: []Category{CategoryCore}}}, wantInError: "unknown subject"},
		{name: "no categories", accepted: []CategoryAssignment{{SubjectID: objectID}}, wantInError: "no categories"},
		{name: "invalid category", accepted: []CategoryAssignment{{SubjectID: objectID, Categories: []Category{"entrypoint"}}}, wantInError: "invalid category"},
		{name: "duplicate category", accepted: []CategoryAssignment{{SubjectID: objectID, Categories: []Category{CategoryCore, CategoryCore}}}, wantInError: "repeats category"},
		{name: "duplicate row", accepted: []CategoryAssignment{
			{SubjectID: objectID, Categories: []Category{CategoryCore}},
			{SubjectID: objectID, Categories: []Category{CategoryCore}},
		}, wantInError: "duplicate categorization assignment"},
		{name: "conflicting row", accepted: []CategoryAssignment{
			{SubjectID: objectID, Categories: []Category{CategoryCore}},
			{SubjectID: objectID, Categories: []Category{CategoryDependency}},
		}, wantInError: "conflicting categorization assignments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Enrich(base, categorizationTestDocumentationSHA256, test.accepted)
			if err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("Enrich error = %v, want %q", err, test.wantInError)
			}
		})
	}
}

func TestEnrichRejectsReservedPlatformDependencyAuthority(t *testing.T) {
	base := categorizationPlatformIndex(t)
	platformObjectID := objectWithSourceRef(t, base, "platform-raf").ID
	packageObjectID := objectWithSourceRef(t, base, "axios-post").ID
	platformPatternID := patternWithSourceRef(
		t, relationWithSourceRef(t, base, "platform-call"), "platform-call-pattern",
	).ID
	packagePatternID := patternWithSourceRef(
		t, relationWithSourceRef(t, base, "axios-call"), "axios-call-pattern",
	).ID
	platformStructuralPatternID := patternWithSourceRef(
		t, relationWithSourceRef(t, base, "platform-structural-call"), "platform-structural-pattern",
	).ID

	for _, test := range []struct {
		name       string
		assignment CategoryAssignment
		wantError  bool
	}{
		{
			name: "platform external object",
			assignment: CategoryAssignment{
				SubjectID: platformObjectID, Categories: []Category{CategoryDependency},
			},
			wantError: true,
		},
		{
			name: "exact platform-only invocation pattern mixed row",
			assignment: CategoryAssignment{
				SubjectID:  platformPatternID,
				Categories: []Category{CategoryBackgroundActivity, CategoryDependency},
			},
			wantError: true,
		},
		{
			name: "real package external object",
			assignment: CategoryAssignment{
				SubjectID: packageObjectID, Categories: []Category{CategoryDependency},
			},
		},
		{
			name: "exact package invocation pattern",
			assignment: CategoryAssignment{
				SubjectID: packagePatternID, Categories: []Category{CategoryDependency},
			},
		},
		{
			name: "non-external structural pattern targeting platform",
			assignment: CategoryAssignment{
				SubjectID: platformStructuralPatternID, Categories: []Category{CategoryDependency},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if supported := CategorySupported(
				base, test.assignment.SubjectID, CategoryDependency,
			); supported == test.wantError {
				t.Fatalf("CategorySupported dependency = %t, want %t", supported, !test.wantError)
			}
			_, err := Enrich(
				base, categorizationTestDocumentationSHA256,
				[]CategoryAssignment{test.assignment},
			)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "unsupported category") {
					t.Fatalf("Enrich error = %v, want unsupported category", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Enrich accepted package authority: %v", err)
			}
		})
	}
}

func TestCategorizationValidationRejectsSealedPlatformDependencyAuthority(t *testing.T) {
	base := categorizationPlatformIndex(t)
	platformObjectID := objectWithSourceRef(t, base, "platform-raf").ID
	platformPatternID := patternWithSourceRef(
		t, relationWithSourceRef(t, base, "platform-call"), "platform-call-pattern",
	).ID

	for _, test := range []struct {
		name      string
		subjectID string
		allowed   Category
	}{
		{name: "platform object", subjectID: platformObjectID, allowed: CategoryCore},
		{name: "platform-only exact pattern", subjectID: platformPatternID, allowed: CategoryBackgroundActivity},
	} {
		t.Run(test.name, func(t *testing.T) {
			enriched, err := Enrich(base, categorizationTestDocumentationSHA256, []CategoryAssignment{{
				SubjectID: test.subjectID, Categories: []Category{test.allowed},
			}})
			if err != nil {
				t.Fatalf("Enrich allowed category: %v", err)
			}
			tampered := enriched.Snapshot()
			tampered.Categorization.Assignments[0].Categories[0] = CategoryDependency
			resealCategorizationTestIndex(t, &tampered)
			if err := tampered.Validate(); err == nil ||
				!strings.Contains(err.Error(), "unsupported category") {
				t.Fatalf("Validate error = %v, want unsupported category", err)
			}
		})
	}
}

func TestEnrichSealsEmptySparseResultAndRejectsSecondEnrichment(t *testing.T) {
	base := categorizationTestIndex(t)
	enriched, err := Enrich(base, categorizationTestDocumentationSHA256, nil)
	if err != nil {
		t.Fatalf("Enrich empty: %v", err)
	}
	if enriched.Categorization == nil || enriched.Categorization.Assignments == nil ||
		len(enriched.Categorization.Assignments) != 0 ||
		enriched.Categorization.BaseIndexSHA256 != base.SHA256 ||
		enriched.Categorization.ReducedDocumentationSHA256 != categorizationTestDocumentationSHA256 {
		t.Fatalf("empty categorization = %#v", enriched.Categorization)
	}
	if _, err := Enrich(enriched, categorizationTestDocumentationSHA256, nil); err == nil || !strings.Contains(err.Error(), "already enriched") {
		t.Fatalf("second Enrich error = %v", err)
	}
	encoded, err := Encode(enriched)
	if err != nil {
		t.Fatalf("Encode empty enrichment: %v", err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatalf("Decode empty enrichment: %v", err)
	}
}

func TestBaseRestoresExactAdapterProjectionFromEnrichedIndex(t *testing.T) {
	base := categorizationTestIndex(t)
	enriched, err := Enrich(base, categorizationTestDocumentationSHA256, nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	restored, err := Base(enriched)
	if err != nil {
		t.Fatalf("Base enriched: %v", err)
	}
	if !reflect.DeepEqual(restored, base) {
		t.Fatalf("restored base changed adapter projection:\n got=%#v\nwant=%#v", restored, base)
	}
	unEnriched, err := Base(base)
	if err != nil {
		t.Fatalf("Base plain: %v", err)
	}
	if !reflect.DeepEqual(unEnriched, base) {
		t.Fatalf("plain base snapshot changed projection")
	}
	unEnriched.Objects[0].Name = "changed"
	if base.Objects[0].Name == "changed" {
		t.Fatal("Base aliases input storage")
	}
}

func TestCategorizationValidationRejectsBrokenBaseBindingAndNonCanonicalState(t *testing.T) {
	base := categorizationTestIndex(t)
	objectID := objectWithSourceRef(t, base, "target-a").ID
	patternID := patternWithSourceRef(t, relationWithSourceRef(t, base, "relation"), "pattern").ID
	enriched, err := Enrich(base, categorizationTestDocumentationSHA256, []CategoryAssignment{
		{SubjectID: objectID, Categories: []Category{CategoryCore, CategoryDependency}},
		{SubjectID: patternID, Categories: []Category{CategoryInbound}},
	})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	tests := []struct {
		name        string
		mutate      func(*Index)
		wantInError string
	}{
		{name: "base sha", mutate: func(index *Index) {
			index.Categorization.BaseIndexSHA256 = strings.Repeat("f", 64)
		}, wantInError: "base sha256 mismatch"},
		{name: "documentation sha", mutate: func(index *Index) {
			index.Categorization.ReducedDocumentationSHA256 = "broken"
		}, wantInError: "invalid categorization"},
		{name: "changed deterministic base", mutate: func(index *Index) {
			index.Objects[0].Name = "changed name"
		}, wantInError: "base sha256 mismatch"},
		{name: "assignment order", mutate: func(index *Index) {
			index.Categorization.Assignments[0], index.Categorization.Assignments[1] =
				index.Categorization.Assignments[1], index.Categorization.Assignments[0]
		}, wantInError: "assignments are not canonical"},
		{name: "category order", mutate: func(index *Index) {
			for position := range index.Categorization.Assignments {
				if len(index.Categorization.Assignments[position].Categories) == 2 {
					categories := index.Categorization.Assignments[position].Categories
					categories[0], categories[1] = categories[1], categories[0]
				}
			}
		}, wantInError: "categories are not canonical"},
		{name: "unknown subject", mutate: func(index *Index) {
			index.Categorization.Assignments[0].SubjectID = "program-pattern-" + strings.Repeat("f", 64)
		}, wantInError: "invalid categorization assignment"},
		{name: "nil assignments", mutate: func(index *Index) {
			index.Categorization.Assignments = nil
		}, wantInError: "invalid categorization"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := enriched.Snapshot()
			test.mutate(&tampered)
			resealCategorizationTestIndex(t, &tampered)
			if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("Validate error = %v, want %q", err, test.wantInError)
			}
		})
	}

	staleSeal := enriched.Snapshot()
	staleSeal.Categorization.Assignments[0].Categories[0] = CategoryBackgroundActivity
	if err := staleSeal.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("stale semantic seal error = %v", err)
	}
}

func categorizationTestIndex(t *testing.T) Index {
	t.Helper()
	input := shapeInput()
	pattern := validRelationPatternInput()
	input.Relations = []RelationInput{{
		SourceRef: "relation", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
		Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax_call"}},
		Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return index
}

func categorizationPlatformIndex(t *testing.T) Index {
	t.Helper()
	location := func(line int) *Location {
		return &Location{Path: "src/runtime.ts", Line: line, Column: 1}
	}
	objects := []ObjectInput{
		{
			SourceRef: "animate", Kind: ObjectFunction, Name: "animate",
			Visibility: VisibilityInternal, Location: location(1),
		},
		{
			SourceRef: "platform-raf", Kind: ObjectExternalSymbol,
			Name: "platform:javascript.requestAnimationFrame", Visibility: VisibilityPublic,
			External: &ExternalSymbol{
				AuthorityKind: ExternalAuthorityPlatform,
				PackagePath:   "raw-javascript-runtime", Name: "requestAnimationFrame",
			},
		},
		{
			SourceRef: "axios-post", Kind: ObjectExternalSymbol,
			Name: "axios.default.post", Visibility: VisibilityPublic,
			External: &ExternalSymbol{
				AuthorityKind: ExternalAuthorityPackage,
				PackagePath:   "platform:misleading-package-identity", Receiver: "default", Name: "post",
			},
		},
	}
	relations := []RelationInput{
		{
			SourceRef: "platform-call", Kind: RelationInvokesExternal,
			FromRef: "animate", ToRefs: []string{"platform-raf"},
			Resolution: ResolutionExact, Location: location(3), TargetsObserved: 1,
			Witnesses:         []Witness{{Kind: "compiler", Location: location(3)}},
			WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []RelationPatternInput{{
				SourceRef: "platform-call-pattern", Form: PatternCall,
				Selector: "requestAnimationFrame", Location: location(3),
				Arguments: []PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
		{
			SourceRef: "axios-call", Kind: RelationInvokesExternal,
			FromRef: "animate", ToRefs: []string{"axios-post"},
			Resolution: ResolutionExact, Location: location(4), TargetsObserved: 1,
			Witnesses:         []Witness{{Kind: "compiler", Location: location(4)}},
			WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []RelationPatternInput{{
				SourceRef: "axios-call-pattern", Form: PatternCall,
				Selector: "post", Location: location(4),
				Arguments: []PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
		{
			SourceRef: "platform-structural-call", Kind: RelationCalls,
			FromRef: "animate", ToRefs: []string{"platform-raf"},
			Resolution: ResolutionExact, Location: location(5), TargetsObserved: 1,
			Witnesses:         []Witness{{Kind: "compiler", Location: location(5)}},
			WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []RelationPatternInput{{
				SourceRef: "platform-structural-pattern", Form: PatternCall,
				Selector: "requestAnimationFrame", Location: location(5),
				Arguments: []PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
	}
	index, err := New(Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: TargetInput{
			Language: "jsts", Kind: "application", Name: "runtime", Selector: "jsts:runtime",
			Sources:       []TargetSource{{FileRef: "f1", Path: "src/runtime.ts"}},
			AnchorFileRef: "f1",
			Seeds:         []TargetSeedInput{{ObjectRef: "animate", Kind: SeedCallable, Location: location(1)}},
		},
		Objects: objects, Relations: relations,
		Coverage: CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("New platform categorization index: %v", err)
	}
	return index
}

func resealCategorizationTestIndex(t *testing.T, index *Index) {
	t.Helper()
	index.SHA256 = ""
	digest, err := indexDigest(*index)
	if err != nil {
		t.Fatalf("indexDigest: %v", err)
	}
	index.SHA256 = digest
}
