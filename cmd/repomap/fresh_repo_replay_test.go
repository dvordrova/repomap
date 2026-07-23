package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestParseFreshRepoOnboardingCLIOptions(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReplay bool
		wantReplan bool
		wantErr    bool
	}{
		{
			name:    "live run requires repository",
			args:    []string{"--run-dir", "run"},
			wantErr: true,
		},
		{
			name:       "saved replay needs only run directory",
			args:       []string{"--run-dir", "run", "--replay-saved"},
			wantReplay: true,
		},
		{
			name: "live run keeps repository",
			args: []string{"--run-dir", "run", "--repo", "repo"},
		},
		{
			name:       "saved primary replan keeps repository",
			args:       []string{"--run-dir", "run", "--repo", "repo", "--replan-saved"},
			wantReplan: true,
		},
		{
			name:    "saved primary replan requires repository",
			args:    []string{"--run-dir", "run", "--replan-saved"},
			wantErr: true,
		},
		{
			name:    "replay and replan are mutually exclusive",
			args:    []string{"--run-dir", "run", "--repo", "repo", "--replay-saved", "--replan-saved"},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseFreshRepoOnboardingCLIOptions(test.args, &bytes.Buffer{})
			if test.wantErr {
				if err == nil {
					t.Fatalf("parse options = %#v, want error", options)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if options.RunDir != "run" || options.ReplaySaved != test.wantReplay ||
				options.ReplanSaved != test.wantReplan {
				t.Fatalf("options = %#v", options)
			}
		})
	}
}

