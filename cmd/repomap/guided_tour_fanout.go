package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const (
	guidedTourExperimentLeafLimit  = guidedtour.MaxLeafTasks
	guidedTourFanoutConcurrency    = 3
	guidedTourFanoutAggregateBytes = 1 << 20

	guidedTourFanoutFile       = "guided_story.fanout.json"
	guidedTourFanoutLeavesFile = "guided_tour_fanout_leaves.json"
	guidedTourFanInFile        = "guided_tour_fan_in.json"
)

type guidedTourLeafFailure struct {
	TaskID string              `json:"task_id"`
	Kind   guidedtour.LeafKind `json:"kind"`
	Reason string              `json:"reason"`
}

type guidedTourFanoutArtifact struct {
	Version      int                     `json:"version"`
	BundleSHA256 string                  `json:"bundle_sha256"`
	LeafLimit    int                     `json:"leaf_limit"`
	Results      []guidedtour.LeafResult `json:"validated_results"`
	Failures     []guidedTourLeafFailure `json:"failures"`
}

type guidedTourLeafCall struct {
	task       guidedtour.LeafTask
	prompt     guidedtour.Prompt
	cacheInput modelresearch.StageCacheInput
}

type guidedTourLeafCompletion struct {
	result      guidedtour.LeafResult
	response    modelresearch.StageResponse
	cacheInput  modelresearch.StageCacheInput
	failure     *guidedTourLeafFailure
	taskID      string
	terminalErr error
}

