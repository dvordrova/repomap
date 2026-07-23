package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestPublishFreshMechanismReplacesExistingRootWithoutReplayHashDrift(t *testing.T) {
	repoRoot := t.TempDir()
	runDir := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "pipeline.go"), strings.Join([]string{
		"package fixture",
		"",
		"func Accept(input string) string {",
		"\treturn Transform(input)",
		"}",
		"",
		"func Transform(value string) string {",
		"\treturn Emit(value)",
		"}",
		"",
		"func Emit(value string) string {",
		"\treturn finalize(value)",
		"}",
		"",
		"func finalize(value string) string { return \"[\" + value + \"]\" }",
	}, "\n")+"\n")
	writeFile(t, filepath.Join(runDir, "snapshot.json"), `{
  "repo_name":"fixture",
  "go_facts":{
    "modules":[{"id":"module-fixture","module_path":"example.com/fixture","module_dir":".","display_name":"."}],
    "packages":[{
      "canonical_package_path":"example.com/fixture",
      "name":"fixture",
      "owning_module_id":"module-fixture",
      "module_path":"example.com/fixture",
      "package_directory":".",
      "module_relative_path":".",
      "display_path":".",
      "locality":"local",
      "files":["pipeline.go"]
    }]
  }
	}`)
	writeFile(t, filepath.Join(runDir, "llm_bundle.json"), `{
  "allowed_paths":["pipeline.go"],
  "source_signals":[{
    "path":"pipeline.go",
    "line":3,
    "category":"request_handler",
    "snippet":"func Accept(input string) string",
    "reason":"saved fixture entry"
  }]
}`)
	writeFile(t, filepath.Join(runDir, "orientation_report.json"), `{
  "project_guess":"A fixture accepts an input value, transforms it, and returns output."
}`)

	first := freshPublicationReplayFixture(t, repoRoot, runDir, "first")
	if _, steps, err := publishFreshMechanism(
		runDir,
		first.identity,
		first.candidateID,
		first.probeRaw,
		first.supplement,
		first.recordRaw,
		first.artifact,
	); err != nil {
		t.Fatalf("publish first mechanism: %v", err)
	} else if steps < 3 {
		t.Fatalf("first visible steps = %d, want at least 3", steps)
	}

	// Build the second saved response against the clean, accepted first replay.
	// Publication must not let the old root Mechanism contaminate the new
	// bundle with a transient stale-input warning.
	second := freshPublicationReplayFixture(t, repoRoot, runDir, "second")
	if _, steps, err := publishFreshMechanism(
		runDir,
		second.identity,
		second.candidateID,
		second.probeRaw,
		second.supplement,
		second.recordRaw,
		second.artifact,
	); err != nil {
		t.Fatalf("publish second mechanism without a model call: %v", err)
	} else if steps < 3 {
		t.Fatalf("second visible steps = %d, want at least 3", steps)
	}

	replayed, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, mechanism := range replayed.UserMechanisms {
		seen[mechanism.ArtifactID] = true
	}
	for _, fixture := range []freshPublicationReplay{first, second} {
		if !seen[fixture.artifact.ID] {
			t.Errorf("published artifact %q is unavailable after replay", fixture.artifact.ID)
		}
	}
	for _, warning := range replayed.Warnings {
		if strings.Contains(warning, "replay bundle hash does not match current facts") {
			t.Fatalf("publication leaked transient replay warning: %s", warning)
		}
	}
}

type freshPublicationReplay struct {
	identity    semanticdiscovery.MechanismIdentity
	candidateID string
	probeRaw    []byte
	supplement  report.SemanticSupplement
	recordRaw   []byte
	artifact    semanticdiscovery.Artifact
}

