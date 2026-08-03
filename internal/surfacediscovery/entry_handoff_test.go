package surfacediscovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestAnalyzePublishesExactNeutralEntryHandoffs(t *testing.T) {
	repository, input := writeNeutralEntryHandoffFixture(t)
	result, err := AnalyzeWithInput(DefaultOptions(repository), input)
	if err != nil {
		t.Fatal(err)
	}

	if result.Grounding.Version != ArchitectureGroundingVersion || ArchitectureGroundingVersion != 5 {
		t.Fatalf("grounding version = %d, want 5", result.Grounding.Version)
	}
	if len(result.Grounding.EntryHandoffs) != 2 {
		t.Fatalf("entry handoffs = %#v, want two neutral repository callees", result.Grounding.EntryHandoffs)
	}
	byName := make(map[string]EntryHandoff)
	for _, handoff := range result.Grounding.EntryHandoffs {
		byName[handoff.Callee.Name] = handoff
		if handoff.ProcessEntrypoint.Name != "main" ||
			handoff.ProcessEntrypoint.Package != "example.com/neutral" ||
			handoff.TargetPackage != handoff.Callee.Package ||
			handoff.Certainty != "static" ||
			handoff.Producer.Operation != "collect_entry_direct_static_handoff" ||
			!strings.Contains(strings.Join(handoff.Limitations, " "), "transitive reachability are not observed") {
			t.Fatalf("entry handoff contract = %#v", handoff)
		}
	}
	if byName["Rill"].WitnessCount != 2 || byName["Mica"].WitnessCount != 1 {
		t.Fatalf("merged witness counts = %#v", byName)
	}
	coverage := result.Grounding.Coverage.EntryHandoffs
	if !coverage.Complete || len(coverage.Reasons) != 0 ||
		coverage.CandidatesConsidered != 2 || coverage.CandidatesCollected != 2 ||
		coverage.CandidatesPublished != 2 || coverage.WitnessesConsidered != 3 ||
		len(coverage.CandidateSetSHA256) != 64 {
		t.Fatalf("entry handoff coverage = %#v", coverage)
	}

	for _, handoff := range result.Grounding.EntryHandoffs {
		for _, anchor := range result.Grounding.Anchors {
			for _, member := range anchor.AssociatedMembers {
				if member.ID == handoff.Callee.ID {
					t.Fatalf("entry handoff callee leaked into behavior anchors: %#v", anchor)
				}
			}
		}
	}
	if len(result.Grounding.Relationships) != 0 {
		t.Fatalf("neutral entry handoffs became Architecture relationships: %#v", result.Grounding.Relationships)
	}
}