func ensureGuidedTourFanoutExperiment(
	ctx context.Context,
	bundle guidedtour.Bundle,
	runDir string,
	profile string,
	model string,
	provider guidedTourEditor,
	providerEndpointSHA256 string,
) (outcome guidedTourOutcome, returnErr error) {
	started := time.Now()
	defer func() {
		outcome.WallMillis = time.Since(started).Milliseconds()
	}()

	if ctx == nil {
		return outcome, fmt.Errorf("guided tour fan-out: context is required")
	}
	if err := ctx.Err(); err != nil {
		return outcome, err
	}
	if provider == nil {
		return outcome, fmt.Errorf("guided tour fan-out: provider is required")
	}
	bundleSHA, canonicalBundle, err := guidedtour.BundleHash(bundle)
	if err != nil {
		return outcome, err
	}
	if err := removeGuidedTourFanoutOutputs(runDir); err != nil {
		return outcome, err
	}
	if err := writeGuidedTourArtifact(
		filepath.Join(runDir, guidedTourBundleFile),
		append(canonicalBundle, '\n'),
	); err != nil {
		return outcome, err
	}
	tasks, err := guidedtour.PlanLeafTasks(bundle, guidedTourExperimentLeafLimit)
	if err != nil {
		return outcome, err
	}
	outcome.LeafTasks = len(tasks)

	policy, err := modelresearch.DefaultPolicy().WithGuidedTourBudget(
		len(tasks)+1,
		guidedTourFanoutAggregateBytes,
	)
	if err != nil {
		return outcome, fmt.Errorf("guided tour fan-out: configure bounded experiment: %w", err)
	}
	repository := modelresearch.RepositoryContext{
		Identity: bundle.RepoName, Revision: "captured-run",
		DirtySHA256: bundleSHA, Scenario: "saved-artifacts",
	}
	runsDir := filepath.Dir(runDir)
	validResults := make([]guidedtour.LeafResult, 0, len(tasks))
	failures := make([]guidedTourLeafFailure, 0)
	misses := make([]guidedTourLeafCall, 0, len(tasks))

	for _, task := range tasks {
		prompt, promptErr := guidedtour.BuildLeafPrompt(task)
		if promptErr != nil {
			return outcome, promptErr
		}
		request, requestErr := provider.GuidedTourPromptJSON(prompt)
		if requestErr != nil {
			return outcome, fmt.Errorf("guided tour fan-out: build leaf request: %w", requestErr)
		}
		taskSHA, _, hashErr := guidedtour.LeafTaskHash(task)
		if hashErr != nil {
			return outcome, hashErr
		}
		cacheInput := modelresearch.StageCacheInput{
			RunsDir: runsDir,
			Fingerprint: modelresearch.FingerprintInput{
				Repository: repository, Stage: "guided_story_leaf/" + task.ID,
				PromptVersion: guidedtour.LeafPromptVersion,
				Profile:       profile, Model: model,
				ProviderEndpointSHA256: providerEndpointSHA256,
				RequestSHA256:          modelresearch.SHA256(request),
				EvidenceBundleHash:     taskSHA, PolicyVersion: policy.Version,
			},
			Request: request, EvidenceBundleHash: taskSHA,
		}
		outcome.RequestBytes += len(request)
		cached, found, cacheErr := modelresearch.LoadStageResponse(cacheInput)
		if cacheErr != nil {
			failures = append(failures, guidedTourLeafFailure{
				TaskID: task.ID, Kind: task.Kind,
				Reason: "invalid cache rejected without a replacement call",
			})
			continue
		}
		if !found {
			misses = append(misses, guidedTourLeafCall{
				task: task, prompt: prompt, cacheInput: cacheInput,
			})
			continue
		}
		artifact, parseErr := guidedtour.ParseLeafArtifact(cached.Content)
		if parseErr == nil {
			artifact = guidedtour.NormalizeLeafArtifact(artifact)
			parseErr = guidedtour.ValidateLeafArtifact(task, artifact)
		}
		if parseErr != nil {
			if isSemanticResourceLimit(parseErr) {
				return terminateGuidedTourFanoutResourceLimit(
					runDir,
					outcome,
					fmt.Errorf("guided tour fan-out: cached leaf resource limit: %w", parseErr),
				)
			}
			if err := modelresearch.InvalidateStageResponse(cacheInput); err != nil {
				return outcome, fmt.Errorf(
					"guided tour fan-out: invalidate rejected leaf cache %q: %w",
					task.ID,
					err,
				)
			}
			misses = append(misses, guidedTourLeafCall{
				task: task, prompt: prompt, cacheInput: cacheInput,
			})
			continue
		}
		outcome.CacheHits++
		addGuidedTourResponseMetrics(&outcome, cached)
		validResults = append(validResults, guidedtour.LeafResult{Task: task, Artifact: artifact})
	}

	reservedCalls := 0
	reservedBytes := 0
	runnable := make([]guidedTourLeafCall, 0, len(misses))
	for _, call := range misses {
		requestBytes := len(call.cacheInput.Request)
		if allowed, reason := allowsFanoutLeafCall(
			policy,
			reservedCalls,
			reservedBytes,
			requestBytes,
		); !allowed {
			failures = append(failures, guidedTourLeafFailure{
				TaskID: call.task.ID, Kind: call.task.Kind,
				Reason: "leaf skipped: " + reason,
			})
			continue
		}
		reservedCalls++
		reservedBytes += requestBytes
		runnable = append(runnable, call)
	}
	outcome.SemanticCalls += len(runnable)
	outcome.AttemptedBytes += reservedBytes
	outcome.Attempted = len(runnable) > 0

	completionChannel := make(chan guidedTourLeafCompletion, len(runnable))
	semaphore := make(chan struct{}, guidedTourFanoutConcurrency)
	var wait sync.WaitGroup
	for _, call := range runnable {
		call := call
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				completionChannel <- guidedTourLeafCompletion{failure: &guidedTourLeafFailure{
					TaskID: call.task.ID, Kind: call.task.Kind, Reason: "leaf canceled",
				}}
				return
			}
			completionChannel <- executeGuidedTourLeaf(ctx, provider, call)
		}()
	}
	wait.Wait()
	close(completionChannel)
	terminal := make([]guidedTourLeafCompletion, 0)
	successful := make([]guidedTourLeafCompletion, 0)
	for completion := range completionChannel {
		addGuidedTourResponseMetrics(&outcome, completion.response)
		if completion.terminalErr != nil {
			terminal = append(terminal, completion)
			continue
		}
		if completion.failure != nil {
			failures = append(failures, *completion.failure)
			continue
		}
		successful = append(successful, completion)
	}
	if len(terminal) > 0 {
		sort.Slice(terminal, func(i, j int) bool {
			return terminal[i].taskID < terminal[j].taskID
		})
		return terminateGuidedTourFanoutResourceLimit(
			runDir,
			outcome,
			fmt.Errorf("guided tour fan-out: leaf resource limit: %w", terminal[0].terminalErr),
		)
	}
	sort.Slice(successful, func(i, j int) bool {
		return successful[i].taskID < successful[j].taskID
	})
	for _, completion := range successful {
		if _, err := modelresearch.SaveStageResponse(completion.cacheInput, completion.response); err != nil {
			failures = append(failures, guidedTourLeafFailure{
				TaskID: completion.result.Task.ID,
				Kind:   completion.result.Task.Kind,
				Reason: "validated leaf cache write failed",
			})
			continue
		}
		validResults = append(validResults, completion.result)
	}
	if err := ctx.Err(); err != nil {
		return outcome, err
	}

	sort.Slice(validResults, func(i, j int) bool {
		return validResults[i].Task.ID < validResults[j].Task.ID
	})
	sort.Slice(failures, func(i, j int) bool {
		return failures[i].TaskID < failures[j].TaskID
	})
	outcome.LeafSucceeded = len(validResults)
	outcome.LeafFailed = len(failures)
	for _, result := range validResults {
		if len(result.Artifact.Observations) == 0 && len(result.Artifact.MissingEvidence) > 0 {
			outcome.LeafInsufficient++
		}
	}
	if err := writeGuidedTourFanoutArtifact(
		runDir,
		bundleSHA,
		validResults,
		failures,
	); err != nil {
		return outcome, err
	}
	if len(validResults) == 0 {
		outcome.ValidationState = "rejected_no_valid_leaves"
		return outcome, fmt.Errorf("guided tour fan-out: no leaf passed local validation")
	}

	finalPrompt, err := guidedtour.BuildFanInPrompt(bundle, validResults)
	if err != nil {
		return outcome, err
	}
	finalRequest, err := provider.GuidedTourPromptJSON(finalPrompt)
	if err != nil {
		return outcome, fmt.Errorf("guided tour fan-out: build fan-in request: %w", err)
	}
	outcome.RequestBytes += len(finalRequest)
	if len(finalRequest) > policy.GuidedTour.MaxRequestBytes {
		outcome.ValidationState = "skipped_fan_in_stage_byte_budget_exhausted"
		return outcome, fmt.Errorf(
			"guided tour fan-out: fan-in skipped: stage_byte_budget_exhausted",
		)
	}
	finalFactsSHA := modelresearch.SHA256([]byte(finalPrompt.System + "\x00" + finalPrompt.User))
	finalCache := modelresearch.StageCacheInput{
		RunsDir: runsDir,
		Fingerprint: modelresearch.FingerprintInput{
			Repository: repository, Stage: "guided_story_fan_in",
			PromptVersion: guidedtour.FanInPromptVersion,
			Profile:       profile, Model: model,
			ProviderEndpointSHA256: providerEndpointSHA256,
			RequestSHA256:          modelresearch.SHA256(finalRequest),
			EvidenceBundleHash:     finalFactsSHA, PolicyVersion: policy.Version,
		},
		Request: finalRequest, EvidenceBundleHash: finalFactsSHA,
	}
	cached, found, cacheErr := modelresearch.LoadStageResponse(finalCache)
	if cacheErr != nil {
		outcome.ValidationState = "invalid_fan_in_cache"
		return outcome, fmt.Errorf(
			"guided tour fan-out: reject fan-in cache without another provider call: %w",
			cacheErr,
		)
	}
	var cachedFinalArtifact guidedtour.FanInArtifact
	if found {
		cachedFinalArtifact, cacheErr = validateGuidedTourFanInResponse(
			bundle,
			validResults,
			cached.Content,
		)
		if cacheErr != nil {
			if isSemanticResourceLimit(cacheErr) {
				return terminateGuidedTourFanoutResourceLimit(
					runDir,
					outcome,
					fmt.Errorf("guided tour fan-out: cached fan-in resource limit: %w", cacheErr),
				)
			}
			if err := modelresearch.InvalidateStageResponse(finalCache); err != nil {
				outcome.ValidationState = "invalid_fan_in_cache"
				return outcome, fmt.Errorf("guided tour fan-out: invalidate rejected fan-in cache: %w", err)
			}
			found = false
		}
	}
	if found {
		outcome.CacheHits++
		addGuidedTourResponseMetrics(&outcome, cached)
		outcome.UnsupportedClaims += countGuidedTourFanInUnsupportedClaims(bundle, cached.Content)
		if err := writeGuidedTourFanInArtifact(runDir, cachedFinalArtifact); err != nil {
			return outcome, err
		}
		outcome.Cached = outcome.SemanticCalls == 0 && outcome.LeafFailed == 0
		if cachedFinalArtifact.Verdict == guidedtour.FanInVerdictInsufficientEvidence {
			outcome.ValidationState = "insufficient_evidence"
			return outcome, fmt.Errorf(
				"guided tour fan-out: fan-in concluded insufficient evidence",
			)
		}
		if err := saveGuidedTourFanInStory(bundle, cachedFinalArtifact, runDir); err != nil {
			return outcome, err
		}
		if outcome.LeafFailed > 0 {
			outcome.ValidationState = "cached_partial"
		} else {
			outcome.ValidationState = "cached"
		}
		return outcome, nil
	}

	if allowed, reason := allowsFanoutCall(
		policy,
		reservedCalls,
		reservedBytes,
		len(finalRequest),
	); !allowed {
		outcome.ValidationState = "skipped_fan_in_" + reason
		return outcome, fmt.Errorf("guided tour fan-out: fan-in skipped: %s", reason)
	}
	outcome.Attempted = true
	outcome.SemanticCalls++
	outcome.AttemptedBytes += len(finalRequest)
	providerStarted := time.Now()
	providerResult, callErr := provider.EditGuidedTourMeasured(ctx, finalPrompt)
	finalResponse := modelresearch.StageResponse{
		Content:     providerResult.Content,
		InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
		PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
		PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
		LatencyMillis:         time.Since(providerStarted).Milliseconds(),
	}
	addGuidedTourResponseMetrics(&outcome, finalResponse)
	outcome.UnsupportedClaims += countGuidedTourFanInUnsupportedClaims(bundle, providerResult.Content)
	if err := ctx.Err(); err != nil {
		return outcome, err
	}
	if callErr != nil {
		outcome.ValidationState = "failed_fan_in"
		wrapped := fmt.Errorf("guided tour fan-out: fan-in provider call: %w", callErr)
		if isSemanticResourceLimit(callErr) {
			return terminateGuidedTourFanoutResourceLimit(runDir, outcome, wrapped)
		}
		return outcome, wrapped
	}
	artifact, err := validateGuidedTourFanInResponse(
		bundle,
		validResults,
		providerResult.Content,
	)
	if err != nil {
		outcome.ValidationState = "rejected_fan_in"
		wrapped := fmt.Errorf("guided tour fan-out: validate fan-in response: %w", err)
		if isSemanticResourceLimit(err) {
			return terminateGuidedTourFanoutResourceLimit(runDir, outcome, wrapped)
		}
		return outcome, wrapped
	}
	if _, err := modelresearch.SaveStageResponse(finalCache, finalResponse); err != nil {
		return outcome, fmt.Errorf("guided tour fan-out: save validated fan-in cache: %w", err)
	}
	if err := writeGuidedTourFanInArtifact(runDir, artifact); err != nil {
		return outcome, err
	}
	if artifact.Verdict == guidedtour.FanInVerdictInsufficientEvidence {
		outcome.ValidationState = "insufficient_evidence"
		return outcome, fmt.Errorf(
			"guided tour fan-out: fan-in concluded insufficient evidence",
		)
	}
	if err := saveGuidedTourFanInStory(bundle, artifact, runDir); err != nil {
		return outcome, err
	}
	if outcome.LeafFailed > 0 {
		outcome.ValidationState = "accepted_partial"
	} else {
		outcome.ValidationState = "accepted"
	}
	return outcome, nil
}

