package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const pavedPathStatusVersion = 1

const pavedPathAttemptVersion = 2

const pavedPathEditorEvidenceLimit = 80

const legacyPavedPathEditorEvidenceLimit = 48

const pavedPathSystemPrompt = `You edit a small operating guide from bounded repository-owned operational evidence. The supplied evidence objects and opaque IDs are the complete authority. A Paved Path is a reading and planning aid, not proof that a command succeeds. Return valid JSON only. Never invent or alter a command, argument, endpoint, target, file, relationship, or ID. Never combine incompatible procedures.`

const pavedPathUserPrompt = `Return exactly this JSON shape:
{
  "version": 1,
  "paths": [
    {
      "title": "short human title",
      "goal": "what this repository-owned procedure enables",
      "prerequisite_evidence_ids": ["zero or more exact evidence ids"],
      "actions": [
        {
          "evidence_id": "exact supplied evidence id",
          "instruction": "bounded instruction about this exact evidence",
          "command": "exact supplied command or empty",
          "endpoint": "exact supplied endpoint or empty"
        }
      ],
      "expected_evidence_ids": ["zero or more exact evidence ids"],
      "troubleshooting_evidence_ids": ["zero or more exact evidence ids"],
      "related_study_direction_ids": ["zero or more exact supplied study ids"],
      "ordering_basis": "documented_procedure | script_sequence | editorial"
    }
  ]
}

Rules:
- Produce zero to eight paths. A smaller exact result is better than a generic recipe.
- Use only supplied opaque evidence and study IDs.
- Copy command and endpoint values byte-for-byte from the selected evidence. Leave them empty when absent.
- A multi-step path needs at least two meaningful actions. One action is allowed only for a genuinely sufficient exact command.
- Preserve documented ordering when one source describes a procedure. Otherwise label the order editorial; do not imply execution proof.
- Documentation expresses intended procedure. Scripts and configuration express executable structure. Neither proves success.
- Do not merge conflicting versions, ports, paths, or alternative workflows.
- Do not include credentials or reconstruct redacted values.
- Do not tell the user to run a command that is absent from the supplied evidence.
- Use concise human language. Do not mention evidence, validation, confidence, gaps, provider metadata, or internal IDs in prose.

Variable bounded operational bundle JSON:
`

type pavedPathStatus struct {
	Version        int                           `json:"version"`
	State          string                        `json:"state"`
	Failure        string                        `json:"failure_reason,omitempty"`
	Evidence       int                           `json:"evidence"`
	EditorEvidence int                           `json:"editor_evidence"`
	Paths          int                           `json:"paths"`
	Landmarks      int                           `json:"landmarks"`
	Metrics        semanticDiscoveryStageMetrics `json:"model_stage"`
	WallMillis     int64                         `json:"wall_ms"`
	LocalReplay    bool                          `json:"local_replay,omitempty"`
}

type pavedPathAttempt struct {
	Version           int                           `json:"version"`
	PromptVersion     string                        `json:"prompt_version"`
	BundleSHA256      string                        `json:"bundle_sha256"`
	ValidationState   string                        `json:"validation_state"`
	FailureReason     string                        `json:"failure_reason,omitempty"`
	Metrics           semanticDiscoveryStageMetrics `json:"metrics"`
	EditorEvidenceIDs []string                      `json:"editor_evidence_ids,omitempty"`
	StudyIDs          []string                      `json:"study_direction_ids,omitempty"`
	Response          json.RawMessage               `json:"response,omitempty"`
	RawResponse       string                        `json:"raw_response,omitempty"`
}

