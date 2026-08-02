package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
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
}

func (stub *architectureSynthesisStub) ArchitectureProviderEndpointSHA256() string {
	endpoint := stub.endpoint
	if endpoint == "" {
		endpoint = "https://architecture.test/v1/chat/completions"
	}
	digest, _ := modelresearch.ProviderEndpointSHA256(endpoint)
	return digest
}

func (stub *architectureSynthesisStub) SynthesizeComponentLandscapeMeasured(
	_ context.Context,
	_ componentmap.SynthesisPrompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{Content: append([]byte(nil), stub.response...), Attempts: 1}, stub.err
}

func (stub *architectureSynthesisStub) ComponentSynthesisPromptJSON(prompt componentmap.SynthesisPrompt) ([]byte, error) {
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

func TestEnsureArchitectureSynthesisCachesOneCanonicalEnglishResult(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: architectureSynthesisTestResponse(t, bundle)}
	runsDir := t.TempDir()
	run := func(name string) architectureSynthesisOutcome {
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
			architectureSynthesisOptions{providerEndpointSHA256: provider.ArchitectureProviderEndpointSHA256()},
		)
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}

	if outcome := run("canonical"); outcome.Cached {
		t.Fatalf("first canonical outcome = %#v, want provider call", outcome)
	}
	if outcome := run("english-render"); !outcome.Cached {
		t.Fatalf("English presentation source = %#v, want canonical cache replay", outcome)
	}
	if outcome := run("russian-render"); !outcome.Cached {
		t.Fatalf("Russian presentation source = %#v, want canonical cache replay", outcome)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one canonical English call", provider.calls)
	}

	saved, err := os.ReadFile(filepath.Join(runsDir, "russian-render", report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(saved, &record); err != nil {
		t.Fatal(err)
	}
	if record.Call == nil || record.Call.Metadata.OutputLanguage != "en" {
		t.Fatalf("canonical cached record metadata = %#v", record.Call)
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

func TestEnsureArchitectureSynthesisPersistsDeterministicFallbackForInvalidOutput(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: []byte("not json")}
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-one")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision-invalid",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.FallbackReason != componentmap.FallbackRejectedMalformed || provider.calls != 1 {
		t.Fatalf("outcome = %#v, calls = %d", outcome, provider.calls)
	}
	saved, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}
	landscape, err := componentmap.ReplaySynthesis(bundle, "revision-invalid", saved)
	if err != nil {
		t.Fatal(err)
	}
	if !landscape.Fallback || landscape.FallbackReason != componentmap.FallbackRejectedMalformed {
		t.Fatalf("fallback landscape = %#v", landscape)
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
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), saved, 0o600); err != nil {
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
		architectureSynthesisOutcome{InputBytes: 1200, Attempted: true},
		errors.New("architecture synthesis: provider call: unavailable"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile)); err != nil {
		t.Fatalf("non-resource failure status was not retained: %v", err)
	}
}

func TestArchitectureSynthesisStatusRecordsFailedProviderAttempt(t *testing.T) {
	t.Parallel()

	status := architectureSynthesisStatus(
		architectureSynthesisOutcome{InputBytes: 1200, LatencyMillis: 4321, Attempted: true},
		errors.New("architecture synthesis: provider call: llm response content is empty"),
	)
	if status.State != report.ArchitectureSynthesisFailed ||
		status.ErrorCode != "empty_response" ||
		status.ProviderRequestCount != 1 ||
		status.PromptBytes != 1200 ||
		status.LatencyMillis != 4321 {
		t.Fatalf("status = %#v", status)
	}
}

func TestArchitectureSynthesisStatusSeparatesProposalLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outcome    architectureSynthesisOutcome
		accepted   bool
		normalized bool
		rejected   bool
		fallback   bool
	}{
		{
			name: "accepted",
			outcome: architectureSynthesisOutcome{
				ProviderCallSucceeded: true,
				ResponseParsed:        true,
				ValidationOutcome:     componentmap.ValidationAccepted,
				ArchitectureSource:    componentmap.SourceValidatedModel,
				ArchitectureLevel:     1,
			},
			accepted: true,
		},
		{
			name: "normalized",
			outcome: architectureSynthesisOutcome{
				Cached:                true,
				ProviderCallSucceeded: true,
				ResponseParsed:        true,
				ValidationOutcome:     componentmap.ValidationAcceptedNormalized,
				ArchitectureSource:    componentmap.SourceNormalizedModel,
				ArchitectureLevel:     2,
				NormalizationCount:    1,
			},
			accepted: true, normalized: true,
		},
		{
			name: "rejected fallback",
			outcome: architectureSynthesisOutcome{
				ProviderCallSucceeded: true,
				ResponseParsed:        true,
				ValidationOutcome:     componentmap.ValidationRejected,
				ArchitectureSource:    componentmap.SourceLocalAnchors,
				ArchitectureLevel:     3,
				FallbackSelected:      true,
				FallbackReason:        componentmap.FallbackRejectedUnknownAnchor,
			},
			rejected: true, fallback: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status := architectureSynthesisStatus(test.outcome, nil)
			if err := status.Validate(); err != nil {
				t.Fatalf("Validate() error = %v; status = %#v", err, status)
			}
			if status.ProposalAccepted != test.accepted || status.ProposalNormalized != test.normalized ||
				status.ProposalRejected != test.rejected || status.FallbackSelected != test.fallback {
				t.Fatalf("status = %#v", status)
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
	if err == nil || !strings.Contains(err.Error(), "call_budget_exhausted") {
		t.Fatalf("error = %v, want call budget exhaustion", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want zero", provider.calls)
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
	proposal := componentmap.Proposal{
		Version: componentmap.ContractVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Application",
			Components: []componentmap.ProposedComponent{{
				Name: "Runtime", MemberIDs: []componentmap.MemberID{bundle.Candidates[0].ID},
			}},
		}},
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
