package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

type semanticDiscoveryEditorStub struct {
	mu          sync.Mutex
	calls       map[string]int
	opportunity semanticdiscovery.OpportunityProposal
	leaves      map[string]semanticdiscovery.LeafArtifact
	fanIn       semanticdiscovery.FanInArtifact
	monolithic  semanticdiscovery.FanInArtifact
}

func TestNewSemanticDiscoveryStagePlanRejectsNilProvider(t *testing.T) {
	t.Parallel()

	_, err := newSemanticDiscoveryStagePlan(nil, semanticdiscovery.Prompt{}, "test_stage")
	if err == nil {
		t.Fatal("newSemanticDiscoveryStagePlan accepted a nil provider")
	}
}

func (stub *semanticDiscoveryEditorStub) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *semanticDiscoveryEditorStub) DiscoverSemanticsMeasured(
	_ context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls[prompt.Version]++
	var value any
	switch prompt.Version {
	case semanticdiscovery.OpportunityPromptVersion:
		value = stub.opportunity
	case semanticdiscovery.LeafPromptVersion:
		for _, leaf := range stub.leaves {
			value = leaf
			break
		}
	case semanticdiscovery.FanInPromptVersion:
		value = stub.fanIn
	case semanticdiscovery.MonolithicPromptVersion:
		value = stub.monolithic
	default:
		value = map[string]any{"unexpected": prompt.Version}
	}
	encoded, err := json.Marshal(value)
	return modelresearch.ProviderResult{
		Content: encoded, Attempts: 1,
		InputTokens: 100, OutputTokens: 20,
		PromptCacheHitTokens: 10, PromptCacheMissTokens: 90,
	}, err
}