func editPavedPathsForRun(
	ctx context.Context,
	runDir string,
	repoRoot string,
	stderr io.Writer,
) (pavedPathStatus, error) {
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return pavedPathStatus{}, fmt.Errorf("paved paths: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	return preparePavedPaths(ctx, runDir, repoRoot, client, nil)
}

func preparePavedPaths(
	ctx context.Context,
	runDir string,
	repoRoot string,
	provider semanticDiscoveryEditor,
	expectedRepository *freshness.RepositoryState,
) (status pavedPathStatus, returnErr error) {
	started := time.Now()
	status = pavedPathStatus{Version: pavedPathStatusVersion, State: "started"}
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil && status.State == "started" {
			status.State = "failed"
			status.Failure = semanticDiscoveryReason(returnErr.Error())
		}
		if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.StatusFile), status); err != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w; save paved path status: %v", returnErr, err)
			} else {
				returnErr = fmt.Errorf("paved paths: save status: %w", err)
			}
		}
	}()
	if ctx == nil || provider == nil {
		return status, fmt.Errorf("paved paths: context and provider are required")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	for _, name := range []string{pavedpath.BundleFile, pavedpath.AttemptFile, pavedpath.RecordFile} {
		if err := os.Remove(filepath.Join(runDir, name)); err != nil && !os.IsNotExist(err) {
			return status, fmt.Errorf("paved paths: remove stale %s: %w", name, err)
		}
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return status, fmt.Errorf("paved paths: read saved run: %w", err)
	}
	inventory, err := readOperationalInventory(repoRoot)
	if err != nil {
		return status, err
	}
	bundle, err := pavedpath.Collect(repoRoot, data.RepoName, inventory)
	if err != nil {
		return status, err
	}
	if expectedRepository != nil {
		current, captureErr := freshness.CaptureRepository(ctx, repoRoot)
		if captureErr != nil {
			return status, fmt.Errorf("paved paths: capture replay repository: %w", captureErr)
		}
		if err := verifyOperationalReplayState(*expectedRepository, current, bundle.AllowedPaths); err != nil {
			return status, err
		}
	}
	status.Evidence = len(bundle.Evidence)
	bundleSHA, err := pavedpath.BundleHash(bundle)
	if err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.BundleFile), bundle); err != nil {
		return status, fmt.Errorf("paved paths: save operational evidence: %w", err)
	}
	studyIDs := reportStudyDirectionIDs(data)
	if len(bundle.Evidence) == 0 {
		record, buildErr := pavedpath.BuildRecord(bundle, pavedpath.Proposal{Version: pavedpath.ProposalVersion}, studyIDs)
		if buildErr != nil {
			return status, buildErr
		}
		if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.RecordFile), record); err != nil {
			return status, err
		}
		status.State = "empty"
		return status, nil
	}
	editorBundle := selectPavedPathEditorBundle(bundle, pavedPathEditorEvidenceLimit)
	status.EditorEvidence = len(editorBundle.Evidence)
	promptBundle, err := json.Marshal(struct {
		Bundle   pavedpath.Bundle `json:"bundle"`
		StudyIDs []string         `json:"study_direction_ids,omitempty"`
	}{Bundle: editorBundle, StudyIDs: studyIDs})
	if err != nil {
		return status, fmt.Errorf("paved paths: encode provider bundle: %w", err)
	}
	prompt := semanticdiscovery.Prompt{
		Version:         pavedpath.PromptVersion,
		System:          pavedPathSystemPrompt,
		User:            pavedPathUserPrompt + string(promptBundle),
		ThinkingProfile: semanticdiscovery.ThinkingHigh,
		ProgressLabel:   "paved path editing",
	}
	plan, err := newSemanticDiscoveryStagePlan(provider, prompt, "paved_paths")
	if err != nil {
		return status, err
	}
	metrics := semanticDiscoveryStageMetrics{
		Stage: plan.name, PromptVersion: plan.prompt.Version, RequestBytes: len(plan.request),
		ProviderCall: true,
	}
	attempt := pavedPathAttempt{
		Version: pavedPathAttemptVersion, PromptVersion: prompt.Version, BundleSHA256: bundleSHA,
		ValidationState: "started", Metrics: metrics,
		EditorEvidenceIDs: pavedPathEvidenceIDs(editorBundle.Evidence),
		StudyIDs:          sortedV32IDs(studyIDs),
	}
	callStarted := time.Now()
	providerResult, callErr := provider.DiscoverSemanticsMeasured(ctx, prompt)
	metrics.addResponse(providerResult, time.Since(callStarted))
	status.Metrics = metrics
	attempt.Metrics = metrics
	if ctxErr := ctx.Err(); ctxErr != nil {
		attempt.ValidationState = "canceled"
		attempt.FailureReason = semanticDiscoveryReason(ctxErr.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt)
		return status, ctxErr
	}
	if callErr != nil {
		metrics.Status = "failed_provider"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(callErr.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt)
		return publishPavedPathLandmarks(runDir, bundle, studyIDs, status, attempt, fmt.Errorf("paved paths: provider call: %w", callErr))
	}
	if json.Valid(providerResult.Content) {
		attempt.Response = append(json.RawMessage(nil), providerResult.Content...)
	} else {
		attempt.RawResponse = string(providerResult.Content)
	}
	proposal, err := pavedpath.DecodeProposal(providerResult.Content)
	if err != nil {
		metrics.Status = "rejected"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(err.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt)
		return publishPavedPathLandmarks(runDir, bundle, studyIDs, status, attempt, err)
	}
	record, err := pavedpath.BuildRecordScoped(
		bundle,
		proposal,
		attempt.StudyIDs,
		attempt.EditorEvidenceIDs,
	)
	if err != nil {
		metrics.Status = "rejected"
		status.Metrics = metrics
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(err.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt)
		return publishPavedPathLandmarks(runDir, bundle, studyIDs, status, attempt, err)
	}
	if len(proposal.Paths) > 0 && len(record.Paths) == 0 {
		metrics.Status = "accepted_with_no_publishable_paths"
		status.Metrics = metrics
		status.Paths = 0
		status.Landmarks = len(record.Landmarks)
		status.State = "landmarks"
		status.Failure = "all_proposed_paths_rejected"
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = status.Failure
		if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt); err != nil {
			return status, err
		}
		if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.RecordFile), record); err != nil {
			return status, fmt.Errorf("paved paths: save landmark-only record: %w", err)
		}
		return status, nil
	}
	metrics.Status = "accepted"
	status.Metrics = metrics
	status.Paths = len(record.Paths)
	status.Landmarks = len(record.Landmarks)
	status.State = "published"
	if len(record.Paths) == 0 {
		status.State = "landmarks"
	}
	attempt.Metrics = metrics
	attempt.ValidationState = metrics.Status
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt); err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.RecordFile), record); err != nil {
		return status, fmt.Errorf("paved paths: save record: %w", err)
	}
	return status, nil
}

