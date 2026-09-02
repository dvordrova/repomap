package programindex

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestScaleWarningsReportLargeRetainedCollectionsWithoutPerRowOutput(t *testing.T) {
	const largeCollection = 129
	objects := make([]ObjectInput, largeCollection+1)
	objects[0] = ObjectInput{
		SourceRef: "caller", Kind: ObjectFunction, Name: "caller", Visibility: VisibilityInternal,
		Location: &Location{Path: "app.go", Line: 1, Column: 1},
	}
	linkParts := make([]string, MaxSymbolLinkIdentityParts+1)
	for position := range linkParts {
		linkParts[position] = fmt.Sprintf("link-part-%03d", position)
	}
	for position := 0; position <= MaxSymbolLinkIdentitiesPerObject; position++ {
		objects[0].SymbolLinkIdentities = append(objects[0].SymbolLinkIdentities, SymbolLinkIdentityInput{
			Domain: fmt.Sprintf("fixture.link.%03d", position), Parts: append([]string(nil), linkParts...),
		})
	}
	refs := make([]string, largeCollection)
	for position := range refs {
		ref := fmt.Sprintf("target-%03d", position)
		refs[position] = ref
		objects[position+1] = ObjectInput{
			SourceRef: ref, Kind: ObjectFunction, Name: ref, Visibility: VisibilityInternal,
			Location: &Location{Path: "app.go", Line: position + 2, Column: 1},
		}
	}

	witnesses := make([]Witness, largeCollection)
	patterns := make([]RelationPatternInput, largeCollection)
	for position := 0; position < largeCollection; position++ {
		witnesses[position] = Witness{
			Kind: "callsite", Location: &Location{Path: "app.go", Line: position + 2, Column: 1},
		}
		patterns[position] = RelationPatternInput{
			SourceRef: fmt.Sprintf("pattern-%03d", position),
			Form:      PatternCall, Selector: "register",
			Location:                 &Location{Path: "app.go", Line: position + 2, Column: 1},
			ReceiverOriginRefs:       refs,
			ReceiverOriginResolution: ResolutionAlternatives,
			ReceiverOriginsObserved:  len(refs),
			Arguments:                []PatternArgumentInput{}, ArgumentsObserved: 0,
		}
	}
	parts := make([]PatternPartInput, largeCollection)
	for position := range parts {
		if position%2 == 0 {
			parts[position] = PatternPartInput{Kind: PatternPartLiteral, Text: "x"}
		} else {
			parts[position] = PatternPartInput{Kind: PatternPartHole}
		}
	}
	arguments := make([]PatternArgumentInput, largeCollection)
	for position := range arguments {
		arguments[position] = PatternArgumentInput{
			Position: position + 1, Kind: PatternDynamic,
			ObjectRefs: []string{}, ObjectsObserved: 0,
		}
	}
	arguments[0] = PatternArgumentInput{
		Position: 1, Kind: PatternStringTemplate, Parts: parts,
		ObjectRefs: refs, Resolution: ResolutionAlternatives, ObjectsObserved: len(refs),
	}
	longLiteral := strings.Repeat("x", MaxTextBytes+1)
	arguments[1] = PatternArgumentInput{
		Position: 2, Kind: PatternLiteralString, Value: longLiteral,
		ObjectRefs: []string{}, ObjectsObserved: 0,
	}
	patterns[0].Arguments = arguments
	patterns[0].ArgumentsObserved = len(arguments)

	index, err := New(Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: TargetInput{
			Language: "go", Kind: "package", Name: "fixture", Selector: "fixture",
			Sources:       []TargetSource{{FileRef: "f1", Path: "app.go"}},
			AnchorFileRef: "f1",
			Seeds:         []TargetSeedInput{{ObjectRef: "caller", Kind: SeedCallable, Location: &Location{Path: "app.go", Line: 1, Column: 1}}},
		},
		Objects: objects,
		Relations: []RelationInput{{
			SourceRef: "registration", Kind: RelationCalls, FromRef: "caller", ToRefs: refs,
			Resolution: ResolutionAlternatives, Location: &Location{Path: "app.go", Line: 2, Column: 1},
			TargetsObserved: len(refs), Witnesses: witnesses, WitnessesObserved: len(witnesses),
			Patterns: patterns, PatternsObserved: len(patterns),
		}},
		Coverage: CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	warnings := ScaleWarnings(index)
	want := []ScaleWarning{
		{Kind: ScaleWarningRelationTargets, AdvisorySize: 64, AffectedCollections: 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningRelationWitnesses, AdvisorySize: 64, AffectedCollections: 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningRelationPatterns, AdvisorySize: 64, AffectedCollections: 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningPatternArguments, AdvisorySize: 128, AffectedCollections: 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningTemplateParts, AdvisorySize: 64, AffectedCollections: 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningPatternObjectRefs, AdvisorySize: 64, AffectedCollections: largeCollection + 1, MaximumRetained: largeCollection},
		{Kind: ScaleWarningSymbolLinks, AdvisorySize: MaxSymbolLinkIdentitiesPerObject, AffectedCollections: 1, MaximumRetained: MaxSymbolLinkIdentitiesPerObject + 1},
		{Kind: ScaleWarningSymbolLinkParts, AdvisorySize: MaxSymbolLinkIdentityParts, AffectedCollections: MaxSymbolLinkIdentitiesPerObject + 1, MaximumRetained: MaxSymbolLinkIdentityParts + 1},
		{Kind: ScaleWarningSemanticText, AdvisorySize: MaxTextBytes, AffectedCollections: 1, MaximumRetained: len(longLiteral)},
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Fatalf("warnings = %#v, want %#v", warnings, want)
	}
	relation := index.Relations[0]
	var pattern RelationPattern
	for _, candidate := range relation.Patterns {
		if len(candidate.Arguments) == largeCollection {
			pattern = candidate
			break
		}
	}
	if len(pattern.Arguments) != largeCollection {
		t.Fatalf("large retained pattern missing: %#v", relation.Patterns)
	}
	argument := pattern.Arguments[0]
	if len(relation.ToIDs) != largeCollection || len(relation.Witnesses) != largeCollection ||
		len(relation.Patterns) != largeCollection || len(pattern.Arguments) != largeCollection ||
		len(pattern.ReceiverOriginIDs) != largeCollection || len(argument.Parts) != largeCollection ||
		len(argument.ObjectIDs) != largeCollection || relation.TargetsOmitted != 0 ||
		relation.WitnessesOmitted != 0 || relation.PatternsOmitted != 0 || pattern.ArgumentsOmitted != 0 ||
		pattern.ReceiverOriginsOmitted != 0 || argument.ObjectsOmitted != 0 {
		t.Fatalf("diagnostics changed retained index: %#v", index.Coverage)
	}
}

func TestScaleWarningsReportFormerObjectAndRelationTotalsWithoutFiltering(t *testing.T) {
	index := Index{
		Target: Target{
			Sources: make([]TargetSource, MaxTargetSources+1),
			Seeds:   make([]TargetSeed, MaxTargetSeeds+1),
		},
		Objects:   make([]Object, advisoryObjects+1),
		Relations: make([]Relation, advisoryRelations+1),
	}
	warnings := ScaleWarnings(index)
	want := []ScaleWarning{
		{Kind: ScaleWarningTargetSources, AdvisorySize: MaxTargetSources, AffectedCollections: 1, MaximumRetained: MaxTargetSources + 1},
		{Kind: ScaleWarningTargetSeeds, AdvisorySize: MaxTargetSeeds, AffectedCollections: 1, MaximumRetained: MaxTargetSeeds + 1},
		{Kind: ScaleWarningObjects, AdvisorySize: advisoryObjects, AffectedCollections: 1, MaximumRetained: advisoryObjects + 1},
		{Kind: ScaleWarningRelations, AdvisorySize: advisoryRelations, AffectedCollections: 1, MaximumRetained: advisoryRelations + 1},
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Fatalf("total warnings = %#v, want %#v", warnings, want)
	}
	if len(index.Target.Sources) != MaxTargetSources+1 || len(index.Target.Seeds) != MaxTargetSeeds+1 ||
		len(index.Objects) != advisoryObjects+1 || len(index.Relations) != advisoryRelations+1 {
		t.Fatal("warning aggregation changed retained totals")
	}
}

func TestNewRetainsTargetSourcesAndSeedsPastFormerThreshold(t *testing.T) {
	count := MaxTargetSources + 1
	sources := make([]TargetSource, count)
	objects := make([]ObjectInput, count)
	seeds := make([]TargetSeedInput, count)
	for position := 0; position < count; position++ {
		fileRef := fmt.Sprintf("f%05d", position)
		filePath := fmt.Sprintf("pkg/file_%05d.go", position)
		objectRef := fmt.Sprintf("object-%05d", position)
		sources[position] = TargetSource{FileRef: fileRef, Path: filePath}
		objects[position] = ObjectInput{
			SourceRef: objectRef, Kind: ObjectFunction, Name: objectRef,
			Visibility: VisibilityInternal,
			Location:   &Location{Path: filePath, Line: 1, Column: 1},
		}
		seeds[position] = TargetSeedInput{
			ObjectRef: objectRef, Kind: SeedCallable,
			Location: &Location{Path: filePath, Line: 1, Column: 1},
		}
	}
	index, err := New(Input{
		ScenarioSHA256: strings.Repeat("6", 64),
		SourceSHA256:   strings.Repeat("7", 64),
		Target: TargetInput{
			Language: "go", Kind: "package", Name: "large", Selector: "large",
			Sources: sources, AnchorFileRef: sources[0].FileRef, Seeds: seeds,
		},
		Objects: objects, Relations: []RelationInput{},
		Coverage: CoverageInput{Measured: true, ObjectsObserved: len(objects)},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(index.Target.Sources) != count || len(index.Target.Seeds) != count || len(index.Objects) != count {
		t.Fatalf("retained sources/seeds/objects = %d/%d/%d, want %d each",
			len(index.Target.Sources), len(index.Target.Seeds), len(index.Objects), count)
	}
	warnings := ScaleWarnings(index)
	if len(warnings) < 2 || warnings[0].Kind != ScaleWarningTargetSources ||
		warnings[1].Kind != ScaleWarningTargetSeeds {
		t.Fatalf("warnings = %#v, want target source and seed warnings", warnings)
	}
}

func TestScaleWarningsAreEmptyForOrdinaryAndMalformedInputsCannotFail(t *testing.T) {
	index, err := New(Input{
		ScenarioSHA256: strings.Repeat("4", 64), SourceSHA256: strings.Repeat("5", 64),
		Target: TargetInput{
			Language: "go", Kind: "package", Name: "small", Selector: "small",
			Sources:       []TargetSource{{FileRef: "f1", Path: "main.go"}},
			AnchorFileRef: "f1",
			Seeds:         []TargetSeedInput{{ObjectRef: "main", Kind: SeedCallable, Location: &Location{Path: "main.go", Line: 1, Column: 1}}},
		},
		Objects: []ObjectInput{{
			SourceRef: "main", Kind: ObjectFunction, Name: "main", Visibility: VisibilityInternal,
			Location: &Location{Path: "main.go", Line: 1, Column: 1},
		}},
		Relations: []RelationInput{},
		Coverage:  CoverageInput{Measured: true, ObjectsObserved: 1, RelationsObserved: 0},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	warnings := ScaleWarnings(index)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if malformedWarnings := ScaleWarnings(Index{}); len(malformedWarnings) != 0 {
		t.Fatalf("empty malformed diagnostic input = %#v, want no warning and no failure", malformedWarnings)
	}
}
