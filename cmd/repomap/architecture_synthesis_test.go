package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/workspacegraph"
)

type architectureSynthesisStub struct {
	calls    int
	response []byte
	// responseBytes can retain measured provider-envelope bytes when Content
	// is deliberately unavailable (for example response-byte overflow).
	responseBytes int
	err           error
	maxTokens     int
	endpoint      string
	prompts       []componentmap.SynthesisPrompt
	pending       *componentmap.SynthesisPrompt
	bodies        [][]byte
	finish        string
	onCall        func()
}

type architectureSynthesisWireResponse struct {
	Records []any `json:"records"`
}

type architectureSynthesisWireSubsystem struct {
	Kind        string `json:"kind"`
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type architectureSynthesisWireComponent struct {
	Kind         string                            `json:"kind"`
	SubsystemRef string                            `json:"subsystem_ref"`
	Name         string                            `json:"name"`
	Description  string                            `json:"description"`
	MemberRefs   []componentmap.SynthesisMemberRef `json:"member_refs"`
	AnchorRefs   []componentmap.SynthesisAnchorRef `json:"anchor_refs"`
	Hypothesis   bool                              `json:"hypothesis"`
}

func architectureSynthesisOutcomeFixture(
	validation componentmap.ValidationOutcome,
) architectureSynthesisOutcome {
	return architectureSynthesisOutcome{
		InputBytes: 1200, ResponseBytes: 500, ResponseContentBytes: 450,
		Attempted: true, TransportAttempts: 1,
		ProviderCallSucceeded: true, ResponseParsed: true,
		ValidationOutcome: validation, ArchitectureSource: componentmap.SourceValidatedModel,
		ArchitectureLevel: 2, UsageReported: true, InputTokens: 100, OutputTokens: 50,
		FinishReason: "stop", ResponseComplete: true, ResponseState: componentmap.ResponseCaptured,
		LocalCandidateCount: 2, RequestedConceptualCount: 2, AnchorCount: 1,
		MembershipCounted: true, MemberOccurrences: 2, DistinctMembers: 2,
		CoveredConceptualCount:     2,
		RequestedPrimaryScopeCount: 2,
		CoveredPrimaryScopeCount:   2,
		UncoveredPrimaryScopeCount: 0,
	}
}

func TestArchitectureSuccessConsoleExplainsPartialCoverage(t *testing.T) {
	t.Parallel()

	outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationAcceptedPartial)
	outcome.ArchitectureSource = componentmap.SourcePartialModel
	outcome.RequestedConceptualCount = 28
	outcome.CoveredConceptualCount = 10
	outcome.UncoveredConceptualCount = 18
	outcome.RequestedPrimaryScopeCount = 18
	outcome.CoveredPrimaryScopeCount = 6
	outcome.UncoveredPrimaryScopeCount = 12
	outcome.CoveredSupportingEvidenceCount = 4
	outcome.ValidationCodes = []string{"proposal.partial_member_coverage"}
	outcome.ResponseShape = &report.ArchitectureSynthesisResponseShape{
		JSONValid: true, Grammar: "nested", SubsystemCount: 3,
		ComponentCount: 4, MemberRefCount: 10, AnchorRefCount: 9,
	}
	outcome.SemanticExchangePath = "semantic_exchanges/" + strings.Repeat("a", 64) + "/exchange.v2.json"

	lines := strings.Join(architectureSuccessConsoleLines("/tmp/run", outcome), "\n")
	for _, want := range []string{
		"source: partial_model",
		"conceptual coverage: 10/28",
		"local unclassified remainder: 18",
		"primary scope coverage: 6/18 (uncovered=12)",
		"supporting evidence covered: 4",
		"validation diagnostics: proposal.partial_member_coverage",
		"response shape: grammar=nested subsystems=3 components=4 member_refs=10",
		"status: /tmp/run/architecture_synthesis_status.json",
		"exchange: /tmp/run/semantic_exchanges/" + strings.Repeat("a", 64) + "/exchange.v2.json",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("Architecture success log missing %q:\n%s", want, lines)
		}
	}
}

func TestArchitectureFailureConsoleExplainsPrimaryScopeQualityRejection(t *testing.T) {
	t.Parallel()

	outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationRejected)
	outcome.Failure = &report.ArchitectureSynthesisFailure{
		Stage: "response_validation", Code: "architecture.proposal_rejected",
	}
	outcome.ValidationCodes = []string{"proposal.empty_primary_scope_coverage"}
	outcome.RequestedPrimaryScopeCount = 18
	outcome.CoveredPrimaryScopeCount = 0
	outcome.UncoveredPrimaryScopeCount = 18
	outcome.CoveredSupportingEvidenceCount = 10

	lines := strings.Join(architectureFailureConsoleLines("/tmp/run", outcome), "\n")
	for _, want := range []string{
		"code: architecture.proposal_rejected",
		"validation diagnostics: proposal.empty_primary_scope_coverage",
		"primary scope coverage: 0/18 (uncovered=18)",
		"supporting evidence covered: 10",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("Architecture failure log missing %q:\n%s", want, lines)
		}
	}
}

func (stub *architectureSynthesisStub) ArchitectureProviderEndpointSHA256() string {
	endpoint := stub.endpoint
	if endpoint == "" {
		endpoint = "https://architecture.test/v1/chat/completions"
	}
	digest, _ := modelresearch.ProviderEndpointSHA256(endpoint)
	return digest
}

func (stub *architectureSynthesisStub) SynthesizeComponentLandscapeBodyMeasured(
	_ context.Context,
	body []byte,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	stub.bodies = append(stub.bodies, append([]byte(nil), body...))
	if stub.pending != nil {
		stub.prompts = append(stub.prompts, *stub.pending)
		stub.pending = nil
	}
	if stub.onCall != nil {
		stub.onCall()
	}
	result := modelresearch.ProviderResult{
		Content: append([]byte(nil), stub.response...), Attempts: 1,
		RequestBytes: len(body), ResponseBytes: len(stub.response),
	}
	if stub.responseBytes > 0 {
		result.ResponseBytes = stub.responseBytes
	}
	if stub.err == nil || stub.finish == "length" {
		result.UsageReported = true
		result.InputTokens = 101
		result.OutputTokens = 53
		result.FinishReason = stub.finish
		if result.FinishReason == "" {
			result.FinishReason = "stop"
		}
	}
	return result, stub.err
}

func TestArchitectureAcceptedRecordIsRemovedWhenResearchStateWriteFails(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	policy := modelresearch.DefaultPolicy()
	state := modelresearch.NewState(policy, modelresearch.RepositoryContext{
		Identity: runDir, Revision: "revision", Scenario: "go-default",
	})
	if err := modelresearch.WriteState(runDir, state); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(runDir, modelresearch.StateFile)
	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	provider.onCall = func() {
		if err := os.Remove(statePath); err != nil {
			t.Fatalf("remove research state: %v", err)
		}
		if err := os.Mkdir(statePath, 0o700); err != nil {
			t.Fatalf("replace research state with directory: %v", err)
		}
	}
	_, err := ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision",
		"openai-compatible/bearer", "fixture-model", provider,
	)
	if err == nil {
		t.Fatal("accepted synthesis unexpectedly survived research-state failure")
	}
	if _, statErr := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("accepted run synthesis remains after state failure: %v", statErr)
	}
}

func (stub *architectureSynthesisStub) ComponentSynthesisPromptJSON(prompt componentmap.SynthesisPrompt) ([]byte, error) {
	promptCopy := prompt
	stub.pending = &promptCopy
	maxTokens := stub.maxTokens
	if maxTokens == 0 {
		maxTokens = 64_000
	}
	return json.Marshal(struct {
		Prompt    componentmap.SynthesisPrompt `json:"prompt"`
		MaxTokens int                          `json:"max_tokens"`
	}{Prompt: prompt, MaxTokens: maxTokens})
}

