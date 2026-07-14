package main

import (
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
	calls    int
	response []byte
	err      error
}

func (stub *architectureSynthesisStub) SynthesizeComponentLandscapeMeasured(
	_ context.Context,
	_ componentmap.SynthesisPrompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{Content: append([]byte(nil), stub.response...), Attempts: 1}, stub.err
}

func (stub *architectureSynthesisStub) ComponentSynthesisPromptJSON(prompt componentmap.SynthesisPrompt) ([]byte, error) {
	return json.Marshal(prompt)
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

func TestEnsureArchitectureSynthesisPersistsDeterministicFallbackForInvalidOutput(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{response: []byte("not json")}
	runDir := filepath.Join(t.TempDir(), "run")
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

func TestEnsureArchitectureSynthesisDoesNotRetryCorruptSavedRecord(t *testing.T) {
	t.Parallel()

	bundle := architectureSynthesisTestBundle()
	provider := &architectureSynthesisStub{err: errors.New("must not be called")}
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheKey, err := componentmap.SynthesisCacheKeyForProvider(
		"revision-corrupt", bundle, "openai-compatible/bearer", "test-model",
	)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(runsDir, architectureSynthesisCacheDirectory)
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), []byte(`{"broken"`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = ensureArchitectureSynthesis(
		context.Background(), bundle, runDir, "revision-corrupt",
		"openai-compatible/bearer", "test-model", provider,
	)
	if err == nil || !strings.Contains(err.Error(), "without another provider call") {
		t.Fatalf("error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
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