func validateGuidedTourFanInResponse(
	bundle guidedtour.Bundle,
	results []guidedtour.LeafResult,
	raw []byte,
) (guidedtour.FanInArtifact, error) {
	artifact, err := guidedtour.ParseFanInArtifact(raw)
	if err != nil {
		return guidedtour.FanInArtifact{}, err
	}
	if err := guidedtour.ValidateFanInArtifact(bundle, results, artifact); err != nil {
		return guidedtour.FanInArtifact{}, err
	}
	return artifact, nil
}

func countGuidedTourFanInUnsupportedClaims(bundle guidedtour.Bundle, raw []byte) int {
	artifact, err := guidedtour.ParseFanInArtifact(raw)
	if err != nil || artifact.Proposal == nil {
		return 0
	}
	return guidedtour.UnsupportedBehaviorClaimCount(bundle, *artifact.Proposal)
}

func executeGuidedTourLeaf(
	ctx context.Context,
	provider guidedTourEditor,
	call guidedTourLeafCall,
) guidedTourLeafCompletion {
	started := time.Now()
	providerResult, err := provider.EditGuidedTourMeasured(ctx, call.prompt)
	response := modelresearch.StageResponse{
		Content:     providerResult.Content,
		InputTokens: providerResult.InputTokens, OutputTokens: providerResult.OutputTokens,
		PromptCacheHitTokens:  providerResult.PromptCacheHitTokens,
		PromptCacheMissTokens: providerResult.PromptCacheMissTokens,
		LatencyMillis:         time.Since(started).Milliseconds(),
	}
	if err != nil {
		if isSemanticResourceLimit(err) {
			return guidedTourLeafCompletion{
				response: response, taskID: call.task.ID,
				terminalErr: fmt.Errorf(
					"guided tour fan-out: leaf provider call: %w",
					err,
				),
			}
		}
		return guidedTourLeafCompletion{
			response: response,
			failure: &guidedTourLeafFailure{
				TaskID: call.task.ID, Kind: call.task.Kind, Reason: "leaf provider call failed",
			},
		}
	}
	artifact, err := guidedtour.ParseLeafArtifact(providerResult.Content)
	if err == nil {
		artifact = guidedtour.NormalizeLeafArtifact(artifact)
		err = guidedtour.ValidateLeafArtifact(call.task, artifact)
	}
	if err != nil {
		if isSemanticResourceLimit(err) {
			return guidedTourLeafCompletion{
				response: response, taskID: call.task.ID,
				terminalErr: fmt.Errorf(
					"guided tour fan-out: validate leaf response: %w",
					err,
				),
			}
		}
		return guidedTourLeafCompletion{
			response: response,
			failure: &guidedTourLeafFailure{
				TaskID: call.task.ID, Kind: call.task.Kind,
				Reason: guidedTourLeafFailureReason(
					"leaf response failed local validation",
					err,
				),
			},
		}
	}
	canonicalArtifact, err := json.Marshal(artifact)
	if err != nil {
		return guidedTourLeafCompletion{
			response: response,
			failure: &guidedTourLeafFailure{
				TaskID: call.task.ID, Kind: call.task.Kind,
				Reason: "validated leaf canonical encoding failed",
			},
		}
	}
	response.Content = canonicalArtifact
	return guidedTourLeafCompletion{
		result:     guidedtour.LeafResult{Task: call.task, Artifact: artifact},
		response:   response,
		cacheInput: call.cacheInput,
		taskID:     call.task.ID,
	}
}

