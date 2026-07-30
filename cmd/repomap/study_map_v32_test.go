package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestStudyPromptsKeepReadingLabelsCanonicalUnderLocalization(t *testing.T) {
	t.Parallel()

	const want = "reading_anchors.label is a closed schema value"
	for name, prompt := range map[string]string{
		"legacy":    studyMapUserPrompt,
		"split-v32": studyMapDirectionTask,
	} {
		if !strings.Contains(prompt, want) ||
			!strings.Contains(prompt, "the report localizes it later") {
			t.Fatalf("%s Study prompt does not protect reading labels", name)
		}
	}
}

type studyMapV32ReviewProviderStub struct {
	mu         sync.Mutex
	failPlanID string
	calls      []string
}

func (stub *studyMapV32ReviewProviderStub) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	if stub.failPlanID != "" && strings.Contains(prompt.User, stub.failPlanID) {
		return nil, errors.New("fixture request planning failure")
	}
	return json.Marshal(prompt)
}

func (stub *studyMapV32ReviewProviderStub) DiscoverSemanticsMeasured(
	_ context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	const marker = "Fixed bounded review bundle JSON:\n"
	markerIndex := strings.LastIndex(prompt.User, marker)
	if markerIndex < 0 {
		return modelresearch.ProviderResult{}, errors.New("fixture review bundle is absent")
	}
	bundle, err := studymap.DecodeReviewBundle([]byte(prompt.User[markerIndex+len(marker):]))
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	roles := []studymap.ReadingRole{
		studymap.ReadingRolePublicOrCLIEntry,
		studymap.ReadingRoleCoreOrchestration,
		studymap.ReadingRoleEffectOrIntegrationBoundary,
	}
	proposal := studymap.ReviewProposal{
		Version: studymap.ReviewProposalVersion, DirectionID: bundle.DirectionID,
	}
	for index, anchor := range bundle.Anchors {
		proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
			AnchorID:             anchor.AnchorID,
			Fit:                  studymap.AnchorFitDirect,
			SupportedObservation: "This fragment defines the selected function.",
			Role:                 roles[index%len(roles)],
			OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
		})
	}
	raw, err := json.Marshal(proposal)
	stub.mu.Lock()
	stub.calls = append(stub.calls, bundle.DirectionID)
	stub.mu.Unlock()
	return modelresearch.ProviderResult{
		Content: raw, Attempts: 1, InputTokens: 20, OutputTokens: 10,
	}, err
}

