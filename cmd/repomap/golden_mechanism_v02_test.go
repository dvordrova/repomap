package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestGoldenMechanismV02FreshSynthesisUsesOneProviderCall(t *testing.T) {
	t.Parallel()

	input := goldenMechanismV02UnitInput(t)
	response := validGoldenMechanismV3Response(t, input)
	stub := &goldenMechanismV01ProviderStub{response: response}

	result, err := executeGoldenMechanismV02Synthesis(
		context.Background(),
		input,
		stub,
	)
	if err != nil {
		t.Fatalf("executeGoldenMechanismV02Synthesis() error = %v", err)
	}
	if stub.calls != 1 || !result.Metrics.ProviderCall {
		t.Fatalf("provider calls = %d, metrics = %#v", stub.calls, result.Metrics)
	}
	if len(result.Artifacts) != 1 || result.Reduction.DroppedArtifacts != 0 {
		t.Fatalf("synthesis = %#v", result)
	}
}

func TestGoldenMechanismV02RejectsWidenedSequenceAfterOneCall(t *testing.T) {
	t.Parallel()

	input := goldenMechanismV02UnitInput(t)
	var response semanticdiscovery.FanInArtifact
	if err := json.Unmarshal(validGoldenMechanismV3Response(t, input), &response); err != nil {
		t.Fatal(err)
	}
	response.Artifacts[0].Claims[0].Text =
		"When browsing is configured and the target is not hidden, the request handler then calls the browse handler."
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	stub := &goldenMechanismV01ProviderStub{response: raw}

	result, err := executeGoldenMechanismV02Synthesis(
		context.Background(),
		input,
		stub,
	)
	if err == nil || !strings.Contains(err.Error(), semanticdiscovery.LocalSequenceScopeWidened) {
		t.Fatalf("executeGoldenMechanismV02Synthesis() error = %v", err)
	}
	if stub.calls != 1 || result.Reduction.DroppedArtifacts != 1 ||
		len(result.Reduction.Issues) != 1 ||
		len(result.Reduction.Issues[0].Reasons) != 1 ||
		result.Reduction.Issues[0].Reasons[0].Code !=
			semanticdiscovery.FanInReasonLocalSequenceScope {
		t.Fatalf("calls = %d, reduction = %#v", stub.calls, result.Reduction)
	}
}

func TestReserveGoldenMechanismV3CallIsExclusive(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), goldenMechanismV3ReservationFile)
	if err := reserveGoldenMechanismV3Call(
		path,
		goldenMechanismProjectionV2SHA256,
		goldenMechanismSequenceFactID,
	); err != nil {
		t.Fatalf("reserveGoldenMechanismV3Call(first) error = %v", err)
	}
	if err := reserveGoldenMechanismV3Call(
		path,
		goldenMechanismProjectionV2SHA256,
		goldenMechanismSequenceFactID,
	); err == nil || !strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("reserveGoldenMechanismV3Call(second) error = %v", err)
	}
}

func goldenMechanismV02UnitInput(t *testing.T) goldenMechanismV02Input {
	t.Helper()
	projection := readGoldenProjectionFixture(t)
	entrySourceGroup := ""
	for _, fact := range projection.Facts {
		if fact.ID == goldenDirectoryListingEntryFactID {
			entrySourceGroup = fact.SourceGroup
			break
		}
	}
	sequence := semanticdiscovery.Fact{
		ID: goldenMechanismSequenceFactID, Kind: semanticdiscovery.FactSourceSignal,
		Statement:   "Within the saved request handler's directory-handling source block, if the browse-enabled and not-hidden predicate's true branch is entered, that same branch directly returns the browse call. This same-function and same-branch relation does not establish selection of the enclosing directory branch, call success, absence of other actions in broader handling, or wider runtime order.",
		Keywords:    []string{"browse branch", "conditional local sequence", "directory listing"},
		SourceGroup: entrySourceGroup,
		Capabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityBranch,
			semanticdiscovery.CapabilityDirectCall,
			semanticdiscovery.CapabilityLimitation,
			semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityStatic,
		},
		Scope: semanticdiscovery.FactScopeLocal,
	}
	projection.Facts = append(projection.Facts, sequence)
	projection.Candidate.EnrichmentSupportIDs = append(
		projection.Candidate.EnrichmentSupportIDs,
		sequence.ID,
	)
	slices.Sort(projection.Candidate.EnrichmentSupportIDs)
	bundle := semanticdiscovery.Bundle{
		Version: semanticdiscovery.BundleVersion, RepoName: "caddy",
		Facts: append(
			append([]semanticdiscovery.Fact(nil), projection.Leaf.Task.Facts...),
			sequence,
		),
	}
	leaf, err := buildGoldenMechanismLeaf(bundle, projection.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	projection.Leaf = leaf
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		t.Fatal(err)
	}
	return goldenMechanismV02Input{
		projection: projection, sequenceFact: sequence,
		bundle: bundle, proposal: proposal, leaf: leaf,
	}
}