func architectureSynthesisTestCacheKey(
	t *testing.T,
	bundle componentmap.CandidateBundle,
	revision,
	profile,
	model string,
	provider *architectureSynthesisStub,
) string {
	t.Helper()
	base, err := componentmap.SynthesisCacheKeyForProvider(
		revision,
		bundle,
		profile,
		model,
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	request, err := provider.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	key, err := architectureSynthesisExternalCacheKey(
		base,
		provider.ArchitectureProviderEndpointSHA256(),
		modelresearch.SHA256(request),
	)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEnsureArchitectureSynthesisCachesOneCallPerRevision(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	response := architectureSynthesisTestResponse(t, bundle)
	provider := &architectureSynthesisStub{response: response}
	runsDir := t.TempDir()
	firstRun := filepath.Join(runsDir, "run-one")
	secondRun := filepath.Join(runsDir, "run-two")
	for _, dir := range []string{firstRun, secondRun} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	first, err := ensureArchitectureSynthesis(
		context.Background(), bundle, firstRun, "revision-one",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || first.FallbackReason != "" || provider.calls != 1 {
		t.Fatalf("first outcome = %#v, calls = %d", first, provider.calls)
	}

	secondWriter, err := debugdump.OpenWriter(secondRun, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureArchitectureSynthesisWithOptions(
		context.Background(), bundle, secondRun, "revision-one",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			exchangeWriter:         secondWriter,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if closeErr := secondWriter.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || provider.calls != 1 {
		t.Fatalf("second outcome = %#v, calls = %d; want cache replay without another call", second, provider.calls)
	}
	saved, err := os.ReadFile(filepath.Join(secondRun, report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	landscape, err := componentmap.ReplaySynthesis(bundle, "revision-one", saved)
	if err != nil {
		t.Fatal(err)
	}
	if landscape.Fallback || landscape.Subsystems[0].Components[0].Name != "Runtime" {
		t.Fatalf("cached landscape = %#v", landscape)
	}
	records := readArchitectureSemanticExchangeRecords(t, secondRun)
	if len(records) != 1 || records[0].State != debugdump.SemanticStateCacheHit ||
		records[0].RequestProvenance != debugdump.SemanticRequestPrepared ||
		records[0].Outcome.Code != "cache_hit" {
		t.Fatalf("cached semantic exchange = %#v", records)
	}
	metrics := make(map[string]int, len(records[0].Outcome.Metrics))
	for _, metric := range records[0].Outcome.Metrics {
		metrics[metric.Name] = metric.Value
	}
	if metrics["requested_primary_scope_count"] != 1 ||
		metrics["covered_primary_scope_count"] != 1 ||
		metrics["uncovered_primary_scope_count"] != 0 {
		t.Fatalf("cached primary-scope metrics = %#v", metrics)
	}
}

func TestEnsureArchitectureSynthesisSendsAndJournalsOneExactPreparedBody(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := provider.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	provider.pending = nil

	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, runDir, "revision-exact-body",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, exchangeWriter: writer,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || len(provider.bodies) != 1 ||
		!bytes.Equal(provider.bodies[0], wantBody) || outcome.InputBytes != len(wantBody) {
		t.Fatalf("exact provider body/call outcome = calls %d bodies %d outcome %#v", provider.calls, len(provider.bodies), outcome)
	}
	savedRecord, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	var synthesisRecord componentmap.SynthesisRecord
	if err := json.Unmarshal(savedRecord, &synthesisRecord); err != nil {
		t.Fatal(err)
	}
	if synthesisRecord.ProviderRequestSHA256 != modelresearch.SHA256(wantBody) ||
		synthesisRecord.ProviderEndpointSHA256 != provider.ArchitectureProviderEndpointSHA256() {
		t.Fatalf(
			"provider identity = request %q endpoint %q, want exact sent body %q and endpoint %q",
			synthesisRecord.ProviderRequestSHA256,
			synthesisRecord.ProviderEndpointSHA256,
			modelresearch.SHA256(wantBody),
			provider.ArchitectureProviderEndpointSHA256(),
		)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, nil); err != nil {
		t.Fatal(err)
	}
	statusJSON, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusJSON, &status); err != nil {
		t.Fatal(err)
	}
	if status.Version != report.ArchitectureSynthesisStatusVersion || status.RequestBytes != len(wantBody) ||
		status.ResponseBytes != len(provider.response) ||
		status.ResponseContentBytes != len(provider.response) ||
		status.LocalCandidateCount != 1 || status.RequestedConceptualCount != 1 ||
		status.StructuralLocatorCount != 0 || status.AnchorCount != 0 ||
		!status.MembershipCounted || status.MemberOccurrences != 1 || status.DistinctMembers != 1 ||
		status.RequestedPrimaryScopeCount != 1 || status.CoveredPrimaryScopeCount != 1 ||
		status.UncoveredPrimaryScopeCount != 0 || status.CoveredSupportingEvidenceCount != 0 ||
		!status.UsageReported || status.InputTokens != 101 || status.OutputTokens != 53 ||
		status.FinishReason != "stop" || !status.ResponseComplete ||
		status.ResponseState != string(componentmap.ResponseCaptured) || status.TransportAttempts != 1 {
		t.Fatalf("closed Architecture parity status = %#v", status)
	}

	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil || len(directories) != 1 {
		t.Fatalf("semantic exchange directories = %v / %v", directories, err)
	}
	metadata, err := os.ReadFile(filepath.Join(
		runDir, debugdump.SemanticExchangesDir, directories[0].Name(),
		debugdump.SemanticExchangeMetaFile,
	))
	if err != nil {
		t.Fatal(err)
	}
	var exchange debugdump.SemanticExchangeRecord
	if err := json.Unmarshal(metadata, &exchange); err != nil {
		t.Fatal(err)
	}
	if exchange.RequestProvenance != debugdump.SemanticRequestExactSent ||
		exchange.SemanticCalls != 1 || exchange.TransportAttempts != 1 ||
		exchange.Outcome.Code != "accepted" {
		t.Fatalf("semantic exchange = %#v", exchange)
	}
	metrics := make(map[string]int, len(exchange.Outcome.Metrics))
	for _, metric := range exchange.Outcome.Metrics {
		metrics[metric.Name] = metric.Value
	}
	if metrics["requested_primary_scope_count"] != status.RequestedPrimaryScopeCount ||
		metrics["covered_primary_scope_count"] != status.CoveredPrimaryScopeCount ||
		metrics["uncovered_primary_scope_count"] != status.UncoveredPrimaryScopeCount ||
		metrics["covered_supporting_evidence_count"] != status.CoveredSupportingEvidenceCount {
		t.Fatalf("live Architecture status/exchange coverage mismatch: status=%#v metrics=%#v", status, metrics)
	}
	savedRequest, err := os.ReadFile(filepath.Join(
		runDir, debugdump.SemanticExchangesDir, directories[0].Name(),
		exchange.Request.File,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(savedRequest, wantBody) {
		t.Fatal("journaled Architecture request differs from the exact sent provider body")
	}
}

func TestArchitectureAllDroppedSupportingOnlySalvageKeepsStatusExchangeAndMetadataInParity(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	packageID := bundle.Candidates[0].ID
	symbol := bundle.Candidates[0]
	symbol.ID = componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "opaque-runtime-start"}
	symbol.Name = "Runtime.Start"
	symbol.ParentID = &packageID
	symbol.Facts = append([]componentmap.LocalFact(nil), symbol.Facts...)
	symbol.Facts[0].Value = "Start"
	bundle.Candidates = append(bundle.Candidates, symbol)

	request, _, err := componentmap.BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var supportingRef string
	for _, candidate := range request.Candidates {
		if candidate.CoverageRole == componentmap.SynthesisCoverageSupportingEvidence {
			supportingRef = candidate.Ref.Ref
		}
	}
	if supportingRef == "" {
		t.Fatal("fixture has no supporting-evidence ref")
	}
	response := mustArchitectureJSON(t, map[string]any{
		"subsystems": []any{map[string]any{
			"name": "Runtime evidence", "description": "Supporting symbol only",
			"components": []any{map[string]any{
				"name": "Start", "description": "Startup evidence",
				"member_refs": []string{supportingRef}, "anchor_refs": []string{},
			}},
		}},
	})
	provider := &architectureSynthesisStub{response: response}
	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{RunID: "primary-quality", Command: "test"}); err != nil {
		t.Fatal(err)
	}
	outcome, synthesisErr := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, runDir, "revision-primary-quality",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, exchangeWriter: writer,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(synthesisErr, errArchitectureSynthesisRejected) || outcome.Failure == nil ||
		outcome.Failure.Code != "architecture.proposal_rejected" ||
		!slices.Contains(outcome.ValidationCodes, "proposal.supporting_only_unit_coverage_salvaged") ||
		!slices.Contains(outcome.ValidationCodes, "proposal.invalid_subsystem_count") ||
		slices.Contains(outcome.ValidationCodes, "proposal.empty_primary_scope_coverage") ||
		slices.Contains(outcome.ValidationCodes, "proposal.supporting_only_unit_coverage") {
		t.Fatalf("all-dropped supporting-only salvage outcome/error = %#v / %v", outcome, synthesisErr)
	}
	if !outcome.MembershipCounted || outcome.MemberOccurrences != 0 || outcome.DistinctMembers != 0 ||
		outcome.CoveredConceptualCount != 0 || outcome.UncoveredConceptualCount != 2 ||
		len(outcome.UncoveredConceptualIDs) != 2 ||
		outcome.RequestedPrimaryScopeCount != 1 || outcome.CoveredPrimaryScopeCount != 0 ||
		outcome.UncoveredPrimaryScopeCount != 1 || outcome.CoveredSupportingEvidenceCount != 0 {
		t.Fatalf("all-dropped supporting-only salvage accounting = %#v", outcome)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, synthesisErr); err != nil {
		t.Fatal(err)
	}
	statusData, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("all-dropped salvage status: %v; status=%#v", err, status)
	}
	if !status.MembershipCounted || status.MemberOccurrences != 0 || status.DistinctMembers != 0 ||
		status.CoveredConceptualCount != 0 || status.UncoveredConceptualCount != 0 ||
		len(status.UncoveredConceptualIDs) != 0 || !status.ProposalRejected ||
		status.RequestedPrimaryScopeCount != 1 || status.CoveredPrimaryScopeCount != 0 ||
		status.UncoveredPrimaryScopeCount != 1 || status.CoveredSupportingEvidenceCount != 0 {
		t.Fatalf("all-dropped salvage status = %#v", status)
	}

	records := readArchitectureSemanticExchangeRecords(t, runDir)
	if len(records) != 1 || records[0].Outcome.Code != "architecture.proposal_rejected" {
		t.Fatalf("all-dropped salvage exchange = %#v", records)
	}
	exchangeMetrics := make(map[string]int, len(records[0].Outcome.Metrics))
	for _, metric := range records[0].Outcome.Metrics {
		exchangeMetrics[metric.Name] = metric.Value
	}
	if _, leaked := exchangeMetrics["covered_conceptual_count"]; leaked {
		t.Fatalf("failed exchange published accepted conceptual coverage: %#v", exchangeMetrics)
	}
	if exchangeMetrics["requested_primary_scope_count"] != status.RequestedPrimaryScopeCount ||
		exchangeMetrics["covered_primary_scope_count"] != status.CoveredPrimaryScopeCount ||
		exchangeMetrics["uncovered_primary_scope_count"] != status.UncoveredPrimaryScopeCount ||
		exchangeMetrics["covered_supporting_evidence_count"] != status.CoveredSupportingEvidenceCount {
		t.Fatalf("all-dropped salvage status/exchange mismatch: status=%#v metrics=%#v", status, exchangeMetrics)
	}

	diagnostic := architectureAtlasFirstDiagnostic(outcome, synthesisErr, false)
	if err := recordAtlasFirstStageDiagnostic(runDir, diagnostic); err != nil {
		t.Fatal(err)
	}
	metadata := readAtlasFirstMetadataFixture(t, runDir)
	var metadataOutcome *debugdump.SemanticOutcome
	for _, attempt := range metadata.RequestAttempts {
		if attempt.Stage == debugdump.SemanticStageArchitecture {
			metadataOutcome = attempt.Outcome
		}
	}
	if metadataOutcome == nil || metadataOutcome.Code != "architecture.proposal_rejected" {
		t.Fatalf("all-dropped salvage metadata = %#v", metadata)
	}
	metadataMetrics := make(map[string]int, len(metadataOutcome.Metrics))
	for _, metric := range metadataOutcome.Metrics {
		metadataMetrics[metric.Name] = metric.Value
	}
	for _, name := range []string{
		"requested_primary_scope_count", "covered_primary_scope_count",
		"uncovered_primary_scope_count", "covered_supporting_evidence_count",
	} {
		if metadataMetrics[name] != exchangeMetrics[name] {
			t.Fatalf("all-dropped salvage exchange/metadata metric %s = %d/%d", name, exchangeMetrics[name], metadataMetrics[name])
		}
	}
}

func TestArchitectureSynthesisDiagnosticCodesRetainsAllDistinctCodes(t *testing.T) {
	diagnostics := []componentmap.Diagnostic{
		{Code: "proposal.first"},
		{Code: "proposal.second"},
		{Code: "proposal.third"},
		{Code: "proposal.fourth"},
		{Code: "proposal.fifth"},
		{Code: "proposal.sixth"},
		{Code: "proposal.second"},
	}
	want := []string{
		"proposal.first",
		"proposal.second",
		"proposal.third",
		"proposal.fourth",
		"proposal.fifth",
		"proposal.sixth",
	}
	if got := architectureSynthesisDiagnosticCodes(diagnostics); !slices.Equal(got, want) {
		t.Fatalf("diagnostic codes = %v, want %v", got, want)
	}
}

func TestEnsureArchitectureSynthesisPersistsResolvedManyToManyMembershipEvidence(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	second := bundle.Candidates[0]
	second.ID = componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-storage"}
	second.Name = "local storage"
	second.Facts = append([]componentmap.LocalFact(nil), second.Facts...)
	second.Facts[0].Value = "storage package"
	bundle.Candidates = append(bundle.Candidates, second)

	request, _, err := componentmap.BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Candidates) != 2 {
		t.Fatalf("request candidates = %d", len(request.Candidates))
	}
	response, err := json.Marshal(architectureSynthesisWireResponse{
		Records: []any{
			architectureSynthesisWireSubsystem{
				Kind: "subsystem", Ref: "g1", Name: "Repository",
			},
			architectureSynthesisWireComponent{
				Kind: "component", SubsystemRef: "g1", Name: "Runtime",
				MemberRefs: []componentmap.SynthesisMemberRef{request.Candidates[0].Ref},
				AnchorRefs: []componentmap.SynthesisAnchorRef{},
			},
			architectureSynthesisWireComponent{
				Kind: "component", SubsystemRef: "g1", Name: "Storage",
				MemberRefs: []componentmap.SynthesisMemberRef{
					request.Candidates[0].Ref,
					request.Candidates[1].Ref,
				},
				AnchorRefs: []componentmap.SynthesisAnchorRef{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &architectureSynthesisStub{response: response}
	runDir := t.TempDir()
	outcome, err := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, runDir, "revision-conflict",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache:           true,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if err != nil {
		t.Fatalf("many-to-many synthesis error = %v", err)
	}
	if !outcome.MembershipCounted || outcome.MemberOccurrences != 3 || outcome.DistinctMembers != 2 ||
		len(outcome.ValidationCodes) != 0 {
		t.Fatalf("many-to-many synthesis evidence = %#v", outcome)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, err); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(encoded, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != report.ArchitectureSynthesisSucceeded || status.MemberOccurrences != 3 ||
		status.DistinctMembers != 2 || !status.ProposalAccepted || len(status.ValidationCodes) != 0 {
		t.Fatalf("persisted many-to-many synthesis status = %#v", status)
	}
}

func TestEnsureArchitectureSynthesisPersistsAndReplaysExactPartialCoverage(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	omitted := bundle.Candidates[0]
	omitted.ID = componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-storage"}
	omitted.Name = "local storage"
	omitted.Facts = append([]componentmap.LocalFact(nil), omitted.Facts...)
	omitted.Facts[0].Value = "storage package"
	bundle.Candidates = append(bundle.Candidates, omitted)
	request, _, err := componentmap.BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(architectureSynthesisWireResponse{Records: []any{
		architectureSynthesisWireSubsystem{Kind: "subsystem", Ref: "g1", Name: "Application"},
		architectureSynthesisWireComponent{
			Kind: "component", SubsystemRef: "g1", Name: "Runtime",
			MemberRefs: []componentmap.SynthesisMemberRef{request.Candidates[0].Ref},
			AnchorRefs: []componentmap.SynthesisAnchorRef{},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &architectureSynthesisStub{response: response}
	runsDir := t.TempDir()
	firstRun := filepath.Join(runsDir, "run-one")
	secondRun := filepath.Join(runsDir, "run-two")
	for _, runDir := range []string{firstRun, secondRun} {
		if err := os.Mkdir(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	options := architectureSynthesisOptions{
		providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
	}
	first, err := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, firstRun, "revision-partial",
		"openai-compatible/bearer", "test-model", provider, options,
	)
	if err != nil {
		t.Fatalf("partial synthesis: %v; outcome=%#v", err, first)
	}
	wantUncovered := []componentmap.MemberID{omitted.ID}
	if first.ValidationOutcome != componentmap.ValidationAcceptedPartial ||
		first.CoveredConceptualCount != 1 || first.UncoveredConceptualCount != 1 ||
		first.DistinctMembers != 1 || first.MemberOccurrences != 1 ||
		!slices.Equal(first.UncoveredConceptualIDs, wantUncovered) {
		t.Fatalf("partial outcome = %#v", first)
	}
	if err := persistArchitectureSynthesisStatus(firstRun, first, nil); err != nil {
		t.Fatal(err)
	}
	statusBytes, err := os.ReadFile(filepath.Join(firstRun, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusBytes, &status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("partial status: %v", err)
	}
	if !status.ProposalAccepted || !status.ProposalPartial || status.ProposalRejected ||
		status.CoveredConceptualCount != 1 || status.UncoveredConceptualCount != 1 ||
		!slices.Equal(status.UncoveredConceptualIDs, wantUncovered) {
		t.Fatalf("persisted partial status = %#v", status)
	}

	second, err := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, secondRun, "revision-partial",
		"openai-compatible/bearer", "test-model", provider, options,
	)
	if err != nil {
		t.Fatalf("replay partial synthesis: %v", err)
	}
	if provider.calls != 1 || !second.Cached ||
		second.ValidationOutcome != componentmap.ValidationAcceptedPartial ||
		second.CoveredConceptualCount != 1 || second.UncoveredConceptualCount != 1 ||
		!slices.Equal(second.UncoveredConceptualIDs, wantUncovered) {
		t.Fatalf("cached partial outcome calls=%d outcome=%#v", provider.calls, second)
	}
}

func TestEnsureArchitectureSynthesisRejectsIncompleteParityEvidenceBeforeWrites(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	validResponse := architectureSynthesisTestResponse(t, bundle)
	for _, test := range []struct {
		name         string
		response     []byte
		finishReason string
		wantCode     string
	}{
		{
			name:     "provider did not report complete response",
			response: validResponse, finishReason: "content_filter",
			wantCode: "response.incomplete",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runsDir := t.TempDir()
			runDir := filepath.Join(runsDir, "run")
			if err := os.Mkdir(runDir, 0o700); err != nil {
				t.Fatal(err)
			}
			provider := &architectureSynthesisStub{
				response: test.response,
				finish:   test.finishReason,
			}
			outcome, err := ensureArchitectureSynthesisWithOptions(
				t.Context(), bundle, runDir, "revision-incomplete-evidence",
				"openai-compatible/bearer", "test-model", provider,
				architectureSynthesisOptions{
					providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
				},
			)
			if !errors.Is(err, errArchitectureSynthesisRejected) {
				t.Fatalf("incomplete evidence error = %v", err)
			}
			if outcome.ValidationOutcome != componentmap.ValidationRejected ||
				len(outcome.ValidationCodes) == 0 || outcome.ValidationCodes[0] != test.wantCode {
				t.Fatalf("incomplete parity evidence outcome = %#v", outcome)
			}
			if _, statErr := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("incomplete parity evidence wrote accepted run record: %v", statErr)
			}
			cacheFiles, globErr := filepath.Glob(filepath.Join(
				runsDir,
				architectureSynthesisCacheDirectory,
				"*.json",
			))
			if globErr != nil {
				t.Fatal(globErr)
			}
			if len(cacheFiles) != 0 {
				t.Fatalf("incomplete parity evidence wrote accepted cache: %v", cacheFiles)
			}
			if err := persistArchitectureSynthesisStatus(runDir, outcome, err); err != nil {
				t.Fatalf("persist closed incomplete-evidence status: %v", err)
			}
		})
	}
}

func TestEnsureArchitectureSynthesisCacheMissesAcrossConfiguredMaxTokens(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{
		response:  architectureSynthesisTestResponse(t, bundle),
		maxTokens: 8_000,
	}
	runsDir := t.TempDir()
	run := func(name string) architectureSynthesisOutcome {
		t.Helper()
		runDir := filepath.Join(runsDir, name)
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		outcome, err := ensureArchitectureSynthesis(
			context.Background(), bundle, runDir, "revision-max-tokens",
			"openai-compatible/bearer", "test-model", provider,
		)
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}

	if first := run("same-run"); first.Cached {
		t.Fatalf("first outcome = %#v", first)
	}
	provider.maxTokens = 16_000
	if changed := run("same-run"); changed.Cached {
		t.Fatalf("changed-max outcome = %#v, want cache miss", changed)
	}
	if warm := run("warm"); !warm.Cached {
		t.Fatalf("warm outcome = %#v, want exact-request cache hit", warm)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one call per exact max_tokens request", provider.calls)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(runsDir, architectureSynthesisCacheDirectory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 2 {
		t.Fatalf("exact max_tokens cache variants = %v, want two coexisting records", cacheFiles)
	}
}

func TestEnsureArchitectureSynthesisCacheMissesAcrossProviderEndpoints(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	response := architectureSynthesisTestResponse(t, bundle)
	providerA := &architectureSynthesisStub{response: response, endpoint: "https://provider-a.test/v1/chat/completions"}
	providerB := &architectureSynthesisStub{response: response, endpoint: "https://provider-b.test/v1/chat/completions"}
	runsDir := t.TempDir()
	run := func(name string, provider *architectureSynthesisStub) architectureSynthesisOutcome {
		t.Helper()
		runDir := filepath.Join(runsDir, name)
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		outcome, err := ensureArchitectureSynthesis(
			context.Background(), bundle, runDir, "revision-endpoint",
			"openai-compatible/bearer", "test-model", provider,
		)
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}

	if first := run("a-cold", providerA); first.Cached {
		t.Fatalf("provider A cold outcome = %#v", first)
	}
	if warm := run("a-warm", providerA); !warm.Cached {
		t.Fatalf("provider A warm outcome = %#v", warm)
	}
	if first := run("b-cold", providerB); first.Cached {
		t.Fatalf("provider B reused provider A response: %#v", first)
	}
	if warm := run("b-warm", providerB); !warm.Cached {
		t.Fatalf("provider B warm outcome = %#v", warm)
	}
	if providerA.calls != 1 || providerB.calls != 1 {
		t.Fatalf("provider calls A/B = %d/%d, want one cold call each", providerA.calls, providerB.calls)
	}
}

func TestEnsureArchitectureSynthesisRefetchesCacheWithTamperedProviderIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		tamper func(*componentmap.SynthesisRecord)
	}{
		{
			name: "exact provider request body",
			tamper: func(record *componentmap.SynthesisRecord) {
				record.ProviderRequestSHA256 = modelresearch.SHA256([]byte("different provider request"))
			},
		},
		{
			name: "provider endpoint identity",
			tamper: func(record *componentmap.SynthesisRecord) {
				record.ProviderEndpointSHA256 = modelresearch.SHA256([]byte("different provider endpoint"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			bundle := architectureSynthesisTestBundle()
			provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
			runsDir := t.TempDir()
			coldRun := filepath.Join(runsDir, "cold")
			warmRun := filepath.Join(runsDir, "warm")
			for _, runDir := range []string{coldRun, warmRun} {
				if err := os.Mkdir(runDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			cold, err := ensureArchitectureSynthesis(
				context.Background(), bundle, coldRun, "revision-provider-identity",
				"openai-compatible/bearer", "test-model", provider,
			)
			if err != nil || cold.Cached || provider.calls != 1 {
				t.Fatalf("cold outcome/calls = %#v / %d / %v", cold, provider.calls, err)
			}
			cacheKey := architectureSynthesisTestCacheKey(
				t, bundle, "revision-provider-identity",
				"openai-compatible/bearer", "test-model", provider,
			)
			cachePath := filepath.Join(runsDir, architectureSynthesisCacheDirectory, cacheKey+".json")
			saved, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			var record componentmap.SynthesisRecord
			if err := json.Unmarshal(saved, &record); err != nil {
				t.Fatal(err)
			}
			test.tamper(&record)
			tampered, err := json.MarshalIndent(record, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			tampered = append(tampered, '\n')
			if err := os.WriteFile(cachePath, tampered, 0o600); err != nil {
				t.Fatal(err)
			}

			refetched, err := ensureArchitectureSynthesis(
				context.Background(), bundle, warmRun, "revision-provider-identity",
				"openai-compatible/bearer", "test-model", provider,
			)
			if err != nil {
				t.Fatal(err)
			}
			if refetched.Cached || provider.calls != 2 {
				t.Fatalf("tampered cache outcome/calls = %#v / %d, want one exact refetch", refetched, provider.calls)
			}

			replacement, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := componentmap.BuildSynthesisPrompt(bundle)
			if err != nil {
				t.Fatal(err)
			}
			requestBody, err := provider.ComponentSynthesisPromptJSON(prompt)
			if err != nil {
				t.Fatal(err)
			}
			_, err = componentmap.ReplaySynthesisResultForProvider(
				bundle,
				"revision-provider-identity",
				componentmap.SynthesisProviderIdentity{
					RequestSHA256:  modelresearch.SHA256(requestBody),
					EndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
				},
				replacement,
			)
			if err != nil {
				t.Fatalf("replacement cache does not bind the exact provider exchange: %v", err)
			}
		})
	}
}

func TestEnsureArchitectureSynthesisCachesByRequestedOutputLanguage(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	runsDir := t.TempDir()
	run := func(name, language string) architectureSynthesisOutcome {
		t.Helper()
		runDir := filepath.Join(runsDir, name)
		if err := os.Mkdir(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		outcome, err := ensureArchitectureSynthesisWithOptions(
			context.Background(),
			bundle,
			runDir,
			"revision-language",
			"openai-compatible/bearer",
			"test-model",
			provider,
			architectureSynthesisOptions{
				providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
				outputLanguage:         language,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}

	if outcome := run("english-cold", "en"); outcome.Cached {
		t.Fatalf("first canonical outcome = %#v, want provider call", outcome)
	}
	if outcome := run("english-warm", "en"); !outcome.Cached {
		t.Fatalf("English presentation source = %#v, want canonical cache replay", outcome)
	}
	if outcome := run("russian-cold", "ru"); outcome.Cached {
		t.Fatalf("Russian presentation source = %#v, want language-isolated provider call", outcome)
	}
	if outcome := run("russian-warm", "ru"); !outcome.Cached {
		t.Fatalf("Russian presentation source = %#v, want Russian cache replay", outcome)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one English and one Russian call", provider.calls)
	}
	if len(provider.prompts) != 2 ||
		strings.Contains(provider.prompts[0].System, "prose in Russian") ||
		!strings.Contains(provider.prompts[1].System, "name and description prose in Russian") {
		t.Fatalf("provider prompts = %#v", provider.prompts)
	}

	saved, err := os.ReadFile(filepath.Join(runsDir, "russian-warm", report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(saved, &record); err != nil {
		t.Fatal(err)
	}
	if record.Call == nil || record.Call.Metadata.OutputLanguage != "ru" {
		t.Fatalf("Russian cached record metadata = %#v", record.Call)
	}
}

func TestEnsureArchitectureSynthesisNoCacheCallsProviderPerRun(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	runsDir := t.TempDir()
	firstRun := filepath.Join(runsDir, "run-one")
	secondRun := filepath.Join(runsDir, "run-two")
	for _, dir := range []string{firstRun, secondRun} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		outcome, err := ensureArchitectureSynthesisWithOptions(
			context.Background(), bundle, dir, "revision-one",
			"openai-compatible/bearer", "test-model", provider,
			architectureSynthesisOptions{disableCache: true, providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256()},
		)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Cached {
			t.Fatalf("no-cache outcome = %#v", outcome)
		}
		if _, err := os.Stat(filepath.Join(dir, report.ArchitectureSynthesisFile)); err != nil {
			t.Fatalf("per-run architecture artifact: %v", err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one call per run", provider.calls)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(runsDir, architectureSynthesisCacheDirectory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 0 {
		t.Fatalf("no-cache populated shared architecture cache: %v", cacheFiles)
	}
}

func TestEnsureArchitectureSynthesisRejectsInvalidOutputWithoutPublishingOrCachingIt(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: []byte("not json")}
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-one")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	staleRunPath := filepath.Join(runDir, report.ArchitectureSynthesisFile)
	if err := os.WriteFile(staleRunPath, []byte("stale accepted record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision-invalid",
		"openai-compatible/bearer", "test-model", provider,
	)
	if !errors.Is(err, errArchitectureSynthesisRejected) {
		t.Fatalf("ensureArchitectureSynthesis() error = %v, want closed rejection", err)
	}
	if outcome.FallbackReason != componentmap.FallbackRejectedMalformed || provider.calls != 1 {
		t.Fatalf("outcome = %#v, calls = %d", outcome, provider.calls)
	}
	if _, statErr := os.Stat(staleRunPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected architecture artifact exists: %v", statErr)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, err); err != nil {
		t.Fatal(err)
	}
	statusJSON, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusJSON, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != report.ArchitectureSynthesisFailed || status.ErrorCode != "invalid_response" ||
		!status.ProposalRejected || status.FallbackSelected || status.FallbackReason != "" ||
		strings.Join(status.ValidationCodes, ",") != "response.no_json" {
		t.Fatalf("closed rejection status = %#v", status)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(runsDir, architectureSynthesisCacheDirectory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 0 {
		t.Fatalf("rejected local fallback entered shared cache: %v", cacheFiles)
	}

	cacheKey := architectureSynthesisTestCacheKey(
		t, bundle, "revision-invalid", "openai-compatible/bearer", "test-model", provider,
	)
	cacheDir := filepath.Join(runsDir, architectureSynthesisCacheDirectory)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fallback, err := componentmap.RecordSynthesisResponse(
		bundle,
		"revision-invalid",
		"openai-compatible/bearer",
		"test-model",
		0,
		[]byte("not json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	savedFallback, err := json.Marshal(fallback.Record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), savedFallback, 0o600); err != nil {
		t.Fatal(err)
	}
	provider.response = architectureSynthesisTestResponse(t, bundle)
	secondRun := filepath.Join(runsDir, "run-two")
	if err := os.Mkdir(secondRun, 0o700); err != nil {
		t.Fatal(err)
	}
	refetched, err := ensureArchitectureSynthesis(
		context.Background(), bundle, secondRun, "revision-invalid",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if refetched.Cached || refetched.FallbackSelected || provider.calls != 2 {
		t.Fatalf("rejected shared record was reused: outcome=%#v calls=%d", refetched, provider.calls)
	}
	thirdRun := filepath.Join(runsDir, "run-three")
	if err := os.Mkdir(thirdRun, 0o700); err != nil {
		t.Fatal(err)
	}
	warm, err := ensureArchitectureSynthesis(
		context.Background(), bundle, thirdRun, "revision-invalid",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !warm.Cached || provider.calls != 2 {
		t.Fatalf("accepted replacement was not reusable: outcome=%#v calls=%d", warm, provider.calls)
	}
}

func TestEnsureArchitectureSynthesisResourceLimitPublishesOnlyFailedStatus(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider func() *architectureSynthesisStub
	}{
		{
			name: "provider resource error",
			provider: func() *architectureSynthesisStub {
				return &architectureSynthesisStub{
					response: []byte(`{"kind":"subsystem","ref":"g1"`),
					finish:   "length",
					err: &modelresearch.ResourceLimitError{
						Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitOutputTokens,
						Limit: 64_000, ConfiguredMaxTokens: 64_000, FinishReason: "length",
						Observed: 64_000, ObservedKnown: true, OutputTokens: 64_000,
					},
				}
			},
		},
		{
			name: "response decoder resource error",
			provider: func() *architectureSynthesisStub {
				return &architectureSynthesisStub{
					response: bytes.Repeat(
						[]byte("x"),
						modelresearch.ProviderResponseByteLimit+1,
					),
				}
			},
		},
		{
			name: "provider response-byte resource error without retained content",
			provider: func() *architectureSynthesisStub {
				return &architectureSynthesisStub{
					responseBytes: modelresearch.ProviderResponseByteLimit + 1,
					err: &modelresearch.ResourceLimitError{
						Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitResponseBytes,
						Limit:         modelresearch.ProviderResponseByteLimit,
						Observed:      modelresearch.ProviderResponseByteLimit + 1,
						ObservedKnown: true, ObservedAtLeast: true,
					},
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runsDir := t.TempDir()
			runDir := filepath.Join(runsDir, "run")
			if err := os.Mkdir(runDir, 0o700); err != nil {
				t.Fatal(err)
			}
			state := modelresearch.NewState(
				modelresearch.DefaultPolicy(),
				modelresearch.RepositoryContext{
					Identity: "fixture", Revision: "revision-resource", Scenario: "go-default",
				},
			)
			if err := modelresearch.WriteState(runDir, state); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(runDir, modelresearch.StateFile)
			beforeState, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}

			provider := test.provider()
			outcome, synthesisErr := ensureArchitectureSynthesis(
				context.Background(), architectureSynthesisTestBundle(), runDir,
				"revision-resource", "openai-compatible/bearer", "test-model", provider,
			)
			var limitErr *modelresearch.ResourceLimitError
			if !errors.As(synthesisErr, &limitErr) || provider.calls != 1 {
				t.Fatalf("synthesis error/provider calls = %#v/%d", synthesisErr, provider.calls)
			}
			if !isArchitectureOutputResourceExhausted(synthesisErr) {
				t.Fatalf("attempted output exhaustion was not typed as publishable: %#v", synthesisErr)
			}
			if err := persistArchitectureSynthesisStatus(runDir, outcome, synthesisErr); err != nil {
				t.Fatal(err)
			}
			afterState, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(beforeState, afterState) {
				t.Fatal("attempted Architecture resource exhaustion did not record model research metrics")
			}
			var recorded modelresearch.State
			if err := json.Unmarshal(afterState, &recorded); err != nil {
				t.Fatal(err)
			}
			if recorded.Architecture.Status != "resource_limited" ||
				recorded.Architecture.SemanticCalls != 1 ||
				recorded.Usage.SemanticCalls != 1 {
				t.Fatalf("resource-limited accounting = %#v", recorded.Architecture)
			}
			statusData, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
			if err != nil {
				t.Fatalf("failed Architecture status was not persisted: %v", err)
			}
			var status report.ArchitectureSynthesisStatus
			if err := json.Unmarshal(statusData, &status); err != nil {
				t.Fatal(err)
			}
			if status.State != report.ArchitectureSynthesisFailed ||
				status.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit ||
				status.ProviderRequestCount != 1 || status.ConfiguredMaxTokens <= 0 {
				t.Fatalf("failed status = %#v", status)
			}
			if test.name == "provider resource error" {
				if status.FinishReason != "length" || status.ResponseComplete ||
					status.ObservedOutputTokens <= 0 || !status.UsageReported {
					t.Fatalf("length-ended failed status = %#v", status)
				}
			} else if status.ResponseBytes == 0 {
				t.Fatalf("response-byte failed status = %#v", status)
			}
			if status.Failure == nil || status.Failure.Code != "architecture.provider_output_limit" {
				t.Fatalf("resource-limit failure diagnostic = %#v", status.Failure)
			}
			if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("resource exhaustion published a synthesis record: %v", err)
			}
			cacheFiles, err := filepath.Glob(filepath.Join(
				runsDir,
				architectureSynthesisCacheDirectory,
				"*.json",
			))
			if err != nil {
				t.Fatal(err)
			}
			if len(cacheFiles) != 0 {
				t.Fatalf("resource exhaustion populated cache: %v", cacheFiles)
			}
		})
	}
}

func TestArchitectureOutputExhaustionJournalsOneRedactedExchange(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{
		response: []byte(`{"records":[{"kind":"subsystem","ref":"g1","name":"Fixture core","description":"Fixture grouping"},{"kind":"component","ref":"c1","subsystem_ref":"g1","name":"Fixture component","description":"Fixture grouping","member_refs":[{"kind":"package","ref":"p1"},{"kind":"package","ref":"p2"},{"kind":"package","ref":"p3"},{"kind":"package","ref":"p1"}`),
		finish:   "length",
		err: &modelresearch.ResourceLimitError{
			Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitOutputTokens,
			Limit: 64_000, ConfiguredMaxTokens: 64_000, FinishReason: "length",
			Observed: 64_000, ObservedKnown: true, OutputTokens: 64_000,
		},
	}
	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wantBody, err := provider.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	outcome, synthesisErr := ensureArchitectureSynthesisWithOptions(
		t.Context(), bundle, runDir, "revision-exchange-length",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, exchangeWriter: writer,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !isArchitectureOutputResourceExhausted(synthesisErr) {
		t.Fatalf("synthesis error = %#v, want typed output exhaustion", synthesisErr)
	}
	if provider.calls != 1 || len(provider.bodies) != 1 ||
		!bytes.Equal(provider.bodies[0], wantBody) {
		t.Fatalf("provider calls/bodies = %d/%d, want exactly one exact body", provider.calls, len(provider.bodies))
	}
	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil || len(directories) != 1 {
		t.Fatalf("failed attempt journaled %d exchange dirs, want exactly 1 (err %v)", len(directories), err)
	}
	var recorded debugdump.SemanticExchangeRecord
	recordData, err := os.ReadFile(filepath.Join(
		runDir, debugdump.SemanticExchangesDir, directories[0].Name(),
		debugdump.SemanticExchangeMetaFile,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(recordData, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Stage != debugdump.SemanticStageArchitecture ||
		recorded.State != debugdump.SemanticStateProviderFailed ||
		recorded.SemanticCalls != 1 || recorded.TransportAttempts != 1 ||
		recorded.Outcome.Phase != "provider_call" ||
		recorded.Outcome.Code != "architecture.provider_output_limit" {
		t.Fatalf("failed exchange metadata = %#v", recorded)
	}
	if outcome.Failure == nil || outcome.Failure.Code != recorded.Outcome.Code {
		t.Fatalf("output-limit outcome/exchange mismatch = %#v / %#v", outcome.Failure, recorded.Outcome)
	}
	if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed attempt published a synthesis record: %v", err)
	}
}

func TestPersistArchitectureSynthesisStatusRetainsNonResourceFailure(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := persistArchitectureSynthesisStatus(
		runDir,
		architectureSynthesisOutcome{
			InputBytes: 1200, Attempted: true, TransportAttempts: 1,
			LocalCandidateCount: 2, RequestedConceptualCount: 2,
		},
		errors.New("architecture synthesis: provider call: unavailable"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile)); err != nil {
		t.Fatalf("non-resource failure status was not retained: %v", err)
	}
}

func TestArchitectureSemanticFailureIsPublishableOnlyAfterDurableFailedStatus(t *testing.T) {
	cause := &architectureResponseRejected{
		cause: errors.New("architecture synthesis: validate response: malformed fixture"),
	}
	outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationRejected)
	outcome.MemberOccurrences = 3
	outcome.ValidationCodes = []string{"proposal.duplicate_member_id"}

	t.Run("durable failed status permits continuation", func(t *testing.T) {
		runDir := t.TempDir()
		failure := persistAndClassifyArchitectureSynthesisStatus(runDir, outcome, cause)
		if !isPublishableArchitectureFailure(failure) ||
			!errors.Is(failure, errArchitectureSynthesisRejected) {
			t.Fatalf("durably recorded semantic failure = %T / %v", failure, failure)
		}
		encoded, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
		if err != nil {
			t.Fatal(err)
		}
		var status report.ArchitectureSynthesisStatus
		if err := json.Unmarshal(encoded, &status); err != nil {
			t.Fatal(err)
		}
		if status.State != report.ArchitectureSynthesisFailed || !status.ProposalRejected ||
			status.ProposalAccepted {
			t.Fatalf("durable failed Architecture status = %#v", status)
		}
	})

	t.Run("status write failure is terminal", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile), 0o700); err != nil {
			t.Fatal(err)
		}
		failure := persistAndClassifyArchitectureSynthesisStatus(runDir, outcome, cause)
		if failure == nil || isPublishableArchitectureFailure(failure) ||
			!errors.Is(failure, errArchitectureSynthesisRejected) {
			t.Fatalf("status persistence failure = %T / %v", failure, failure)
		}
	})

	t.Run("durable live provider failure permits local Canvas", func(t *testing.T) {
		providerCause := &architectureProviderCallFailed{
			cause: errors.New("architecture synthesis: live provider unavailable"),
		}
		providerOutcome := architectureSynthesisOutcome{
			InputBytes: 1200, Attempted: true, TransportAttempts: 1,
			LocalCandidateCount: 2, RequestedConceptualCount: 2, AnchorCount: 1,
		}
		failure := persistAndClassifyArchitectureSynthesisStatus(
			t.TempDir(), providerOutcome, providerCause,
		)
		if !isPublishableArchitectureFailure(failure) || !errors.Is(failure, providerCause) {
			t.Fatalf("durable provider failure = %T / %v", failure, failure)
		}
	})

	t.Run("provider setup failure remains terminal", func(t *testing.T) {
		failure := persistAndClassifyArchitectureSynthesisStatus(
			t.TempDir(), outcome, errors.New("architecture synthesis: provider unavailable"),
		)
		if failure == nil || isPublishableArchitectureFailure(failure) {
			t.Fatalf("provider setup failure = %T / %v", failure, failure)
		}
	})
}

// TestArchitectureOutputExhaustionClassification is the Decision 215
// classification proof: the attempted output exhaustion is publishable only
// after the failed status and model-research accounting are durable; a
// status-write failure, an accounting-write failure, cancellation, and a
// pre-call resource limit remain terminal.
func TestArchitectureOutputExhaustionClassification(t *testing.T) {
	limitCause := &architectureOutputResourceExhausted{cause: &modelresearch.ResourceLimitError{
		Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitOutputTokens,
		Limit: 64_000, ConfiguredMaxTokens: 64_000, FinishReason: "length",
		Observed: 64_000, ObservedKnown: true, OutputTokens: 64_000,
	}}
	lengthOutcome := architectureSynthesisOutcome{
		Attempted: true, InputBytes: 5904, ResponseBytes: 201396,
		ResponseContentBytes: 201396, TransportAttempts: 1,
		LocalCandidateCount: 3, RequestedConceptualCount: 2, StructuralLocatorCount: 1,
		AnchorCount: 4, UsageReported: true, InputTokens: 42197, OutputTokens: 64000,
		FinishReason: "length", ResponseComplete: false,
	}
	policy := modelresearch.DefaultPolicy()
	usage := modelresearch.Usage{}

	t.Run("publishable only after durable status and accounting", func(t *testing.T) {
		runDir := t.TempDir()
		state := modelresearch.NewState(policy, modelresearch.RepositoryContext{
			Identity: "fixture", Revision: "revision-output", Scenario: "go-default",
		})
		if err := modelresearch.WriteState(runDir, state); err != nil {
			t.Fatal(err)
		}
		// Simulate the stage owner: accounting first (C), then status + class.
		if err := recordArchitectureResearch(runDir, lengthOutcome, "resource_limited", false, policy, usage); err != nil {
			t.Fatal(err)
		}
		failure := persistAndClassifyArchitectureSynthesisStatus(runDir, lengthOutcome, limitCause)
		if !isPublishableArchitectureFailure(failure) || !errors.Is(failure, limitCause) {
			t.Fatalf("durably recorded output exhaustion = %T / %v", failure, failure)
		}
		var recorded modelresearch.State
		stateData, err := os.ReadFile(filepath.Join(runDir, modelresearch.StateFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(stateData, &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Architecture.Status != "resource_limited" || recorded.Architecture.SemanticCalls != 1 {
			t.Fatalf("resource-limited accounting = %#v", recorded.Architecture)
		}
	})

	t.Run("status write failure is terminal", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile), 0o700); err != nil {
			t.Fatal(err)
		}
		failure := persistAndClassifyArchitectureSynthesisStatus(runDir, lengthOutcome, limitCause)
		if failure == nil || isPublishableArchitectureFailure(failure) ||
			!errors.Is(failure, limitCause) {
			t.Fatalf("status persistence failure = %T / %v", failure, failure)
		}
	})

	t.Run("accounting write failure is terminal", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, modelresearch.StateFile), 0o700); err != nil {
			t.Fatal(err)
		}
		failure := classifyArchitectureOutputResourceExhaustion(
			runDir, lengthOutcome, policy, usage, limitCause.cause,
		)
		if failure == nil || isArchitectureOutputResourceExhausted(failure) ||
			!errors.Is(failure, limitCause.cause) {
			t.Fatalf("accounting persistence failure = %T / %v", failure, failure)
		}
	})

	t.Run("cancellation is terminal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runDir := t.TempDir()
		if err := modelresearch.WriteState(runDir, modelresearch.NewState(policy, modelresearch.RepositoryContext{
			Identity: "fixture", Revision: "revision-cancel", Scenario: "go-default",
		})); err != nil {
			t.Fatal(err)
		}
		provider := &architectureSynthesisStub{
			response: []byte(`{"records":[{"kind":"subsystem","ref":"g1"`),
			finish:   "length",
			onCall: func() {
				cancel()
			},
			err: limitCause.cause,
		}
		_, err := ensureArchitectureSynthesis(
			ctx, architectureSynthesisTestBundle(), runDir,
			"revision-cancel", "openai-compatible/bearer", "test-model", provider,
		)
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Architecture attempt = %v, want context.Canceled", err)
		}
		if _, statErr := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("cancelled attempt persisted a status: %v", statErr)
		}
	})

	t.Run("pre-call resource limit remains terminal", func(t *testing.T) {
		runDir := t.TempDir()
		state := modelresearch.NewState(policy, modelresearch.RepositoryContext{
			Identity: "fixture", Revision: "revision-precall", Scenario: "go-default",
		})
		state.Usage.RequestBytes = policy.MaxTotalRequestBytes
		if err := modelresearch.WriteState(runDir, state); err != nil {
			t.Fatal(err)
		}
		provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, architectureSynthesisTestBundle())}
		_, err := ensureArchitectureSynthesis(
			context.Background(), architectureSynthesisTestBundle(), runDir,
			"revision-precall", "openai-compatible/bearer", "test-model", provider,
		)
		if err == nil || isArchitectureOutputResourceExhausted(err) ||
			isPublishableArchitectureFailure(err) {
			t.Fatalf("pre-call resource limit = %T / %v, want terminal", err, err)
		}
		if provider.calls != 0 {
			t.Fatalf("pre-call limit made %d provider calls", provider.calls)
		}
	})
}

func TestArchitectureSynthesisStatusRecordsFailedProviderAttempt(t *testing.T) {
	t.Parallel()

	status := architectureSynthesisStatus(
		architectureSynthesisOutcome{
			InputBytes: 1200, LatencyMillis: 4321, Attempted: true,
			TransportAttempts: 1, LocalCandidateCount: 2, RequestedConceptualCount: 2,
			ProviderCallSucceeded: true, ResponseBytes: 128,
			ResponseState: componentmap.ResponseEmpty,
			FinishReason:  "stop", ResponseComplete: true,
		},
		fmt.Errorf("architecture synthesis: provider call: %w", deepseek.ErrResponseContentEmpty),
	)
	if status.State != report.ArchitectureSynthesisFailed ||
		status.ErrorCode != "empty_response" ||
		status.ProviderRequestCount != 1 ||
		status.RequestBytes != 1200 ||
		status.LatencyMillis != 4321 {
		t.Fatalf("status = %#v", status)
	}
	if status.Failure == nil || status.Failure.Code != "architecture.empty_response" {
		t.Fatalf("empty-response failure = %#v", status.Failure)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("truthful empty-response status: %v; status=%#v", err, status)
	}
}

func TestArchitectureEmptyResponseKeepsSuccessfulCallAndDecodeFailure(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	provider := &architectureSynthesisStub{
		responseBytes: 128, finish: "stop", err: deepseek.ErrResponseContentEmpty,
	}
	outcome, synthesisErr := ensureArchitectureSynthesisWithOptions(
		t.Context(), architectureSynthesisTestBundle(), runDir,
		"revision-empty", "openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, exchangeWriter: writer,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(synthesisErr, errArchitectureSynthesisRejected) ||
		!outcome.ProviderCallSucceeded || outcome.ResponseParsed ||
		outcome.ResponseBytes != 128 || outcome.ResponseContentBytes != 0 ||
		outcome.ResponseState != componentmap.ResponseEmpty || outcome.Failure == nil ||
		outcome.Failure.Code != "architecture.empty_response" {
		t.Fatalf("empty response outcome/error = %#v / %v", outcome, synthesisErr)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, synthesisErr); err != nil {
		t.Fatal(err)
	}
	statusData, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatal(err)
	}
	if status.Failure == nil || status.Failure.Code != "architecture.empty_response" ||
		!status.ProviderCallSucceeded || status.ResponseState != "empty" {
		t.Fatalf("empty response status = %#v", status)
	}
	records := readArchitectureSemanticExchangeRecords(t, runDir)
	if len(records) != 1 || records[0].State != debugdump.SemanticStateRejected ||
		records[0].ValidationCode != debugdump.SemanticValidationDecode ||
		records[0].Outcome.Code != "architecture.empty_response" {
		t.Fatalf("empty response exchange = %#v", records)
	}
}

func TestArchitectureSynthesisStatusSeparatesLocalCandidatesFromRequestedConceptualCoverage(t *testing.T) {
	t.Parallel()

	outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationAccepted)
	outcome.LocalCandidateCount = 50
	outcome.RequestedConceptualCount = 42
	outcome.StructuralLocatorCount = 8
	outcome.MemberOccurrences = 42
	outcome.DistinctMembers = 42
	outcome.CoveredConceptualCount = 42
	outcome.RequestedPrimaryScopeCount = 42
	outcome.CoveredPrimaryScopeCount = 42
	outcome.UncoveredPrimaryScopeCount = 0
	outcome.CoveredSupportingEvidenceCount = 0
	status := architectureSynthesisStatus(outcome, nil)
	if err := status.Validate(); err != nil {
		t.Fatalf("truthful 50/42/8 Architecture status: %v; status=%#v", err, status)
	}
	if status.CandidateCount != 0 || status.LocalCandidateCount != 50 ||
		status.RequestedConceptualCount != 42 || status.StructuralLocatorCount != 8 ||
		status.DistinctMembers != 42 {
		t.Fatalf("Architecture role/coverage status = %#v", status)
	}

	incomplete := status
	incomplete.DistinctMembers = 41
	if err := incomplete.Validate(); err == nil {
		t.Fatal("Architecture status accepted 41/42 conceptual response coverage")
	}
}

func TestArchitectureSynthesisStatusSeparatesProposalLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		outcome      architectureSynthesisOutcome
		accepted     bool
		partial      bool
		normalized   bool
		rejected     bool
		fallback     bool
		synthesisErr error
	}{
		{
			name:     "accepted",
			outcome:  architectureSynthesisOutcomeFixture(componentmap.ValidationAccepted),
			accepted: true,
		},
		{
			name: "normalized",
			outcome: func() architectureSynthesisOutcome {
				outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationAcceptedNormalized)
				outcome.Cached = true
				outcome.Attempted = false
				outcome.TransportAttempts = 0
				outcome.ArchitectureSource = componentmap.SourceNormalizedModel
				outcome.NormalizationCount = 1
				return outcome
			}(),
			accepted: true, normalized: true,
		},
		{
			name: "accepted partial",
			outcome: func() architectureSynthesisOutcome {
				outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationAcceptedPartial)
				outcome.LocalCandidateCount = 3
				outcome.RequestedConceptualCount = 3
				outcome.MemberOccurrences = 2
				outcome.DistinctMembers = 2
				outcome.CoveredConceptualCount = 2
				outcome.UncoveredConceptualCount = 1
				outcome.RequestedPrimaryScopeCount = 3
				outcome.CoveredPrimaryScopeCount = 2
				outcome.UncoveredPrimaryScopeCount = 1
				outcome.CoveredSupportingEvidenceCount = 0
				outcome.UncoveredConceptualIDs = []componentmap.MemberID{{
					Kind: componentmap.MemberPackage, Value: "opaque-storage",
				}}
				outcome.ArchitectureSource = componentmap.SourcePartialModel
				return outcome
			}(),
			accepted: true, partial: true,
		},
		{
			name: "rejected enrichment preserves local canvas",
			outcome: func() architectureSynthesisOutcome {
				outcome := architectureSynthesisOutcomeFixture(componentmap.ValidationRejected)
				outcome.ArchitectureSource = componentmap.SourceLocalAnchors
				outcome.ArchitectureLevel = 3
				outcome.FallbackSelected = true
				outcome.FallbackReason = componentmap.FallbackRejectedUnknownAnchor
				return outcome
			}(),
			rejected: true, synthesisErr: errArchitectureSynthesisRejected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status := architectureSynthesisStatus(test.outcome, test.synthesisErr)
			if err := status.Validate(); err != nil {
				t.Fatalf("Validate() error = %v; status = %#v", err, status)
			}
			if status.ProposalAccepted != test.accepted || status.ProposalPartial != test.partial ||
				status.ProposalNormalized != test.normalized ||
				status.ProposalRejected != test.rejected || status.FallbackSelected != test.fallback {
				t.Fatalf("status = %#v", status)
			}
			if test.synthesisErr != nil && (status.ArchitectureSource != "" || status.ArchitectureLevel != 0) {
				t.Fatalf("failed enrichment claimed visible Architecture ownership: %#v", status)
			}
		})
	}
}

func TestEnsureArchitectureSynthesisRefetchesCorruptSavedRecord(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheKey := architectureSynthesisTestCacheKey(
		t, bundle, "revision-corrupt", "openai-compatible/bearer", "test-model", provider,
	)
	cacheDir := filepath.Join(runsDir, architectureSynthesisCacheDirectory)
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), []byte(`{"broken"`), 0o600); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision-corrupt",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatalf("ensureArchitectureSynthesis() error = %v", err)
	}
	if provider.calls != 1 || outcome.Cached {
		t.Fatalf("provider calls/outcome = %d/%#v, want one replacement call", provider.calls, outcome)
	}
	saved, err := os.ReadFile(filepath.Join(cacheDir, cacheKey+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := componentmap.ReplaySynthesis(bundle, "revision-corrupt", saved); err != nil {
		t.Fatalf("replacement cache does not replay: %v", err)
	}
}

func TestEnsureArchitectureSynthesisCachedResourceLimitIsTerminal(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{
		err: errors.New("resource-invalid cache must not call provider"),
	}
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run")
	cacheDir := filepath.Join(runsDir, architectureSynthesisCacheDirectory)
	for _, dir := range []string{runDir, cacheDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cacheKey := architectureSynthesisTestCacheKey(
		t, bundle, "revision-resource-cache",
		"openai-compatible/bearer", "test-model", provider,
	)
	cachePath := filepath.Join(cacheDir, cacheKey+".json")
	if err := os.WriteFile(
		cachePath,
		bytes.Repeat([]byte{'x'}, modelresearch.SemanticRecordByteLimit+1),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision-resource-cache",
		"openai-compatible/bearer", "test-model", provider,
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != modelresearch.ResourceLimitRecordBytes ||
		limitErr.Limit != modelresearch.SemanticRecordByteLimit || provider.calls != 0 {
		t.Fatalf("cached resource error/provider calls = %#v / %d / %v", limitErr, provider.calls, err)
	}
	if info, statErr := os.Stat(cachePath); statErr != nil ||
		info.Size() != int64(modelresearch.SemanticRecordByteLimit+1) {
		t.Fatalf("terminal resource cache was removed or changed: info=%#v err=%v", info, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("resource-invalid cache was applied to run: %v", statErr)
	}
}

func TestEnsureArchitectureSynthesisRefetchesLanguageUnknownActiveCache(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	response := architectureSynthesisTestResponse(t, bundle)
	legacy, err := componentmap.RecordSynthesisResponse(
		bundle,
		"revision-language-unknown",
		"openai-compatible/bearer",
		"test-model",
		0,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(legacy.Record)
	if err != nil {
		t.Fatal(err)
	}
	saved = bytes.Replace(saved, []byte(`,"output_language":"en"`), nil, 1)
	if bytes.Contains(saved, []byte(`"output_language"`)) {
		t.Fatalf("test fixture still has an explicit language: %s", saved)
	}

	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run")
	cacheDir := filepath.Join(runsDir, architectureSynthesisCacheDirectory)
	for _, dir := range []string{runDir, cacheDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	provider := &architectureSynthesisStub{response: response}
	cacheKey := architectureSynthesisTestCacheKey(
		t, bundle, "revision-language-unknown",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), saved, 0o600); err != nil {
		t.Fatal(err)
	}

	outcome, err := ensureArchitectureSynthesisWithOptions(
		context.Background(),
		bundle,
		runDir,
		"revision-language-unknown",
		"openai-compatible/bearer",
		"test-model",
		provider,
		architectureSynthesisOptions{providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Cached || provider.calls != 1 {
		t.Fatalf("outcome/calls = %#v/%d, want refetch for unknown active language", outcome, provider.calls)
	}

	replacement, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(replacement, &record); err != nil {
		t.Fatal(err)
	}
	if record.Call == nil || record.Call.Metadata.OutputLanguage != "en" {
		t.Fatalf("replacement record metadata = %#v, want explicit English", record.Call)
	}
}

func TestEnsureArchitectureSynthesisCannotExceedFourSemanticCalls(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	policy := modelresearch.DefaultPolicy()
	state := modelresearch.NewState(policy, modelresearch.RepositoryContext{
		Identity: runDir, Revision: "abc", Scenario: "go-default",
	})
	state.Usage.SemanticCalls = policy.MaxSemanticCalls
	state.Usage.RequestBytes = 100 << 10
	if err := modelresearch.WriteState(runDir, state); err != nil {
		t.Fatal(err)
	}
	provider := &architectureSynthesisStub{err: errors.New("must not be called")}
	_, err := ensureArchitectureSynthesis(
		context.Background(), architectureSynthesisTestBundle(), runDir, "revision-budget",
		"openai-compatible/bearer", "test-model", provider,
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != "architecture_synthesis" ||
		limitErr.Kind != modelresearch.ResourceLimitSemanticCalls ||
		limitErr.Limit != policy.MaxSemanticCalls || limitErr.Observed != policy.MaxSemanticCalls {
		t.Fatalf("error = %#v / %v, want typed call budget exhaustion", limitErr, err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want zero", provider.calls)
	}
}

func TestArchitectureSynthesisBudgetFailuresAreTypedTerminalResources(t *testing.T) {
	t.Parallel()
	policy := modelresearch.DefaultPolicy()
	tests := []struct {
		name    string
		reason  string
		usage   modelresearch.Usage
		request int
		kind    modelresearch.ResourceLimitKind
		limit   int
		seen    int
	}{
		{name: "stage bytes", reason: "stage_byte_budget_exhausted", request: policy.Architecture.MaxRequestBytes + 1, kind: modelresearch.ResourceLimitRequestBytes, limit: policy.Architecture.MaxRequestBytes, seen: policy.Architecture.MaxRequestBytes + 1},
		{name: "total bytes", reason: "total_byte_budget_exhausted", usage: modelresearch.Usage{RequestBytes: policy.MaxTotalRequestBytes}, request: 1, kind: modelresearch.ResourceLimitRequestBytes, limit: policy.MaxTotalRequestBytes, seen: policy.MaxTotalRequestBytes + 1},
		{name: "semantic calls", reason: "call_budget_exhausted", usage: modelresearch.Usage{SemanticCalls: policy.MaxSemanticCalls}, request: 1, kind: modelresearch.ResourceLimitSemanticCalls, limit: policy.MaxSemanticCalls, seen: policy.MaxSemanticCalls},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var resourceErr *modelresearch.ResourceLimitError
			err := architectureSynthesisBudgetError(test.reason, policy, test.usage, test.request)
			if !errors.As(err, &resourceErr) || resourceErr.Stage != "architecture_synthesis" ||
				resourceErr.Kind != test.kind || resourceErr.Limit != test.limit ||
				resourceErr.Observed != test.seen || !resourceErr.ObservedKnown {
				t.Fatalf("budget error = %#v / %v", resourceErr, err)
			}
		})
	}
}

func TestPrepareAuthorizedArchitectureUsesCompleteCasdoorGraph(t *testing.T) {
	const edgeCount = 90
	runDir, authority := architectureAuthorizedGraphRun(
		t,
		architectureCasdoorGraphFacts(edgeCount),
	)
	before, err := report.ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil {
		t.Fatalf("authorized read before synthesis: %v", err)
	}
	input, err := report.BuildArchitectureCanvasInput(before)
	if err != nil {
		t.Fatalf("BuildArchitectureCanvasInput: %v", err)
	}
	requestBefore, requestBytesBefore, err := componentmap.BuildSynthesisRequest(input.CandidateBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	if len(requestBefore.Relations) != 0 {
		t.Fatalf("pre-provider relations = %d, want 0 (Decision 223: package imports aggregate into units)", len(requestBefore.Relations))
	}
	if len(requestBefore.Units) == 0 {
		t.Fatalf("pre-provider units = 0, want the bounded unit catalog")
	}
	totalOut := 0
	for _, unit := range requestBefore.Units {
		totalOut += unit.RelationOutCount
	}
	if totalOut == 0 {
		t.Fatalf("unit relation_out_count aggregate is empty; the complete casdoor import graph is missing")
	}

	provider := &architectureSynthesisStub{
		response: architectureSynthesisTestResponse(t, input.CandidateBundle),
	}
	outcome, err := prepareArchitectureSynthesisWithOptions(
		context.Background(), runDir, "revision-authorized-casdoor",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, runAuthority: &authority,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if err != nil {
		t.Fatalf("prepareArchitectureSynthesisWithOptions: %v", err)
	}
	if provider.calls != 1 || len(provider.prompts) != 1 {
		t.Fatalf("provider calls/prompts = %d/%d, want 1/1", provider.calls, len(provider.prompts))
	}
	var sent componentmap.SynthesisRequest
	const userPrefix = "Bounded candidate request:\n"
	if !strings.HasPrefix(provider.prompts[0].User, userPrefix) {
		t.Fatalf("Architecture user prompt omitted bounded request prefix")
	}
	if err := json.Unmarshal(
		[]byte(strings.TrimPrefix(provider.prompts[0].User, userPrefix)),
		&sent,
	); err != nil {
		t.Fatalf("decode exact sent Architecture request: %v", err)
	}
	if len(sent.Relations) != 0 {
		t.Fatalf("exact sent supporting_relations = %d, want 0 (Decision 223: package imports aggregate into units)", len(sent.Relations))
	}
	if len(sent.Units) == 0 {
		t.Fatalf("exact sent units = 0, want the bounded unit catalog")
	}
	sentTotalOut := 0
	for _, unit := range sent.Units {
		sentTotalOut += unit.RelationOutCount
	}
	if sentTotalOut == 0 {
		t.Fatalf("exact sent unit relation_out_count aggregate is empty; the complete casdoor import graph is missing")
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, nil); err != nil {
		t.Fatalf("persist Architecture status: %v", err)
	}

	replayed, err := report.ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil {
		t.Fatalf("authorized replay: %v", err)
	}
	replayedInput, err := report.BuildArchitectureCanvasInput(replayed)
	if err != nil {
		t.Fatalf("BuildArchitectureCanvasInput after replay: %v", err)
	}
	_, requestBytesAfter, err := componentmap.BuildSynthesisRequest(replayedInput.CandidateBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest after replay: %v", err)
	}
	if !bytes.Equal(requestBytesAfter, requestBytesBefore) {
		t.Fatalf("authorized Architecture request changed after replay")
	}
	// Decision 223: raw package-import edges are gone; the request must be
	// materially smaller than the equivalent raw-edges request. 90 edges at
	// ~40 bytes each would add ~3.6KB; the aggregate keeps the wire compact.
	if len(requestBytesBefore) > 30000 {
		t.Fatalf("request bytes = %d, want compact unit-aggregated wire under 30KB", len(requestBytesBefore))
	}
	replayedPrompt, err := componentmap.BuildSynthesisPromptForLanguage(
		replayedInput.CandidateBundle,
		"en",
	)
	if err != nil {
		t.Fatalf("BuildSynthesisPromptForLanguage after replay: %v", err)
	}
	replayProvider := &architectureSynthesisStub{}
	replayedBody, err := replayProvider.ComponentSynthesisPromptJSON(replayedPrompt)
	if err != nil {
		t.Fatalf("ComponentSynthesisPromptJSON after replay: %v", err)
	}
	if len(provider.bodies) != 1 || !bytes.Equal(replayedBody, provider.bodies[0]) {
		t.Fatalf("authorized external Architecture body changed after replay")
	}
}

func TestPrepareAuthorizedArchitectureSkipsProviderWithoutExactGraph(t *testing.T) {
	facts := architectureCasdoorGraphFacts(1)
	facts.InternalEdges = make([]gofacts.Edge, workspacegraph.MaxExactEdges+1)
	for index := range facts.InternalEdges {
		facts.InternalEdges[index] = gofacts.Edge{
			From: facts.Packages[0].CanonicalPath,
			To:   facts.Packages[1].CanonicalPath,
		}
	}
	runDir, authority := architectureAuthorizedGraphRun(t, facts)
	provider := &architectureSynthesisStub{err: errors.New("provider must not be called")}
	outcome, synthesisErr := prepareArchitectureSynthesisWithOptions(
		context.Background(), runDir, "revision-incomplete-graph",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{
			disableCache: true, runAuthority: &authority,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	if !report.IsExactWorkspaceGraphUnavailable(synthesisErr) {
		t.Fatalf("synthesis error = %T / %v, want exact graph unavailable", synthesisErr, synthesisErr)
	}
	if provider.calls != 0 || outcome.Attempted {
		t.Fatalf("incomplete exact graph reached provider: calls=%d outcome=%#v", provider.calls, outcome)
	}
	publishable := persistAndClassifyArchitectureSynthesisStatus(runDir, outcome, synthesisErr)
	if !isPublishableArchitectureFailure(publishable) ||
		!report.IsExactWorkspaceGraphUnavailable(publishable) {
		t.Fatalf("durable incomplete-graph outcome = %T / %v", publishable, publishable)
	}
	encoded, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatalf("read Architecture status: %v", err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(encoded, &status); err != nil {
		t.Fatalf("decode Architecture status: %v", err)
	}
	if status.State != report.ArchitectureSynthesisUnavailable ||
		status.UnavailableCode != report.ArchitectureSynthesisUnavailableExactWorkspaceGraphCode ||
		status.ProviderRequestCount != 0 {
		t.Fatalf("exact-graph unavailable status = %#v", status)
	}
}

func TestSynthesizeArchitecturePreflightsExactGraphBeforeProviderConfiguration(t *testing.T) {
	facts := architectureCasdoorGraphFacts(1)
	facts.InternalEdges = make([]gofacts.Edge, workspacegraph.MaxExactEdges+1)
	for index := range facts.InternalEdges {
		facts.InternalEdges[index] = gofacts.Edge{
			From: facts.Packages[0].CanonicalPath,
			To:   facts.Packages[1].CanonicalPath,
		}
	}
	runDir, authority := architectureAuthorizedGraphRun(t, facts)

	// This configuration is deliberately invalid. The provider-free exact
	// graph preflight owns the earlier failure and must make provider setup
	// unreachable.
	t.Setenv("REPOMAP_LLM_AUTH", "invalid")
	outcome, synthesisErr := synthesizeArchitectureForRun(
		context.Background(),
		runDir,
		authority,
		newRunOutput(nil),
		true,
		"en",
	)
	if !isPublishableArchitectureFailure(synthesisErr) ||
		!report.IsExactWorkspaceGraphUnavailable(synthesisErr) {
		t.Fatalf(
			"synthesis error = %T / %v, want durable exact graph unavailable",
			synthesisErr,
			synthesisErr,
		)
	}
	if outcome.Attempted {
		t.Fatalf("provider-free preflight outcome = %#v, want unattempted", outcome)
	}
	encoded, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatalf("read Architecture status: %v", err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(encoded, &status); err != nil {
		t.Fatalf("decode Architecture status: %v", err)
	}
	if status.State != report.ArchitectureSynthesisUnavailable ||
		status.UnavailableCode != report.ArchitectureSynthesisUnavailableExactWorkspaceGraphCode ||
		status.ProviderRequestCount != 0 {
		t.Fatalf("pre-provider exact-graph status = %#v", status)
	}
}

func TestPrepareAuthorizedArchitectureReportsCandidateExhaustionAsTypedResource(t *testing.T) {
	runDir, authority := architectureAuthorizedGraphRun(
		t,
		architectureCasdoorGraphFacts(512),
	)
	provider := &architectureSynthesisStub{err: errors.New("provider must not be called")}
	outcome, synthesisErr := prepareArchitectureSynthesisWithOptions(
		context.Background(),
		runDir,
		"revision-candidate-limit",
		"openai-compatible/bearer",
		"test-model",
		provider,
		architectureSynthesisOptions{
			disableCache: true, runAuthority: &authority,
			providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256(),
		},
	)
	var resourceErr *modelresearch.ResourceLimitError
	if !errors.As(synthesisErr, &resourceErr) ||
		resourceErr.Kind != modelresearch.ResourceLimitCatalogItems ||
		resourceErr.Stage != "architecture_input_candidates" ||
		resourceErr.Limit != 512 || resourceErr.Observed <= resourceErr.Limit ||
		!resourceErr.ObservedKnown {
		t.Fatalf("candidate exhaustion = %#v / %v", resourceErr, synthesisErr)
	}
	if provider.calls != 0 || outcome.Attempted {
		t.Fatalf("candidate exhaustion reached provider: calls=%d outcome=%#v", provider.calls, outcome)
	}
}

func TestPrepareArchitectureSynthesisSupportsLandscapeWithoutFlowProof(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeArchitectureSynthesisFixture(t, runDir, "snapshot.json", `{"repo_name":"fixture"}`)
	writeArchitectureSynthesisFixture(t, runDir, "orientation_report.json", `{"project_guess":"fixture"}`)
	writeArchitectureSynthesisFixture(t, runDir, "llm_bundle.json", `{
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`)

	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, input.CandidateBundle)}
	outcome, err := prepareArchitectureSynthesis(
		context.Background(), runDir, "revision-landscape-only",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || outcome.InputBytes == 0 {
		t.Fatalf("provider calls/outcome = %d / %#v, want one bounded synthesis", provider.calls, outcome)
	}
	if err := persistArchitectureSynthesisStatus(runDir, outcome, nil); err != nil {
		t.Fatal(err)
	}

	replayed, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ArchitectureCanvas == nil || replayed.ArchitectureCanvas.Fallback ||
		len(replayed.ArchitectureCanvas.Components) == 0 || len(replayed.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("replayed canvas = %#v, want synthesized landscape without invented flows", replayed.ArchitectureCanvas)
	}
}

func TestPrepareArchitectureSynthesisSupportsOnePackageLibrary(t *testing.T) {
	t.Parallel()

	runDir := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeArchitectureSynthesisFixture(t, runDir, "snapshot.json", `{
		"repo_name":"fixture",
		"filtered_files":["wal.go"],
		"go_facts":{
			"modules":[{
				"id":"module-one","module_path":"example.com/wal","module_dir":".",
				"go_mod":"go.mod","main":true,"display_name":".","packages_count":1
			}],
			"packages":[{
				"canonical_package_path":"example.com/wal","name":"wal",
				"owning_module_id":"module-one","module_path":"example.com/wal",
				"package_directory":".","module_relative_path":".",
				"display_path":"wal","locality":"local","files":["wal.go"]
			}],
			"packages_count":1
		}
	}`)
	writeArchitectureSynthesisFixture(t, runDir, "orientation_report.json", `{"project_guess":"fixture library"}`)
	writeArchitectureSynthesisFixture(t, runDir, "llm_bundle.json", `{
		"repo_name":"example.com/wal",
		"go":{
			"modules_count":1,"packages_count":1,
			"module_summaries":[{
				"module_path":"example.com/wal","module_dir":".",
				"packages_count":1,"entrypoints_count":0,"role_guess":"repository_root"
			}]
		}
	}`)

	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.CandidateBundle.Candidates) != 1 ||
		input.CandidateBundle.RepositoryArchetype != componentmap.ArchetypeLibraryFramework {
		t.Fatalf("one-package library bundle = %#v", input.CandidateBundle)
	}
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, input.CandidateBundle)}
	outcome, err := prepareArchitectureSynthesisWithOptions(
		context.Background(), runDir, "revision-one-package",
		"openai-compatible/bearer", "test-model", provider,
		architectureSynthesisOptions{disableCache: true, providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || !outcome.ProviderCallSucceeded ||
		outcome.ValidationOutcome != componentmap.ValidationAccepted {
		t.Fatalf("provider calls/outcome = %d / %#v", provider.calls, outcome)
	}
}

func writeArchitectureSynthesisFixture(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func architectureAuthorizedGraphRun(
	t *testing.T,
	facts gofacts.Facts,
) (string, report.RunAuthority) {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repository, "go.mod"),
		[]byte("module github.com/casdoor/casdoor\n\ngo 1.24\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod")
	commitTestRepository(t, repository)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatalf("capture initial repository: %v", err)
	}

	runDir := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(filepath.Join(runDir, "flows"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeArchitectureSynthesisFixture(
		t,
		runDir,
		"snapshot.json",
		string(mustArchitectureJSON(t, map[string]any{
			"repo_name": "github.com/casdoor/casdoor",
			"go_facts":  facts,
		})),
	)
	writeArchitectureSynthesisFixture(t, runDir, "llm_bundle.json", `{}`)
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("read saved graph run: %v", err)
	}
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatalf("capture current repository: %v", err)
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		context.Background(),
		repository,
		initial,
		current,
		report.CapturedInputPaths(data),
		false,
	)
	if err != nil {
		t.Fatalf("ConfirmRunAuthorityScoped: %v", err)
	}
	return runDir, authority
}

func architectureCasdoorGraphFacts(edgeCount int) gofacts.Facts {
	const modulePath = "github.com/casdoor/casdoor"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages:      make([]gofacts.PackageFact, edgeCount+1),
		InternalEdges: make([]gofacts.Edge, edgeCount),
	}
	for index := range facts.Packages {
		directory := fmt.Sprintf("pkg%03d", index)
		canonicalPath := modulePath + "/" + directory
		facts.Packages[index] = gofacts.PackageFact{
			CanonicalPath:     canonicalPath,
			Name:              directory,
			ModuleID:          "root-id",
			ModulePath:        modulePath,
			PackageDir:        directory,
			ModuleRelativeDir: directory,
		}
		if index > 0 {
			facts.InternalEdges[index-1] = gofacts.Edge{
				From: facts.Packages[index-1].CanonicalPath,
				To:   canonicalPath,
			}
		}
	}
	return facts
}

func mustArchitectureJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func readArchitectureSemanticExchangeRecords(
	t *testing.T,
	runDir string,
) []debugdump.SemanticExchangeRecord {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	records := make([]debugdump.SemanticExchangeRecord, 0, len(directories))
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(
			runDir,
			debugdump.SemanticExchangesDir,
			directory.Name(),
			debugdump.SemanticExchangeMetaFile,
		))
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func architectureSynthesisTestBundle() componentmap.CandidateBundle {
	memberID := componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-runtime"}
	return componentmap.CandidateBundle{
		Version:             componentmap.ContractVersion,
		RepositoryArchetype: componentmap.ArchetypeApplication,
		GroundingMode:       componentmap.GroundingPackages,
		Candidates: []componentmap.Candidate{{
			ID: memberID, Role: componentmap.CandidateRoleConceptualMember, Name: "local runtime",
			Facts: []componentmap.LocalFact{{
				Kind: componentmap.FactDeclaration, Value: "runtime package",
				Certainty: evidence.CertaintyStatic,
				Provenance: []evidence.Provenance{{
					Provider: "test", Version: "v1", Operation: "fixture",
				}},
			}},
		}},
	}
}

func architectureSynthesisTestResponse(t *testing.T, bundle componentmap.CandidateBundle) []byte {
	t.Helper()
	request, _, err := componentmap.BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	memberRefs := make([]componentmap.SynthesisMemberRef, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		memberRefs = append(memberRefs, candidate.Ref)
	}
	anchorRefs := make([]componentmap.SynthesisAnchorRef, 0, len(request.BehaviorAnchors))
	for _, anchor := range request.BehaviorAnchors {
		anchorRefs = append(anchorRefs, anchor.Ref)
	}
	proposal := architectureSynthesisWireResponse{
		Records: []any{
			architectureSynthesisWireSubsystem{
				Kind: "subsystem", Ref: "g1", Name: "Application",
			},
			architectureSynthesisWireComponent{
				Kind: "component", SubsystemRef: "g1",
				Name: "Runtime", MemberRefs: memberRefs, AnchorRefs: anchorRefs,
				Hypothesis: len(anchorRefs) == 0 && bundle.GroundingMode != componentmap.GroundingPackages,
			},
		},
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
