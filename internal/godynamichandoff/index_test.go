package godynamichandoff

import (
	"fmt"
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
			{
				Kind: CallableBinding, CallerID: "n-main", Invocation: InvocationBinding,
				Callsite: Location{Path: "main.go", Line: 12, Column: 20},
				Slot: Slot{
					ContainerType: "example.com/app.Server", DeclaredType: "example.com/app.Handler",
					Field: "Handler", Signature: "func()",
				},
				Resolution: ResolutionExact,
				Candidates: []Candidate{{FunctionID: "n-handler", Evidence: EvidenceUniqueValueFlow}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if index.SHA256 == "" || len(index.Scenario.Tags) != 1 || len(index.Handoffs) != 4 {
		t.Fatalf("non-canonical index: %#v", index)
	}
	if index.Coverage.InterfaceInvokes != 1 || index.Coverage.FunctionValueCalls != 1 ||
		index.Coverage.CallbackTransfers != 1 || index.Coverage.CallableBindings != 1 ||
		index.Coverage.ExactResolutions != 3 ||
		index.Coverage.AlternativeResolutions != 1 || index.Coverage.CandidatesIndexed != 5 ||
		index.Coverage.HandoffsObserved != 4 || index.Coverage.HandoffsOmitted != 0 {
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

func TestInterfaceFieldAssignmentsRemainPossibleAndKeepSources(t *testing.T) {
	a := Location{Path: "factory.go", Line: 5, Column: 10}
	b := Location{Path: "factory.go", Line: 9, Column: 10}
	input := Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("f", 64),
		Functions: []Function{
			{ID: "caller", Package: "example.com/app", Symbol: "Facade.Run", Location: Location{Path: "facade.go", Line: 3, Column: 1}},
			{ID: "engine", Package: "example.com/app", Symbol: "Engine.Run", Location: Location{Path: "engine.go", Line: 3, Column: 1}},
		},
		Handoffs: []Handoff{{
			Kind: InterfaceInvoke, CallerID: "caller", Invocation: InvocationSynchronous,
			Callsite:   Location{Path: "facade.go", Line: 4, Column: 2},
			Slot:       Slot{ContainerType: "example.com/app.Facade", Field: "engine", DeclaredType: "example.com/app.Runner", Method: "Run", Signature: "func()"},
			Resolution: ResolutionAlternatives, CandidatesConsidered: 2,
			Candidates: []Candidate{
				{FunctionID: "engine", Evidence: EvidenceInterfaceFieldAssignment, Assignments: []Location{b, a}},
				{FunctionID: "engine", Evidence: EvidenceInterfaceFieldAssignment, Assignments: []Location{a}},
			},
		}},
	}
	index, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	handoff := index.Handoffs[0]
	if len(handoff.Candidates) != 1 || handoff.CandidatesOmitted != 1 || len(handoff.Candidates[0].Assignments) != 2 {
		t.Fatalf("field sources or open receiver lost: %+v", handoff)
	}
	input.Handoffs[0].Candidates[0], input.Handoffs[0].Candidates[1] = input.Handoffs[0].Candidates[1], input.Handoffs[0].Candidates[0]
	reordered, err := New(input)
	if err != nil || reordered.SHA256 != index.SHA256 {
		t.Fatalf("assignment order changed identity: %v", err)
	}
	snapshot := index.Snapshot()
	snapshot.Handoffs[0].Candidates[0].Assignments[0].Line = 100
	if index.Handoffs[0].Candidates[0].Assignments[0].Line == 100 {
		t.Fatal("Snapshot aliases assignment locations")
	}
	input.Handoffs[0].Resolution = ResolutionExact
	input.Handoffs[0].CandidatesConsidered = 1
	if _, err := New(input); err == nil {
		t.Fatal("observed field store accepted as an exact runtime call")
	}
	input.Handoffs[0].Resolution = ResolutionAlternatives
	input.Handoffs[0].CandidatesConsidered = 2
	input.Handoffs[0].Candidates = []Candidate{{FunctionID: "engine", Evidence: EvidenceInterfaceFieldAssignment}}
	if _, err := New(input); err == nil {
		t.Fatal("field candidate accepted without its source")
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

func TestNewRetainsKnownPartialAlternativeAndCountsOnlyOpenFrontier(t *testing.T) {
	functions := []Function{
		{ID: "n-caller", Package: "example.com/app", Symbol: "example.com/app.run", Location: Location{Path: "run.go", Line: 3, Column: 6}},
		{ID: "n-known", Package: "example.com/app", Symbol: "example.com/app.known", Location: Location{Path: "run.go", Line: 8, Column: 6}},
	}
	input := Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("e", 64),
		Functions:              functions,
		Handoffs: []Handoff{{
			Kind: FunctionValueCall, CallerID: "n-caller", Invocation: InvocationSynchronous,
			Callsite: Location{Path: "run.go", Line: 5, Column: 5}, Slot: Slot{Signature: "func()"},
			Resolution:           ResolutionAlternatives,
			Candidates:           []Candidate{{FunctionID: "n-known", Evidence: EvidenceUniqueValueFlow}},
			CandidatesConsidered: 2,
		}},
	}
	index, err := New(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handoff := index.Handoffs[0]
	if handoff.Resolution != ResolutionAlternatives || len(handoff.Candidates) != 1 ||
		handoff.Candidates[0].FunctionID != "n-known" || handoff.CandidatesConsidered != 2 ||
		handoff.CandidatesOmitted != 1 || index.Coverage.CandidatesIndexed != 1 ||
		index.Coverage.CandidatesConsidered != 2 || index.Coverage.CandidatesOmitted != 1 {
		t.Fatalf("known-partial handoff = %#v, coverage = %#v", handoff, index.Coverage)
	}

	invalidExact := input
	invalidExact.Handoffs = append([]Handoff(nil), input.Handoffs...)
	invalidExact.Handoffs[0].Resolution = ResolutionExact
	if _, err := New(invalidExact); err == nil || !strings.Contains(err.Error(), "invalid exact resolution") {
		t.Fatalf("exact resolution accepted an open frontier: %v", err)
	}

	incompleteAlternative := input
	incompleteAlternative.Handoffs = append([]Handoff(nil), input.Handoffs...)
	incompleteAlternative.Handoffs[0].CandidatesConsidered = 1
	if _, err := New(incompleteAlternative); err == nil || !strings.Contains(err.Error(), "open frontier") {
		t.Fatalf("single complete candidate accepted as alternatives: %v", err)
	}
}

func TestCallableReceiverFieldsAreCanonicalOwnedAndAnchored(t *testing.T) {
	first := ReceiverField{Field: "Name", Literal: `"restore"`, Location: Location{Path: "main.go", Line: 4, Column: 5}}
	second := ReceiverField{Field: "Name", Literal: `"recover"`, Location: Location{Path: "main.go", Line: 7, Column: 5}}
	input := Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("f", 64),
		Functions: []Function{
			{ID: "factory", Package: "example.com/app", Symbol: "example.com/app.factory", Location: Location{Path: "main.go", Line: 3, Column: 6}},
			{ID: "callback", Package: "example.com/app", Symbol: "example.com/app.callback", Location: Location{Path: "main.go", Line: 10, Column: 6}},
		},
		Handoffs: []Handoff{{Kind: CallableBinding, CallerID: "factory", Invocation: InvocationBinding,
			Callsite: Location{Path: "main.go", Line: 5, Column: 5}, Slot: Slot{ContainerType: "example.com/app.Action", Field: "Run", DeclaredType: "func()"},
			Resolution: ResolutionExact, Candidates: []Candidate{{FunctionID: "callback", Evidence: EvidenceDirectFunctionValue}},
			ReceiverFields: []ReceiverField{second, first, first},
		}},
	}
	index, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Handoffs[0].ReceiverFields) != 2 {
		t.Fatal("distinct assignments were merged")
	}
	input.Handoffs[0].ReceiverFields = []ReceiverField{first, second}
	again, err := New(input)
	if err != nil || again.SHA256 != index.SHA256 {
		t.Fatalf("field order changed sealed digest: %v", err)
	}
	input.Handoffs[0].ReceiverFields[0].Literal = `"changed"`
	copy := index.Snapshot()
	copy.Handoffs[0].ReceiverFields[0].Literal = `"changed"`
	if index.Handoffs[0].ReceiverFields[0] != first {
		t.Fatal("field storage aliases input or snapshot")
	}
	for _, invalid := range []ReceiverField{
		{Field: "Name", Literal: `"x"`},
		{Field: "Run", Literal: `"x"`, Location: first.Location},
		{Field: "Name", Literal: "", Location: first.Location},
	} {
		input.Handoffs[0].ReceiverFields = []ReceiverField{invalid}
		if _, err := New(input); err == nil {
			t.Fatalf("accepted invalid field: %+v", invalid)
		}
	}
}

func TestTransferredInterfaceMethodKeepsItsRecipientAndUncertainty(t *testing.T) {
	input := Input{
		Scenario: Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"}, SourceDirectCallSHA256: strings.Repeat("a", 64),
		Functions: []Function{{ID: "from", Package: "example.com/app", Symbol: "example.com/app.install", Location: Location{Path: "main.go", Line: 1, Column: 1}}, {ID: "method", Package: "example.com/app", Symbol: "example.com/app.Service.Apply", Location: Location{Path: "main.go", Line: 7, Column: 1}}},
		Handoffs:  []Handoff{{Kind: CallbackTransfer, CallerID: "from", Invocation: InvocationSynchronous, Callsite: Location{Path: "main.go", Line: 3, Column: 2}, StaticTarget: StaticTarget{Package: "company/framework", Name: "Mount"}, Slot: Slot{Parameter: 2, DeclaredType: "company/framework.Service", Method: "Apply", Signature: "func(Request) Response"}, Resolution: ResolutionAlternatives, Candidates: []Candidate{{FunctionID: "method", Evidence: EvidenceConcreteInterfaceValue}}, CandidatesConsidered: 2}},
	}
	index, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	if index.Handoffs[0].CandidatesOmitted != 1 || index.Coverage.CallbackTransfers != 1 {
		t.Fatal("object transfer lost its open frontier")
	}
	input.Handoffs[0].Slot.Method = ""
	if _, err := New(input); err == nil {
		t.Fatal("accepted interface without its declared method")
	}
	input.Handoffs[0].Slot.Method = "Apply"
	input.Handoffs[0].Candidates[0].Evidence = EvidenceDirectFunctionValue
	if _, err := New(input); err == nil {
		t.Fatal("ordinary function gained interface method authority")
	}
}

func TestNewRetainsCandidatesAndTextBeyondFormerLocalThresholds(t *testing.T) {
	const (
		formerCandidatesPerHandoff = 32
		formerTextBytes            = 16 * 1024
	)
	functions := []Function{{
		ID: "n-caller", Package: "example.com/app", Symbol: "example.com/app.run",
		Location: Location{Path: "run.go", Line: 1, Column: 1},
	}}
	candidates := make([]Candidate, 0, formerCandidatesPerHandoff+1)
	for position := 0; position <= formerCandidatesPerHandoff; position++ {
		id := fmt.Sprintf("n-candidate-%02d", position)
		functions = append(functions, Function{
			ID: id, Package: "example.com/app", Symbol: "example.com/app." + id,
			Location: Location{Path: "run.go", Line: position + 2, Column: 1},
		})
		candidates = append(candidates, Candidate{
			FunctionID: id, Evidence: EvidenceValueFlowAlternative,
		})
	}
	longSignature := "func(" + strings.Repeat("x", formerTextBytes+1) + ")"
	index, err := New(Input{
		Scenario:               Scenario{ID: "go:linux/amd64", GOOS: "linux", GOARCH: "amd64"},
		SourceDirectCallSHA256: strings.Repeat("d", 64),
		Functions:              functions,
		Handoffs: []Handoff{{
			Kind: FunctionValueCall, CallerID: "n-caller", Invocation: InvocationSynchronous,
			Callsite: Location{Path: "run.go", Line: 100, Column: 1},
			Slot:     Slot{Signature: longSignature}, Resolution: ResolutionAlternatives,
			Candidates: candidates,
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(index.Handoffs) != 1 || len(index.Handoffs[0].Candidates) != formerCandidatesPerHandoff+1 ||
		index.Handoffs[0].CandidatesOmitted != 0 || index.Handoffs[0].Slot.Signature != longSignature {
		t.Fatalf("retained handoff = %#v", index.Handoffs)
	}
	if index.Coverage.CandidatesIndexed != formerCandidatesPerHandoff+1 ||
		index.Coverage.CandidatesOmitted != 0 {
		t.Fatalf("candidate coverage = %#v", index.Coverage)
	}
}
