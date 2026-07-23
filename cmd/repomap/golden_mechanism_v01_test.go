package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

type goldenMechanismV01ProviderStub struct {
	response []byte
	calls    int
}

func (stub *goldenMechanismV01ProviderStub) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *goldenMechanismV01ProviderStub) DiscoverSemanticsMeasured(
	_ context.Context,
	_ semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{Content: append([]byte(nil), stub.response...), Attempts: 1}, nil
}

func TestRejectedGoldenMechanismFixtureReportsExactContractMismatch(t *testing.T) {
	projectionRaw, err := os.ReadFile("testdata/golden_mechanism_caddy_projection_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var projection goldenProjection
	if err := json.Unmarshal(projectionRaw, &projection); err != nil {
		t.Fatal(err)
	}
	wantFactIDs := []string{
		"gmf-0ec8ef0974e6bca4e91a0355",
		"gmf-63fee9ff4e4ab91b356962ee",
		"gmf-7afe2d8279da85ce2346ac3a",
		"gmf-b774c86f6b35d386bcb95299",
		"gmf-e7c6e36ae4486ba9ea1213df",
		"gmf-fc86a4e7f0832a9377f2085c",
	}
	gotFactIDs := make([]string, 0, len(projection.Facts))
	for _, fact := range projection.Facts {
		gotFactIDs = append(gotFactIDs, fact.ID)
	}
	slices.Sort(gotFactIDs)
	if !slices.Equal(gotFactIDs, wantFactIDs) {
		t.Fatalf("fixed golden facts = %v, want %v", gotFactIDs, wantFactIDs)
	}

	responseRaw, err := os.ReadFile("testdata/golden_mechanism_rejected_response_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var response goldenMechanismResponseAttempt
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		t.Fatal(err)
	}
	parsed, err := semanticdiscovery.ParseFanInArtifact([]byte(response.Content))
	if err != nil {
		t.Fatal(err)
	}
	bundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "caddy",
		Facts:    projection.Leaf.Task.Facts,
	}
	_, reduction, err := semanticdiscovery.ReduceFanInArtifact(
		bundle,
		[]semanticdiscovery.LeafResult{projection.Leaf},
		semanticdiscovery.NormalizeFanInArtifact(parsed),
	)
	if err == nil {
		t.Fatal("saved rejected response unexpectedly passed current validation")
	}
	if len(reduction.Issues) < 1 || reduction.Issues[0].Code != "invalid_proposal" {
		t.Fatalf("reduction issues = %#v", reduction.Issues)
	}
	reasons := reduction.Issues[0].Reasons
	assertGoldenDiagnostic(t, reasons, semanticdiscovery.FanInReasonUnknownRepositoryReference, "artifact.summary", -1)
	assertGoldenDiagnostic(t, reasons, semanticdiscovery.FanInReasonUnknownRepositoryReference, "claim.title", 3)
	assertGoldenDiagnostic(t, reasons, semanticdiscovery.FanInReasonUnknownRepositoryReference, "claim.text", 5)
	assertGoldenDiagnostic(t, reasons, semanticdiscovery.FanInReasonUnsupportedSequence, "claim.text", 0)
	assertGoldenDiagnostic(t, reasons, semanticdiscovery.FanInReasonLimitationNotExplicit, "claim.text", 6)
	encodedDiagnostics, err := json.Marshal(reduction)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"sort/page", "format/output", "forbidden/not-found"} {
		if strings.Contains(string(encodedDiagnostics), forbidden) {
			t.Fatalf("diagnostics echoed rejected model prose %q", forbidden)
		}
	}
	if got := classifyGoldenMechanismValidationFailure(reduction); got != "prompt_validator_contract_mismatch" {
		t.Fatalf("failure class = %q", got)
	}
}

func TestGoldenMechanismV01FreshSynthesisUsesOneProviderCall(t *testing.T) {
	projection := readGoldenProjectionFixture(t)
	bundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "caddy",
		Facts:    projection.Leaf.Task.Facts,
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	response := validGoldenMechanismV2Response(t)
	stub := &goldenMechanismV01ProviderStub{response: response}

	result, err := executeGoldenMechanismSynthesis(
		context.Background(),
		bundle,
		proposal,
		projection.Leaf,
		stub,
	)
	if err != nil {
		t.Fatalf("executeGoldenMechanismSynthesis() error = %v", err)
	}
	if stub.calls != 1 || result.Metrics.ProviderCall != true {
		t.Fatalf("provider calls = %d, metrics = %#v", stub.calls, result.Metrics)
	}
	if len(result.Artifacts) != 1 || result.Reduction.DroppedArtifacts != 0 {
		t.Fatalf("synthesis = %#v", result)
	}
	if _, err := summarizeGoldenMechanismArtifact(projection.Candidate, result.Artifacts[0]); err != nil {
		t.Fatalf("summarizeGoldenMechanismArtifact() error = %v", err)
	}
}

func TestReserveGoldenMechanismV2CallIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), goldenMechanismV2ReservationFile)
	if err := reserveGoldenMechanismV2Call(path); err != nil {
		t.Fatalf("reserveGoldenMechanismV2Call(first) error = %v", err)
	}
	if err := reserveGoldenMechanismV2Call(path); err == nil || !strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("reserveGoldenMechanismV2Call(second) error = %v", err)
	}
}

func readGoldenProjectionFixture(t *testing.T) goldenProjection {
	t.Helper()
	raw, err := os.ReadFile("testdata/golden_mechanism_caddy_projection_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var projection goldenProjection
	if err := json.Unmarshal(raw, &projection); err != nil {
		t.Fatal(err)
	}
	return projection
}

func validGoldenMechanismV2Response(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/golden_mechanism_rejected_response_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var wrapper goldenMechanismResponseAttempt
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatal(err)
	}
	artifact, err := semanticdiscovery.ParseFanInArtifact([]byte(wrapper.Content))
	if err != nil {
		t.Fatal(err)
	}
	proposal := &artifact.Artifacts[0]
	proposal.Summary = strings.ReplaceAll(proposal.Summary, "sort/page", "sorting and paging")
	proposal.Summary = strings.ReplaceAll(proposal.Summary, "format/output", "formatting and output")
	proposal.Claims[0].Text = strings.ReplaceAll(
		proposal.Claims[0].Text,
		", then directly calls",
		" and directly calls",
	)
	proposal.Claims[3].Title = strings.ReplaceAll(proposal.Claims[3].Title, "offset/limit", "offset and limit")
	proposal.Claims[5].Text = strings.ReplaceAll(proposal.Claims[5].Text, "forbidden/not-found", "forbidden and not-found")
	proposal.Claims[6].Title = "Evidence gap"
	proposal.Claims[6].Text = "Evidence gap: Direct behavior tests for sorting, paging, and response representations remain uninspected by the bounded probe."
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertGoldenDiagnostic(
	t *testing.T,
	reasons []semanticdiscovery.FanInReductionReason,
	code string,
	field string,
	claimIndex int,
) {
	t.Helper()
	for _, reason := range reasons {
		if reason.Code != code || reason.Field != field {
			continue
		}
		if claimIndex < 0 && reason.ClaimIndex == nil {
			return
		}
		if reason.ClaimIndex != nil && *reason.ClaimIndex == claimIndex {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want %s/%s claim %d", reasons, code, field, claimIndex)
}