func TestSemanticDiscoveryRunsFanInForMissingOnlyLeafWithoutStageCache(t *testing.T) {
	t.Parallel()

	bundle, proposal, selected, leaves, fanIn, monolithic := semanticDiscoveryFixture(t)
	providerProposal := proposal
	providerProposal.Candidates = append(providerProposal.Candidates, semanticdiscovery.OpportunityCandidate{
		Kind:             semanticdiscovery.ArtifactMechanism,
		Title:            "Unsupported opportunity",
		QuestionAnswered: "What does an unknown fact establish?",
		SupportIDs:       []string{"unknown-fact"},
		ExpectedValue:    semanticdiscovery.ExpectedValueLow,
		Confidence:       semanticdiscovery.ConfidenceLow,
	})
	leafWithInvalidItem := leaves[0].Artifact
	leafWithInvalidItem.Observations = []semanticdiscovery.LeafObservation{{
		Text:       "internal/analyzer.go handles package metadata",
		SupportIDs: append([]string(nil), leaves[0].Task.Candidate.SupportIDs...),
	}}
	stub := &semanticDiscoveryEditorStub{
		calls:       make(map[string]int),
		opportunity: providerProposal,
		leaves: map[string]semanticdiscovery.LeafArtifact{
			leaves[0].Task.ID: leafWithInvalidItem,
		},
		fanIn: fanIn, monolithic: monolithic,
	}
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}

	first, err := ensureSemanticDiscoveryFanout(
		context.Background(), bundle, runDir, stub,
	)
	if err != nil {
		t.Fatalf("ensureSemanticDiscoveryFanout() error = %v", err)
	}
	if first.Outcome.LeafInsufficient != 1 || first.Outcome.Artifacts != 1 ||
		first.Outcome.LeafReductionIssues != 1 {
		t.Fatalf("missing-only outcome = %#v", first.Outcome)
	}
	if stub.calls[semanticdiscovery.FanInPromptVersion] != 1 {
		t.Fatalf("fan-in calls = %d, want 1 for a missing-only leaf", stub.calls[semanticdiscovery.FanInPromptVersion])
	}
	if first.Outcome.SemanticCalls != 3 || first.Outcome.PromptCacheHitTokens != 30 ||
		first.Outcome.PromptCacheMissTokens != 270 {
		t.Fatalf("first run metrics = %#v", first.Outcome)
	}
	var firstOpportunity semanticDiscoveryOpportunityArtifact
	readSemanticDiscoveryTestJSON(
		t,
		filepath.Join(runDir, semanticDiscoveryOpportunityFile),
		&firstOpportunity,
	)
	if len(firstOpportunity.Normalization.Issues) == 0 {
		t.Fatal("first opportunity normalization diagnostics are empty")
	}
	rawRecord, err := os.ReadFile(filepath.Join(runDir, semanticdiscovery.RecordFile))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := semanticdiscovery.ReplayRecord(bundle, rawRecord)
	if err != nil {
		t.Fatalf("ReplayRecord() error = %v", err)
	}
	if len(replayed) != 1 || replayed[0].Verdict != semanticdiscovery.VerdictInsufficientEvidence {
		t.Fatalf("replayed artifacts = %#v", replayed)
	}

	second, err := ensureSemanticDiscoveryFanout(
		context.Background(), bundle, runDir, stub,
	)
	if err != nil {
		t.Fatalf("second ensureSemanticDiscoveryFanout() error = %v", err)
	}
	if second.Outcome.SemanticCalls != 3 {
		t.Fatalf("second run calls = %d, want 3", second.Outcome.SemanticCalls)
	}
	var secondOpportunity semanticDiscoveryOpportunityArtifact
	readSemanticDiscoveryTestJSON(
		t,
		filepath.Join(runDir, semanticDiscoveryOpportunityFile),
		&secondOpportunity,
	)
	if !reflect.DeepEqual(secondOpportunity.Normalization, firstOpportunity.Normalization) {
		t.Fatalf(
			"second opportunity diagnostics = %#v, want preserved %#v",
			secondOpportunity.Normalization,
			firstOpportunity.Normalization,
		)
	}
	if second.Outcome.LeafReductionIssues != first.Outcome.LeafReductionIssues {
		t.Fatalf(
			"second reduction issues = %d, want preserved %d",
			second.Outcome.LeafReductionIssues,
			first.Outcome.LeafReductionIssues,
		)
	}
	for version, want := range map[string]int{
		semanticdiscovery.OpportunityPromptVersion: 2,
		semanticdiscovery.LeafPromptVersion:        2,
		semanticdiscovery.FanInPromptVersion:       2,
	} {
		if got := stub.calls[version]; got != want {
			t.Fatalf("provider calls for %s = %d, want %d", version, got, want)
		}
	}

	baseline, err := executeSemanticMonolithic(
		context.Background(), bundle, selected, stub,
	)
	if err != nil {
		t.Fatalf("executeSemanticMonolithic() error = %v", err)
	}
	if baseline.Outcome.Artifacts != 1 || baseline.Outcome.UnsupportedClaims != 0 ||
		baseline.Outcome.SemanticCalls != 1 {
		t.Fatalf("monolithic outcome = %#v", baseline.Outcome)
	}
}

func readSemanticDiscoveryTestJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func TestCompareSemanticDiscoveryTokensMarksUnequalUsage(t *testing.T) {
	t.Parallel()
	comparison := compareSemanticDiscoveryTokens([]semanticDiscoveryVariantMetrics{
		{SelectedCandidates: 5, InputTokens: 4_000, OutputTokens: 6_000},
		{SelectedCandidates: 5, InputTokens: 2_000, OutputTokens: 2_000},
	})
	if !comparison.SameBundleAndCandidates || comparison.FanOutTotalTokens != 10_000 ||
		comparison.MonolithicTotalTokens != 4_000 ||
		comparison.LargerToSmallerRatioPermille != 2_500 ||
		comparison.ComparableWithin25Percent {
		t.Fatalf("token comparison = %#v", comparison)
	}
}

func TestValidateSemanticOpportunityNormalizationReportAcceptsReducedExpectedPathSupport(t *testing.T) {
	t.Parallel()

	err := validateSemanticOpportunityNormalizationReport(semanticdiscovery.NormalizationReport{
		Issues: []semanticdiscovery.NormalizationIssue{
			{
				CandidateIndex: 0,
				Code:           "expected_path_support_reduced",
				Detail:         "input_trigger: entry",
			},
			{
				CandidateIndex: 0,
				Code:           "architecture_anchor_support_reduced",
				Detail:         "fact-outside-candidate",
			},
		},
	})
	if err != nil {
		t.Fatalf("validateSemanticOpportunityNormalizationReport() error = %v", err)
	}
}