func selectPavedPathEditorBundle(bundle pavedpath.Bundle, limit int) pavedpath.Bundle {
	if limit <= 0 || limit >= len(bundle.Evidence) {
		return bundle
	}
	selected := make([]pavedpath.Evidence, 0, limit)
	seen := make(map[string]struct{}, limit)
	appendMatching := func(concrete bool) {
		for _, item := range bundle.Evidence {
			if len(selected) == limit {
				return
			}
			isConcrete := len(item.Commands) > 0 || item.Endpoint != ""
			if isConcrete != concrete {
				continue
			}
			if _, exists := seen[item.ID]; exists {
				continue
			}
			selected = append(selected, item)
			seen[item.ID] = struct{}{}
		}
	}
	appendMatching(true)
	appendMatching(false)
	result := bundle
	result.Evidence = selected
	result.AllowedPaths = nil
	paths := make(map[string]struct{})
	for _, item := range selected {
		paths[item.Path] = struct{}{}
	}
	for filePath := range paths {
		result.AllowedPaths = append(result.AllowedPaths, filePath)
	}
	sort.Strings(result.AllowedPaths)
	result.Stats.Truncated = true
	return result
}

func pavedPathEvidenceIDs(items []pavedpath.Evidence) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func sortedV32IDs(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func readOperationalInventory(repoRoot string) ([]string, error) {
	tracked, err := gitfiles.List(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("paved paths: collect tracked inventory: %w", err)
	}
	if len(tracked) == 0 || len(tracked) > 20_000 {
		return nil, fmt.Errorf("paved paths: tracked inventory is empty or oversized")
	}
	return tracked, nil
}

func verifyOperationalReplayState(
	expected freshness.RepositoryState,
	current freshness.RepositoryState,
	paths []string,
) error {
	if expected.Identity != current.Identity || expected.Head != current.Head {
		return fmt.Errorf("paved paths: replay repository identity or revision changed")
	}
	expectedDirty := make(map[string]freshness.DirtyFile, len(expected.Dirty))
	for _, item := range expected.Dirty {
		expectedDirty[item.Path] = item
	}
	currentDirty := make(map[string]freshness.DirtyFile, len(current.Dirty))
	for _, item := range current.Dirty {
		currentDirty[item.Path] = item
	}
	for _, filePath := range paths {
		before, wasDirty := expectedDirty[filePath]
		after, isDirty := currentDirty[filePath]
		if wasDirty != isDirty || wasDirty && (before.Kind != after.Kind ||
			before.Mode != after.Mode || before.ContentSHA256 != after.ContentSHA256) {
			return fmt.Errorf("paved paths: operational input %q changed since the saved run", filePath)
		}
	}
	return nil
}

func publishPavedPathLandmarks(
	runDir string,
	bundle pavedpath.Bundle,
	studyIDs []string,
	status pavedPathStatus,
	attempt pavedPathAttempt,
	cause error,
) (pavedPathStatus, error) {
	record, err := pavedpath.BuildRecord(bundle, pavedpath.Proposal{Version: pavedpath.ProposalVersion}, studyIDs)
	if err != nil {
		return status, fmt.Errorf("%v; publish landmarks: %w", cause, err)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.RecordFile), record); err != nil {
		return status, fmt.Errorf("%v; save landmarks: %w", cause, err)
	}
	status.Paths = 0
	status.Landmarks = len(record.Landmarks)
	status.State = "landmarks"
	status.Failure = semanticDiscoveryReason(cause.Error())
	if attempt.ValidationState == "started" {
		attempt.ValidationState = "rejected"
		attempt.FailureReason = status.Failure
		_ = writeGoldenJSON(filepath.Join(runDir, pavedpath.AttemptFile), attempt)
	}
	return status, cause
}

func reportStudyDirectionIDs(data *report.ReportData) []string {
	if data == nil || data.StudyMap == nil {
		return nil
	}
	result := make([]string, 0, len(data.StudyMap.Directions))
	for _, direction := range data.StudyMap.Directions {
		if id := strings.TrimSpace(direction.ID); id != "" {
			result = append(result, id)
		}
	}
	return result
}
