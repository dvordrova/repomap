package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

const guidedTourBundleFile = "guided_tour_bundle.json"

const guidedTourMonolithicFile = "guided_story.monolithic.json"

type guidedTourEditor interface {
	GuidedTourPromptJSON(guidedtour.Prompt) ([]byte, error)
	EditGuidedTourMeasured(context.Context, guidedtour.Prompt) (modelresearch.ProviderResult, error)
}

type guidedTourOutcome struct {
	Skipped               bool
	Cached                bool
	Attempted             bool
	SemanticCalls         int
	CacheHits             int
	AttemptedBytes        int
	RequestBytes          int
	ResponseBytes         int
	InputTokens           int
	OutputTokens          int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	UnsupportedClaims     int
	LatencyMillis         int64
	WallMillis            int64
	LeafTasks             int
	LeafSucceeded         int
	LeafInsufficient      int
	LeafFailed            int
	ValidationState       string
}

func editGuidedTourForRun(ctx context.Context, runDir string, stderr io.Writer) (guidedTourOutcome, error) {
	bundle, err := guidedTourBundleForRun(runDir)
	if errors.Is(err, report.ErrNoGuidedTourCandidates) {
		return guidedTourOutcome{Skipped: true}, nil
	}
	if err != nil {
		return guidedTourOutcome{}, err
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return guidedTourOutcome{}, fmt.Errorf("guided tour: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	fmt.Fprintln(stderr, "repomap: editing one bounded onboarding story from saved facts")
	return ensureGuidedTour(
		ctx, bundle, runDir,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
	)
}

func prepareGuidedTour(
	ctx context.Context,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
) (guidedTourOutcome, error) {
	bundle, err := guidedTourBundleForRun(runDir)
	if err != nil {
		return guidedTourOutcome{}, err
	}
	return ensureGuidedTour(ctx, bundle, runDir, profile, model, provider)
}

func guidedTourBundleForRun(runDir string) (guidedtour.Bundle, error) {
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour: read saved run: %w", err)
	}
	bundle, err := report.BuildGuidedTourBundle(data)
	if err != nil {
		if errors.Is(err, report.ErrNoGuidedTourCandidates) {
			return guidedtour.Bundle{}, err
		}
		return guidedtour.Bundle{}, fmt.Errorf("guided tour: build bounded story candidates: %w", err)
	}
	return bundle, nil
}

func ensureGuidedTour(
	ctx context.Context,
	bundle guidedtour.Bundle,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
) (guidedTourOutcome, error) {
	return ensureGuidedTourWithOptions(
		ctx,
		bundle,
		runDir,
		profile,
		model,
		provider,
		guidedTourRunOptions{},
	)
}

type guidedTourRunOptions struct {
	independentExperiment bool
	outputFile            string
}

func ensureGuidedTourWithOptions(
	ctx context.Context,
	bundle guidedtour.Bundle,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
	options guidedTourRunOptions,
) (outcome guidedTourOutcome, returnErr error) {
	started := time.Now()
	defer func() {
		outcome.WallMillis = time.Since(started).Milliseconds()
	}()

	if ctx == nil {
		return guidedTourOutcome{}, fmt.Errorf("guided tour: context is required")
	}
	if err := ctx.Err(); err != nil {
		return guidedTourOutcome{}, err
	}
	if provider == nil {
		return guidedTourOutcome{}, fmt.Errorf("guided tour: provider is required")
	}
	bundleSHA, canonicalBundle, err := guidedtour.BundleHash(bundle)
	if err != nil {
		return guidedTourOutcome{}, err
	}
	if err := writeGuidedTourArtifact(filepath.Join(runDir, guidedTourBundleFile), append(canonicalBundle, '\n')); err != nil {
		return guidedTourOutcome{}, err
	}
	prompt, err := guidedtour.BuildPrompt(bundle)
	if err != nil {
		return guidedTourOutcome{}, err
	}
	request, err := provider.GuidedTourPromptJSON(prompt)
	if err != nil {
		return guidedTourOutcome{}, fmt.Errorf("guided tour: build provider request: %w", err)
	}

	policy := modelresearch.DefaultPolicy()
	usage := modelresearch.Usage{}
	repository := modelresearch.RepositoryContext{
		Identity: bundle.RepoName, Revision: "captured-run",
		DirtySHA256: bundleSHA, Scenario: "saved-artifacts",
	}
	if !options.independentExperiment {
		if state, stateErr := modelresearch.ReadState(runDir); stateErr == nil {
			policy, err = state.Policy.WithGuidedTour()
			if err != nil {
				return guidedTourOutcome{}, fmt.Errorf("guided tour: upgrade research policy: %w", err)
			}
			usage = state.Usage
			repository = state.Repository
		} else if !errors.Is(stateErr, os.ErrNotExist) {
			return guidedTourOutcome{}, fmt.Errorf("guided tour: read research budget: %w", stateErr)
		}
	}

	outcome = guidedTourOutcome{RequestBytes: len(request)}
	cacheInput := modelresearch.StageCacheInput{
		RunsDir: filepath.Dir(runDir),
		Fingerprint: modelresearch.FingerprintInput{
			Repository: repository, Stage: "guided_story_editor",
			PromptVersion: guidedtour.PromptVersion,
			Profile:       profile, Model: model,
			EvidenceBundleHash: bundleSHA, PolicyVersion: policy.Version,
		},
		Request: request, EvidenceBundleHash: bundleSHA,
	}
	if cached, found, cacheErr := modelresearch.LoadStageResponse(cacheInput); cacheErr != nil {
		outcome.ValidationState = "invalid_cache"
		return outcome, fmt.Errorf("guided tour: reject optional cache without another provider call: %w", cacheErr)
	} else if found {
		outcome.Cached = true
		outcome.CacheHits = 1
		outcome.ResponseBytes = cached.ResponseBytes
		outcome.InputTokens = cached.InputTokens
		outcome.OutputTokens = cached.OutputTokens
		outcome.PromptCacheHitTokens = cached.PromptCacheHitTokens
		outcome.PromptCacheMissTokens = cached.PromptCacheMissTokens
		outcome.UnsupportedClaims = countGuidedTourProposalUnsupportedClaims(bundle, cached.Content)
		outcome.LatencyMillis = cached.LatencyMillis
		if err := saveGuidedTourRecordTo(bundle, cached.Content, runDir, options.outputFile); err != nil {
			outcome.ValidationState = "invalid_cached_response"
			return outcome, fmt.Errorf("guided tour: reject cached response: %w", err)
		}
		outcome.ValidationState = "cached"
		if !options.independentExperiment {
			if err := recordGuidedTourResearch(runDir, outcome, policy, usage); err != nil {
				return outcome, err
			}
		}
		return outcome, nil
	}
	if allowed, reason := policy.Allows(policy.GuidedTour, usage, len(request)); !allowed {
		outcome.ValidationState = "skipped_" + reason
		budgetErr := fmt.Errorf("guided tour: %s", reason)
		if !options.independentExperiment {
			if recordErr := recordGuidedTourResearch(runDir, outcome, policy, usage); recordErr != nil {
				return outcome, errors.Join(budgetErr, recordErr)
			}
		}
		return outcome, budgetErr
	}

	outcome.Attempted = true
	outcome.SemanticCalls = 1
	outcome.AttemptedBytes = len(request)
	providerStarted := time.Now()
	providerResult, callErr := provider.EditGuidedTourMeasured(ctx, prompt)
	outcome.LatencyMillis = time.Since(providerStarted).Milliseconds()
	outcome.ResponseBytes = len(providerResult.Content)
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.PromptCacheHitTokens = providerResult.PromptCacheHitTokens
	outcome.PromptCacheMissTokens = providerResult.PromptCacheMissTokens
	outcome.UnsupportedClaims = countGuidedTourProposalUnsupportedClaims(bundle, providerResult.Content)
	if err := ctx.Err(); err != nil {
		return outcome, err
	}
	if callErr != nil {
		outcome.ValidationState = "failed"
		err := fmt.Errorf("guided tour: provider call: %w", callErr)
		if !options.independentExperiment {
			if recordErr := recordGuidedTourResearch(runDir, outcome, policy, usage); recordErr != nil {
				return outcome, errors.Join(err, recordErr)
			}
		}
		return outcome, err
	}
	if err := validateGuidedTourResponse(bundle, providerResult.Content); err != nil {
		outcome.ValidationState = "rejected"
		validationErr := fmt.Errorf("guided tour: validate response: %w", err)
		if !options.independentExperiment {
			if recordErr := recordGuidedTourResearch(runDir, outcome, policy, usage); recordErr != nil {
				return outcome, errors.Join(validationErr, recordErr)
			}
		}
		return outcome, validationErr
	}
	if _, err := modelresearch.SaveStageResponse(cacheInput, modelresearch.StageResponse{
		Content:     providerResult.Content,
		InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
		PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
		PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
		LatencyMillis:         outcome.LatencyMillis,
	}); err != nil {
		return outcome, fmt.Errorf("guided tour: save validated cache: %w", err)
	}
	if err := saveGuidedTourRecordTo(bundle, providerResult.Content, runDir, options.outputFile); err != nil {
		return outcome, err
	}
	outcome.ValidationState = "accepted"
	if !options.independentExperiment {
		if err := recordGuidedTourResearch(runDir, outcome, policy, usage); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

func validateGuidedTourResponse(bundle guidedtour.Bundle, raw []byte) error {
	proposal, err := guidedtour.ParseProposal(raw)
	if err != nil {
		return err
	}
	return guidedtour.ValidateProposal(bundle, proposal)
}

func countGuidedTourProposalUnsupportedClaims(bundle guidedtour.Bundle, raw []byte) int {
	proposal, err := guidedtour.ParseProposal(raw)
	if err != nil {
		return 0
	}
	return guidedtour.UnsupportedBehaviorClaimCount(bundle, proposal)
}

func saveGuidedTourRecord(bundle guidedtour.Bundle, raw []byte, runDir string) error {
	return saveGuidedTourRecordTo(bundle, raw, runDir, report.GuidedStoryFile)
}

func saveGuidedTourRecordTo(bundle guidedtour.Bundle, raw []byte, runDir, outputFile string) error {
	proposal, err := guidedtour.ParseProposal(raw)
	if err != nil {
		return err
	}
	record, err := guidedtour.EncodeRecord(bundle, proposal)
	if err != nil {
		return err
	}
	if outputFile == "" {
		outputFile = report.GuidedStoryFile
	}
	return writeGuidedTourArtifact(
		filepath.Join(runDir, outputFile),
		append(record, '\n'),
	)
}

func recordGuidedTourResearch(
	runDir string,
	outcome guidedTourOutcome,
	policy modelresearch.Policy,
	usage modelresearch.Usage,
) error {
	state, err := modelresearch.ReadState(runDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	state.GuidedTour = modelresearch.StageMetrics{
		Stage: "guided_story_editor", Status: outcome.ValidationState,
		RequestBytes: outcome.RequestBytes, ResponseBytes: outcome.ResponseBytes,
		InputTokens: outcome.InputTokens, OutputTokens: outcome.OutputTokens,
		PromptCacheHitTokens:  outcome.PromptCacheHitTokens,
		PromptCacheMissTokens: outcome.PromptCacheMissTokens,
		LatencyMillis:         outcome.LatencyMillis, CacheHit: outcome.Cached,
	}
	if outcome.SemanticCalls > 0 {
		state.GuidedTour.SemanticCalls = outcome.SemanticCalls
		state.Usage.SemanticCalls = usage.SemanticCalls + outcome.SemanticCalls
		state.Usage.RequestBytes = usage.RequestBytes + outcome.AttemptedBytes
	}
	state.Policy = policy
	return modelresearch.WriteState(runDir, state)
}

func writeGuidedTourArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("guided tour: create artifact directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".repomap-guided-tour-")
	if err != nil {
		return fmt.Errorf("guided tour: create temporary artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("guided tour: protect temporary artifact: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("guided tour: write artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("guided tour: close artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("guided tour: replace artifact: %w", err)
	}
	return nil
}
