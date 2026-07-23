package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestSemanticSupplementRoundTripBindsBaseAndEnrichedBundles(t *testing.T) {
	data := semanticSearchTestReport()
	facts := []semanticdiscovery.Fact{{
		ID: "gmf-roundtrip-output", Kind: semanticdiscovery.FactSourceSignal,
		Statement:    "The bounded representation is written to the client response.",
		Keywords:     []string{"answer_aspect:response_output"},
		SourceGroup:  "gmsg-roundtrip-output",
		Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "gme-roundtrip-output", Kind: "probe", Label: "response output",
			Path: "internal/report/report.go", Line: 12,
		}},
	}}
	record, enriched, err := PrepareSemanticSupplement(
		data,
		"semantic-candidate-roundtrip",
		strings.Repeat("a", 64),
		facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	enrichedSHA, _, err := semanticdiscovery.BundleHash(enriched)
	if err != nil {
		t.Fatal(err)
	}
	if enrichedSHA != record.EnrichedBundleSHA256 || record.BaseBundleSHA256 == enrichedSHA {
		t.Fatalf("supplement hashes = base %q enriched %q", record.BaseBundleSHA256, record.EnrichedBundleSHA256)
	}
	if record.Version != semanticSupplementVersion || len(record.CandidateBindings) != 1 ||
		record.CandidateBindings[0].CandidateID != "semantic-candidate-roundtrip" ||
		record.CandidateID != "" || record.ProbeSHA256 != "" {
		t.Fatalf("current supplement binding = %#v", record)
	}

	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), GoldenMechanismFactsFile)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	replayed := semanticSearchTestReport()
	if warning := loadSemanticSupplement(replayed, path); warning != "" {
		t.Fatalf("loadSemanticSupplement warning = %q", warning)
	}
	if len(replayed.SemanticSupplementalFacts) != 1 ||
		replayed.SemanticSupplementalFacts[0].ID != facts[0].ID {
		t.Fatalf("replayed supplemental facts = %#v", replayed.SemanticSupplementalFacts)
	}
	replayedBundle, err := BuildSemanticDiscoveryBundle(replayed)
	if err != nil {
		t.Fatal(err)
	}
	replayedSHA, _, err := semanticdiscovery.BundleHash(replayedBundle)
	if err != nil {
		t.Fatal(err)
	}
	if replayedSHA != record.EnrichedBundleSHA256 {
		t.Fatalf("replayed bundle hash = %q, want %q", replayedSHA, record.EnrichedBundleSHA256)
	}
}

func TestSemanticSupplementLoadsLegacySingleCandidate(t *testing.T) {
	data := semanticSearchTestReport()
	fact := semanticdiscovery.Fact{
		ID: "gmf-legacy-output", Kind: semanticdiscovery.FactSourceSignal,
		Statement:    "The bounded representation is written to the client response.",
		SourceGroup:  "gmsg-legacy-output",
		Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect},
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "gme-legacy-output", Kind: "probe", Path: "internal/report/report.go", Line: 12,
		}},
	}
	current, _, err := PrepareSemanticSupplement(
		data,
		"semantic-candidate-legacy",
		strings.Repeat("9", 64),
		[]semanticdiscovery.Fact{fact},
	)
	if err != nil {
		t.Fatal(err)
	}
	legacy := SemanticSupplement{
		Version:              semanticSupplementLegacyVersion,
		CandidateID:          "semantic-candidate-legacy",
		ProbeSHA256:          strings.Repeat("9", 64),
		BaseBundleSHA256:     current.BaseBundleSHA256,
		EnrichedBundleSHA256: current.EnrichedBundleSHA256,
		Facts:                current.Facts,
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), GoldenMechanismFactsFile)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	replayed := semanticSearchTestReport()
	loaded, warning := loadSemanticSupplementRecord(replayed, path)
	if warning != "" {
		t.Fatalf("loadSemanticSupplementRecord warning = %q", warning)
	}
	ids, err := semanticSupplementCandidateIDs(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != semanticSupplementLegacyVersion ||
		len(ids) != 1 || ids[0] != legacy.CandidateID ||
		len(replayed.SemanticSupplementalFacts) != 1 {
		t.Fatalf("legacy replay = record %#v ids %#v facts %#v", loaded, ids, replayed.SemanticSupplementalFacts)
	}
}

