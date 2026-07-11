package quality

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

func TestEvaluateReportsIndependentQualitySlices(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.DirectionCoverage.Complete || !result.ImportantEvidence.Complete {
		t.Fatalf("orientation quality = %#v, %#v", result.DirectionCoverage, result.ImportantEvidence)
	}
	if !result.Passed {
		t.Fatalf("Passed = false for complete fixture: %#v", result)
	}
	if !result.Grounding.Valid || !result.SemanticDrilldown.Complete {
		t.Fatalf("evidence quality = %#v, %#v", result.Grounding, result.SemanticDrilldown)
	}
	if result.Grounding.ReferencedPathCount != 2 || result.Grounding.UnscoredProseEvidenceCount != 0 {
		t.Fatalf("grounding counts = %#v", result.Grounding)
	}
	if !result.SemanticDrilldown.TestTarget.Matched || !result.SemanticDrilldown.TargetsAgree {
		t.Fatalf("drilldown target agreement = %#v", result.SemanticDrilldown)
	}
	if !result.ForbiddenTripwires.Clear {
		t.Fatalf("forbidden tripwires = %#v", result.ForbiddenTripwires)
	}
	measuredContracts := []ContractCheck{
		result.ContractAdherence.OrientationContext,
		result.ContractAdherence.SourceBundle,
		result.ContractAdherence.SourceResponse,
		result.ContractAdherence.TestEvidence,
	}
	for index, check := range measuredContracts {
		if !check.Decoded || !check.Valid || !check.Measured || !check.Clean {
			t.Fatalf("contract[%d] = %#v", index, check)
		}
	}
	orientationContract := result.ContractAdherence.OrientationResponse
	if !orientationContract.Decoded || !orientationContract.Valid ||
		orientationContract.Measured || orientationContract.Clean ||
		!contains(orientationContract.WarningCodes, orientationContractUnmeasuredWarning) {
		t.Fatalf("orientation contract = %#v", orientationContract)
	}
	sourceEvaluation := result.ContractAdherence.SourceResponse.Evaluation
	if sourceEvaluation == nil ||
		sourceEvaluation.Version != sourceexplain.EvaluationVersion ||
		sourceEvaluation.Score != sourceEvaluation.MaxScore ||
		sourceEvaluation.MaxScore != 100 {
		t.Fatalf("source contract evaluation = %#v", sourceEvaluation)
	}
	if result.BytesAndLatency.Orientation.ReplayInputBytes != len(loaded.OrientationContext) ||
		result.BytesAndLatency.Orientation.ResponseBytes != len(loaded.OrientationResponse) ||
		result.BytesAndLatency.Orientation.ModelContextBytes != 4096 ||
		result.BytesAndLatency.Orientation.ProviderRequestBytes != nil ||
		result.BytesAndLatency.Source.ReplayInputBytes != len(loaded.SourceBundle) ||
		result.BytesAndLatency.Source.ResponseBytes != len(loaded.SourceResponse) ||
		result.BytesAndLatency.Source.ModelContextBytes != 3001 ||
		result.BytesAndLatency.Source.ProviderRequestBytes == nil ||
		*result.BytesAndLatency.Source.ProviderRequestBytes != 6601 ||
		result.BytesAndLatency.TestEvidenceBytes != len(loaded.TestEvidence) {
		t.Fatalf("bytes and latency = %#v", result.BytesAndLatency)
	}
	if result.BytesAndLatency.Orientation.LatencyMillis == nil ||
		*result.BytesAndLatency.Orientation.LatencyMillis != 125 {
		t.Fatalf("orientation latency = %#v", result.BytesAndLatency.Orientation.LatencyMillis)
	}
}