func TestExecuteSemanticFanInReducesInvalidProposalOnEveryRun(t *testing.T) {
	t.Parallel()

	bundle, proposal, _, leaves, fanIn, _ := semanticDiscoveryFixture(t)
	const warningFactID = "fact-warning-aggregate"
	bundle.Facts = append(bundle.Facts,
		semanticdiscovery.Fact{
			ID: warningFactID, Kind: semanticdiscovery.FactWarning,
			Statement:    "The saved warning aggregate does not establish validation behavior",
			Keywords:     []string{"warning aggregate", "validation behavior"},
			SourceGroup:  "group-warning-aggregate",
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityLimitation},
			Scope:        semanticdiscovery.FactScopeRepository,
		},
		semanticdiscovery.Fact{
			ID: "fact-warning-secondary", Kind: semanticdiscovery.FactWarning,
			Statement:    "A second saved warning also leaves validation behavior unresolved",
			Keywords:     []string{"warning aggregate", "validation behavior"},
			SourceGroup:  "group-warning-secondary",
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityLimitation},
			Scope:        semanticdiscovery.FactScopeRepository,
		},
	)
	proposal.Candidates = append(proposal.Candidates, semanticdiscovery.OpportunityCandidate{
		Kind:             semanticdiscovery.ArtifactRepositoryPattern,
		Title:            "Warning validation pattern",
		QuestionAnswered: "What validation behavior do saved warnings establish?",
		SupportIDs:       []string{warningFactID, "fact-warning-secondary"},
		MissingInformation: []string{
			"The saved warning aggregate does not establish validation behavior",
		},
		ExpectedValue: semanticdiscovery.ExpectedValueHigh,
		Confidence:    semanticdiscovery.ConfidenceLow,
	})
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(bundle, proposal)
	if len(normalization.Issues) != 0 {
		t.Fatalf("NormalizeOpportunityProposal() = %#v", normalization)
	}
	selected, err := semanticdiscovery.SelectOpportunities(bundle, proposal, 2)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, selected)
	if err != nil {
		t.Fatal(err)
	}
	var patternTask semanticdiscovery.LeafTask
	for _, task := range tasks {
		if task.Candidate.Kind == semanticdiscovery.ArtifactRepositoryPattern {
			patternTask = task
		}
	}
	if patternTask.ID == "" {
		t.Fatal("repository-pattern task was not selected")
	}
	missingText := "The saved warning aggregate does not establish validation behavior"
	patternLeaf := semanticdiscovery.LeafResult{
		Task: patternTask,
		Artifact: semanticdiscovery.LeafArtifact{
			Version:     semanticdiscovery.LeafArtifactVersion,
			TaskID:      patternTask.ID,
			CandidateID: patternTask.Candidate.ID,
			Status:      semanticdiscovery.LeafStatusInsufficientEvidence,
			CandidateConnection: semanticdiscovery.LeafCandidateConnection{
				CandidateID: patternTask.Candidate.ID,
				Relation:    "needs_combination",
				Explanation: "The bounded warning observation needs combination",
				SupportIDs:  []string{warningFactID},
			},
			MissingEvidence: []semanticdiscovery.LeafMissingEvidence{{
				Explanation: missingText, SupportIDs: []string{warningFactID},
				MissingCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior},
			}},
		},
	}
	if err := semanticdiscovery.ValidateLeafArtifact(patternTask, patternLeaf.Artifact); err != nil {
		t.Fatalf("ValidateLeafArtifact() = %v", err)
	}
	leaves = append(leaves, patternLeaf)
	fanIn.Artifacts = append(fanIn.Artifacts, semanticdiscovery.ArtifactProposal{
		CandidateID: patternTask.Candidate.ID,
		Verdict:     semanticdiscovery.VerdictInsufficientEvidence,
		Title:       "Warning validation pattern",
		Summary:     missingText,
		Claims: []semanticdiscovery.ProposedClaim{{
			Title: "Behavior gap", Text: missingText,
			Basis: semanticdiscovery.ClaimUnresolved, SupportIDs: []string{warningFactID},
		}},
	})

	stub := &semanticDiscoveryEditorStub{
		calls: make(map[string]int), fanIn: fanIn,
	}
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	call := func() ([]semanticdiscovery.Artifact, semanticDiscoveryStageMetrics, int, semanticdiscovery.FanInReductionReport, error) {
		_, artifacts, stage, unsupported, reduction, err := executeSemanticFanIn(
			context.Background(), bundle, proposal, selected, leaves, runDir,
			stub, &semanticDiscoveryBudget{},
		)
		return artifacts, stage, unsupported, reduction, err
	}
	firstArtifacts, firstStage, firstUnsupported, firstReduction, err := call()
	if err != nil {
		t.Fatalf("first executeSemanticFanIn() = %v", err)
	}
	if firstStage.Status != "accepted" || len(firstArtifacts) != 1 ||
		firstUnsupported != 1 || firstReduction.KeptArtifacts != 1 ||
		firstReduction.DroppedArtifacts != 1 || len(firstReduction.Issues) != 1 {
		t.Fatalf("first fan-in = stage %#v artifacts %d unsupported %d reduction %#v", firstStage, len(firstArtifacts), firstUnsupported, firstReduction)
	}
	secondArtifacts, secondStage, secondUnsupported, secondReduction, err := call()
	if err != nil {
		t.Fatalf("second executeSemanticFanIn() = %v", err)
	}
	if secondStage.Status != "accepted" || len(secondArtifacts) != len(firstArtifacts) ||
		secondUnsupported != firstUnsupported || !reflect.DeepEqual(secondReduction, firstReduction) {
		t.Fatalf("second fan-in = stage %#v artifacts %d unsupported %d reduction %#v", secondStage, len(secondArtifacts), secondUnsupported, secondReduction)
	}
	if got := stub.calls[semanticdiscovery.FanInPromptVersion]; got != 2 {
		t.Fatalf("fan-in provider calls = %d, want 2", got)
	}
}

