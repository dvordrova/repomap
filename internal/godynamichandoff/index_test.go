package godynamichandoff

import (
	"strings"
	"testing"
)

func TestNewSealsExactAndUncertainDynamicHandoffs(t *testing.T) {
	functions := []Function{
		{ID: "n-main", Package: "example.com/app", Symbol: "example.com/app.main", Location: Location{Path: "main.go", Line: 8, Column: 6}},
		{ID: "n-serve", Package: "example.com/app", Symbol: "example.com/app.serve", Location: Location{Path: "server.go", Line: 12, Column: 6}},
		{ID: "n-a", Package: "example.com/app", Symbol: "(*example.com/app.A).Run", Location: Location{Path: "runner.go", Line: 10, Column: 13}},
		{ID: "n-b", Package: "example.com/app", Symbol: "(*example.com/app.B).Run", Location: Location{Path: "runner.go", Line: 18, Column: 13}},
		{ID: "n-handler", Package: "example.com/app", Symbol: "example.com/app.handler", Location: Location{Path: "handler.go", Line: 7, Column: 6}},
	}
	index, err := New(Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64", Tags: []string{"netgo", "netgo"}},
		SourceDirectCallSHA256: strings.Repeat("a", 64),
		Functions:              functions,
		Handoffs: []Handoff{
			{
				Kind: InterfaceInvoke, CallerID: "n-serve", Invocation: InvocationSynchronous,
				Callsite:   Location{Path: "server.go", Line: 21, Column: 12},
				Slot:       Slot{DeclaredType: "example.com/app.Runner", Method: "Run", Signature: "func(context.Context) error"},
				Resolution: ResolutionAlternatives,
				Candidates: []Candidate{
					{FunctionID: "n-b", Evidence: EvidenceInterfaceValueAlternative},
					{FunctionID: "n-a", Evidence: EvidenceInterfaceValueAlternative},
				},
			},
			{
				Kind: FunctionValueCall, CallerID: "n-serve", Invocation: InvocationDeferred,
				Callsite: Location{Path: "server.go", Line: 27, Column: 10},
				Slot:     Slot{Signature: "func()"}, Resolution: ResolutionExact,
				Candidates: []Candidate{{FunctionID: "n-handler", Evidence: EvidenceUniqueValueFlow}},
			},
			{
				Kind: CallbackTransfer, CallerID: "n-main", Invocation: InvocationSynchronous,
				Callsite:     Location{Path: "main.go", Line: 11, Column: 17},
				StaticTarget: StaticTarget{Package: "net/http", Name: "HandleFunc"},
				Slot:         Slot{Signature: "func(http.ResponseWriter, *http.Request)", Parameter: 2},
				Resolution:   ResolutionExact,
				Candidates:   []Candidate{{FunctionID: "n-handler", Evidence: EvidenceClosureValue}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if index.SHA256 == "" || len(index.Scenario.Tags) != 1 || len(index.Handoffs) != 3 {
		t.Fatalf("non-canonical index: %#v", index)
	}
	if index.Coverage.InterfaceInvokes != 1 || index.Coverage.FunctionValueCalls != 1 ||
		index.Coverage.CallbackTransfers != 1 || index.Coverage.ExactResolutions != 2 ||
		index.Coverage.AlternativeResolutions != 1 || index.Coverage.CandidatesIndexed != 4 ||
		index.Coverage.HandoffsObserved != 3 || index.Coverage.HandoffsOmitted != 0 {
		t.Fatalf("coverage = %#v", index.Coverage)
	}

	snapshot := index.Snapshot()
	snapshot.Handoffs[0].Candidates[0].FunctionID = "changed"
	if index.Handoffs[0].Candidates[0].FunctionID == "changed" {
		t.Fatal("Snapshot aliases candidate storage")
	}
	tampered := index.Snapshot()
	for position := range tampered.Handoffs {
		if tampered.Handoffs[position].Kind == InterfaceInvoke {
			tampered.Handoffs[position].Resolution = ResolutionExact
			break
		}
	}
	if err := tampered.Validate(); err == nil {
		t.Fatal("interface alternatives were accepted as an exact runtime call")
	}
}

func TestNewRejectsInterfaceRuntimeCandidateWithoutValueFlowAuthority(t *testing.T) {
	_, err := New(Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("b", 64),
		Functions: []Function{
			{ID: "n-caller", Package: "example.com/app", Symbol: "example.com/app.main", Location: Location{Path: "main.go", Line: 3, Column: 6}},
			{ID: "n-handler", Package: "example.com/app", Symbol: "example.com/app.handler", Location: Location{Path: "main.go", Line: 7, Column: 6}},
		},
		Handoffs: []Handoff{{
			Kind: InterfaceInvoke, CallerID: "n-caller", Invocation: InvocationSynchronous,
			Callsite:   Location{Path: "main.go", Line: 4, Column: 10},
			Slot:       Slot{DeclaredType: "example.com/app.Runner", Method: "Run", Signature: "func()"},
			Resolution: ResolutionAlternatives,
			Candidates: []Candidate{{FunctionID: "n-handler", Evidence: EvidenceValueFlowAlternative}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "lacks concrete SSA value flow") {
		t.Fatalf("New error = %v, want interface runtime-authority rejection", err)
	}
}

func TestNewRetainsHonestCandidateOmissionAccounting(t *testing.T) {
	index, err := New(Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("c", 64),
		Functions: []Function{
			{ID: "n-caller", Package: "example.com/app", Symbol: "example.com/app.run", Location: Location{Path: "run.go", Line: 3, Column: 6}},
			{ID: "n-a", Package: "example.com/app", Symbol: "example.com/app.a", Location: Location{Path: "run.go", Line: 8, Column: 6}},
			{ID: "n-b", Package: "example.com/app", Symbol: "example.com/app.b", Location: Location{Path: "run.go", Line: 9, Column: 6}},
		},
		Handoffs: []Handoff{{
			Kind: FunctionValueCall, CallerID: "n-caller", Invocation: InvocationGoroutine,
			Callsite: Location{Path: "run.go", Line: 5, Column: 5}, Slot: Slot{Signature: "func()"},
			Resolution: ResolutionUnresolved, CandidatesConsidered: 9,
		}},
		Coverage: CoverageInput{UnsupportedCallers: 2, InvalidCallsites: 3, UnsupportedStaticTargets: 4},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if index.Handoffs[0].CandidatesOmitted != 9 || index.Coverage.CandidatesOmitted != 9 {
		t.Fatalf("omission accounting = %#v / %#v", index.Handoffs[0], index.Coverage)
	}
	if index.Coverage.HandoffsObserved != 10 || index.Coverage.HandoffsIndexed != 1 ||
		index.Coverage.HandoffsOmitted != 9 {
		t.Fatalf("handoff omission accounting = %#v", index.Coverage)
	}
}