func TestReviewStudyMapDirectionsReviewsEveryCandidateAndRecordsPreparationFailures(
	t *testing.T,
) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	directions.Directions[0].AnchorIDs[2] = "fact-unknown"
	directions.Directions[0].ReadingAnchors[2].AnchorID = "fact-unknown"
	for index := range directions.Directions {
		directions.Directions[index].DirectionID = ""
	}
	var err error
	directions, err = studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	provider := &studyMapV32ReviewProviderStub{
		failPlanID: directions.Directions[1].DirectionID,
	}
	runDir := t.TempDir()
	reviews, summaries, stages, issues, err := reviewStudyMapDirections(
		context.Background(),
		runDir,
		bundle,
		directions,
		"fixture-bundle-sha",
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != studymap.MaxCandidates || len(stages) != studymap.MaxCandidates {
		t.Fatalf("reviewed attempts/stages = %d/%d, want %d", len(summaries), len(stages), studymap.MaxCandidates)
	}
	if len(reviews) != studymap.MaxCandidates-2 {
		t.Fatalf("accepted reviews = %d, want %d", len(reviews), studymap.MaxCandidates-2)
	}
	provider.mu.Lock()
	providerCalls := len(provider.calls)
	provider.mu.Unlock()
	if providerCalls != studymap.MaxCandidates-2 {
		t.Fatalf("provider calls = %d, want %d", providerCalls, studymap.MaxCandidates-2)
	}
	if !hasStudyMapReviewIssue(issues, directions.Directions[0].DirectionID, "review_bundle_build_failed") {
		t.Fatalf("bundle failure issues = %#v", issues)
	}
	if !hasStudyMapReviewIssue(issues, directions.Directions[1].DirectionID, "review_request_plan_failed") {
		t.Fatalf("plan failure issues = %#v", issues)
	}
	for _, summary := range summaries[:2] {
		if summary.ValidationState != "rejected" || summary.IssueCode == "" || summary.Metrics.ProviderCall {
			t.Fatalf("local rejection summary = %#v", summary)
		}
	}
	attemptFiles, err := filepath.Glob(filepath.Join(runDir, studyMapReviewAttemptsDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(attemptFiles) != studymap.MaxCandidates {
		t.Fatalf("saved attempt files = %d, want %d", len(attemptFiles), studymap.MaxCandidates)
	}
}

func TestNormalizedDirectionArtifactRoundTripsWithoutRewritingRawAttempt(t *testing.T) {
	t.Parallel()

	_, normalized := studyMapV32ReviewFixture(t)
	rawProposal := normalized
	rawProposal.Directions = append([]studymap.DirectionCandidate(nil), normalized.Directions...)
	for index := range rawProposal.Directions {
		rawProposal.Directions[index].DirectionID = ""
	}
	rejectedPosition := len(rawProposal.Directions) - 1
	rawProposal.Directions[rejectedPosition].AnchorIDs =
		rawProposal.Directions[rejectedPosition].AnchorIDs[:2]
	rawProposal.Directions[rejectedPosition].ReadingAnchors =
		rawProposal.Directions[rejectedPosition].ReadingAnchors[:2]
	raw, err := json.Marshal(rawProposal)
	if err != nil {
		t.Fatal(err)
	}
	decoded, diagnostics, err := studymap.DecodeDirectionProposalWithDiagnostics(raw)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Received != len(rawProposal.Directions) ||
		diagnostics.Accepted != len(rawProposal.Directions)-1 ||
		diagnostics.Rejected != 1 ||
		len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Position != rejectedPosition ||
		diagnostics.Issues[0].Code != "invalid_anchor_selection" {
		t.Fatalf("direction diagnostics = %#v", diagnostics)
	}
	runDir := t.TempDir()
	if err := writeNormalizedDirectionProposal(filepath.Join(runDir, studyMapDirectionsFile), decoded); err != nil {
		t.Fatal(err)
	}
	attempt := studyMapV32StageAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyCandidatesPromptVersion,
		ValidationState:      "accepted",
		DirectionDiagnostics: &diagnostics,
		Response:             append(json.RawMessage(nil), raw...),
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), attempt); err != nil {
		t.Fatal(err)
	}
	savedDirections, err := os.ReadFile(filepath.Join(runDir, studyMapDirectionsFile))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := studymap.DecodeNormalizedDirectionProposal(savedDirections)
	if err != nil {
		t.Fatalf("decode saved normalized artifact: %v", err)
	}
	if replayed.Directions[0].DirectionID == "" {
		t.Fatal("saved normalized artifact omitted local direction ID")
	}
	savedAttemptRaw, err := os.ReadFile(filepath.Join(runDir, studyMapDirectionsAttempt))
	if err != nil {
		t.Fatal(err)
	}
	var savedAttempt studyMapV32StageAttempt
	if err := json.Unmarshal(savedAttemptRaw, &savedAttempt); err != nil {
		t.Fatal(err)
	}
	var compactSaved bytes.Buffer
	if err := json.Compact(&compactSaved, savedAttempt.Response); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(compactSaved.Bytes(), raw) {
		t.Fatal("saved attempt response was replaced by the normalized projection")
	}
	var replayedRaw studymap.DirectionProposal
	if err := json.Unmarshal(savedAttempt.Response, &replayedRaw); err != nil {
		t.Fatal(err)
	}
	if replayedRaw.Directions[0].DirectionID != "" {
		t.Fatal("raw attempt gained a locally derived direction ID")
	}
	if savedAttempt.DirectionDiagnostics == nil ||
		!reflect.DeepEqual(*savedAttempt.DirectionDiagnostics, diagnostics) {
		t.Fatalf(
			"saved direction diagnostics = %#v, want %#v",
			savedAttempt.DirectionDiagnostics,
			diagnostics,
		)
	}
}

func TestClearStudyMapV32OutputsLeavesUnrelatedRunArtifacts(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	stale := []string{
		studymap.RecordFile,
		studymap.BundleFile,
		studymap.AttemptFile,
		studymap.StatusFile,
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	}
	for _, name := range stale {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	attemptDir := filepath.Join(runDir, studyMapReviewAttemptsDir)
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stale.json"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(runDir, "report.json")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clearStudyMapV32Outputs(runDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("stale output %q remains: %v", name, err)
		}
	}
	if _, err := os.Stat(attemptDir); !os.IsNotExist(err) {
		t.Fatalf("stale attempt directory remains: %v", err)
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated run artifact = %q, %v", data, err)
	}
}

