package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

type guidedTourEditorStub struct {
	response  []byte
	responses [][]byte
	calls     int
}

func (s *guidedTourEditorStub) GuidedTourPromptJSON(prompt guidedtour.Prompt) ([]byte, error) {
	return json.Marshal(prompt)
}

func (s *guidedTourEditorStub) EditGuidedTourMeasured(
	_ context.Context,
	_ guidedtour.Prompt,
) (modelresearch.ProviderResult, error) {
	s.calls++
	response := s.response
	if len(s.responses) >= s.calls {
		response = s.responses[s.calls-1]
	}
	return modelresearch.ProviderResult{
		Content: response, InputTokens: 120, OutputTokens: 80, Attempts: 1,
	}, nil
}

func TestEnsureGuidedTourCachesOnlyValidatedProposal(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, false)}

	first, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || !second.Cached || provider.calls != 1 {
		t.Fatalf("outcomes cached=%t/%t calls=%d", first.Cached, second.Cached, provider.calls)
	}
	saved, err := os.ReadFile(filepath.Join(runDir, report.GuidedStoryFile))
	if err != nil {
		t.Fatal(err)
	}
	story, err := guidedtour.ReplayRecord(bundle, saved)
	if err != nil {
		t.Fatal(err)
	}
	if story.CandidateID != bundle.Candidates[0].ID || len(story.Steps) != 3 {
		t.Fatalf("story = %#v", story)
	}
}

func TestEnsureGuidedTourSelfHealsSemanticallyInvalidCache(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	cacheInput := guidedTourMonolithicCacheInput(t, bundle, runDir, "test", "fixture-model")
	if _, err := modelresearch.SaveStageResponse(cacheInput, modelresearch.StageResponse{
		Content: guidedTourTestProposal(t, bundle, true),
	}); err != nil {
		t.Fatal(err)
	}

	invalidProvider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, true)}
	if _, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", invalidProvider,
	); err == nil {
		t.Fatal("semantically invalid replacement was accepted")
	}
	if invalidProvider.calls != 2 {
		t.Fatalf("ordinary bounded proposal attempts = %d, want 2", invalidProvider.calls)
	}
	if _, found, err := modelresearch.LoadStageResponse(cacheInput); err != nil || found {
		t.Fatalf("invalid replacement cache = found %t, err %v", found, err)
	}

	provider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, false)}
	refetched, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if refetched.Cached || refetched.SemanticCalls != 1 || provider.calls != 1 {
		t.Fatalf("self-healed monolith = %#v, provider calls %d", refetched, provider.calls)
	}

	warmProvider := &guidedTourEditorStub{}
	warm, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", warmProvider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !warm.Cached || warm.SemanticCalls != 0 || warmProvider.calls != 0 {
		t.Fatalf("valid warm monolith = %#v, provider calls %d", warm, warmProvider.calls)
	}
}

func TestEnsureGuidedTourCachesOneCanonicalEnglishStory(t *testing.T) {
	runsDir := t.TempDir()
	bundle := guidedTourTestBundle()
	english := guidedTourTestProposal(t, bundle, false)
	provider := &guidedTourEditorStub{response: english}

	run := func(name string) (guidedTourOutcome, guidedtour.Story) {
		t.Helper()
		runDir := filepath.Join(runsDir, name)
		outcome, runErr := ensureGuidedTourWithOptions(
			context.Background(),
			bundle,
			runDir,
			"test",
			"fixture-model",
			provider,
			guidedTourRunOptions{},
		)
		if runErr != nil {
			t.Fatal(runErr)
		}
		saved, readErr := os.ReadFile(filepath.Join(runDir, report.GuidedStoryFile))
		if readErr != nil {
			t.Fatal(readErr)
		}
		story, replayErr := guidedtour.ReplayRecord(bundle, saved)
		if replayErr != nil {
			t.Fatal(replayErr)
		}
		return outcome, story
	}

	first, canonicalStory := run("canonical-first")
	replayedForEnglish, englishStory := run("en-replay")
	replayedForRussian, russianPresentationSource := run("ru-replay")

	if first.Cached ||
		!replayedForEnglish.Cached || !replayedForRussian.Cached ||
		provider.calls != 1 {
		t.Fatalf(
			"cache hits = %t/%t/%t, provider calls = %d",
			first.Cached,
			replayedForEnglish.Cached,
			replayedForRussian.Cached,
			provider.calls,
		)
	}
	if englishStory.Title != canonicalStory.Title ||
		russianPresentationSource.Title != canonicalStory.Title {
		t.Fatalf(
			"cached canonical story titles = %q/%q, original = %q",
			englishStory.Title,
			russianPresentationSource.Title,
			canonicalStory.Title,
		)
	}
}