func TestEvaluateExposesMissingExpectedEvidenceSeparately(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*LoadedTask)
		check  func(*testing.T, Result)
	}{
		{
			name: "direction",
			mutate: func(loaded *LoadedTask) {
				loaded.Task.Expected.Directions = append(loaded.Task.Expected.Directions, DirectionExpectation{
					ID:             "lease-lifecycle",
					Aliases:        []string{"lease lifecycle"},
					ImportantPaths: []string{"server/lease.go"},
				})
			},
			check: func(t *testing.T, result Result) {
				t.Helper()
				if result.DirectionCoverage.Complete || !contains(result.DirectionCoverage.Missing, "lease-lifecycle") {
					t.Fatalf("direction coverage = %#v", result.DirectionCoverage)
				}
			},
		},
		{
			name: "important path",
			mutate: func(loaded *LoadedTask) {
				loaded.Task.Expected.Directions[0].ImportantPaths = append(
					loaded.Task.Expected.Directions[0].ImportantPaths,
					"server/missing.go",
				)
			},
			check: func(t *testing.T, result Result) {
				t.Helper()
				if result.ImportantEvidence.Complete ||
					!contains(result.ImportantEvidence.Missing, "write-request:server/missing.go") {
					t.Fatalf("important evidence = %#v", result.ImportantEvidence)
				}
			},
		},
		{
			name: "source predicate",
			mutate: func(loaded *LoadedTask) {
				loaded.Task.Expected.Drilldown.SourcePredicates = append(
					loaded.Task.Expected.Drilldown.SourcePredicates,
					sourceexplain.PredicateMapsError,
				)
			},
			check: func(t *testing.T, result Result) {
				t.Helper()
				if result.SemanticDrilldown.Complete ||
					!contains(result.SemanticDrilldown.MissingPredicates, string(sourceexplain.PredicateMapsError)) {
					t.Fatalf("semantic drilldown = %#v", result.SemanticDrilldown)
				}
			},
		},
		{
			name: "test reference",
			mutate: func(loaded *LoadedTask) {
				loaded.Task.Expected.Drilldown.TestReferencePaths = append(
					loaded.Task.Expected.Drilldown.TestReferencePaths,
					"server/missing_test.go",
				)
			},
			check: func(t *testing.T, result Result) {
				t.Helper()
				if result.SemanticDrilldown.Complete ||
					!contains(result.SemanticDrilldown.MissingTests, "server/missing_test.go") {
					t.Fatalf("semantic drilldown = %#v", result.SemanticDrilldown)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			loaded := evaluationFixture(t)
			test.mutate(&loaded)
			result, err := Evaluate(loaded)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			test.check(t, result)
			if result.Passed {
				t.Fatal("Passed = true with missing expected evidence")
			}
		})
	}
}

func TestEvaluateAcceptsLegacyCandidateOnlyOrientationContract(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	loaded.OrientationResponse = marshalFixture(t, struct {
		ProjectGuess   string                      `json:"project_guess"`
		Confidence     float64                     `json:"confidence"`
		CandidateFlows []flowexplain.CandidateFlow `json:"candidate_flows"`
	}{
		ProjectGuess:   response.ProjectGuess,
		Confidence:     response.Confidence,
		CandidateFlows: response.CandidateFlows,
	})

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	orientationContract := result.ContractAdherence.OrientationResponse
	if !orientationContract.Valid || orientationContract.Measured || orientationContract.Clean ||
		!contains(orientationContract.WarningCodes, orientationContractUnmeasuredWarning) || !result.Passed {
		t.Fatalf("legacy orientation result = %#v", result)
	}
}

func TestEvaluateToleratesUnknownOrientationFieldsLikeProductDecoder(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	var response map[string]any
	if err := json.Unmarshal(loaded.OrientationResponse, &response); err != nil {
		t.Fatal(err)
	}
	response["provider_extra"] = "ignored"
	candidates := response["candidate_flows"].([]any)
	candidates[0].(map[string]any)["candidate_extra"] = true
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	orientationContract := result.ContractAdherence.OrientationResponse
	if !orientationContract.Valid || orientationContract.Measured || orientationContract.Clean ||
		!contains(orientationContract.WarningCodes, orientationContractUnmeasuredWarning) || !result.Passed {
		t.Fatalf("orientation result = %#v", result)
	}
}

func TestEvaluateMeasuresRawOrientationContract(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	loaded.Task.Captures.Orientation.ResponseForm = ResponseFormProviderContent

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	contract := result.ContractAdherence.OrientationResponse
	if !contract.Decoded || !contract.Valid || !contract.Measured || !contract.Clean || !result.Passed {
		t.Fatalf("raw orientation result = %#v", result)
	}
}