func TestSemanticSupplementStaleBaseDegradesWithoutFacts(t *testing.T) {
	data := semanticSearchTestReport()
	fact := semanticdiscovery.Fact{
		ID: "gmf-stale-output", Kind: semanticdiscovery.FactSourceSignal,
		Statement:    "The bounded representation is written to the client response.",
		SourceGroup:  "gmsg-stale-output",
		Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "gme-stale-output", Kind: "probe", Path: "internal/report/report.go", Line: 12,
		}},
	}
	record, _, err := PrepareSemanticSupplement(
		data,
		"semantic-candidate-stale",
		strings.Repeat("b", 64),
		[]semanticdiscovery.Fact{fact},
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), GoldenMechanismFactsFile)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	changed := semanticSearchTestReport()
	changed.ProjectGuess = "changed saved orientation"
	warning := loadSemanticSupplement(changed, path)
	if !strings.Contains(warning, "stale") {
		t.Fatalf("warning = %q, want stale", warning)
	}
	if len(changed.SemanticSupplementalFacts) != 0 {
		t.Fatalf("stale supplemental facts were retained: %#v", changed.SemanticSupplementalFacts)
	}
}

func TestReplaySavedGoldenMechanismInvalidRecordPreservesBaseArtifacts(t *testing.T) {
	data := semanticSearchTestReport()
	baseArtifacts := []semanticdiscovery.Artifact{{
		Version: semanticdiscovery.ArtifactVersion,
		ID:      "semantic-artifact-existing", CandidateID: "semantic-candidate-existing",
		Kind: semanticdiscovery.ArtifactMechanism, Title: "Existing explanation",
		Summary: "Existing explanation remains available.", Question: "What remains available?",
		Verdict:    semanticdiscovery.VerdictSupported,
		Statements: []semanticdiscovery.Statement{{ID: "statement-existing", Text: "Existing explanation remains available.", Basis: semanticdiscovery.ClaimDirect}},
		Steps:      []semanticdiscovery.Step{{ID: "step-existing", Title: "Existing", Explanation: "Existing explanation remains available.", StatementIDs: []string{"statement-existing"}}},
		Confidence: semanticdiscovery.ConfidenceHigh,
	}}
	data.SemanticArtifacts = append([]semanticdiscovery.Artifact(nil), baseArtifacts...)
	fact := semanticdiscovery.Fact{
		ID: "gmf-invalid-replay", Kind: semanticdiscovery.FactSourceSignal,
		Statement:    "The bounded response writer writes the listing output.",
		SourceGroup:  "gmsg-invalid-replay",
		Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect},
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "gme-invalid-replay", Kind: "probe", Path: "internal/report/report.go", Line: 12,
		}},
	}
	supplement, _, err := PrepareSemanticSupplement(
		data,
		"semantic-candidate-invalid-replay",
		strings.Repeat("c", 64),
		[]semanticdiscovery.Fact{fact},
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(supplement)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	factsPath := filepath.Join(dir, GoldenMechanismFactsFile)
	recordPath := filepath.Join(dir, GoldenMechanismRecordFile)
	if err := os.WriteFile(factsPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data.SemanticSupplementalFacts = nil
	data.SemanticArtifacts = append([]semanticdiscovery.Artifact(nil), baseArtifacts...)
	warning := replaySavedGoldenMechanism(data, factsPath, recordPath)
	if !strings.Contains(warning, "stale or invalid") {
		t.Fatalf("warning = %q", warning)
	}
	if len(data.SemanticArtifacts) != 1 || data.SemanticArtifacts[0].ID != baseArtifacts[0].ID {
		t.Fatalf("invalid golden replay changed base artifacts: %#v", data.SemanticArtifacts)
	}
}

func TestReplaySavedGoldenMechanismReplacesBoundCandidateSet(t *testing.T) {
	data := semanticSearchTestReport()
	data.SemanticArtifacts = nil
	facts := []semanticdiscovery.Fact{
		{
			ID: "gmf-configuration-call", Kind: semanticdiscovery.FactSourceSignal,
			Statement:    "The local request handler calls the configuration loader.",
			SourceGroup:  "gmsg-configuration-call",
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall},
			Scope:        semanticdiscovery.FactScopeLocal,
			Evidence: []semanticdiscovery.EvidenceRef{{
				ID: "gme-configuration-call", Kind: "probe", Path: "internal/report/report.go", Line: 12,
			}},
		},
		{
			ID: "gmf-status-call", Kind: semanticdiscovery.FactSourceSignal,
			Statement:    "The local response handler calls the status writer.",
			SourceGroup:  "gmsg-status-call",
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall},
			Scope:        semanticdiscovery.FactScopeLocal,
			Evidence: []semanticdiscovery.EvidenceRef{{
				ID: "gme-status-call", Kind: "probe", Path: "internal/report/report.go", Line: 12,
			}},
		},
	}
	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(facts)
	preliminary, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	opportunity, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		preliminary,
		semanticdiscovery.OpportunityProposal{
			Version: semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{
				{
					Kind: semanticdiscovery.ArtifactMechanism, Title: "Configuration loader call",
					QuestionAnswered: "How is local configuration loading invoked?",
					SupportIDs:       []string{facts[0].ID},
					ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
					Confidence:       semanticdiscovery.ConfidenceHigh,
				},
				{
					Kind: semanticdiscovery.ArtifactMechanism, Title: "Status writer call",
					QuestionAnswered: "How is local status writing invoked?",
					SupportIDs:       []string{facts[1].ID},
					ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
					Confidence:       semanticdiscovery.ConfidenceHigh,
				},
			},
		},
	)
	if len(normalization.Issues) != 0 || len(opportunity.Candidates) != 2 {
		t.Fatalf("normalization = %#v, opportunity = %#v", normalization, opportunity)
	}

	bindings := make([]SemanticSupplementCandidateBinding, 0, len(opportunity.Candidates))
	for index, candidate := range opportunity.Candidates {
		bindings = append(bindings, SemanticSupplementCandidateBinding{
			CandidateID: candidate.ID,
			ProbeSHA256: strings.Repeat(string(rune('d'+index)), 64),
			FactIDs:     append([]string(nil), candidate.SupportIDs...),
		})
	}
	data.SemanticSupplementalFacts = nil
	supplement, enriched, err := PrepareSemanticSupplementSet(data, bindings, facts)
	if err != nil {
		t.Fatal(err)
	}
	preliminarySHA, _, err := semanticdiscovery.BundleHash(preliminary)
	if err != nil {
		t.Fatal(err)
	}
	enrichedSHA, _, err := semanticdiscovery.BundleHash(enriched)
	if err != nil {
		t.Fatal(err)
	}
	if preliminarySHA != enrichedSHA {
		t.Fatalf("candidate planning bundle %q differs from prepared bundle %q", preliminarySHA, enrichedSHA)
	}

	selected, err := semanticdiscovery.SelectOpportunities(enriched, opportunity, 2)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(enriched, selected)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]semanticdiscovery.LeafResult, 0, len(tasks))
	proposals := make([]semanticdiscovery.ArtifactProposal, 0, len(tasks))
	for _, task := range tasks {
		fact := semanticSupplementFactByID(t, enriched.Facts, task.Candidate.SupportIDs[0])
		leaf := semanticdiscovery.LeafArtifact{
			Version:     semanticdiscovery.LeafArtifactVersion,
			TaskID:      task.ID,
			CandidateID: task.Candidate.ID,
			Status:      semanticdiscovery.LeafStatusUsable,
			Observations: []semanticdiscovery.LeafObservation{{
				Text: fact.Statement, SupportIDs: []string{fact.ID},
			}},
			CandidateConnection: semanticdiscovery.LeafCandidateConnection{
				CandidateID: task.Candidate.ID,
				Relation:    "needs_combination",
				Explanation: "This bounded observation is available for synthesis.",
				SupportIDs:  []string{fact.ID},
			},
		}
		if err := semanticdiscovery.ValidateLeafArtifact(task, leaf); err != nil {
			t.Fatalf("ValidateLeafArtifact(%q): %v", task.Candidate.ID, err)
		}
		results = append(results, semanticdiscovery.LeafResult{Task: task, Artifact: leaf})
		proposals = append(proposals, semanticdiscovery.ArtifactProposal{
			CandidateID: task.Candidate.ID,
			Verdict:     semanticdiscovery.VerdictSupported,
			Title:       task.Candidate.Title,
			Summary:     fact.Statement,
			Claims: []semanticdiscovery.ProposedClaim{{
				Title:      task.Candidate.Title,
				Text:       fact.Statement,
				Basis:      semanticdiscovery.ClaimDirect,
				SupportIDs: []string{fact.ID},
				ObservationRefs: []semanticdiscovery.ObservationRef{{
					TaskID: task.ID, ObservationIndex: 0,
				}},
			}},
		})
	}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion, Artifacts: proposals,
	}
	recordRaw, err := semanticdiscovery.EncodeRecord(
		enriched,
		opportunity,
		selected,
		results,
		fanIn,
	)
	if err != nil {
		t.Fatal(err)
	}
	supplementRaw, err := json.Marshal(supplement)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	factsPath := filepath.Join(dir, GoldenMechanismFactsFile)
	recordPath := filepath.Join(dir, GoldenMechanismRecordFile)
	if err := os.WriteFile(factsPath, supplementRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, recordRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	baseArtifacts := []semanticdiscovery.Artifact{
		{ID: "stale-artifact-one", CandidateID: opportunity.Candidates[0].ID},
		{ID: "stale-artifact-two", CandidateID: opportunity.Candidates[1].ID},
		{ID: "artifact-unrelated", CandidateID: "semantic-candidate-unrelated"},
	}
	data.SemanticSupplementalFacts = nil
	data.SemanticArtifacts = append([]semanticdiscovery.Artifact(nil), baseArtifacts...)
	if warning := replaySavedGoldenMechanism(data, factsPath, recordPath); warning != "" {
		t.Fatalf("replaySavedGoldenMechanism warning = %q", warning)
	}
	if len(data.SemanticArtifacts) != 3 {
		t.Fatalf("replayed artifacts = %#v", data.SemanticArtifacts)
	}
	for _, candidate := range opportunity.Candidates {
		artifact := semanticSupplementArtifactByCandidateID(t, data.SemanticArtifacts, candidate.ID)
		if strings.HasPrefix(artifact.ID, "stale-artifact-") || artifact.Title != candidate.Title {
			t.Fatalf("candidate %q was not replaced: %#v", candidate.ID, artifact)
		}
	}
	if artifact := semanticSupplementArtifactByCandidateID(
		t,
		data.SemanticArtifacts,
		"semantic-candidate-unrelated",
	); artifact.ID != "artifact-unrelated" {
		t.Fatalf("unrelated artifact changed: %#v", artifact)
	}
}

func semanticSupplementFactByID(
	t *testing.T,
	facts []semanticdiscovery.Fact,
	id string,
) semanticdiscovery.Fact {
	t.Helper()
	for _, fact := range facts {
		if fact.ID == id {
			return fact
		}
	}
	t.Fatalf("fact %q not found", id)
	return semanticdiscovery.Fact{}
}

func semanticSupplementArtifactByCandidateID(
	t *testing.T,
	artifacts []semanticdiscovery.Artifact,
	candidateID string,
) semanticdiscovery.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.CandidateID == candidateID {
			return artifact
		}
	}
	t.Fatalf("artifact for candidate %q not found", candidateID)
	return semanticdiscovery.Artifact{}
}