func guidedTourLeafFailureReason(prefix string, err error) string {
	const maxReasonBytes = 512
	if err == nil {
		return prefix
	}
	reason := prefix + ": " + strings.Join(strings.Fields(err.Error()), " ")
	if len(reason) > maxReasonBytes {
		reason = reason[:maxReasonBytes]
	}
	return reason
}

func allowsFanoutCall(
	policy modelresearch.Policy,
	reservedCalls int,
	reservedBytes int,
	requestBytes int,
) (bool, string) {
	if requestBytes <= 0 {
		return false, "empty_request"
	}
	if requestBytes > policy.GuidedTour.MaxRequestBytes {
		return false, "stage_byte_budget_exhausted"
	}
	if reservedCalls >= policy.MaxGuidedTourCalls {
		return false, "guided_call_budget_exhausted"
	}
	if reservedBytes+requestBytes > policy.MaxGuidedTourBytes {
		return false, "guided_byte_budget_exhausted"
	}
	return true, "within_budget"
}

func allowsFanoutLeafCall(
	policy modelresearch.Policy,
	reservedCalls int,
	reservedBytes int,
	requestBytes int,
) (bool, string) {
	if allowed, reason := allowsFanoutCall(
		policy,
		reservedCalls,
		reservedBytes,
		requestBytes,
	); !allowed {
		return false, reason
	}
	if reservedCalls+1 >= policy.MaxGuidedTourCalls {
		return false, "fan_in_call_reserved"
	}
	if reservedBytes+requestBytes+policy.GuidedTour.MaxRequestBytes > policy.MaxGuidedTourBytes {
		return false, "fan_in_byte_budget_reserved"
	}
	return true, "within_budget"
}

