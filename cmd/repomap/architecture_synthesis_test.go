package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

type architectureSynthesisStub struct {
	calls     int
	response  []byte
	err       error
	maxTokens int
	endpoint  string
	prompts   []componentmap.SynthesisPrompt
	pending   *componentmap.SynthesisPrompt
	bodies    [][]byte
	finish    string
	onCall    func()
}

type architectureSynthesisWireResponse struct {
	Subsystems []architectureSynthesisWireSubsystem `json:"subsystems"`
}

type architectureSynthesisWireSubsystem struct {
	Name        string                               `json:"name"`
	Description string                               `json:"description,omitempty"`
	Components  []architectureSynthesisWireComponent `json:"components"`
}

type architectureSynthesisWireComponent struct {
	Name        string                            `json:"name"`
	Description string                            `json:"description,omitempty"`
	MemberRefs  []componentmap.SynthesisMemberRef `json:"member_refs"`
	AnchorRefs  []componentmap.SynthesisAnchorRef `json:"anchor_refs,omitempty"`
	Hypothesis  bool                              `json:"hypothesis,omitempty"`
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
		CandidateCount: 2, AnchorCount: 1,
		MembershipCounted: true, MemberOccurrences: 2, DistinctMembers: 2,
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
	if stub.err == nil {
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

	second, err := ensureArchitectureSynthesis(
		context.Background(), bundle, secondRun, "revision-one",
		"openai-compatible/bearer", "test-model", provider,
	)
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
	if status.Version != 4 || status.RequestBytes != len(wantBody) ||
		status.ResponseBytes != len(provider.response) ||
		status.ResponseContentBytes != len(provider.response) ||
		status.CandidateCount != 1 || status.AnchorCount != 0 ||
		!status.MembershipCounted || status.MemberOccurrences != 1 || status.DistinctMembers != 1 ||
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
		exchange.SemanticCalls != 1 || exchange.TransportAttempts != 1 {
		t.Fatalf("semantic exchange = %#v", exchange)
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

func TestArchitectureResponseMembershipCountsExactCanonicalAndRequestLocalShapes(t *testing.T) {
	for _, test := range []struct {
		name       string
		response   string
		counted    bool
		occurrence int
		distinct   int
	}{
		{
			name:     "canonical bridge duplicate",
			response: `{"subsystems":[{"components":[{"member_ids":[{"kind":"package","value":"a"}]},{"member_ids":[{"kind":"package","value":"a"},{"kind":"file","value":"b"}]}]}]}`,
			counted:  true, occurrence: 3, distinct: 2,
		},
		{
			name:     "request local refs",
			response: `{"subsystems":[{"components":[{"member_refs":[{"kind":"package","ref":"p1"},{"kind":"file","ref":"f1"}]}]}]}`,
			counted:  true, occurrence: 2, distinct: 2,
		},
		{name: "not exact json", response: "```json\n{}\n```"},
		{name: "mixed identities", response: `{"subsystems":[{"components":[{"member_ids":[],"member_refs":[]}]}]}`},
		{name: "non-closed ref", response: `{"subsystems":[{"components":[{"member_refs":[{"kind":"package","ref":"p1","path":"private"}]}]}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			counted, occurrence, distinct := architectureResponseMembershipCounts([]byte(test.response))
			if counted != test.counted || occurrence != test.occurrence || distinct != test.distinct {
				t.Fatalf("counts = %t/%d/%d", counted, occurrence, distinct)
			}
		})
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

func TestEnsureArchitectureSynthesisPersistsConflictingMembershipEvidence(t *testing.T) {
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
		Subsystems: []architectureSynthesisWireSubsystem{{
			Name: "Repository",
			Components: []architectureSynthesisWireComponent{
				{Name: "Runtime", MemberRefs: []componentmap.SynthesisMemberRef{request.Candidates[0].Ref}},
				{Name: "Storage", MemberRefs: []componentmap.SynthesisMemberRef{
					request.Candidates[0].Ref,
					request.Candidates[1].Ref,
				}},
			},
		}},
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
	if !errors.Is(err, errArchitectureSynthesisRejected) {
		t.Fatalf("conflicting synthesis error = %v", err)
	}
	if !outcome.MembershipCounted || outcome.MemberOccurrences != 3 || outcome.DistinctMembers != 2 ||
		strings.Join(outcome.ValidationCodes, ",") != "proposal.conflicting_membership" {
		t.Fatalf("conflicting synthesis evidence = %#v", outcome)
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
	if status.State != report.ArchitectureSynthesisFailed || status.MemberOccurrences != 3 ||
		status.DistinctMembers != 2 ||
		strings.Join(status.ValidationCodes, ",") != "proposal.conflicting_membership" {
		t.Fatalf("persisted conflicting synthesis status = %#v", status)
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
			name:     "normalized fenced response has no exact membership envelope",
			response: append(append([]byte("```json\n"), validResponse...), []byte("\n```")...),
			wantCode: "response.membership_unavailable",
		},
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
		strings.Join(status.ValidationCodes, ",") != "response.no_json,proposal.unsupported_version" {
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

func TestEnsureArchitectureSynthesisResourceLimitDoesNotPublishPartialArtifacts(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider func() *architectureSynthesisStub
	}{
		{
			name: "provider resource error",
			provider: func() *architectureSynthesisStub {
				return &architectureSynthesisStub{err: &modelresearch.ResourceLimitError{
					Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitOutputTokens,
					Limit: 64_000, ConfiguredMaxTokens: 64_000, FinishReason: "length",
				}}
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
			if err := persistArchitectureSynthesisStatus(runDir, outcome, synthesisErr); err != nil {
				t.Fatal(err)
			}
			afterState, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(beforeState, afterState) {
				t.Fatal("terminal resource error mutated model research stage metrics")
			}
			for _, name := range []string{
				report.ArchitectureSynthesisFile,
				report.ArchitectureSynthesisStatusFile,
			} {
				if _, err := os.Lstat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("terminal resource error published %s: %v", name, err)
				}
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
				t.Fatalf("terminal resource error populated cache: %v", cacheFiles)
			}
		})
	}
}

func TestPersistArchitectureSynthesisStatusRetainsNonResourceFailure(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := persistArchitectureSynthesisStatus(
		runDir,
		architectureSynthesisOutcome{
			InputBytes: 1200, Attempted: true, TransportAttempts: 1, CandidateCount: 2,
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
	outcome.ValidationCodes = []string{"proposal.conflicting_membership"}

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
		failure := persistAndClassifyArchitectureSynthesisStatus(
			t.TempDir(), outcome, providerCause,
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

func TestArchitectureSynthesisStatusRecordsFailedProviderAttempt(t *testing.T) {
	t.Parallel()

	status := architectureSynthesisStatus(
		architectureSynthesisOutcome{
			InputBytes: 1200, LatencyMillis: 4321, Attempted: true,
			TransportAttempts: 1, CandidateCount: 2,
		},
		errors.New("architecture synthesis: provider call: llm response content is empty"),
	)
	if status.State != report.ArchitectureSynthesisFailed ||
		status.ErrorCode != "empty_response" ||
		status.ProviderRequestCount != 1 ||
		status.RequestBytes != 1200 ||
		status.LatencyMillis != 4321 {
		t.Fatalf("status = %#v", status)
	}
}

func TestArchitectureSynthesisStatusSeparatesProposalLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		outcome      architectureSynthesisOutcome
		accepted     bool
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
			if status.ProposalAccepted != test.accepted || status.ProposalNormalized != test.normalized ||
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

func architectureSynthesisTestBundle() componentmap.CandidateBundle {
	memberID := componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-runtime"}
	return componentmap.CandidateBundle{
		Version:             componentmap.ContractVersion,
		RepositoryArchetype: componentmap.ArchetypeApplication,
		GroundingMode:       componentmap.GroundingPackages,
		Candidates: []componentmap.Candidate{{
			ID: memberID, Name: "local runtime",
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
		Subsystems: []architectureSynthesisWireSubsystem{{
			Name: "Application",
			Components: []architectureSynthesisWireComponent{{
				Name: "Runtime", MemberRefs: memberRefs, AnchorRefs: anchorRefs,
				Hypothesis: len(anchorRefs) == 0 && bundle.GroundingMode != componentmap.GroundingPackages,
			}},
		}},
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