func validGoldenMechanismV3Response(
	t *testing.T,
	input goldenMechanismV02Input,
) []byte {
	t.Helper()
	observation := make(map[string]int)
	for index, item := range input.leaf.Artifact.Observations {
		if len(item.SupportIDs) == 1 {
			observation[item.SupportIDs[0]] = index
		}
	}
	refs := func(ids ...string) []semanticdiscovery.ObservationRef {
		result := make([]semanticdiscovery.ObservationRef, 0, len(ids))
		for _, id := range ids {
			index, exists := observation[id]
			if !exists {
				t.Fatalf("leaf observation for %s is unavailable", id)
			}
			result = append(result, semanticdiscovery.ObservationRef{
				TaskID: input.leaf.Task.ID, ObservationIndex: index,
			})
		}
		return result
	}
	claim := func(
		title string,
		text string,
		ids ...string,
	) semanticdiscovery.ProposedClaim {
		return semanticdiscovery.ProposedClaim{
			Title: title, Text: text, Basis: semanticdiscovery.ClaimDirect,
			SupportIDs: ids, ObservationRefs: refs(ids...),
		}
	}
	const (
		collection = "gmf-63fee9ff4e4ab91b356962ee"
		options    = "gmf-e7c6e36ae4486ba9ea1213df"
		sortPage   = "gmf-b774c86f6b35d386bcb95299"
		format     = "gmf-7afe2d8279da85ce2346ac3a"
		branches   = "gmf-fc86a4e7f0832a9377f2085c"
	)
	artifact := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID: input.projection.Candidate.ID,
			Verdict:     semanticdiscovery.VerdictMixed,
			Title:       "How Caddy Builds Directory Listings",
			Summary:     "The file server builds a directory listing by collecting entries, applying request sorting and paging, choosing a representation, and writing the response.",
			Claims: []semanticdiscovery.ProposedClaim{
				claim(
					"Directory browsing branch",
					"Within this branch, when browsing is configured and the target is not hidden, the request handler then calls the browse handler.",
					goldenDirectoryListingEntryFactID,
					input.sequenceFact.ID,
				),
				claim(
					"Collect listing items",
					"Directory browsing reads directory entries, passes them to listing construction, and appends structured items.",
					collection,
				),
				claim(
					"Read request options",
					"Query handling reads sorting, order, limit, and offset controls and passes them to listing transformation.",
					options,
				),
				claim(
					"Sort and page",
					"Listing transformation sorts supported fields in ascending or reverse order, then slices items for valid offset and limit values.",
					sortPage,
				),
				claim(
					"Select and write the representation",
					"The representation branch selects JSON, plain text, or HTML and writes the buffered result to the HTTP response writer.",
					format,
				),
				claim(
					"Handle important alternatives",
					"Alternative branches return redirects, forbidden and not-found outcomes, internal errors, not-modified responses, or template failures.",
					branches,
				),
				{
					Title:      "Evidence gap",
					Text:       "Evidence gap: Direct behavior tests for sorting, paging, and response representations remain uninspected.",
					Basis:      semanticdiscovery.ClaimUnresolved,
					SupportIDs: []string{format, sortPage},
					MissingRefs: []semanticdiscovery.MissingEvidenceRef{{
						TaskID: input.leaf.Task.ID, MissingIndex: 0,
					}},
				},
			},
			Aliases: append(
				[]string(nil),
				input.projection.Candidate.IntentContract.LocalSearchAliases...,
			),
			LikelyQuestions: []string{input.projection.Candidate.QuestionAnswered},
		}},
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
