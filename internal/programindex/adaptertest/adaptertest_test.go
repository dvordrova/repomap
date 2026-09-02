package adaptertest

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestReferenceAdapterProvesColdStartContract(t *testing.T) {
	index := AssertConforms(t, ReferenceAdapter{Language: "synthetic-jvm"})
	if index.Target.Language != "synthetic-jvm" || len(index.Target.Seeds) != 1 ||
		len(index.Objects) != 4 || len(index.Relations) != 1 ||
		index.Relations[0].Resolution != programindex.ResolutionExact {
		t.Fatalf("reference index = %#v", index)
	}
	for _, object := range index.Objects {
		if object.SourceRef != "external.send" {
			continue
		}
		if object.External == nil ||
			object.External.AuthorityKind != programindex.ExternalAuthorityPackage ||
			object.External.PackagePath != "example.client" {
			t.Fatalf("reference external authority = %#v", object.External)
		}
		return
	}
	t.Fatal("reference external symbol is missing")
}

func TestRegistrationContractKeepsFullFamilyAndExactNestedJoins(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/service.ref", Line: line, Column: 3}
	}
	input := programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64),
		SourceSHA256:   strings.Repeat("d", 64),
		Target: programindex.TargetInput{
			Language: "reference-language", Kind: "application", Name: "registration family",
			Selector: "reference:registration", Sources: []programindex.TargetSource{{
				FileRef: "source", Path: "src/service.ref",
			}}, AnchorFileRef: "source",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "owner", Kind: programindex.SeedCallable, Location: location(8)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "owner", Kind: programindex.ObjectFunction, Name: "run", Visibility: programindex.VisibilityPublic, Location: location(8)},
			{SourceRef: "callback-a", Kind: programindex.ObjectFunction, Name: "callbackA", Visibility: programindex.VisibilityInternal, Location: location(20)},
			{SourceRef: "callback-b", Kind: programindex.ObjectFunction, Name: "callbackB", Visibility: programindex.VisibilityInternal, Location: location(24)},
			{SourceRef: "factory", Kind: programindex.ObjectExternalSymbol, Name: "dependency.Factory", Visibility: programindex.VisibilityUnknown,
				External: &programindex.ExternalSymbol{
					AuthorityKind: programindex.ExternalAuthorityPackage,
					PackagePath:   "dependency", Name: "Factory",
				}},
			{SourceRef: "receiver", Kind: programindex.ObjectVariable, Name: "receiver", Visibility: programindex.VisibilityInternal, Location: location(9)},
			{SourceRef: "result", Kind: programindex.ObjectVariable, Name: "call result", Visibility: programindex.VisibilityInternal, Location: location(10)},
		},
		Relations: []programindex.RelationInput{
			{
				SourceRef: "registration", Kind: programindex.RelationCalls, FromRef: "owner",
				Resolution: programindex.ResolutionUnresolved, Invocation: "direct", Location: location(10), TargetsObserved: 2,
				Witnesses:         []programindex.Witness{{Kind: "syntax_call", Location: location(10)}, {Kind: "syntax_call", Location: location(11)}},
				WitnessesObserved: 2, PatternsObserved: 2,
				Patterns: []programindex.RelationPatternInput{
					{SourceRef: "pattern-a", Form: programindex.PatternCall, Selector: "bind", Location: location(10),
						ResultRef: "result", ReceiverRef: "receiver", ReceiverOriginRefs: []string{"factory"},
						ReceiverOriginResolution: programindex.ResolutionExact, ReceiverOriginsObserved: 1,
						ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{
							Position: 1, Kind: programindex.PatternDynamic, ObjectRefs: []string{"callback-a"},
							Resolution: programindex.ResolutionExact, ObjectsObserved: 1,
						}}},
					{SourceRef: "pattern-b", Form: programindex.PatternCall, Selector: "bind", Location: location(11),
						ReceiverRef: "receiver", ReceiverOriginRefs: []string{"factory"},
						ReceiverOriginResolution: programindex.ResolutionExact, ReceiverOriginsObserved: 1,
						ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{
							Position: 1, Kind: programindex.PatternDynamic, ObjectRefs: []string{"callback-b"},
							Resolution: programindex.ResolutionExact, ObjectsObserved: 1,
						}}},
				},
			},
			{
				SourceRef: "callback-a-transfer", Kind: programindex.RelationPassesCallback, FromRef: "owner",
				ToRefs: []string{"callback-a"}, Resolution: programindex.ResolutionExact, Location: location(10), TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "callback_argument", Location: location(10)}}, WitnessesObserved: 1,
				SourceArgument: &programindex.PatternArgumentRefInput{
					RelationSourceRef: "registration", PatternSourceRef: "pattern-a", Position: 1,
				},
			},
			{
				SourceRef: "callback-b-transfer", Kind: programindex.RelationPassesCallback, FromRef: "owner",
				ToRefs: []string{"callback-b"}, Resolution: programindex.ResolutionExact, Location: location(11), TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "callback_argument", Location: location(11)}}, WitnessesObserved: 1,
				SourceArgument: &programindex.PatternArgumentRefInput{
					RelationSourceRef: "registration", PatternSourceRef: "pattern-b", Position: 1,
				},
			},
			{
				SourceRef: "continuation", Kind: programindex.RelationCalls, FromRef: "owner",
				Resolution: programindex.ResolutionUnresolved, Invocation: "direct", Location: location(12), TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "syntax_call", Location: location(12)}}, WitnessesObserved: 1,
				PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
					SourceRef: "continuation-pattern", Form: programindex.PatternCall, Selector: "configure", Location: location(12),
					ReceiverRef: "result", Arguments: []programindex.PatternArgumentInput{}, ArgumentsObserved: 0,
				}},
			},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 6, RelationsObserved: 4},
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	objectID := func(sourceRef string) string {
		t.Helper()
		for _, object := range index.Objects {
			if object.SourceRef == sourceRef {
				return object.ID
			}
		}
		t.Fatalf("object source ref %q not found", sourceRef)
		return ""
	}
	owner := objectID("owner")
	callbackA := objectID("callback-a")
	callbackB := objectID("callback-b")
	factory := objectID("factory")
	receiver := objectID("receiver")

	pattern := func(line int, callbackID string, requireResult bool) Pattern {
		return Pattern{
			Form: programindex.PatternCall, Selector: "bind", RequireResult: requireResult,
			ReceiverID: receiver, Path: "src/service.ref", Line: line,
			ReceiverOrigins: ObjectAuthority{IDs: []string{factory}, Resolution: programindex.ResolutionExact, Observed: 1},
			Observed:        1, Arguments: []Argument{{
				Position: 1, Kind: programindex.PatternDynamic,
				Objects: ObjectAuthority{IDs: []string{callbackID}, Resolution: programindex.ResolutionExact, Observed: 1},
			}},
		}
	}
	AssertRegistration(t, index, Registration{
		Name: "two-row neutral registration",
		Registration: Relation{
			Kind: programindex.RelationCalls, FromID: owner, Resolution: programindex.ResolutionUnresolved,
			Invocation: "direct", Path: "src/service.ref", Line: 10,
			TargetsObserved: 2, TargetsOmitted: 2, WitnessesObserved: 2, PatternsObserved: 2,
			Patterns: []Pattern{pattern(10, callbackA, true), pattern(11, callbackB, false)},
		},
		Callbacks: []Callback{
			{RegistrationPattern: 0, ArgumentPosition: 1, Relation: Relation{
				Kind: programindex.RelationPassesCallback, FromID: owner, ToIDs: []string{callbackA},
				Resolution: programindex.ResolutionExact, Path: "src/service.ref", Line: 10,
				TargetsObserved: 1, WitnessesObserved: 1,
			}},
			{RegistrationPattern: 1, ArgumentPosition: 1, Relation: Relation{
				Kind: programindex.RelationPassesCallback, FromID: owner, ToIDs: []string{callbackB},
				Resolution: programindex.ResolutionExact, Path: "src/service.ref", Line: 11,
				TargetsObserved: 1, WitnessesObserved: 1,
			}},
		},
		Continuation: &Relation{
			Kind: programindex.RelationCalls, FromID: owner, Resolution: programindex.ResolutionUnresolved,
			Invocation: "direct", Path: "src/service.ref", Line: 12,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []Pattern{{
				Form: programindex.PatternCall, Selector: "configure", Path: "src/service.ref", Line: 12,
			}},
		},
		ResultPattern: 0, ContinuationPattern: 0, RequireComplete: true,
	})
}
