package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const goldenMechanismV1FrozenFixtureSHA256 = "3b1690bf73cbbd2426bd81560fd28468bf6441772ec8a4d0fbce3a0b3b83f647"

// This opt-in test checks the immutable cold-run fixture without making a
// provider call or publishing into the run directory.
func TestGoldenMechanismV1FrozenContractAndCombinedReplay(t *testing.T) {
	runDir := os.Getenv("REPOMAP_GOLDEN_V1_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_GOLDEN_V1_RUN to the frozen v1 run directory")
	}
	input, err := loadGoldenMechanismV1Input(context.Background(), runDir)
	if err != nil {
		t.Fatal(err)
	}
	if input.FixtureHash != goldenMechanismV1FrozenFixtureSHA256 {
		t.Fatalf("fixture hash = %s", input.FixtureHash)
	}
	raw := validGoldenMechanismV1Response(t, input)
	evaluated, err := evaluateGoldenMechanismResponse(
		input.Bundle,
		input.Proposal,
		input.Leaf,
		raw,
	)
	if err != nil {
		t.Fatalf("frozen contract is unsatisfiable: %v; reduction = %#v", err, evaluated.Reduction)
	}
	assessment, err := semanticdiscovery.AssessClaimCoverage(
		input.Bundle,
		[]semanticdiscovery.LeafResult{input.Leaf},
		evaluated.FanIn.Artifacts[0],
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.CoveredAspectIDs) != 8 ||
		!slices.Equal(assessment.UncoveredAspectIDs, []string{"known_unknowns"}) {
		t.Fatalf("claim coverage = %#v", assessment)
	}
	summary, err := summarizeGoldenMechanismArtifact(
		input.Projection.Candidate,
		evaluated.Artifacts[0],
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.LocalRubricScore != 4 {
		t.Fatalf("rubric score = %d, want 4", summary.LocalRubricScore)
	}
	_, artifacts, err := combineGoldenMechanismV1Record(input, evaluated.FanIn)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireGoldenArtifacts(
		artifacts,
		[]semanticdiscovery.Artifact{input.Existing.Artifact, evaluated.Artifacts[0]},
	); err != nil {
		t.Fatal(err)
	}
}

func validGoldenMechanismV1Response(
	t *testing.T,
	input goldenMechanismV1Input,
) []byte {
	t.Helper()
	facts := make(map[string]semanticdiscovery.Fact, len(input.Projection.Facts))
	for _, fact := range input.Projection.Facts {
		facts[fact.ID] = fact
	}
	observations := make(map[string]int, len(input.Leaf.Artifact.Observations))
	for index, observation := range input.Leaf.Artifact.Observations {
		if len(observation.SupportIDs) == 1 {
			observations[observation.SupportIDs[0]] = index
		}
	}
	claim := func(id, title string) semanticdiscovery.ProposedClaim {
		fact, exists := facts[id]
		if !exists {
			t.Fatalf("fact %s is unavailable", id)
		}
		index, exists := observations[id]
		if !exists {
			t.Fatalf("observation for %s is unavailable", id)
		}
		return semanticdiscovery.ProposedClaim{
			Title: title, Text: fact.Statement, Basis: semanticdiscovery.ClaimDirect,
			SupportIDs: []string{id},
			ObservationRefs: []semanticdiscovery.ObservationRef{{
				TaskID: input.Leaf.Task.ID, ObservationIndex: index,
			}},
		}
	}
	ids := make(map[string]string, len(input.Projection.Facts))
	for _, fact := range input.Projection.Facts {
		for _, keyword := range fact.Keywords {
			ids[keyword] = fact.ID
		}
	}
	boundaryID := ids["answer_aspect:known_unknowns"]
	artifact := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID: input.Projection.Candidate.ID,
			Verdict:     semanticdiscovery.VerdictMixed,
			Title:       goldenCaddyfileErrorTitle,
			Summary: "A top-level Caddyfile request matcher becomes a formatted parser error; " +
				"its wrapping helper attaches file and line, the parser entry chain hands it to " +
				"the public parser, and the built-in adapter returns it. CLI output is not established.",
			Claims: []semanticdiscovery.ProposedClaim{
				claim(ids["answer_aspect:error_origin"], "Matcher error origin"),
				claim(ids["answer_aspect:source_location"], "Source location and import context"),
				claim(ids["answer_aspect:parser_propagation"], "Parser entry propagation"),
				claim(ids["answer_aspect:adapter_propagation"], "Adapter propagation"),
				claim(ids["answer_aspect:test_evidence"], "Exact local test evidence"),
				claim(ids["answer_aspect:important_alternatives"], "Distinct adapter alternatives"),
				{
					Title: "Evidence gap",
					Text: "Evidence gap: User-visible CLI output is not established by this bounded " +
						"proof, and other Caddyfile error families remain unknown.",
					Basis:      semanticdiscovery.ClaimUnresolved,
					SupportIDs: []string{boundaryID},
					MissingRefs: []semanticdiscovery.MissingEvidenceRef{{
						TaskID: input.Leaf.Task.ID, MissingIndex: 0,
					}},
				},
			},
			Aliases: append(
				[]string(nil),
				input.Projection.Candidate.IntentContract.LocalSearchAliases...,
			),
			LikelyQuestions: []string{input.Projection.Candidate.QuestionAnswered},
		}},
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