func TestAggregateStudyMapMetricsReflectsStagesAndOutcome(t *testing.T) {
	t.Parallel()

	accepted := semanticDiscoveryStageMetrics{
		Status: "accepted", ProviderCall: true, LatencyMillis: 30, InputTokens: 10,
	}
	rejected := semanticDiscoveryStageMetrics{
		Status: "rejected", ProviderCall: false, LatencyMillis: 7, InputTokens: 2,
	}
	tests := []struct {
		name       string
		stages     []semanticDiscoveryStageMetrics
		outcomeErr error
		wantStatus string
	}{
		{name: "all accepted", stages: []semanticDiscoveryStageMetrics{accepted, accepted}, wantStatus: "accepted"},
		{name: "accepted calls but rejected outcome", stages: []semanticDiscoveryStageMetrics{accepted, accepted}, outcomeErr: errors.New("reducer rejected"), wantStatus: "rejected"},
		{name: "partial stage results", stages: []semanticDiscoveryStageMetrics{accepted, rejected}, wantStatus: "partial"},
		{name: "local outcome rejected", stages: []semanticDiscoveryStageMetrics{accepted, rejected}, outcomeErr: errors.New("reducer rejected"), wantStatus: "rejected"},
		{name: "not run", wantStatus: "not_run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			metrics := aggregateStudyMapMetrics(test.stages, test.outcomeErr)
			if metrics.Status != test.wantStatus {
				t.Fatalf("aggregate status = %q, want %q", metrics.Status, test.wantStatus)
			}
			wantLatency := int64(0)
			for _, stage := range test.stages {
				wantLatency += stage.LatencyMillis
			}
			if metrics.LatencyMillis != wantLatency {
				t.Fatalf("summed provider latency = %d, want %d", metrics.LatencyMillis, wantLatency)
			}
		})
	}
}

func studyMapV32ReviewFixture(t *testing.T) (studymap.Bundle, studymap.DirectionProposal) {
	t.Helper()

	area := studymap.Area{ID: "area-core", Name: "Core", Responsibility: "Central production code."}
	anchorSpecs := []struct {
		id     string
		path   string
		symbol string
		role   artifactrole.Role
	}{
		{id: "fact-entry", path: "entry.go", symbol: "enter", role: artifactrole.RolePublicAPI},
		{id: "fact-core", path: "core.go", symbol: "process", role: artifactrole.RoleProductionCore},
		{id: "fact-effect", path: "effect.go", symbol: "emit", role: artifactrole.RoleEffectBoundary},
	}
	bundle := studymap.Bundle{
		Version: studymap.BundleVersion, RepoName: "fixture", Areas: []studymap.Area{area},
	}
	for index, spec := range anchorSpecs {
		window, err := sourcewindowfacts.NewWindow(
			"window-"+spec.symbol,
			spec.path,
			10+index*10,
			[]string{
				"func " + spec.symbol + "() int {",
				"\tvalue := 1",
				"\treturn value",
				"}",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		function, err := sourcewindowfacts.ExtractGoFunction(window, spec.symbol)
		if err != nil {
			t.Fatal(err)
		}
		bundle.Anchors = append(bundle.Anchors, studymap.Anchor{
			ID: spec.id, Path: spec.path, Symbol: spec.symbol, Line: function.StartLine + 1,
			Role: spec.role, Statement: spec.symbol + " is a bounded source anchor.",
			AreaIDs: []string{area.ID}, Function: function,
		})
		bundle.AllowedPaths = append(bundle.AllowedPaths, spec.path)
	}
	questions := []string{
		"How does alpha processing work?",
		"How does bravo processing work?",
		"How does charlie processing work?",
		"How does delta processing work?",
		"How does echo processing work?",
		"How does foxtrot processing work?",
		"How does golf processing work?",
		"How does hotel processing work?",
		"How does india processing work?",
		"How does juliet processing work?",
		"How does kilo processing work?",
		"How does lima processing work?",
	}
	directions := studymap.DirectionProposal{Version: studymap.DirectionProposalVersion}
	anchorIDs := []string{"fact-entry", "fact-core", "fact-effect"}
	for _, question := range questions {
		directions.Directions = append(directions.Directions, studymap.DirectionCandidate{
			Question: question, WhyItMatters: "This locates a central repository responsibility.",
			LearningOutcome: "The reader can identify the relevant production code.",
			TargetJob:       studymap.JobFirstContact, LearningStage: studymap.StageCentralOperation,
			AnchorIDs: append([]string(nil), anchorIDs...), AreaIDs: []string{area.ID},
			ReadingAnchors: []studymap.ReadingAnchor{
				{AnchorID: anchorIDs[0], Label: "Start here", WhatToLookFor: "Inspect the public entry declaration."},
				{AnchorID: anchorIDs[1], Label: "Then inspect", WhatToLookFor: "Inspect the central bounded implementation."},
				{AnchorID: anchorIDs[2], Label: "Related implementation", WhatToLookFor: "Inspect the visible output boundary."},
			},
			SearchQueries: []string{fmt.Sprintf("%s source", strings.TrimSuffix(question, "?"))},
		})
	}
	normalized, err := studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, normalized
}

func hasStudyMapReviewIssue(issues []studymap.ReviewIssue, directionID, code string) bool {
	for _, issue := range issues {
		if issue.DirectionID == directionID && issue.Code == code {
			return true
		}
	}
	return false
}