func TestExecuteSemanticMonolithicReducesInvalidProposalOnEveryRun(t *testing.T) {
	t.Parallel()

	bundle, _, selected, _, _, monolithic := semanticDiscoveryFixture(t)
	unknown := monolithic.Artifacts[0]
	unknown.CandidateID = "semantic-candidate-unknown"
	monolithic.Artifacts = append(monolithic.Artifacts, unknown)
	stub := &semanticDiscoveryEditorStub{
		calls: make(map[string]int), monolithic: monolithic,
	}
	call := func() (semanticDiscoveryMonolithicResult, error) {
		return executeSemanticMonolithic(context.Background(), bundle, selected, stub)
	}
	first, err := call()
	if err != nil {
		t.Fatalf("first executeSemanticMonolithic() = %v", err)
	}
	if first.Outcome.ValidationState != "accepted_partial" ||
		first.Outcome.Artifacts != 1 || first.Outcome.UnsupportedClaims != 1 ||
		first.Outcome.FanInReductionIssues != 1 ||
		first.Reduction.KeptArtifacts != 1 || first.Reduction.DroppedArtifacts != 1 ||
		first.Outcome.SemanticCalls != 1 ||
		len(first.Outcome.Stages) != 1 || first.Outcome.Stages[0].Status != "accepted" {
		t.Fatalf("first monolithic = outcome %#v reduction %#v", first.Outcome, first.Reduction)
	}
	second, err := call()
	if err != nil {
		t.Fatalf("second executeSemanticMonolithic() = %v", err)
	}
	if second.Outcome.ValidationState != first.Outcome.ValidationState ||
		second.Outcome.Artifacts != first.Outcome.Artifacts ||
		second.Outcome.UnsupportedClaims != first.Outcome.UnsupportedClaims ||
		second.Outcome.FanInReductionIssues != first.Outcome.FanInReductionIssues ||
		second.Outcome.SemanticCalls != 1 ||
		!reflect.DeepEqual(second.Reduction, first.Reduction) ||
		len(second.Outcome.Stages) != 1 || second.Outcome.Stages[0].Status != "accepted" {
		t.Fatalf("second monolithic = outcome %#v reduction %#v", second.Outcome, second.Reduction)
	}
	if got := stub.calls[semanticdiscovery.MonolithicPromptVersion]; got != 2 {
		t.Fatalf("monolithic provider calls = %d, want 2", got)
	}
}