func TestEvaluateFlagsRawOrientationContractDrift(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	loaded.Task.Captures.Orientation.ResponseForm = ResponseFormProviderContent
	var response map[string]any
	if err := json.Unmarshal(loaded.OrientationResponse, &response); err != nil {
		t.Fatal(err)
	}
	response["provider_extra"] = "ignored by the product parser"
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	contract := result.ContractAdherence.OrientationResponse
	if !contract.Decoded || !contract.Valid || !contract.Measured || contract.Clean || result.Passed ||
		!strings.Contains(contract.Error, "contract drift") {
		t.Fatalf("raw orientation result = %#v", result)
	}
}

func TestEvaluateFlagsIncompleteRawOrientationShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "missing field",
			mutate: func(response map[string]any) {
				delete(response, "warnings")
			},
			want: `required field "warnings"`,
		},
		{
			name: "null array",
			mutate: func(response map[string]any) {
				response["questions_for_human"] = nil
			},
			want: `required field "questions_for_human"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			loaded := evaluationFixture(t)
			loaded.Task.Captures.Orientation.ResponseForm = ResponseFormProviderContent
			var response map[string]any
			if err := json.Unmarshal(loaded.OrientationResponse, &response); err != nil {
				t.Fatal(err)
			}
			test.mutate(response)
			loaded.OrientationResponse = marshalFixture(t, response)

			result, err := Evaluate(loaded)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			contract := result.ContractAdherence.OrientationResponse
			if !contract.Valid || !contract.Measured || contract.Clean || result.Passed ||
				!strings.Contains(contract.Error, test.want) {
				t.Fatalf("raw orientation result = %#v", result)
			}
		})
	}
}

func TestEvaluateCountsUniqueStructuredPathsAndUnscoredProse(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	response.CandidateFlows[0].LikelyFiles = append(
		response.CandidateFlows[0].LikelyFiles,
		"server/handler.go",
	)
	response.CandidateFlows[0].Evidence = append(
		response.CandidateFlows[0].Evidence,
		"handler.go proves the request flow",
	)
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Grounding.ReferencedPathCount != 2 ||
		result.Grounding.UnscoredProseEvidenceCount != 1 ||
		!result.Grounding.Valid {
		t.Fatalf("grounding = %#v", result.Grounding)
	}
}

func TestEvaluateTreatsWildcardEvidenceAsUnscoredProse(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	response.HighLevelMap[0].Evidence = append(
		response.HighLevelMap[0].Evidence,
		"internal/runtime/*",
	)
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Grounding.Valid || result.Grounding.UnscoredProseEvidenceCount != 1 {
		t.Fatalf("grounding = %#v", result.Grounding)
	}
}

func TestEvaluateRejectsUngroundedOrientationPaths(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	response.CandidateFlows[0].LikelyFiles = append(
		response.CandidateFlows[0].LikelyFiles,
		"server/invented.go",
	)
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Grounding.Valid || len(result.Grounding.InvalidReferences) != 1 {
		t.Fatalf("grounding = %#v", result.Grounding)
	}
	invalid := result.Grounding.InvalidReferences[0]
	if invalid.Path != "server/invented.go" || !strings.Contains(invalid.Field, "likely_files") {
		t.Fatalf("invalid grounding reference = %#v", invalid)
	}
	if !result.ContractAdherence.OrientationResponse.Valid {
		t.Fatalf("orientation contract should be independent of grounding: %#v", result.ContractAdherence.OrientationResponse)
	}
	if result.Passed {
		t.Fatal("Passed = true with invalid grounding")
	}
}

func TestEvaluateTripsForbiddenPhraseAcrossCase(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	response.CandidateFlows[0].WhyInteresting = "This is PROVEN by runtime behavior."
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.ForbiddenTripwires.Clear || len(result.ForbiddenTripwires.Triggered) != 1 {
		t.Fatalf("forbidden tripwires = %#v", result.ForbiddenTripwires)
	}
	if result.ForbiddenTripwires.Triggered[0].ID != "runtime-proof" {
		t.Fatalf("triggered = %#v", result.ForbiddenTripwires.Triggered)
	}
	if result.Passed {
		t.Fatal("Passed = true with a forbidden overclaim")
	}
}

func TestEvaluateScansTestEvidenceForDrilldownTripwires(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	var tests testevidence.Bundle
	if err := json.Unmarshal(loaded.TestEvidence, &tests); err != nil {
		t.Fatal(err)
	}
	tests.Warnings = append(tests.Warnings, testevidence.Warning{
		Code:    "fixture.overclaim",
		Message: "These test references prove coverage and all tests pass.",
	})
	loaded.TestEvidence = marshalFixture(t, tests)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.ForbiddenTripwires.Clear ||
		!hasForbiddenMatch(result.ForbiddenTripwires.Triggered, "tests-pass") {
		t.Fatalf("forbidden tripwires = %#v", result.ForbiddenTripwires)
	}
	if !result.ContractAdherence.TestEvidence.Valid {
		t.Fatalf("test evidence contract = %#v", result.ContractAdherence.TestEvidence)
	}
}

func TestEvaluateRequiresSourceAndTestTargetAgreement(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	var tests testevidence.Bundle
	if err := json.Unmarshal(loaded.TestEvidence, &tests); err != nil {
		t.Fatal(err)
	}
	tests.TargetName = "server.Other"
	loaded.TestEvidence = marshalFixture(t, tests)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.SemanticDrilldown.TestTarget.Matched ||
		result.SemanticDrilldown.TargetsAgree ||
		result.SemanticDrilldown.Complete ||
		result.Passed {
		t.Fatalf("semantic drilldown = %#v", result.SemanticDrilldown)
	}
	if !result.ContractAdherence.TestEvidence.Valid {
		t.Fatalf("test evidence should remain internally valid: %#v", result.ContractAdherence.TestEvidence)
	}
}

func TestEvaluateReportsMalformedSourceWithoutDiscardingOtherSlices(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	loaded.SourceResponse = []byte(`{"assessments":`)
	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.ContractAdherence.SourceResponse.Decoded ||
		result.ContractAdherence.SourceResponse.Valid ||
		result.ContractAdherence.SourceResponse.Error == "" ||
		result.ContractAdherence.SourceResponse.Evaluation != nil {
		t.Fatalf("source response contract = %#v", result.ContractAdherence.SourceResponse)
	}
	if result.SemanticDrilldown.Complete ||
		!contains(result.SemanticDrilldown.MissingPredicates, string(sourceexplain.PredicateValidatesInput)) {
		t.Fatalf("semantic drilldown = %#v", result.SemanticDrilldown)
	}
	if !result.DirectionCoverage.Complete || !result.Grounding.Valid {
		t.Fatalf("unrelated orientation slices changed: %#v, %#v", result.DirectionCoverage, result.Grounding)
	}
	if result.Passed {
		t.Fatal("Passed = true with malformed source response")
	}
}

func TestEvaluateRejectsSourceBundleFromAnotherRepository(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	var bundle sourceexplain.Bundle
	if err := json.Unmarshal(loaded.SourceBundle, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.RepoName = "another-repo"
	loaded.SourceBundle = marshalFixture(t, bundle)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	check := result.ContractAdherence.SourceBundle
	if check.Valid || check.Clean || !strings.Contains(check.Error, "does not match task repository") {
		t.Fatalf("source bundle contract = %#v", check)
	}
	if result.SemanticDrilldown.Complete || result.Passed {
		t.Fatalf("cross-repository result = %#v", result)
	}
}

func TestEvaluateRequiresExpectedTestBuildContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testevidence.Bundle)
		check  func(PathCheck) bool
	}{
		{
			name: "goos mismatch",
			mutate: func(bundle *testevidence.Bundle) {
				bundle.Scenarios[0].Build.GOOS = "darwin"
			},
			check: func(path PathCheck) bool {
				return !path.ScenarioCompatible && path.GoplsVersionCompatible
			},
		},
		{
			name: "gopls version mismatch",
			mutate: func(bundle *testevidence.Bundle) {
				bundle.References[0].Provenance[0].Version = "gopls/v0.17.0"
			},
			check: func(path PathCheck) bool {
				return path.ScenarioCompatible && !path.GoplsVersionCompatible
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			loaded := evaluationFixture(t)
			var bundle testevidence.Bundle
			if err := json.Unmarshal(loaded.TestEvidence, &bundle); err != nil {
				t.Fatal(err)
			}
			test.mutate(&bundle)
			loaded.TestEvidence = marshalFixture(t, bundle)

			result, err := Evaluate(loaded)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if len(result.SemanticDrilldown.Tests) != 1 ||
				!test.check(result.SemanticDrilldown.Tests[0]) ||
				result.SemanticDrilldown.Tests[0].ContextCompatible ||
				!contains(result.SemanticDrilldown.IncompatibleTests, "server/handler_test.go") ||
				result.SemanticDrilldown.Complete || result.Passed {
				t.Fatalf("drilldown = %#v", result.SemanticDrilldown)
			}
		})
	}
}

func TestEvaluateMatchesDirectionAliasesOnlyInCandidateNames(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	response.CandidateFlows[0].Name = "Request handling"
	response.CandidateFlows[0].WhyInteresting = "This is the write request flow"
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.DirectionCoverage.Complete ||
		!contains(result.DirectionCoverage.Missing, "write-request") || result.Passed {
		t.Fatalf("direction coverage = %#v", result.DirectionCoverage)
	}
}

func TestEvaluateDoesNotUnionDirectionEvidenceAcrossCandidates(t *testing.T) {
	t.Parallel()

	loaded := evaluationFixture(t)
	response := decodeOrientationFixture(t, loaded.OrientationResponse)
	first := response.CandidateFlows[0]
	first.Name = "Write request entry"
	first.LikelyEntrypoint = "server/handler.go"
	first.LikelyFiles = []string{"server/handler.go"}
	first.Evidence = []string{"server/handler.go"}
	second := first
	second.Name = "Write request storage"
	second.LikelyEntrypoint = "server/store.go"
	second.LikelyFiles = []string{"server/store.go"}
	second.Evidence = []string{"server/store.go"}
	response.CandidateFlows = []flowexplain.CandidateFlow{first, second}
	loaded.OrientationResponse = marshalFixture(t, response)

	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.DirectionCoverage.Complete ||
		!contains(result.DirectionCoverage.Ambiguous, "write-request") ||
		result.ImportantEvidence.Complete || len(result.ImportantEvidence.Missing) != 1 ||
		result.Passed {
		t.Fatalf("direction evidence = coverage:%#v important:%#v", result.DirectionCoverage, result.ImportantEvidence)
	}
}

func evaluationFixture(t *testing.T) LoadedTask {
	t.Helper()

	orientationContext := marshalFixture(t, OrientationGroundingContext{
		Version:  OrientationGroundingContextVersion,
		RepoName: "repo",
		AllowedPaths: []string{
			"server/handler.go",
			"server/handler_test.go",
			"server/store.go",
		},
	})
	orientationResponse := marshalFixture(t, orientationResponse{
		ProjectGuess: "a request server",
		Confidence:   0.8,
		HighLevelMap: []orientationMapItem{{
			Name:         "server",
			Evidence:     []string{"server/handler.go"},
			WhyItMatters: "receives requests",
		}},
		FirstFilesToOpen: []orientationPath{{
			Path:   "server/handler.go",
			Reason: "request entrypoint",
		}},
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "Write request",
			Trigger:          "a client sends a write",
			LikelyEntrypoint: "server/handler.go",
			LikelyFiles:      []string{"server/handler.go", "server/store.go"},
			WhyInteresting:   "shows request validation and storage",
			Evidence:         []string{"server/handler.go", "server/store.go"},
			Confidence:       0.7,
		}},
		ImportantDomainWords: []orientationDomainWord{{
			Word:     "write",
			Guess:    "a mutating request",
			Evidence: []string{"server/store.go"},
		}},
		QuestionsForHuman: []string{},
		UnverifiedPaths:   []orientationPath{},
		Warnings:          []string{},
	})
	sourceBundle := sourceBundleFixture(t)
	sourceBundleJSON := marshalFixture(t, sourceBundle)
	sourceResponse := sourceResponseFixture(t, sourceBundle)
	testEvidence := marshalFixture(t, testEvidenceFixture())
	orientationLatency := int64(125)
	sourceRequestSHA256 := strings.Repeat("4", 64)
	sourceRequestBytes := 6601

	return LoadedTask{
		Task: Task{
			Version: TaskVersion,
			ID:      "synthetic-write-request",
			Repository: Repository{
				Name:     "repo",
				Revision: strings.Repeat("a", 40),
				Scenario: BuildScenario{
					Orientation:  "default",
					GOOS:         "linux",
					GOARCH:       "amd64",
					GoVersion:    "go1.22.0",
					GoplsVersion: "gopls/v0.18.0",
					BuildTags:    []string{},
				},
			},
			Goal: "understand the write request",
			Captures: Captures{
				Orientation: StageCapture{
					Provider:           "fixture",
					Model:              "fixture-model",
					PromptVersion:      "orientation-v1",
					ResponseForm:       ResponseFormNormalizedReport,
					CapturedAt:         "2026-07-10T10:00:00Z",
					ModelContextSHA256: strings.Repeat("b", 64),
					ModelContextBytes:  4096,
					LatencyMillis:      &orientationLatency,
				},
				Source: StageCapture{
					Provider:              "fixture",
					Model:                 "fixture-model",
					PromptVersion:         "source-v1",
					ResponseForm:          ResponseFormProviderContent,
					CapturedAt:            "2026-07-10T10:00:01Z",
					ModelContextSHA256:    strings.Repeat("c", 64),
					ModelContextBytes:     3001,
					ProviderRequestSHA256: &sourceRequestSHA256,
					ProviderRequestBytes:  &sourceRequestBytes,
				},
			},
			Artifacts: Artifacts{
				OrientationContext:  fixtureArtifact("orientation_context.json", "d"),
				OrientationResponse: fixtureArtifact("orientation_response.json", "e"),
				SourceBundle:        fixtureArtifact("source_bundle.json", "f"),
				SourceResponse:      fixtureArtifact("source_response.json", "1"),
				TestEvidence:        fixtureArtifact("test_evidence.json", "2"),
			},
			Expected: Expectations{
				Directions: []DirectionExpectation{{
					ID:             "write-request",
					Aliases:        []string{"write request", "mutating request"},
					ImportantPaths: []string{"server/handler.go", "server/store.go"},
				}},
				Drilldown: DrilldownExpectation{
					Symbol:             "server.Handle",
					Path:               "server/handler.go",
					SourcePredicates:   []sourceexplain.Predicate{sourceexplain.PredicateValidatesInput},
					TestReferencePaths: []string{"server/handler_test.go"},
				},
				ForbiddenOverclaims: []ForbiddenOverclaim{
					{
						ID:          "runtime-proof",
						Scope:       OverclaimScopeOrientation,
						ContainsAll: []string{"proven", "runtime"},
					},
					{
						ID:          "tests-pass",
						Scope:       OverclaimScopeDrilldown,
						ContainsAll: []string{"all tests", "pass"},
					},
				},
			},
		},
		ManifestPath:        "/fixture/task.json",
		OrientationContext:  orientationContext,
		OrientationResponse: orientationResponse,
		SourceBundle:        sourceBundleJSON,
		SourceResponse:      sourceResponse,
		TestEvidence:        testEvidence,
	}
}

func sourceBundleFixture(t *testing.T) sourceexplain.Bundle {
	t.Helper()

	target := evidence.Entity{
		ID:   "target",
		Kind: evidence.EntityMethod,
		Name: "server.Handle",
		Location: &evidence.Location{
			Path:   "server/handler.go",
			Line:   10,
			Column: 1,
		},
	}
	structural := symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "repo",
		Query:    "server.Handle",
		Target: symbol.Fact{
			EvidenceID: "resolution-001",
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
		},
		OutgoingCalls: []symbol.CallFact{{
			EvidenceID: "call-validation",
			Caller:     target,
			Callee: evidence.Entity{
				ID:       "callee-validation",
				Kind:     evidence.EntityFunction,
				Name:     "checkRequest",
				Location: &evidence.Location{Path: "server/validation.go", Line: 5, Column: 1},
			},
			Callsite:  &evidence.Location{Path: "server/handler.go", Line: 11, Column: 12},
			Certainty: evidence.CertaintyStatic,
		}},
		AllowedPaths: []string{"server/handler.go"},
		Warnings:     []string{},
		Truncated:    map[string]int{},
	}
	texts := []string{
		"func (s *server) Handle(r *Request) error {",
		"\tif err := checkRequest(r); err != nil {",
		"\t\treturn err",
		"\t}",
		"\treturn nil",
		"}",
	}
	lines := make([]sourcecard.Line, 0, len(texts))
	for index, text := range texts {
		line := 10 + index
		lines = append(lines, sourcecard.Line{
			EvidenceID: fmt.Sprintf("source-%d", line),
			Line:       line,
			Text:       text,
		})
	}
	card := sourcecard.Card{
		Version:    sourcecard.Version,
		Language:   "go",
		RepoName:   "repo",
		FileSHA256: strings.Repeat("3", 64),
		Target: sourcecard.Target{
			EvidenceID: "resolution-001",
			EntityID:   "target",
			Name:       "server.Handle",
			Kind:       evidence.EntityMethod,
			Path:       "server/handler.go",
			Line:       10,
			Column:     1,
		},
		Window: sourcecard.Window{
			StartLine:     10,
			EndLine:       15,
			IncludedBytes: sourceFixtureBytes(lines),
			StopReason:    sourcecard.StopNextTopLevelFunc,
		},
		Lines:    lines,
		Warnings: []sourcecard.Warning{},
	}
	bundle, err := sourceexplain.Build(structural, card)
	if err != nil {
		t.Fatalf("sourceexplain.Build() error = %v", err)
	}
	return bundle
}

func sourceResponseFixture(t *testing.T, bundle sourceexplain.Bundle) []byte {
	t.Helper()

	question := bundle.Questions[0]
	return marshalFixture(t, struct {
		Assessments []map[string]any `json:"assessments"`
		Unknowns    []map[string]any `json:"unknowns"`
		NextAction  string           `json:"next_action_id"`
	}{
		Assessments: []map[string]any{{
			"question_id":         question.ID,
			"verdict":             sourceexplain.VerdictShown,
			"source_evidence_ids": question.CandidateSourceEvidenceIDs,
		}},
		Unknowns: []map[string]any{
			{
				"kind":               sourceexplain.UnknownTestCoverage,
				"anchor_evidence_id": bundle.Target.EvidenceID,
			},
			{
				"kind":               sourceexplain.UnknownRuntimeReachability,
				"anchor_evidence_id": bundle.Target.EvidenceID,
			},
		},
		NextAction: bundle.AllowedActions[0].ID,
	})
}

func testEvidenceFixture() testevidence.Bundle {
	return testevidence.Bundle{
		Version:    testevidence.BundleVersion,
		TargetName: "server.Handle",
		Searches: []testevidence.Search{{
			AnchorEvidenceID:  "resolution-001",
			SymbolName:        "server.Handle",
			Location:          evidence.Location{Path: "server/handler.go", Line: 10, Column: 1},
			SourceEvidenceIDs: []string{},
		}},
		References: []testevidence.Reference{{
			EvidenceID:     "test-ref-001",
			SearchAnchorID: "resolution-001",
			Path:           "server/handler_test.go",
			Line:           20,
			Column:         2,
			Kind:           testevidence.EvidenceKindTestReference,
			Certainty:      evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider:  "gopls",
				Version:   "gopls/v0.18.0",
				Operation: "references",
			}},
			Scenarios: []string{"default"},
		}},
		Scenarios: []evidence.Scenario{{
			ID:   "default",
			Name: "default",
			Build: evidence.BuildContext{
				GOOS:      "linux",
				GOARCH:    "amd64",
				BuildTags: []string{},
			},
		}},
		Warnings: []testevidence.Warning{},
	}
}

func fixtureArtifact(path, digestCharacter string) ArtifactRef {
	return ArtifactRef{Path: path, SHA256: strings.Repeat(digestCharacter, 64)}
}

func decodeOrientationFixture(t *testing.T, data []byte) orientationResponse {
	t.Helper()
	var response orientationResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode orientation fixture: %v", err)
	}
	return response
}

func marshalFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}

func sourceFixtureBytes(lines []sourcecard.Line) int {
	total := 0
	for index, line := range lines {
		if index > 0 {
			total++
		}
		total += len(line.Text)
	}
	return total
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasForbiddenMatch(values []ForbiddenMatch, want string) bool {
	for _, value := range values {
		if value.ID == want {
			return true
		}
	}
	return false
}
