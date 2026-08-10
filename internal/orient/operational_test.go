package orient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestRunLLMBundleOnlyIncludesAllowlistedOperationalCandidate(t *testing.T) {
	t.Parallel()

	repo := operationalFixtureRepo(t)
	output, err := Run(context.Background(), Options{
		RepoPath:             repo,
		LLMBundleOnly:        true,
		MaxLLMEntrypoints:    10,
		MaxLLMModules:        10,
		MaxLLMFiles:          50,
		MaxLLMEdges:          50,
		MaxLLMSignals:        50,
		MaxLLMSignalsPerFile: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		AllowedPaths []string `json:"allowed_paths"`
		Go           struct {
			OrientationCandidates []gofacts.OrientationCandidate `json:"orientation_candidates"`
		} `json:"go"`
	}
	if err := json.Unmarshal(output, &bundle); err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]struct{}, len(bundle.AllowedPaths))
	for _, path := range bundle.AllowedPaths {
		allowed[path] = struct{}{}
	}
	var operational *gofacts.OrientationCandidate
	for index := range bundle.Go.OrientationCandidates {
		if bundle.Go.OrientationCandidates[index].Kind == "signal_flow" {
			operational = &bundle.Go.OrientationCandidates[index]
			break
		}
	}
	if operational == nil {
		t.Fatalf("orientation candidates = %#v, want signal_flow", bundle.Go.OrientationCandidates)
	}
	if len(operational.OpenFiles) == 0 {
		t.Fatal("operational candidate has no open files")
	}
	for _, path := range operational.OpenFiles {
		if _, ok := allowed[path]; !ok {
			t.Fatalf("operational path %q is not allowlisted", path)
		}
	}
}