func semanticDiscoveryFixture(t *testing.T) (
	semanticdiscovery.Bundle,
	semanticdiscovery.OpportunityProposal,
	[]semanticdiscovery.OpportunityCandidate,
	[]semanticdiscovery.LeafResult,
	semanticdiscovery.FanInArtifact,
	semanticdiscovery.FanInArtifact,
) {
	t.Helper()
	const factID = "fact-dependency-aggregate"
	bundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "fixture",
		Facts: []semanticdiscovery.Fact{{
			ID: factID, Kind: semanticdiscovery.FactDependency,
			Statement:   "The saved dependency aggregate lists package metadata imports",
			Keywords:    []string{"dependency aggregate", "package metadata"},
			SourceGroup: "group-dependency-aggregate",
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityLimitation,
			},
			Scope: semanticdiscovery.FactScopeRepository,
		}},
	}
	rawOpportunity := semanticdiscovery.OpportunityProposal{
		Version: semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{{
			Kind:             semanticdiscovery.ArtifactDependencyUsage,
			Title:            "Package metadata dependency usage",
			QuestionAnswered: "How is package metadata used?",
			SupportIDs:       []string{factID},
			MissingInformation: []string{
				"The saved dependency aggregate does not establish package metadata behavior",
			},
			ExpectedValue: semanticdiscovery.ExpectedValueHigh,
			Confidence:    semanticdiscovery.ConfidenceLow,
		}},
	}
	proposal, _ := semanticdiscovery.NormalizeOpportunityProposal(bundle, rawOpportunity)
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		t.Fatal(err)
	}
	selected, err := semanticdiscovery.SelectOpportunities(bundle, proposal, 1)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, selected)
	if err != nil {
		t.Fatal(err)
	}
	missingText := "The saved dependency aggregate does not establish package metadata behavior"
	leaves := []semanticdiscovery.LeafResult{{
		Task: tasks[0],
		Artifact: semanticdiscovery.LeafArtifact{
			Version:     semanticdiscovery.LeafArtifactVersion,
			TaskID:      tasks[0].ID,
			CandidateID: selected[0].ID,
			Status:      semanticdiscovery.LeafStatusInsufficientEvidence,
			CandidateConnection: semanticdiscovery.LeafCandidateConnection{
				CandidateID: selected[0].ID, Relation: "needs_combination",
				Explanation: "The bounded fact needs combination before a behavior conclusion",
				SupportIDs:  []string{factID},
			},
			MissingEvidence: []semanticdiscovery.LeafMissingEvidence{{
				Explanation: missingText, SupportIDs: []string{factID},
				MissingCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior},
			}},
		},
	}}
	if err := semanticdiscovery.ValidateLeafArtifact(leaves[0].Task, leaves[0].Artifact); err != nil {
		t.Fatal(err)
	}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID: selected[0].ID,
			Verdict:     semanticdiscovery.VerdictInsufficientEvidence,
			Title:       "Package metadata usage remains unresolved",
			Summary:     missingText,
			Claims: []semanticdiscovery.ProposedClaim{{
				Title: "Usage remains unresolved", Text: missingText,
				Basis:      semanticdiscovery.ClaimUnresolved,
				SupportIDs: []string{factID},
				MissingRefs: []semanticdiscovery.MissingEvidenceRef{{
					TaskID: tasks[0].ID, MissingIndex: 0,
				}},
			}},
			Aliases:         []string{"package metadata usage"},
			LikelyQuestions: []string{"How is package metadata used?"},
		}},
	}
	if err := semanticdiscovery.ValidateFanInArtifact(bundle, leaves, fanIn); err != nil {
		t.Fatal(err)
	}
	monolithic := fanIn
	monolithic.Artifacts = append([]semanticdiscovery.ArtifactProposal(nil), fanIn.Artifacts...)
	monolithic.Artifacts[0].Claims = append(
		[]semanticdiscovery.ProposedClaim(nil),
		fanIn.Artifacts[0].Claims...,
	)
	monolithic.Artifacts[0].Claims[0].MissingRefs = nil
	if err := semanticdiscovery.ValidateMonolithicArtifact(bundle, selected, monolithic); err != nil {
		t.Fatal(err)
	}
	return bundle, rawOpportunity, selected, leaves, fanIn, monolithic
}