func addGuidedTourResponseMetrics(
	outcome *guidedTourOutcome,
	response modelresearch.StageResponse,
) {
	outcome.ResponseBytes += len(response.Content)
	outcome.InputTokens += response.InputTokens
	outcome.OutputTokens += response.OutputTokens
	outcome.PromptCacheHitTokens += response.PromptCacheHitTokens
	outcome.PromptCacheMissTokens += response.PromptCacheMissTokens
	outcome.LatencyMillis += response.LatencyMillis
}

func removeGuidedTourFanoutOutputs(runDir string) error {
	for _, name := range []string{
		guidedTourFanoutFile,
		guidedTourFanoutLeavesFile,
		guidedTourFanInFile,
	} {
		artifactPath := filepath.Join(runDir, name)
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("guided tour fan-out: remove stale %s: %w", name, err)
		}
	}
	return nil
}

func terminateGuidedTourFanoutResourceLimit(
	runDir string,
	outcome guidedTourOutcome,
	err error,
) (guidedTourOutcome, error) {
	if cleanupErr := removeGuidedTourFanoutOutputs(runDir); cleanupErr != nil {
		return outcome, errors.Join(err, cleanupErr)
	}
	return outcome, err
}

func writeGuidedTourFanInArtifact(
	runDir string,
	artifact guidedtour.FanInArtifact,
) error {
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("guided tour fan-out: encode fan-in artifact: %w", err)
	}
	return writeGuidedTourArtifact(
		filepath.Join(runDir, guidedTourFanInFile),
		append(encoded, '\n'),
	)
}