func TestAnalyzeEntryHandoffsExcludeDynamicExternalRecursiveAndAuxiliaryCalls(t *testing.T) {
	repository, input := writeNeutralEntryHandoffFixture(t)
	result, err := AnalyzeWithInput(DefaultOptions(repository), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, handoff := range result.Grounding.EntryHandoffs {
		for _, excluded := range []string{"Turn", "fmt.Println", "cmd/helper"} {
			if strings.Contains(handoff.Callee.ID, excluded) {
				t.Fatalf("excluded call promoted to production handoff: %#v", handoff)
			}
		}
		if handoff.Callee.Name == "init" || handoff.Callee.ID == handoff.ProcessEntrypoint.ID {
			t.Fatalf("init promoted to production handoff: %#v", handoff)
		}
	}
}

func TestAnalyzeEntryHandoffProjectionIsPermutationStable(t *testing.T) {
	repository, input := writeNeutralEntryHandoffFixture(t)
	first, err := AnalyzeWithInput(DefaultOptions(repository), input)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(input.Entrypoints)
	second, err := AnalyzeWithInput(DefaultOptions(repository), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Grounding.EntryHandoffs, second.Grounding.EntryHandoffs) ||
		!reflect.DeepEqual(first.Grounding.Coverage.EntryHandoffs, second.Grounding.Coverage.EntryHandoffs) {
		t.Fatalf(
			"permuted producer input changed handoffs:\nfirst=%#v / %#v\nsecond=%#v / %#v",
			first.Grounding.EntryHandoffs,
			first.Grounding.Coverage.EntryHandoffs,
			second.Grounding.EntryHandoffs,
			second.Grounding.Coverage.EntryHandoffs,
		)
	}
}

func TestBoundEntryHandoffsNeverClaimsCompleteAfterPersistenceLimit(t *testing.T) {
	handoffs := make([]EntryHandoff, 3)
	for index := range handoffs {
		handoffs[index].ID = fmt.Sprintf("entry-handoff-%d", index)
	}
	coverage := EntryHandoffCoverage{
		Complete:             true,
		Reasons:              []GroundingCoverageReason{},
		CandidateSetSHA256:   strings.Repeat("a", 64),
		CandidatesConsidered: len(handoffs),
		CandidatesCollected:  len(handoffs),
	}

	got, gotCoverage, limited := boundEntryHandoffs(handoffs, coverage, 2)
	if !limited || len(got) != 2 || gotCoverage.Complete ||
		gotCoverage.CandidatesPublished != 2 ||
		!slices.Contains(gotCoverage.Reasons, GroundingCoveragePersistenceLimit) {
		t.Fatalf("bounded handoffs = %#v, coverage=%#v, limited=%t", got, gotCoverage, limited)
	}
	if gotCoverage.CandidateSetSHA256 != coverage.CandidateSetSHA256 ||
		gotCoverage.CandidatesConsidered != len(handoffs) {
		t.Fatalf("bounded projection lost complete candidate diagnostics: %#v", gotCoverage)
	}
}

func TestBoundCollectedEntryHandoffsKeepsFullCandidateDigestAndClosedReason(t *testing.T) {
	handoffs := make([]EntryHandoff, 3)
	for index := range handoffs {
		handoffs[index] = EntryHandoff{
			ID:           fmt.Sprintf("entry-handoff-%d", index),
			WitnessCount: index + 1,
		}
	}
	wantDigest := entryHandoffCandidateSetSHA256(handoffs)

	got, coverage, limited := boundCollectedEntryHandoffs(handoffs, 6, 2)
	if !limited || len(got) != 2 || coverage.Complete ||
		coverage.CandidatesConsidered != 3 || coverage.CandidatesCollected != 2 ||
		coverage.WitnessesConsidered != 6 || coverage.CandidateSetSHA256 != wantDigest ||
		!slices.Contains(coverage.Reasons, GroundingCoverageCollectionLimit) {
		t.Fatalf("bounded collection = %#v, coverage=%#v, limited=%t", got, coverage, limited)
	}
}

func TestArchitectureGroundingV5JSONCarriesSeparateEntryHandoffs(t *testing.T) {
	grounding := ArchitectureGrounding{
		Version:       ArchitectureGroundingVersion,
		Anchors:       []BehaviorAnchor{},
		Relationships: []BehaviorRelationship{},
		EntryHandoffs: []EntryHandoff{{ID: "entry-handoff-one"}},
		Coverage: GroundingCoverage{EntryHandoffs: EntryHandoffCoverage{
			Complete: true, Reasons: []GroundingCoverageReason{},
			CandidateSetSHA256: strings.Repeat("0", 64), CandidatesConsidered: 1,
			CandidatesCollected: 1, CandidatesPublished: 1, WitnessesConsidered: 1,
		}},
	}
	encoded, err := MarshalDeterministic(grounding)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["entry_handoffs"]; !ok {
		t.Fatalf("grounding v5 lacks entry_handoffs: %s", encoded)
	}
}

func writeNeutralEntryHandoffFixture(t *testing.T) (string, Input) {
	t.Helper()
	repository := t.TempDir()
	for _, directory := range []string{"alpha", "beta", "gamma", filepath.Join("cmd", "helper")} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/neutral\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "alpha", "alpha.go"), `package alpha

func Rill() {}
func init() {}
`)
	writeFixtureFile(t, filepath.Join(repository, "beta", "beta.go"), `package beta

func Mica() {}
`)
	writeFixtureFile(t, filepath.Join(repository, "gamma", "gamma.go"), `package gamma

func Auxiliary() {}
`)
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import (
	"fmt"
	"example.com/neutral/alpha"
	"example.com/neutral/beta"
)

type rotor interface { Turn() }
type localRotor struct{}
func (localRotor) Turn() {}

func main() {
	alpha.Rill()
	alpha.Rill()
	beta.Mica()
	var value rotor = localRotor{}
	value.Turn()
	fmt.Println("external")
	main()
}
`)
	writeFixtureFile(t, filepath.Join(repository, "cmd", "helper", "main.go"), `package main

import "example.com/neutral/gamma"

func main() { gamma.Auxiliary() }
`)
	return repository, Input{
		RepositoryName: "neutral",
		ModuleDirs:     []string{"."},
		Entrypoints: []EntrypointInput{
			{
				Package: "example.com/neutral", PackageDir: ".", ModuleDir: ".", Kind: "primary_binary",
				Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "main.go", Line: 13}},
			},
			{
				Package: "example.com/neutral/cmd/helper", PackageDir: "cmd/helper", ModuleDir: ".", Kind: "test_binary",
				Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/helper/main.go", Line: 5}},
			},
		},
	}
}