func freshPublicationReplayFixture(
	t *testing.T,
	repoRoot string,
	runDir string,
	suffix string,
) freshPublicationReplay {
	t.Helper()
	probe, err := goldenmechanism.Probe(context.Background(), repoRoot, goldenmechanism.Plan{
		MechanismID: "publication-" + suffix,
		Seeds: []goldenmechanism.Seed{
			{OriginFactID: "anchor-" + suffix + "-accept", OriginEvidenceID: "anchor-evidence-accept", Path: "pipeline.go", Symbol: "Accept"},
			{OriginFactID: "anchor-" + suffix + "-transform", OriginEvidenceID: "anchor-evidence-transform", Path: "pipeline.go", Symbol: "Transform"},
			{OriginFactID: "anchor-" + suffix + "-emit", OriginEvidenceID: "anchor-evidence-emit", Path: "pipeline.go", Symbol: "Emit"},
		},
		ExpansionAllowlist: []string{"Transform", "Emit", "finalize"},
		Limits: goldenmechanism.Limits{
			MaxDepth: 1, MaxFiles: 1, MaxFunctions: 3,
			MaxParsedSourceBytes: 32 << 10, MaxSourceBytes: 32 << 10,
			MaxFunctionLines: 40, MaxFunctionBytes: 8 << 10,
			Timeout: time.Second,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	probeRaw, err := marshalGoldenJSON(probe)
	if err != nil {
		t.Fatal(err)
	}
	probeDigest := sha256.Sum256(probeRaw)
	probeSHA := hex.EncodeToString(probeDigest[:])
	facts := freshPublicationFacts(t, probe, suffix)

	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	data.SemanticSupplementalFacts = append([]semanticdiscovery.Fact(nil), facts...)
	preliminaryBundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	supportIDs := make([]string, 0, len(facts))
	for _, fact := range facts {
		supportIDs = append(supportIDs, fact.ID)
	}
	rawCandidate := semanticdiscovery.OpportunityCandidate{
		Kind:               semanticdiscovery.ArtifactMechanism,
		Title:              "Bounded input processing " + suffix,
		QuestionAnswered:   "How does the operation accept, transform, and return a value " + suffix + "?",
		SupportIDs:         supportIDs,
		MissingInformation: []string{},
		ExpectedValue:      semanticdiscovery.ExpectedValueHigh,
		Confidence:         semanticdiscovery.ConfidenceHigh,
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		preliminaryBundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{rawCandidate},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalize candidate: report=%#v proposal=%#v", normalization, proposal)
	}
	candidate := proposal.Candidates[0]
	capabilities := []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior}
	aspects := make([]semanticdiscovery.AnswerAspect, 0, len(facts))
	for index, fact := range facts {
		capability := fact.Capabilities[len(fact.Capabilities)-1]
		capabilities = append(capabilities, capability)
		aspects = append(aspects, semanticdiscovery.AnswerAspect{
			ID: "operation-" + string(rune('1'+index)), Label: "Operation " + string(rune('1'+index)),
			RequiredCapabilities: []semanticdiscovery.Capability{capability}, Key: true,
		})
	}
	capabilities = uniquePublicationCapabilities(capabilities)
	candidate.CapabilityContract = &semanticdiscovery.CapabilityContract{
		RequiredCapabilities:  append([]semanticdiscovery.Capability(nil), capabilities...),
		AvailableCapabilities: capabilities,
		MissingCapabilities:   []semanticdiscovery.Capability{},
		Resolution:            semanticdiscovery.CapabilityResolutionReady,
	}
	candidate.IntentContract = &semanticdiscovery.IntentContract{
		RequiredAnswerAspects: aspects,
		MinCovered:            len(aspects),
		MinKeyCovered:         len(aspects),
		LocalSearchAliases:    []string{"bounded input processing " + suffix},
	}
	proposal.Candidates[0] = candidate

	data.SemanticSupplementalFacts = nil
	supplement, bundle, err := report.PrepareSemanticSupplement(
		data,
		candidate.ID,
		probeSHA,
		facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(
		bundle,
		[]semanticdiscovery.OpportunityCandidate{candidate},
	)
	if err != nil {
		t.Fatal(err)
	}
	observations := make([]semanticdiscovery.LeafObservation, 0, len(facts))
	claims := make([]semanticdiscovery.ProposedClaim, 0, len(facts))
	for index, fact := range facts {
		observations = append(observations, semanticdiscovery.LeafObservation{
			Text: fact.Statement, SupportIDs: []string{fact.ID},
		})
		claims = append(claims, semanticdiscovery.ProposedClaim{
			Title: []string{"Accept the input", "Transform the value", "Return the output"}[index],
			Text:  fact.Statement, Basis: semanticdiscovery.ClaimDirect,
			SupportIDs: []string{fact.ID},
			ObservationRefs: []semanticdiscovery.ObservationRef{{
				TaskID: tasks[0].ID, ObservationIndex: index,
			}},
		})
	}
	leaf := semanticdiscovery.LeafResult{
		Task: tasks[0],
		Artifact: semanticdiscovery.LeafArtifact{
			Version: semanticdiscovery.LeafArtifactVersion,
			TaskID:  tasks[0].ID, CandidateID: candidate.ID,
			Status:       semanticdiscovery.LeafStatusUsable,
			Observations: observations,
			CandidateConnection: semanticdiscovery.LeafCandidateConnection{
				CandidateID: candidate.ID, Relation: "needs_combination",
				Explanation: "The supported operations combine into the requested behavior",
				SupportIDs:  supportIDs,
			},
		},
	}
	if err := semanticdiscovery.ValidateLeafArtifact(leaf.Task, leaf.Artifact); err != nil {
		t.Fatal(err)
	}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID:     candidate.ID,
			Verdict:         semanticdiscovery.VerdictSupported,
			Title:           candidate.Title,
			Summary:         "The operation accepts input, transforms the value, and returns the resulting output.",
			Claims:          claims,
			Aliases:         append([]string(nil), candidate.IntentContract.LocalSearchAliases...),
			LikelyQuestions: []string{candidate.QuestionAnswered},
		}},
	}
	responseRaw, err := json.Marshal(fanIn)
	if err != nil {
		t.Fatal(err)
	}
	evaluated, err := evaluateGoldenMechanismResponse(bundle, proposal, leaf, responseRaw)
	if err != nil {
		t.Fatalf("evaluate fixed response: %v", err)
	}
	if len(evaluated.Artifacts) != 1 {
		t.Fatalf("evaluated artifacts = %#v", evaluated.Artifacts)
	}
	return freshPublicationReplay{
		identity: semanticdiscovery.MechanismIdentity{
			RepositoryNamespace: "example.com/fixture",
			IntentKey:           probe.MechanismID,
			Scope: semanticdiscovery.MechanismScope{
				Kind:  semanticdiscovery.MechanismScopeGoPackage,
				Value: "example.com/fixture",
			},
		},
		candidateID: candidate.ID,
		probeRaw:    probeRaw,
		supplement:  supplement,
		recordRaw:   evaluated.RecordBytes,
		artifact:    evaluated.Artifacts[0],
	}
}