func TestRunOfflineProducesOperationalHintWithoutProvider(t *testing.T) {
	t.Parallel()

	repo := operationalFixtureRepo(t)
	output, err := Run(context.Background(), Options{
		RepoPath:             repo,
		Offline:              true,
		OutputJSON:           true,
		FlowCount:            5,
		MaxLLMFiles:          50,
		MaxLLMSignals:        50,
		MaxLLMSignalsPerFile: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	var report combinedReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	for _, flow := range report.ExplainedFlows {
		if flow.FlowSeed.FlowType != flowexplain.FlowTypeOperational {
			continue
		}
		if !strings.HasSuffix(flow.FlowSeed.Name, " (offline hint)") {
			t.Fatalf("operational flow name = %q, want offline hint suffix", flow.FlowSeed.Name)
		}
		if flow.FlowSeed.ID != flowexplain.GenerateFlowID(strings.TrimSuffix(flow.FlowSeed.Name, " (offline hint)")) {
			t.Fatalf("operational flow id = %q, want display-suffix-independent identity", flow.FlowSeed.ID)
		}
		return
	}
	t.Fatalf("explained flows = %#v, want an operational offline hint", report.ExplainedFlows)
}

func TestOfflineOperationalCandidateIsQualifiedAndConfidenceCapped(t *testing.T) {
	t.Parallel()

	candidate := gofacts.OrientationCandidate{
		Name:      "Background loop",
		Kind:      "signal_flow",
		Priority:  5,
		OpenFiles: []string{"internal/worker/reaper.go"},
		Why:       "strong static source signals",
	}

	flow := offlineCandidateFlow(candidate)

	if flow.FlowType != flowexplain.FlowTypeOperational {
		t.Fatalf("flow type = %q, want operational", flow.FlowType)
	}
	if flow.Confidence != 0.3 {
		t.Fatalf("confidence = %.2f, want 0.30", flow.Confidence)
	}
	if flow.Name != "Background loop (offline hint)" {
		t.Fatalf("name = %q, want one offline hint suffix", flow.Name)
	}
	if flow.WhyInteresting != candidate.Why {
		t.Fatalf("why = %q, want candidate evidence summary", flow.WhyInteresting)
	}
}

func TestMergeOrientationCandidatesUsesNaturalPriorityOrder(t *testing.T) {
	t.Parallel()

	existing := []gofacts.OrientationCandidate{{Name: "request", Kind: "primary_binary", Priority: 5}}
	operational := []gofacts.OrientationCandidate{
		{Name: "lower", Kind: "signal_flow", Priority: 2},
		{Name: "higher", Kind: "signal_flow", Priority: 4},
	}

	merged := mergeOrientationCandidates(existing, operational)

	if len(merged) != 3 {
		t.Fatalf("merged candidates = %#v", merged)
	}
	if merged[0].Name != "request" || merged[1].Name != "higher" || merged[2].Name != "lower" {
		t.Fatalf("merged order = %q, %q, %q", merged[0].Name, merged[1].Name, merged[2].Name)
	}
}

func TestMergeOperationalCandidateFlowsKeepsLocallyGroundedCandidate(t *testing.T) {
	t.Parallel()

	providerFlow := flowexplain.CandidateFlow{
		Name:             "HTTP request handling",
		FlowType:         flowexplain.FlowTypeRequest,
		Trigger:          "GET /items",
		LikelyEntrypoint: "cmd/server/main.go",
		LikelyFiles:      []string{"cmd/server/main.go"},
		WhyInteresting:   "shows request dispatch",
		Evidence:         []string{"exact provider evidence"},
		Confidence:       0.8,
		CandidateBasis:   flowexplain.CandidateBasisModelOrientation,
	}
	report := orientationPart{CandidateFlows: []flowexplain.CandidateFlow{providerFlow}}
	candidates := []gofacts.OrientationCandidate{{
		Name:      "Background loop — periodic ticker created",
		Kind:      "signal_flow",
		OpenFiles: []string{"internal/worker/reaper.go", "internal/worker/queue.go"},
		Why:       "operational flow discovered from one strong signal",
		Priority:  2,
	}}
	signals := []sourcesignals.Signal{{
		Path:     "internal/worker/reaper.go",
		Line:     8,
		Category: "background_loop",
		Weight:   40,
		Reason:   "periodic ticker created",
	}}

	mergeOperationalCandidateFlows(&report, candidates, signals)

	if len(report.CandidateFlows) != 2 {
		t.Fatalf("candidate flows = %#v, want request plus operational", report.CandidateFlows)
	}
	if !reflect.DeepEqual(report.CandidateFlows[0], providerFlow) {
		t.Fatalf("provider flow changed:\n got: %#v\nwant: %#v", report.CandidateFlows[0], providerFlow)
	}
	operational := report.CandidateFlows[1]
	if operational.FlowType != flowexplain.FlowTypeOperational || operational.Confidence != 0.3 {
		t.Fatalf("operational flow = %#v", operational)
	}
	if operational.LikelyEntrypoint != "" ||
		!reflect.DeepEqual(operational.LikelyFiles, candidates[0].OpenFiles) {
		t.Fatalf("local operational entrypoint/files = %q / %#v", operational.LikelyEntrypoint, operational.LikelyFiles)
	}
	if operational.CandidateBasis != flowexplain.CandidateBasisSourceSignalAggregate {
		t.Fatalf("local operational candidate basis = %q", operational.CandidateBasis)
	}
	if len(operational.Evidence) != 1 ||
		operational.Evidence[0] != "internal/worker/reaper.go:8 source_signal periodic ticker created" {
		t.Fatalf("operational evidence = %v", operational.Evidence)
	}
	if err := validateResolvedOrientation(orientationPart{
		ProjectGuess: "worker service", Confidence: 0.3,
		CandidateFlows: []flowexplain.CandidateFlow{operational},
	}); err != nil {
		t.Fatalf("local signal aggregate did not validate: %v", err)
	}
	validSeeds, unverified := flowexplain.ValidateSeedFiles(
		operational.LikelyFiles,
		[]string{"internal/worker/reaper.go", "internal/worker/queue.go", "cmd/server/main.go"},
	)
	queryTerms, aliasTerms := flowexplain.ExtractTerms(
		operational.Name,
		operational.Trigger,
		operational.LikelyEntrypoint,
		operational.Evidence,
	)
	for _, terms := range [][]string{queryTerms, aliasTerms} {
		if slices.Contains(terms, "reaper") || slices.Contains(terms, "worker") || slices.Contains(terms, "internal") {
			t.Fatalf("blank entrypoint synthesized path query terms: %#v", terms)
		}
	}
	selected, _, _, _, _ := flowexplain.SelectFlowFiles(
		[]string{"cmd/server/main.go", "internal/worker/queue.go", "internal/worker/reaper.go"},
		aliasTerms,
		validSeeds,
		nil,
		2,
	)
	if len(unverified) != 0 || len(selected) != 2 ||
		selected[0].Path != "internal/worker/queue.go" || selected[1].Path != "internal/worker/reaper.go" {
		t.Fatalf("local bundle seed selection = valid %#v unverified %#v selected %#v", validSeeds, unverified, selected)
	}
	for _, file := range selected {
		if len(file.MatchedTerms) != 0 || !reflect.DeepEqual(file.Reasons, []string{"likely_file from candidate_flow"}) {
			t.Fatalf("local bundle seed gained query semantics: %#v", file)
		}
	}
	proven := candidates[0]
	proven.Name = "Scheduled maintenance"
	proven.EntrypointPackage = "example.com/project/cmd/server"
	provenReport := orientationPart{}
	mergeOperationalCandidateFlows(&provenReport, []gofacts.OrientationCandidate{proven}, signals)
	if len(provenReport.CandidateFlows) != 1 ||
		provenReport.CandidateFlows[0].LikelyEntrypoint != proven.EntrypointPackage {
		t.Fatalf("producer-owned entrypoint was not retained: %#v", provenReport.CandidateFlows)
	}
}

func TestMergeOperationalCandidateFlowsDoesNotRepairInvalidModelFlow(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "worker service",
		Confidence:   0.8,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:           "Background loop",
			FlowType:       flowexplain.FlowTypeRequest,
			Trigger:        "provider trigger",
			LikelyFiles:    []string{"internal/worker/reaper.go"},
			WhyInteresting: "provider interpretation",
			Evidence:       []string{"provider evidence"},
			Confidence:     0.8,
			CandidateBasis: flowexplain.CandidateBasisModelOrientation,
		}}}
	candidates := []gofacts.OrientationCandidate{{
		Name:      "Background loop",
		Kind:      "signal_flow",
		OpenFiles: []string{"internal/worker/reaper.go"},
	}}
	signals := []sourcesignals.Signal{{
		Path: "internal/worker/reaper.go", Line: 8,
		Category: "background_loop", Weight: 40, Reason: "periodic ticker created",
	}}

	mergeOperationalCandidateFlows(&report, candidates, signals)

	if report.CandidateFlows[0].LikelyEntrypoint != "" {
		t.Fatalf("invalid model likely_entrypoint was repaired: %#v", report.CandidateFlows[0])
	}
	if err := validateResolvedOrientation(report); err == nil ||
		!strings.Contains(err.Error(), "candidate_flows[0] has no likely_entrypoint") {
		t.Fatalf("invalid whole model output error = %v", err)
	}
}