func saveGuidedTourFanInStory(
	bundle guidedtour.Bundle,
	artifact guidedtour.FanInArtifact,
	runDir string,
) error {
	if artifact.Proposal == nil {
		return fmt.Errorf("guided tour fan-out: %s verdict has no proposal", artifact.Verdict)
	}
	proposal, err := json.Marshal(*artifact.Proposal)
	if err != nil {
		return fmt.Errorf("guided tour fan-out: encode validated proposal: %w", err)
	}
	return saveGuidedTourRecordTo(
		bundle,
		proposal,
		runDir,
		guidedTourFanoutFile,
	)
}

func writeGuidedTourFanoutArtifact(
	runDir string,
	bundleSHA string,
	results []guidedtour.LeafResult,
	failures []guidedTourLeafFailure,
) error {
	artifact := guidedTourFanoutArtifact{
		Version: 1, BundleSHA256: bundleSHA,
		LeafLimit: guidedTourExperimentLeafLimit,
		Results:   append([]guidedtour.LeafResult{}, results...),
		Failures:  append([]guidedTourLeafFailure{}, failures...),
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("guided tour fan-out: encode leaf artifact: %w", err)
	}
	return writeGuidedTourArtifact(
		filepath.Join(runDir, guidedTourFanoutLeavesFile),
		append(encoded, '\n'),
	)
}
