package guidedtour

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestBundleHashCanonical(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	reordered := cloneBundle(t, bundle)
	reverseComponents(reordered.Components)
	reverseBeats(reordered.Candidates[0].Beats)
	for index := range reordered.Candidates[0].Beats {
		reverseStrings(reordered.Candidates[0].Beats[index].ComponentIDs)
		reverseStrings(reordered.Candidates[0].Beats[index].SurfaceIDs)
		reverseStrings(reordered.Candidates[0].Beats[index].FlowStepIDs)
		reverseEvidence(reordered.Candidates[0].Beats[index].Evidence)
	}

	firstHash, firstJSON, err := BundleHash(bundle)
	if err != nil {
		t.Fatalf("BundleHash() first error = %v", err)
	}
	secondHash, secondJSON, err := BundleHash(reordered)
	if err != nil {
		t.Fatalf("BundleHash() reordered error = %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("BundleHash() hashes differ: %q != %q", firstHash, secondHash)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("BundleHash() canonical JSON differs\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	if len(firstHash) != 64 {
		t.Fatalf("BundleHash() digest length = %d, want 64", len(firstHash))
	}
}

func TestBuildPromptUsesCanonicalBundle(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	original := cloneBundle(t, bundle)
	_, canonical, err := BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(bundle)
	if err != nil {
		t.Fatalf("BuildPrompt() error = %v", err)
	}
	if prompt.Version != PromptVersion {
		t.Fatalf("BuildPrompt() version = %q, want %q", prompt.Version, PromptVersion)
	}
	if !strings.HasSuffix(prompt.User, string(canonical)) {
		t.Fatalf("BuildPrompt() does not end with canonical bundle")
	}
	for _, required := range []string{"candidate_id", "beat_ids", "gap_ids", "saved_trace", "trace_order"} {
		if !strings.Contains(prompt.User, required) {
			t.Errorf("BuildPrompt() missing %q", required)
		}
	}
	if !reflect.DeepEqual(bundle, original) {
		t.Fatalf("BuildPrompt() mutated its bundle")
	}
}

func TestParseProposalStrictJSON(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(testProposal())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		raw     []byte
		wantErr bool
	}{
		{name: "valid", raw: valid},
		{
			name:    "unknown top-level field",
			raw:     append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"extra":true}`)...),
			wantErr: true,
		},
		{
			name:    "unknown nested field",
			raw:     []byte(`{"version":1,"candidate_id":"trace-1","title":"Tour","summary":"Summary","steps":[{"title":"One","explanation":"Explain","beat_ids":["beat-1"],"extra":true}],"gap_summary":[]}`),
			wantErr: true,
		},
		{name: "trailing json", raw: append(append([]byte{}, valid...), []byte(` {}`)...), wantErr: true},
		{name: "fenced json", raw: append(append([]byte("```json\n"), valid...), []byte("\n```")...), wantErr: true},
		{name: "empty", raw: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseProposal(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProposal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProposal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Bundle, *Proposal)
		wantErr string
	}{
		{name: "valid saved trace"},
		{
			name: "unknown candidate",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.CandidateID = "missing"
			},
			wantErr: "unknown candidate",
		},
		{
			name: "unknown beat",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps[1].BeatIDs[0] = "missing"
			},
			wantErr: "unknown beat",
		},
		{
			name: "duplicate beat",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps[2].BeatIDs[0] = "beat-3"
			},
			wantErr: "repeats beat",
		},
		{
			name: "unknown gap",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.GapSummary[0].GapIDs[0] = "missing"
			},
			wantErr: "unknown gap",
		},
		{
			name: "duplicate gap",
			mutate: func(bundle *Bundle, proposal *Proposal) {
				bundle.Candidates[0].Gaps = append(bundle.Candidates[0].Gaps, Gap{
					ID: "gap-2", Label: "Second gap", Detail: "Also unresolved",
				})
				proposal.GapSummary = append(proposal.GapSummary, ProposedGapSummary{
					Explanation: "Again", GapIDs: []string{"gap-1"},
				})
			},
			wantErr: "repeats gap",
		},
		{
			name: "saved trace omits known gap",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.GapSummary = nil
			},
			wantErr: "omits known candidate gap",
		},
		{
			name: "suggested direction omits known gap",
			mutate: func(bundle *Bundle, proposal *Proposal) {
				bundle.Candidates[0].Kind = CandidateSuggestedDirection
				bundle.Candidates[0].OrderingBasis = OrderingEditorial
				proposal.GapSummary = nil
			},
			wantErr: "omits known candidate gap",
		},
		{
			name: "too few steps",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps = proposal.Steps[:2]
			},
			wantErr: "between 3 and 6",
		},
		{
			name: "empty step",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps[1].BeatIDs = nil
			},
			wantErr: "no beat ids",
		},
		{
			name: "saved trace order decreases",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps[1].BeatIDs = []string{"beat-4"}
				proposal.Steps[2].BeatIDs = []string{"beat-3"}
			},
			wantErr: "decreases beat sequence",
		},
		{
			name: "go file in prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Summary = "Read main.go next"
			},
			wantErr: "path-like reference",
		},
		{
			name: "slash path in prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Steps[0].Explanation = "Continue through internal/server"
			},
			wantErr: "path-like reference",
		},
		{
			name: "readme file in prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Summary = "Read README.md next"
			},
			wantErr: "path-like reference",
		},
		{
			name: "go module file in prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Summary = "Read go.mod next"
			},
			wantErr: "path-like reference",
		},
		{
			name: "makefile in prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Summary = "Read Makefile next"
			},
			wantErr: "path-like reference",
		},
		{
			name: "oversized prose",
			mutate: func(_ *Bundle, proposal *Proposal) {
				proposal.Summary = strings.Repeat("x", maxProposalSummaryBytes+1)
			},
			wantErr: "too long",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bundle := testBundle(t)
			proposal := testProposal()
			if tt.mutate != nil {
				tt.mutate(&bundle, &proposal)
			}
			err := ValidateProposal(bundle, proposal)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateProposal() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateProposal() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProposalRejectsUnsupportedSuggestedDirectionBehavior(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	bundle.Candidates[0].Kind = CandidateSuggestedDirection
	bundle.Candidates[0].OrderingBasis = OrderingEditorial
	proposal := testProposal()
	proposal.Title = "Reading an evidence candidate"
	proposal.Summary = "Inspect the supplied static evidence; runtime order remains unproven."
	proposal.Steps[0].Explanation = "Inspect the exact entry evidence."
	proposal.Steps[1].Explanation = "Read the selected static anchors."
	proposal.Steps[2].Explanation = "Inspect where the bounded evidence ends."
	if err := ValidateProposal(bundle, proposal); err != nil {
		t.Fatalf("safe editorial proposal error = %v", err)
	}

	unsafe := proposal
	unsafe.Steps = append([]ProposedStep{}, proposal.Steps...)
	unsafe.Steps[0].Explanation = "Execution begins here and calls the orientation layer."
	if err := ValidateProposal(bundle, unsafe); err == nil ||
		!strings.Contains(err.Error(), "unsupported behavioral assertion") {
		t.Fatalf("unsafe editorial proposal error = %v", err)
	}

	mixed := proposal
	mixed.Steps = append([]ProposedStep{}, proposal.Steps...)
	mixed.Steps[0].Explanation = "The static evidence does not establish entry behavior and dispatches work."
	if err := ValidateProposal(bundle, mixed); err == nil ||
		!strings.Contains(err.Error(), "unsupported behavioral assertion") {
		t.Fatalf("mixed-negation editorial proposal error = %v", err)
	}

	causal := proposal
	causal.Steps = append([]ProposedStep{}, proposal.Steps...)
	causal.Steps[0].Explanation = "The static evidence does not establish entry behavior because it dispatches work."
	if err := ValidateProposal(bundle, causal); err == nil ||
		!strings.Contains(err.Error(), "unsupported behavioral assertion") {
		t.Fatalf("causal-negation editorial proposal error = %v", err)
	}

	routed := proposal
	routed.Steps = append([]ProposedStep{}, proposal.Steps...)
	routed.Steps[0].Explanation = "The entry routes requests into analysis."
	if err := ValidateProposal(bundle, routed); err == nil ||
		!strings.Contains(err.Error(), "unsupported behavioral assertion") {
		t.Fatalf("routing editorial proposal error = %v", err)
	}

	asBoundary := proposal
	asBoundary.Steps = append([]ProposedStep{}, proposal.Steps...)
	asBoundary.Steps[0].Explanation = "This does not call work as it dispatches another request."
	if err := ValidateProposal(bundle, asBoundary); err == nil ||
		!strings.Contains(err.Error(), "unsupported behavioral assertion") {
		t.Fatalf("as-boundary editorial proposal error = %v", err)
	}

	negated := proposal
	negated.Steps = append([]ProposedStep{}, proposal.Steps...)
	negated.Steps[0].Explanation = "The static evidence does not establish runtime calls."
	if err := ValidateProposal(bundle, negated); err != nil {
		t.Fatalf("explicitly negated editorial proposal error = %v", err)
	}
}

func TestUnsupportedBehaviorClaimCount(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	proposal := testProposal()
	proposal.Steps[0].Explanation = "The entry routes requests as the worker persists results."
	if got := UnsupportedBehaviorClaimCount(bundle, proposal); got != 0 {
		t.Fatalf("saved trace claim count = %d, want 0", got)
	}

	bundle.Candidates[0].Kind = CandidateSuggestedDirection
	bundle.Candidates[0].OrderingBasis = OrderingEditorial
	if got := UnsupportedBehaviorClaimCount(bundle, proposal); got != 2 {
		t.Fatalf("suggested direction claim count = %d, want 2", got)
	}

	proposal.CandidateID = "missing"
	if got := UnsupportedBehaviorClaimCount(bundle, proposal); got != 0 {
		t.Fatalf("unknown candidate claim count = %d, want 0", got)
	}
}

func TestValidateProposalAllowsEditorialReordering(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	bundle.Candidates[0].ID = "direction-1"
	bundle.Candidates[0].Kind = CandidateSuggestedDirection
	bundle.Candidates[0].OrderingBasis = OrderingEditorial
	proposal := testProposal()
	proposal.CandidateID = "direction-1"
	proposal.Steps = []ProposedStep{
		{Title: "Fourth", Explanation: "Teach fourth", BeatIDs: []string{"beat-4"}},
		{Title: "Third", Explanation: "Teach third", BeatIDs: []string{"beat-3"}},
		{Title: "First", Explanation: "Teach earlier beats", BeatIDs: []string{"beat-2", "beat-1"}},
	}
	if err := ValidateProposal(bundle, proposal); err != nil {
		t.Fatalf("ValidateProposal() editorial reorder error = %v", err)
	}
}

func TestMaterializeStoryDerivesLocalReferences(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	proposal := testProposal()
	story, err := MaterializeStory(bundle, proposal)
	if err != nil {
		t.Fatalf("MaterializeStory() error = %v", err)
	}
	if story.CandidateID != "trace-1" || story.CandidateKind != CandidateSavedTrace {
		t.Fatalf("MaterializeStory() candidate = %q/%q", story.CandidateID, story.CandidateKind)
	}
	if got, want := story.Steps[0].ComponentIDs, []string{"component-1", "component-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MaterializeStory() component ids = %#v, want %#v", got, want)
	}
	if got, want := story.Steps[0].SurfaceIDs, []string{"surface-1", "surface-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MaterializeStory() surface ids = %#v, want %#v", got, want)
	}
	if got, want := evidenceIDs(story.Steps[0].Evidence), []string{"evidence-1", "evidence-2", "evidence-shared"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MaterializeStory() evidence ids = %#v, want %#v", got, want)
	}
	if len(story.Steps[0].Components) != 2 || len(story.Components) != 2 {
		t.Fatalf("MaterializeStory() component materialization is incomplete")
	}
	if len(story.GapSummary) != 1 || len(story.GapSummary[0].Gaps) != 1 ||
		story.GapSummary[0].Evidence[0].ID != "evidence-gap" {
		t.Fatalf("MaterializeStory() gap materialization = %#v", story.GapSummary)
	}
	if story.Steps[0].Evidence[0].Location == nil || story.Steps[0].Evidence[0].Location.Path != "cmd/server.go" {
		t.Fatalf("MaterializeStory() did not preserve supplied exact location")
	}

	story.Steps[0].Evidence[0].Location.Path = "changed"
	if bundle.Candidates[0].Beats[0].Evidence[0].Location.Path != "cmd/server.go" {
		t.Fatalf("MaterializeStory() aliases bundle evidence")
	}
}

func TestRecordRoundTripAndReplay(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	proposal := testProposal()
	encoded, err := EncodeRecord(bundle, proposal)
	if err != nil {
		t.Fatalf("EncodeRecord() error = %v", err)
	}
	record, err := DecodeRecord(encoded)
	if err != nil {
		t.Fatalf("DecodeRecord() error = %v", err)
	}
	wantHash, _, err := BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if record.BundleSHA256 != wantHash || !reflect.DeepEqual(record.Proposal, proposal) {
		t.Fatalf("DecodeRecord() = %#v", record)
	}
	story, err := ReplayRecord(bundle, encoded)
	if err != nil {
		t.Fatalf("ReplayRecord() error = %v", err)
	}
	wantStory, err := MaterializeStory(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(story, wantStory) {
		t.Fatalf("ReplayRecord() story differs from materialized story")
	}

	reordered := cloneBundle(t, bundle)
	reverseBeats(reordered.Candidates[0].Beats)
	reorderedStory, err := ReplayRecord(reordered, encoded)
	if err != nil {
		t.Fatalf("ReplayRecord() canonical reorder error = %v", err)
	}
	if !reflect.DeepEqual(reorderedStory, wantStory) {
		t.Fatalf("ReplayRecord() canonical reorder changed the materialized story")
	}
	stale := cloneBundle(t, bundle)
	stale.RepoName = "another-repository"
	if _, err := ReplayRecord(stale, encoded); err == nil || !strings.Contains(err.Error(), "hash does not match") {
		t.Fatalf("ReplayRecord() stale error = %v", err)
	}
}

func TestDecodeRecordStrictJSON(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	encoded, err := EncodeRecord(bundle, testProposal())
	if err != nil {
		t.Fatal(err)
	}
	unknown := append(append([]byte{}, encoded[:len(encoded)-1]...), []byte(`,"extra":true}`)...)
	if _, err := DecodeRecord(unknown); err == nil {
		t.Fatalf("DecodeRecord() accepted unknown field")
	}

	var record Record
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	record.BundleSHA256 = strings.Repeat("A", 64)
	malformed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRecord(malformed); err == nil || !strings.Contains(err.Error(), "hash is malformed") {
		t.Fatalf("DecodeRecord() malformed hash error = %v", err)
	}
}

func TestBundleValidateRejectsInvalidReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Bundle)
		wantErr string
	}{
		{
			name: "duplicate candidate",
			mutate: func(bundle *Bundle) {
				bundle.Candidates = append(bundle.Candidates, bundle.Candidates[0])
			},
			wantErr: "duplicate candidate",
		},
		{
			name: "duplicate beat",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].Beats[1].ID = "beat-1"
			},
			wantErr: "duplicate id",
		},
		{
			name: "unknown component",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].Beats[0].ComponentIDs = []string{"missing"}
			},
			wantErr: "unknown component",
		},
		{
			name: "duplicate component reference",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].Beats[0].ComponentIDs = []string{"component-1", "component-1"}
			},
			wantErr: "repeats id",
		},
		{
			name: "conflicting evidence",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].Beats[1].Evidence[0].ID = "evidence-1"
			},
			wantErr: "conflicting definitions",
		},
		{
			name: "bad ordering basis",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].OrderingBasis = OrderingEditorial
			},
			wantErr: "trace_order",
		},
		{
			name: "too few candidate beats",
			mutate: func(bundle *Bundle) {
				bundle.Candidates[0].Beats = bundle.Candidates[0].Beats[:2]
			},
			wantErr: "at least 3 beats",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bundle := testBundle(t)
			tt.mutate(&bundle)
			err := bundle.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Bundle.Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func testBundle(t *testing.T) Bundle {
	t.Helper()
	location := func(path string, line int) *evidence.Location {
		return &evidence.Location{Path: path, Line: line}
	}
	shared := EvidenceRef{ID: "evidence-shared", Kind: "transition", Label: "Shared transition"}
	return Bundle{
		Version:       BundleVersion,
		RepoName:      "repomap",
		CanvasVersion: 5,
		Components: []Component{
			{ID: "component-2", Name: "Runtime", Description: "Runs the selected behavior"},
			{ID: "component-1", Name: "Entry", Description: "Accepts the request"},
		},
		Candidates: []Candidate{
			{
				ID:            "trace-1",
				Name:          "Serve one request",
				Kind:          CandidateSavedTrace,
				Trigger:       "An incoming request",
				Summary:       "A bounded saved trace through the request path",
				OrderingBasis: OrderingTrace,
				Beats: []Beat{
					{
						ID: "beat-1", Kind: "entry", Label: "Accept request", Detail: "The exact entry receives input",
						Sequence: 1, ComponentIDs: []string{"component-2", "component-1"},
						SurfaceIDs: []string{"surface-2", "surface-1"}, FlowID: "flow-1",
						FlowStepIDs: []string{"flow-step-2", "flow-step-1"},
						Evidence: []EvidenceRef{
							{ID: "evidence-1", Kind: "declaration", Label: "Exact entry", Location: location("cmd/server.go", 10)},
							shared,
						},
					},
					{
						ID: "beat-2", Kind: "dispatch", Label: "Dispatch request", Detail: "The dispatcher selects work",
						Sequence: 2, ComponentIDs: []string{"component-2"}, SurfaceIDs: []string{"surface-2"},
						FlowID: "flow-1", FlowStepIDs: []string{"flow-step-3"},
						Evidence: []EvidenceRef{
							{ID: "evidence-2", Kind: "callsite", Label: "Dispatch call", Location: location("internal/server.go", 20)},
							shared,
						},
					},
					{
						ID: "beat-3", Kind: "work", Label: "Perform work", Detail: "The selected handler performs bounded work",
						Sequence: 3, ComponentIDs: []string{"component-2"}, SurfaceIDs: []string{"surface-2"},
						FlowID: "flow-1", FlowStepIDs: []string{"flow-step-4"},
						Evidence: []EvidenceRef{
							{ID: "evidence-3", Kind: "transition", Label: "Handler transition"},
						},
					},
					{
						ID: "beat-4", Kind: "frontier", Label: "Reach frontier", Detail: "The saved trace stops at an honest frontier",
						Sequence: 4, ComponentIDs: []string{"component-1"}, SurfaceIDs: []string{"surface-1"},
						FlowID: "flow-1", FlowStepIDs: []string{"flow-step-5"},
						Evidence: []EvidenceRef{
							{ID: "evidence-4", Kind: "frontier", Label: "Unresolved frontier"},
						},
					},
				},
				Gaps: []Gap{
					{
						ID: "gap-1", Label: "Runtime outcome", Detail: "Static evidence does not prove the runtime outcome",
						Evidence: []EvidenceRef{{ID: "evidence-gap", Kind: "frontier", Label: "Runtime-only gap"}},
					},
				},
			},
		},
	}
}

func testProposal() Proposal {
	return Proposal{
		Version: ProposalVersion, CandidateID: "trace-1", Title: "How one request moves", Summary: "Follow the saved evidence and its frontier",
		Steps: []ProposedStep{
			{Title: "Enter", Explanation: "Start at the exact entry", BeatIDs: []string{"beat-1", "beat-2"}},
			{Title: "Work", Explanation: "Continue through the selected behavior", BeatIDs: []string{"beat-3"}},
			{Title: "Stop honestly", Explanation: "End where saved evidence ends", BeatIDs: []string{"beat-4"}},
		},
		GapSummary: []ProposedGapSummary{
			{Explanation: "The runtime outcome remains unresolved", GapIDs: []string{"gap-1"}},
		},
	}
}

func cloneBundle(t *testing.T, bundle Bundle) Bundle {
	t.Helper()
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var result Bundle
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func evidenceIDs(values []EvidenceRef) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseBeats(values []Beat) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseComponents(values []Component) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseEvidence(values []EvidenceRef) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