func freshPublicationFacts(
	t *testing.T,
	probe goldenmechanism.Result,
	suffix string,
) []semanticdiscovery.Fact {
	t.Helper()
	statements := map[string]string{
		"Accept":    "The input stage passes the supplied value to the transformation stage.",
		"Transform": "The transformation stage passes the value to the output stage.",
		"Emit":      "The output stage passes the transformed value to the final return helper.",
	}
	functions := make(map[string]goldenmechanism.Function, len(probe.Functions))
	for _, function := range probe.Functions {
		functions[function.ID] = function
	}
	bySymbol := make(map[string]goldenmechanism.Observation)
	for _, observation := range probe.Observations {
		function := functions[observation.FunctionID]
		if _, wanted := statements[function.Symbol]; !wanted || len(observation.Evidence) == 0 {
			continue
		}
		if observation.Basis == goldenmechanism.BasisDeclaration ||
			observation.Basis == goldenmechanism.BasisLexicalOrder {
			continue
		}
		if _, exists := bySymbol[function.Symbol]; !exists {
			bySymbol[function.Symbol] = observation
		}
	}
	result := make([]semanticdiscovery.Fact, 0, len(statements))
	for index, symbol := range []string{"Accept", "Transform", "Emit"} {
		observation, exists := bySymbol[symbol]
		if !exists {
			t.Fatalf("probe has no useful observation for %s: %#v", symbol, probe.Observations)
		}
		references := make([]semanticdiscovery.EvidenceRef, 0, len(observation.Evidence))
		for _, reference := range observation.Evidence {
			references = append(references, semanticdiscovery.EvidenceRef{
				ID: reference.ID, Kind: "bounded_go_syntax", Label: string(observation.Basis),
				Path: reference.Location.Path, Line: reference.Location.Line,
				Column: reference.Location.Column,
			})
		}
		result = append(result, semanticdiscovery.Fact{
			ID:          "publication-fact-" + suffix + "-" + strings.ToLower(symbol),
			Kind:        semanticdiscovery.FactSourceSignal,
			Statement:   statements[symbol],
			Keywords:    []string{"answer_aspect:operation-" + string(rune('1'+index))},
			SourceGroup: "publication-group-" + suffix + "-" + strings.ToLower(symbol),
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityBehavior,
				observation.Capability,
			},
			Scope:    semanticdiscovery.FactScopeLocal,
			Evidence: references,
		})
	}
	return result
}

func uniquePublicationCapabilities(values []semanticdiscovery.Capability) []semanticdiscovery.Capability {
	seen := make(map[semanticdiscovery.Capability]struct{}, len(values))
	result := make([]semanticdiscovery.Capability, 0, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