func TestEnsureGuidedTourNoCacheCallsProviderPerRun(t *testing.T) {
	runsDir := t.TempDir()
	bundle := guidedTourTestBundle()
	provider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, false)}

	for _, name := range []string{"run-one", "run-two"} {
		runDir := filepath.Join(runsDir, name)
		outcome, err := ensureGuidedTourWithOptions(
			context.Background(), bundle, runDir, "test", "fixture-model", provider,
			guidedTourRunOptions{disableCache: true},
		)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Cached {
			t.Fatalf("no-cache outcome = %#v", outcome)
		}
		if _, err := os.Stat(filepath.Join(runDir, report.GuidedStoryFile)); err != nil {
			t.Fatalf("per-run guided story: %v", err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one call per run", provider.calls)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(runsDir, ".model-research", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 0 {
		t.Fatalf("no-cache populated shared model cache: %v", cacheFiles)
	}
}

func TestEnsureGuidedTourRejectsInventedReferenceWithoutSavingIt(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, true)}

	if _, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	); err == nil {
		t.Fatal("ensureGuidedTour() error = nil")
	}
	if _, err := os.Stat(filepath.Join(runDir, report.GuidedStoryFile)); !os.IsNotExist(err) {
		t.Fatalf("invalid proposal artifact exists or stat failed: %v", err)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(filepath.Dir(runDir), ".model-research", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 0 {
		t.Fatalf("invalid proposal populated cache: %v", cacheFiles)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one bounded validation retry", provider.calls)
	}
}

func TestEnsureGuidedTourReportsTypedPathLikeRejection(t *testing.T) {
	t.Parallel()

	bundle := guidedTourTestBundle()
	var proposal guidedtour.Proposal
	if err := json.Unmarshal(guidedTourTestProposal(t, bundle, false), &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Steps[0].Explanation = "Inspect cmd/server.go before continuing."
	rejected, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(t.TempDir(), "run")
	provider := &guidedTourEditorStub{response: rejected}
	outcome, runErr := ensureGuidedTourWithOptions(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
		guidedTourRunOptions{disableCache: true},
	)
	if runErr == nil {
		t.Fatal("path-like proposal was accepted")
	}
	if outcome.ValidatorField != "steps[0].explanation" ||
		outcome.ValidatorRule != guidedtour.ValidationRulePathLikeReference {
		t.Fatalf("typed validator outcome = %#v", outcome)
	}
}

func TestEnsureGuidedTourRetriesRejectedProposalAndPublishesOnlyValidResult(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	bundle := guidedTourTestBundle()
	provider := &guidedTourEditorStub{responses: [][]byte{
		guidedTourTestProposal(t, bundle, true),
		guidedTourTestProposal(t, bundle, false),
	}}

	outcome, err := ensureGuidedTourWithOptions(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
		guidedTourRunOptions{disableCache: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || outcome.RetryCount != 1 || outcome.ValidationState != "accepted" {
		t.Fatalf("provider calls = %d, outcome = %#v", provider.calls, outcome)
	}
	saved, err := os.ReadFile(filepath.Join(runDir, report.GuidedStoryFile))
	if err != nil {
		t.Fatal(err)
	}
	story, err := guidedtour.ReplayRecord(bundle, saved)
	if err != nil {
		t.Fatalf("saved retry result is not valid: %v", err)
	}
	if story.CandidateID != bundle.Candidates[0].ID {
		t.Fatalf("story = %#v", story)
	}
}

func TestEnsureGuidedTourUpgradesLegacyRunForBoundedFifthCall(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	legacy := modelresearch.DefaultPolicy()
	legacy.GuidedTour = modelresearch.StageBudget{}
	legacy.MaxGuidedTourCalls = 0
	legacy.MaxGuidedTourBytes = 0
	legacy.MaxSemanticCalls = 4
	legacy.MaxTotalRequestBytes -= 256 << 10
	state := modelresearch.NewState(legacy, modelresearch.RepositoryContext{
		Identity: t.TempDir(), Revision: "abc", Scenario: "go-default",
	})
	state.Usage = modelresearch.Usage{SemanticCalls: 4, RequestBytes: 128 << 10}
	if err := modelresearch.WriteState(runDir, state); err != nil {
		t.Fatal(err)
	}
	bundle := guidedTourTestBundle()
	provider := &guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, false)}

	if _, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	); err != nil {
		t.Fatal(err)
	}
	accepted, err := modelresearch.ReadState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Policy.MaxSemanticCalls != 5 || accepted.Usage.SemanticCalls != 5 ||
		accepted.GuidedTour.Status != "accepted" || accepted.GuidedTour.SemanticCalls != 1 {
		t.Fatalf("accepted model research = %#v", accepted)
	}
	replayed, err := ensureGuidedTour(
		context.Background(), bundle, runDir, "test", "fixture-model", provider,
	)
	if err != nil {
		t.Fatalf("cache replay at the consumed fifth-call budget: %v", err)
	}
	if !replayed.Cached || provider.calls != 1 {
		t.Fatalf("replayed = %#v, provider calls = %d", replayed, provider.calls)
	}
	updated, err := modelresearch.ReadState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Policy.MaxSemanticCalls != 5 || updated.Usage.SemanticCalls != 5 ||
		updated.GuidedTour.Status != "cached" || !updated.GuidedTour.CacheHit {
		t.Fatalf("updated model research = %#v", updated)
	}
}

func guidedTourTestBundle() guidedtour.Bundle {
	beats := make([]guidedtour.Beat, 0, 3)
	for index, id := range []string{"collect", "interpret", "render"} {
		beats = append(beats, guidedtour.Beat{
			ID: id, Kind: "existing_fact", Label: id, Detail: "bounded existing artifact",
			Sequence: index, ComponentIDs: []string{"component-1"},
			SurfaceIDs: []string{}, FlowStepIDs: []string{}, Evidence: []guidedtour.EvidenceRef{{
				ID: "evidence-" + id, Kind: "saved_fact", Label: "saved exact fact",
			}},
		})
	}
	return guidedtour.Bundle{
		Version: guidedtour.BundleVersion, RepoName: "repomap", CanvasVersion: 5,
		Components: []guidedtour.Component{{
			ID: "component-1", Name: "Analysis pipeline", Description: "bounded component",
		}},
		Candidates: []guidedtour.Candidate{{
			ID: "candidate-1", Name: "From facts to report",
			Kind: guidedtour.CandidateSuggestedDirection, Trigger: "the command starts",
			Summary:       "saved artifacts expose three useful stages",
			OrderingBasis: guidedtour.OrderingEditorial, Beats: beats, Gaps: []guidedtour.Gap{{
				ID: "gap-1", Label: "Runtime order", Detail: "runtime order is not proven",
			}},
		}},
	}
}

func guidedTourMonolithicCacheInput(
	t *testing.T,
	bundle guidedtour.Bundle,
	runDir string,
	profile string,
	model string,
) modelresearch.StageCacheInput {
	t.Helper()
	bundleSHA, _, err := guidedtour.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := guidedtour.BuildPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	policy := modelresearch.DefaultPolicy()
	return modelresearch.StageCacheInput{
		RunsDir: filepath.Dir(runDir),
		Fingerprint: modelresearch.FingerprintInput{
			Repository: modelresearch.RepositoryContext{
				Identity: bundle.RepoName, Revision: "captured-run",
				DirtySHA256: bundleSHA, Scenario: "saved-artifacts",
			},
			Stage: "guided_story_editor", PromptVersion: guidedtour.PromptVersion,
			Profile: profile, Model: model,
			EvidenceBundleHash: bundleSHA, PolicyVersion: policy.Version,
		},
		Request: request, EvidenceBundleHash: bundleSHA,
	}
}

func guidedTourTestProposal(t *testing.T, bundle guidedtour.Bundle, invalid bool) []byte {
	t.Helper()
	beatIDs := []string{
		bundle.Candidates[0].Beats[0].ID,
		bundle.Candidates[0].Beats[1].ID,
		bundle.Candidates[0].Beats[2].ID,
	}
	if invalid {
		beatIDs[2] = "invented-beat"
	}
	proposal := guidedtour.Proposal{
		Version: guidedtour.ProposalVersion, CandidateID: bundle.Candidates[0].ID,
		Title:   "Reading the saved orientation candidate",
		Summary: "This editorial guide follows three supplied static anchors; runtime order remains unproven.",
		Steps: []guidedtour.ProposedStep{
			{Title: "Collect facts", Explanation: "Inspect the supplied collection anchor.", BeatIDs: beatIDs[:1]},
			{Title: "Interpret", Explanation: "Inspect the supplied interpretation anchor.", BeatIDs: beatIDs[1:2]},
			{Title: "Present", Explanation: "Inspect the existing report anchor.", BeatIDs: beatIDs[2:]},
		},
		GapSummary: []guidedtour.ProposedGapSummary{{
			Explanation: "The supplied evidence does not prove runtime order.",
			GapIDs:      []string{"gap-1"},
		}},
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