func TestValidateFreshReplayPlanPrimaryVersionCompatibility(t *testing.T) {
	t.Parallel()

	const (
		candidateID = "semantic-candidate-replay-version"
		question    = "How does the command persist repository state?"
		intentKey   = "primary-path-replay-version"
		factID      = "fact-replay-version-anchor"
		evidenceID  = "evidence-replay-version-anchor"
	)
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               candidateID,
		Kind:             semanticdiscovery.ArtifactMechanism,
		QuestionAnswered: question,
	}
	facts := map[string]semanticdiscovery.Fact{
		factID: {
			ID: factID,
			Evidence: []semanticdiscovery.EvidenceRef{{
				ID: evidenceID,
			}},
		},
	}
	newPlan := func(
		version int,
		maxFunctions int,
		maxAdditionalFunctions int,
		selectedFrontiers int,
		probeMaxFunctions int,
	) freshRepoCandidatePlan {
		return freshRepoCandidatePlan{
			CandidateID:       candidateID,
			Question:          question,
			Kind:              semanticdiscovery.ArtifactMechanism,
			AnchorFactIDs:     []string{factID},
			AnchorEvidenceIDs: []string{evidenceID},
			Identity: semanticdiscovery.MechanismIdentity{
				RepositoryNamespace: "example.test/replay",
				IntentKey:           intentKey,
				Scope: semanticdiscovery.MechanismScope{
					Kind:  semanticdiscovery.MechanismScopeGoPackage,
					Value: "example.test/replay",
				},
			},
			Probe: goldenmechanism.Plan{
				MechanismID: intentKey,
				Seeds: []goldenmechanism.Seed{{
					OriginFactID:     factID,
					OriginEvidenceID: evidenceID,
					Path:             "command.go",
					Symbol:           "Command.Run",
				}},
				Limits: goldenmechanism.Limits{
					MaxDepth:             3,
					MaxFiles:             freshRepoDemoMaxProbeFiles,
					MaxFunctions:         probeMaxFunctions,
					MaxParsedSourceBytes: freshRepoDemoMaxParsedBytes,
					MaxSourceBytes:       freshPrimaryMaxRetainedBytes,
					MaxFunctionLines:     220,
					MaxFunctionBytes:     48 << 10,
					Timeout:              5 * time.Second,
				},
			},
			Primary: &freshPrimaryProbePlan{
				Version:           version,
				CandidateID:       candidateID,
				Question:          question,
				IntentKey:         intentKey,
				SelectedFrontiers: make([]freshPrimaryFrontier, selectedFrontiers),
				Limits: freshPrimaryLimits{
					MaxFrontierExpansions: freshPrimaryMaxFrontiers,
					MaxFiles:              freshPrimaryMaxFiles,
					MaxFunctions:          maxFunctions,
					MaxAdditionalFiles:    freshPrimaryMaxAdditionalFiles,
					MaxAdditionalFuncs:    maxAdditionalFunctions,
					MaxRetainedBytes:      freshPrimaryMaxRetainedBytes,
					MaxDepth:              freshPrimaryMaxDepth,
					Timeout:               freshPrimaryTimeout,
				},
			},
		}
	}

	tests := []struct {
		name    string
		plan    freshRepoCandidatePlan
		wantErr string
	}{
		{
			name: "legacy v1 bounds remain replayable",
			plan: newPlan(1, 15, 11, 2, 15),
		},
		{
			name: "current v2 bounds are replayable",
			plan: newPlan(freshPrimaryPlanVersion, 10, 6, 3, 10),
		},
		{
			name:    "unknown v3 is rejected",
			plan:    newPlan(3, 10, 6, 3, 10),
			wantErr: "unsupported primary plan version 3",
		},
		{
			name: "v2 cannot carry legacy v1 budgets",
			plan: newPlan(
				freshPrimaryPlanVersion,
				15,
				11,
				2,
				15,
			),
			wantErr: "primary plan is outside bounds",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFreshReplayPlan(test.plan, candidate, facts)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate replay plan: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate replay plan error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestEvaluateFreshSavedCandidateUsesUnchangedResponseWithoutProvider(t *testing.T) {
	repoRoot := t.TempDir()
	runDir := t.TempDir()
	sourceLines := []string{
		"package pipeline",
		"",
		"func entry(input string) string {",
		"\tvalue := load(input)",
		"\tif value == \"\" {",
		"\t\treturn fallback()",
		"\t}",
		"\treturn render(value)",
		"}",
		"",
		"func load(input string) string {",
		"\tvalue := normalize(input)",
		"\treturn value",
		"}",
		"",
		"func render(value string) string {",
		"\toutput := transform(value)",
		"\treturn output",
		"}",
		"",
		"func fallback() string {",
		"\treturn \"default\"",
		"}",
		"",
		"func normalize(input string) string { return input }",
		"func transform(input string) string { return input }",
	}
	writeFile(t, filepath.Join(repoRoot, "pipeline.go"), joinLines(sourceLines))
	window, err := sourcewindowfacts.NewWindow(
		"evidence-pipeline-replay",
		"pipeline.go",
		1,
		sourceLines,
	)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := sourcewindowfacts.ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]freshSourceFunction, 0, len(functions))
	for _, function := range functions {
		fact, factErr := freshWindowFunctionFact(function)
		if factErr != nil {
			t.Fatal(factErr)
		}
		sources = append(sources, freshSourceFunction{Function: function, Fact: fact})
	}

	planningData := freshReplayTestReport()
	planningData.SemanticSupplementalFacts = freshFacts(sources)
	planningBundle, err := report.BuildSemanticDiscoveryBundle(planningData)
	if err != nil {
		t.Fatal(err)
	}
	rawCandidate := semanticdiscovery.OpportunityCandidate{
		Kind:             semanticdiscovery.ArtifactMechanism,
		Title:            "Request processing",
		QuestionAnswered: "How does the command process input and produce output?",
		SupportIDs: []string{
			sources[0].Fact.ID,
			sources[1].Fact.ID,
			sources[2].Fact.ID,
		},
		MissingInformation: []string{},
		ExpectedValue:      semanticdiscovery.ExpectedValueHigh,
		Confidence:         semanticdiscovery.ConfidenceHigh,
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		planningBundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{rawCandidate},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalization = %#v, proposal = %#v", normalization, proposal)
	}
	works := selectFreshCandidates(repoRoot, planningData, proposal, sources)
	if len(works) != 1 {
		t.Fatalf("selected works = %d, want 1", len(works))
	}
	savedPlan := works[0].Plan
	probe, err := goldenmechanism.Probe(t.Context(), repoRoot, savedPlan.Probe)
	if err != nil {
		t.Fatal(err)
	}
	probeRaw, err := marshalGoldenJSON(probe)
	if err != nil {
		t.Fatal(err)
	}
	probeFacts, aspects, err := freshProbeFacts(works[0], probe)
	if err != nil {
		t.Fatal(err)
	}
	facts, projectedCandidate, err := freshCandidateProjection(works[0], probeFacts, aspects)
	if err != nil {
		t.Fatal(err)
	}
	evaluationData := freshReplayTestReport()
	_, bundle, err := report.PrepareSemanticSupplement(
		evaluationData,
		projectedCandidate.ID,
		freshSHA256(probeRaw),
		facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	finalProposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projectedCandidate},
	}
	leaf, err := freshValidatedLeaf(bundle, projectedCandidate, probeFacts)
	if err != nil {
		t.Fatal(err)
	}
	probeOffset := len(leaf.Artifact.Observations) - len(probeFacts)
	claims := make([]semanticdiscovery.ProposedClaim, 0, len(probeFacts))
	for index, fact := range probeFacts {
		claims = append(claims, semanticdiscovery.ProposedClaim{
			Title:      aspects[index].Label,
			Text:       fact.Statement,
			Basis:      semanticdiscovery.ClaimDirect,
			SupportIDs: []string{fact.ID},
			ObservationRefs: []semanticdiscovery.ObservationRef{{
				TaskID:           leaf.Task.ID,
				ObservationIndex: probeOffset + index,
			}},
		})
	}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID:     projectedCandidate.ID,
			Verdict:         semanticdiscovery.VerdictSupported,
			Title:           projectedCandidate.Title,
			Summary:         "The entry function calls render, load, and fallback while render calls transform and load calls normalize.",
			Claims:          claims,
			Aliases:         projectedCandidate.IntentContract.LocalSearchAliases,
			LikelyQuestions: []string{projectedCandidate.QuestionAnswered},
		}},
	}
	if err := semanticdiscovery.ValidateFanInArtifact(
		bundle,
		[]semanticdiscovery.LeafResult{leaf},
		fanIn,
	); err != nil {
		t.Fatalf("fixture fan-in: %v; fan-in = %#v", err, fanIn)
	}
	responseContent, err := json.Marshal(fanIn)
	if err != nil {
		t.Fatal(err)
	}
	fixtureEvaluation, err := evaluateGoldenMechanismResponse(
		bundle,
		finalProposal,
		leaf,
		responseContent,
	)
	if err != nil {
		t.Fatalf(
			"fixture response is not accepted before replay: %v; reduction = %#v",
			err,
			fixtureEvaluation.Reduction,
		)
	}

	attemptDir := filepath.Join(runDir, freshRepoDemoAttemptsDir, projectedCandidate.ID)
	if err := writeAtomicFile(filepath.Join(attemptDir, "probe.json"), probeRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeGoldenJSON(
		filepath.Join(attemptDir, "response_attempt.json"),
		goldenMechanismResponseAttempt{
			Version:       1,
			CandidateID:   projectedCandidate.ID,
			PromptVersion: semanticdiscovery.GoldenMechanismPromptVersion,
			Content:       string(responseContent),
		},
	); err != nil {
		t.Fatal(err)
	}
	responsePath := filepath.Join(attemptDir, "response_attempt.json")
	responseBefore, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatal(err)
	}
	rebuiltSources, _, err := freshReplaySources(freshFacts(sources))
	if err != nil {
		t.Fatal(err)
	}
	replayed, rejected, err := evaluateFreshSavedCandidate(
		freshReplayTestReport(),
		runDir,
		proposal.Candidates[0],
		savedPlan,
		rebuiltSources,
	)
	if err != nil || rejected {
		t.Fatalf("saved replay = rejected %t, err %v, attempt %#v", rejected, err, replayed.Attempt)
	}
	if replayed.Artifact.CandidateID != projectedCandidate.ID ||
		len(replayed.Artifact.Statements) < 3 ||
		!bytes.Equal(replayed.Synthesis.RawResponse, responseContent) {
		t.Fatalf("replayed artifact = %#v", replayed.Artifact)
	}
	responseAfter, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(responseBefore, responseAfter) {
		t.Fatal("saved response changed during local replay")
	}
}

func freshReplayTestReport() *report.ReportData {
	return &report.ReportData{
		RepoName:          "fixture",
		DocumentedPurpose: "Process input into output through bounded local operations.",
		OpenablePaths:     []string{"pipeline.go"},
		RepositoryGraph: &report.RepositoryGraph{
			Modules: []report.ModuleInfo{{Path: "example.com/pipeline"}},
			PackageEdges: []report.EdgeInfo{{
				From: "example.com/pipeline",
				To:   "example.com/pipeline/runtime",
			}},
			Packages: []report.PackageInfo{{
				Name:          "pipeline",
				CanonicalPath: "example.com/pipeline",
				ModulePath:    "example.com/pipeline",
				Locality:      "local",
				Files:         []string{"pipeline.go"},
			}},
		},
	}
}

func freshSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
