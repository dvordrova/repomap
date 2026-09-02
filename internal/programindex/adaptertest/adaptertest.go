// Package adaptertest is the cold-start conformance kit for a language
// adapter's one atomic ProgramIndex input snapshot.
package adaptertest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// Adapter is deliberately one atomic operation. A language adapter captures
// one consistent compiler/extractor snapshot and returns all five logical
// ProgramIndex tables together; consumers must not call stateful table getters.
type Adapter interface {
	BuildProgramInput() (programindex.Input, error)
}

// AdapterFunc lets an adapter conformance test wrap its normal build function
// without defining boilerplate.
type AdapterFunc func() (programindex.Input, error)

func (build AdapterFunc) BuildProgramInput() (programindex.Input, error) {
	return build()
}

// AssertConforms proves that one adapter produces a deterministic atomic
// snapshot which the common builder can join, canonicalize, bound, seal and
// round-trip. Adapter-specific fixture assertions should run after this call.
func AssertConforms(t testing.TB, adapter Adapter) programindex.Index {
	t.Helper()
	firstInput, err := adapter.BuildProgramInput()
	if err != nil {
		t.Fatalf("adapter BuildProgramInput: %v", err)
	}
	first, err := programindex.New(firstInput)
	if err != nil {
		t.Fatalf("programindex.New(adapter input): %v", err)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("sealed ProgramIndex.Validate: %v", err)
	}
	encoded, err := programindex.Encode(first)
	if err != nil {
		t.Fatalf("programindex.Encode: %v", err)
	}
	restored, err := programindex.Decode(encoded)
	if err != nil {
		t.Fatalf("programindex.Decode: %v", err)
	}
	if !reflect.DeepEqual(restored, first) {
		t.Fatal("ProgramIndex codec changed the adapter snapshot")
	}

	secondInput, err := adapter.BuildProgramInput()
	if err != nil {
		t.Fatalf("second adapter BuildProgramInput: %v", err)
	}
	second, err := programindex.New(secondInput)
	if err != nil {
		t.Fatalf("second programindex.New(adapter input): %v", err)
	}
	if !reflect.DeepEqual(second, first) {
		t.Fatal("adapter produced a non-deterministic sealed ProgramIndex")
	}
	return first
}

// ReferenceAdapter is a copyable minimal adapter. It intentionally models a
// synthetic language so the example cannot pass through a registered-language
// special case in a semantic or report package.
type ReferenceAdapter struct {
	Language string
}

func (adapter ReferenceAdapter) BuildProgramInput() (programindex.Input, error) {
	language := adapter.Language
	if language == "" {
		language = "reference-language"
	}
	callsite := &programindex.Location{Path: "src/service.ref", Line: 12, Column: 5}
	return programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: "reference service", Selector: "reference:service",
			Sources:       []programindex.TargetSource{{FileRef: "source", Path: "src/service.ref"}},
			AnchorFileRef: "source",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "service.run", Kind: programindex.SeedCallable,
				Location: &programindex.Location{Path: "src/service.ref", Line: 8, Column: 3},
			}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "package", Kind: programindex.ObjectPackage, Name: "example.service", Visibility: programindex.VisibilityPublic},
			{SourceRef: "service", Kind: programindex.ObjectType, Name: "Service", Visibility: programindex.VisibilityPublic,
				OwnerRef: "package", ContainerRef: "package"},
			{SourceRef: "service.run", Kind: programindex.ObjectMethod, Name: "run", Visibility: programindex.VisibilityPublic,
				OwnerRef: "service", ContainerRef: "package",
				Location: &programindex.Location{Path: "src/service.ref", Line: 8, Column: 3},
				SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{
					Domain: language + ".public-callable.v1",
					Parts:  []string{"method", "example.service", "Service", "run"}, Display: "Service.run",
				}}},
			{SourceRef: "external.send", Kind: programindex.ObjectExternalSymbol, Name: "example.client.Client.send",
				Visibility: programindex.VisibilityPublic,
				External: &programindex.ExternalSymbol{
					AuthorityKind: programindex.ExternalAuthorityPackage,
					PackagePath:   "example.client", Receiver: "Client", Name: "send",
				}},
		},
		Relations: []programindex.RelationInput{{
			SourceRef: "service.run->client.send", Kind: programindex.RelationInvokesExternal,
			FromRef: "service.run", ToRefs: []string{"external.send"}, Resolution: programindex.ResolutionExact,
			Location: callsite, TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "compiler_call", Location: callsite}}, WitnessesObserved: 1,
		}},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 1},
	}, nil
}