func TestValidateResolvedOrientationAllowsBlankEntrypointOnlyForLocalSignalAggregate(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		basis    string
		flowType string
		wantErr  bool
	}{
		{name: "local signal aggregate", basis: flowexplain.CandidateBasisSourceSignalAggregate},
		{name: "signal aggregate request", basis: flowexplain.CandidateBasisSourceSignalAggregate, flowType: flowexplain.FlowTypeRequest, wantErr: true},
		{name: "model", basis: flowexplain.CandidateBasisModelOrientation, wantErr: true},
		{name: "local entrypoint", basis: flowexplain.CandidateBasisLocalEntrypoint, wantErr: true},
		{name: "runtime activity", basis: flowexplain.CandidateBasisRuntimeActivity, wantErr: true},
		{name: "unspecified", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			flowType := test.flowType
			if flowType == "" {
				flowType = flowexplain.FlowTypeOperational
			}
			report := orientationPart{
				ProjectGuess: "worker service",
				Confidence:   0.3,
				CandidateFlows: []flowexplain.CandidateFlow{{
					Name: "Periodic maintenance", FlowType: flowType,
					Trigger:     "local static source-signal threshold was met",
					LikelyFiles: []string{"internal/worker/reaper.go"},
					Evidence:    []string{"exact local source signal"}, Confidence: 0.3,
					CandidateBasis: test.basis,
				}},
			}
			err := validateResolvedOrientation(report)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "has no likely_entrypoint") {
					t.Fatalf("blank entrypoint error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("local signal aggregate error = %v", err)
			}
		})
	}
}

func TestMergeOperationalCandidateFlowsGroundsMatchingModelFlow(t *testing.T) {
	t.Parallel()

	report := orientationPart{CandidateFlows: []flowexplain.CandidateFlow{{
		Name:       "Background loop",
		FlowType:   flowexplain.FlowTypeRequest,
		Evidence:   []string{"model summary"},
		Confidence: 0.8,
	}}}
	candidates := []gofacts.OrientationCandidate{{
		Name:      "Background loop",
		Kind:      "signal_flow",
		OpenFiles: []string{"internal/worker/reaper.go"},
	}}
	signals := []sourcesignals.Signal{{
		Path:     "internal/worker/reaper.go",
		Line:     8,
		Category: "background_loop",
		Weight:   40,
		Reason:   "periodic ticker created",
	}}

	mergeOperationalCandidateFlows(&report, candidates, signals)

	if len(report.CandidateFlows) != 1 {
		t.Fatalf("candidate flows = %#v, want one merged flow", report.CandidateFlows)
	}
	flow := report.CandidateFlows[0]
	if flow.FlowType != flowexplain.FlowTypeOperational {
		t.Fatalf("flow type = %q, want operational", flow.FlowType)
	}
	if len(flow.Evidence) != 2 || !strings.Contains(flow.Evidence[1], "source_signal") {
		t.Fatalf("flow evidence = %v, want appended source signal", flow.Evidence)
	}
}

func operationalFixtureRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	writeOperationalFixture(t, filepath.Join(repo, "go.mod"), "module example.com/operational\n\ngo 1.24\n")
	writeOperationalFixture(t, filepath.Join(repo, "main.go"), "package main\nimport \"example.com/operational/internal/worker\"\nfunc main() { worker.Reap() }\n")
	workerDir := filepath.Join(repo, "internal", "worker")
	if err := os.MkdirAll(workerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeOperationalFixture(t, filepath.Join(workerDir, "reaper.go"), `package worker

import "time"

func Reap() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
	}
}
`)
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go", "internal/worker/reaper.go")
	return repo
}

func writeOperationalFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
