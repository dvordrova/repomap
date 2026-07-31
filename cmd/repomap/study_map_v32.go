package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	studyMapV32ReviewConcurrency   = 3
	studyMapMinimumCompleteAnchors = 3

	studyMapBriefShapeFile        = "repository_brief_shape.json"
	studyMapBriefShapeAttempt     = "repository_brief_shape_attempt.json"
	studyMapDirectionsFile        = "study_direction_candidates.json"
	studyMapDirectionsAttempt     = studymap.DirectionsAttemptFile
	studyMapReviewsFile           = "reading_pack_reviews.json"
	studyMapReviewAttemptsDir     = "reading_pack_review_attempts"
	studyMapSourceAttemptFile     = "repository_study_map_source_attempt.json"
	studyMapReviewArtifactVersion = 1
)

const studyMapV32SystemPrompt = `You are an editorial onboarding planner for one bounded repository model. The supplied objects and opaque repository IDs are the complete authority. A Study Direction recommends what to read; it is not a runtime claim or canonical Mechanism. Return valid JSON only. Never invent or alter a file, symbol, component, document, mechanism, relation, fact, repository ID, or runtime order.`

const studyMapV32SharedInput = `The bounded repository bundle below is the complete source for this task. Documentation describes intent, while code anchors identify exact local source. Do not treat editorial reading order as execution order.

Bounded repository bundle JSON:
`

const studyMapBriefShapeTask = `

Task: produce only the repository Brief and Shape. Return exactly:
{
  "version": 1,
  "repository_type": "service_application | library_framework | cli_tool | monorepo | mixed",
  "brief": {
    "what_it_is": {"text": "short answer", "support_ids": ["exact supplied ids"]},
    "problem": {"text": "problem it addresses", "support_ids": ["exact supplied ids"]},
    "main_input": {"text": "main input or trigger", "support_ids": ["exact supplied ids"]},
    "central_responsibility": {"text": "central responsibility", "support_ids": ["exact supplied ids"]},
    "observable_result": {"text": "observable result", "support_ids": ["exact supplied ids"]},
    "domain_terms": [{"term": "term", "meaning": "short meaning", "support_ids": ["exact supplied ids"]}]
  },
  "shape_area_ids": ["one to seven exact supplied area ids"]
}

Use only supplied IDs. Keep every sentence short and independently useful. Leave out a domain term rather than guessing, but provide all five Brief statements from supported repository objects.`

const studyMapDirectionTask = `

Task: produce only bounded Study Direction drafts. Return exactly:
{
  "version": 1,
  "directions": [
    {
      "question": "natural developer question ending in ?",
      "why_it_matters": "one sentence",
      "learning_outcome": "what the reader will understand",
      "target_user_job": "first_contact | use_or_operate | extend_or_integrate | contribute | debug_or_maintain",
      "learning_stage": "orientation | central_operation | core_model | integration | operations | contribution",
      "anchor_ids": [
        "exact supplied code anchor id A",
        "exact supplied code anchor id B",
        "exact supplied code anchor id C"
      ],
      "document_ids": ["zero or more exact supplied document ids"],
      "area_ids": ["one or more exact supplied area ids"],
      "mechanism_id": "exact supplied canonical mechanism id or empty",
      "reading_anchors": [
        {"anchor_id": "exact supplied code anchor id A", "label": "Start here", "what_to_look_for": "bounded editorial reading instruction for A"},
        {"anchor_id": "exact supplied code anchor id B", "label": "Then inspect", "what_to_look_for": "bounded editorial reading instruction for B"},
        {"anchor_id": "exact supplied code anchor id C", "label": "Related implementation", "what_to_look_for": "bounded editorial reading instruction for C"}
      ],
      "search_queries": ["natural search wording"]
    }
  ]
}

Rules:
- Return eight to twelve candidates when supported, never more than twelve. Do not create direction IDs; local code assigns them.
- Every repository object ID must be copied exactly from the bundle.
- Use three to five code anchors and describe each exactly once in reading_anchors.
- reading_anchors.label is a closed schema value. Copy one of the five listed English literals exactly; the report localizes it later.
- Favor central responsibilities and role-diverse packs over narrow helpers, duplicate questions, tests, examples, fixtures, or similarly named implementations.
- Questions may ask about behavior, but reading copy must not assert execution order, causality, or behavior outside the supplied bounded facts.
- Attach a canonical Mechanism only when supplied anchors overlap it.
- The candidate task is independent: do not assume or reconstruct another model response.`

const studyMapReviewSystemPrompt = `You review one fixed source-backed Reading Pack. The supplied direction, opaque anchor IDs, metadata, and exact line-numbered source fragments are the complete authority. Evaluate each anchor independently. Return valid JSON only. Do not create or alter IDs, files, symbols, relations, facts, commands, or runtime order. A supported observation is presentation copy bounded to the visible fragment, not a new repository fact.`

const studyMapReviewTask = `Return exactly:
{
  "version": 1,
  "direction_id": "copy the supplied direction id exactly",
  "reviews": [
    {
      "anchor_id": "copy one supplied anchor id exactly",
      "fit": "direct | supporting | weak | irrelevant",
      "supported_observation": "short sentence limited to visible source",
      "role": "documentation_intent | public_or_cli_entry | core_orchestration | state_or_data_model | effect_or_integration_boundary | representative_implementation | configuration_or_operations | example_or_usage | test_or_verification",
      "overclaim_reasons": ["none | wrong_responsibility | behavior_outside_window | unsupported_runtime_order | unsupported_causality | question_scope_broader | learning_outcome_scope_broader | vague_or_generic"],
      "narrower_display_sentence": "optional shorter replacement"
    }
  ]
}

Review every supplied anchor exactly once. Choose ` + "`none`" + ` alone when no overclaim applies. Use ` + "`irrelevant`" + ` when the exact fragment does not help answer the fixed question. Do not repair the question, infer missing code, or claim an execution sequence.

Fixed bounded review bundle JSON:
`

type studyMapV32StageAttempt struct {
	Version              int                                    `json:"version"`
	PromptVersion        string                                 `json:"prompt_version"`
	BundleSHA256         string                                 `json:"bundle_sha256"`
	ValidationState      string                                 `json:"validation_state"`
	FailureReason        string                                 `json:"failure_reason,omitempty"`
	Metrics              semanticDiscoveryStageMetrics          `json:"metrics"`
	DirectionDiagnostics *studymap.DirectionProposalDiagnostics `json:"direction_diagnostics,omitempty"`
	Response             json.RawMessage                        `json:"response,omitempty"`
	RawResponse          string                                 `json:"raw_response,omitempty"`
}

type studyMapReviewAttempt struct {
	Version         int                           `json:"version"`
	PromptVersion   string                        `json:"prompt_version"`
	BundleSHA256    string                        `json:"bundle_sha256"`
	DirectionID     string                        `json:"direction_id"`
	ValidationState string                        `json:"validation_state"`
	IssueCode       string                        `json:"issue_code,omitempty"`
	FailureReason   string                        `json:"failure_reason,omitempty"`
	Metrics         semanticDiscoveryStageMetrics `json:"metrics"`
	Bundle          *studymap.ReviewBundle        `json:"bundle,omitempty"`
	Response        json.RawMessage               `json:"response,omitempty"`
	RawResponse     string                        `json:"raw_response,omitempty"`
}

type studyMapReviewArtifact struct {
	Version   int                       `json:"version"`
	Reviews   []studymap.ReviewProposal `json:"reviews"`
	Reduction studymap.ReviewReduction  `json:"reduction"`
	Attempts  []studyMapReviewSummary   `json:"attempts"`
}

type studyMapReviewSummary struct {
	DirectionID     string                        `json:"direction_id"`
	ValidationState string                        `json:"validation_state"`
	IssueCode       string                        `json:"issue_code,omitempty"`
	FailureReason   string                        `json:"failure_reason,omitempty"`
	Metrics         semanticDiscoveryStageMetrics `json:"metrics"`
}

type studyMapReviewTaskInput struct {
	index  int
	bundle studymap.ReviewBundle
	plan   semanticDiscoveryStagePlan
}

type studyMapReviewCompletion struct {
	index       int
	directionID string
	bundle      studymap.ReviewBundle
	attempt     studyMapReviewAttempt
	proposal    studymap.ReviewProposal
	issue       studymap.ReviewIssue
	valid       bool
}

type studyMapReviewPreparationFailure struct {
	index       int
	directionID string
	bundleSHA   string
	stage       string
	issueCode   string
	bundle      *studymap.ReviewBundle
	cause       error
}

func clearStudyMapV32Outputs(runDir string) error {
	files := []string{
		studymap.RecordFile,
		studymap.BundleFile,
		studymap.AttemptFile,
		studymap.StatusFile,
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
		studyMapSourceAttemptFile,
	}
	for _, name := range files {
		if err := os.Remove(filepath.Join(runDir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("study map: remove stale %s: %w", name, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(runDir, studyMapReviewAttemptsDir)); err != nil {
		return fmt.Errorf("study map: remove stale review attempts: %w", err)
	}
	return nil
}

func writeNormalizedDirectionProposal(path string, proposal studymap.DirectionProposal) error {
	raw, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("study map: encode normalized directions: %w", err)
	}
	replayed, err := studymap.DecodeNormalizedDirectionProposal(raw)
	if err != nil {
		return fmt.Errorf("study map: validate normalized directions for replay: %w", err)
	}
	if err := writeGoldenJSON(path, replayed); err != nil {
		return fmt.Errorf("study map: save normalized directions: %w", err)
	}
	return nil
}

func prepareStudyMap(
	ctx context.Context,
	runDir string,
	repoRoot string,
	provider semanticDiscoveryEditor,
) (studyMapStatus, error) {
	return prepareStudyMapWithProviderFactory(ctx, runDir, repoRoot, func() (semanticDiscoveryEditor, error) {
		if provider == nil {
			return nil, fmt.Errorf("study map: provider is required")
		}
		return provider, nil
	})
}

// prepareStudyMapWithProviderFactory keeps local source availability ahead of
// provider configuration. Unsupported repositories therefore produce their
// typed local Study outcome without requiring credentials or a network-ready
// client, while supported repositories retain the same canonical bundle and
// provider pipeline.
func prepareStudyMapWithProviderFactory(
	ctx context.Context,
	runDir string,
	repoRoot string,
	providerFactory func() (semanticDiscoveryEditor, error),
) (status studyMapStatus, returnErr error) {
	started := time.Now()
	status = studyMapStatus{Version: studyMapStatusVersion, State: "started"}
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil && status.State == "started" {
			status.State = "failed"
			status.FailureReason = semanticDiscoveryReason(returnErr.Error())
		}
		if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status); err != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w; save study map status: %v", returnErr, err)
			} else {
				returnErr = fmt.Errorf("study map: save status: %w", err)
			}
		}
	}()
	if ctx == nil || providerFactory == nil {
		return status, fmt.Errorf("study map: context and provider factory are required")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if err := clearStudyMapV32Outputs(runDir); err != nil {
		return status, err
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return status, fmt.Errorf("study map: read saved run: %w", err)
	}
	bundle, err := buildStudyMapBundle(runDir, repoRoot, data)
	if err != nil {
		if outcome, ok := studyMapSourceOutcomeCode(err); ok {
			status.State = "failed"
			status.FailureReason = string(outcome)
			return status, nil
		}
		return status, err
	}
	status.Anchors = len(bundle.Anchors)
	status.Areas = len(bundle.Areas)
	status.Documents = len(bundle.Documents)
	status.Mechanisms = len(bundle.Mechanisms)
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.BundleFile), bundle); err != nil {
		return status, fmt.Errorf("study map: save bundle: %w", err)
	}
	provider, err := providerFactory()
	if err != nil {
		return status, err
	}
	if provider == nil {
		return status, fmt.Errorf("study map: provider factory returned no provider")
	}
	record, reduction, stages, editErr := prepareStudyMapV32(ctx, runDir, bundle, provider)
	status.Stages = stages
	status.Metrics = aggregateStudyMapMetrics(stages, editErr)
	status.ProviderLatencyMillis = status.Metrics.LatencyMillis
	attempt := studyMapAttempt{
		Version: 2, PromptVersion: "repository-study-map-split-v2", BundleSHA256: bundleSHA,
		ValidationState: "rejected", Metrics: status.Metrics,
	}
	if editErr != nil {
		attempt.FailureReason = semanticDiscoveryReason(editErr.Error())
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		return status, editErr
	}
	status.RepositoryType = record.RepositoryType
	status.Candidates = reduction.Proposed
	status.Validated = reduction.Reviewed
	status.Selected = len(record.Directions)
	status.State = "published"
	attempt.ValidationState = "accepted"
	legacyProposal := studyMapV32ProposalFromRecord(record)
	if raw, marshalErr := json.Marshal(legacyProposal); marshalErr == nil {
		attempt.Response = raw
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt); err != nil {
		return status, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.RecordFile), record); err != nil {
		return status, fmt.Errorf("study map: save canonical record: %w", err)
	}
	return status, nil
}

// prepareStudyMapV32 runs the split editor. Brief and candidate calls share a
// stable repository-bundle prefix, while each review is independently saved
// and allowed to fail without canceling its siblings.
func prepareStudyMapV32(
	ctx context.Context,
	runDir string,
	bundle studymap.Bundle,
	provider semanticDiscoveryEditor,
) (studymap.Record, studymap.ReviewReduction, []semanticDiscoveryStageMetrics, error) {
	if ctx == nil || provider == nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil,
			fmt.Errorf("study map: context and provider are required")
	}
	if len(bundle.Anchors) < studyMapMinimumCompleteAnchors {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, fmt.Errorf(
			"study map: insufficient code anchors for complete directions: have %d, need at least %d",
			len(bundle.Anchors),
			studyMapMinimumCompleteAnchors,
		)
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, err
	}
	promptBundle, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil,
			fmt.Errorf("study map: encode provider bundle: %w", err)
	}
	shared := studyMapV32SharedInput + string(promptBundle)

	briefRaw, briefMetrics, briefAttempt, err := executeStudyMapV32Stage(
		ctx, provider, semanticdiscovery.Prompt{
			Version: semanticdiscovery.StudyBriefPromptVersion, System: studyMapV32SystemPrompt,
			User: shared + studyMapBriefShapeTask, ThinkingProfile: semanticdiscovery.ThinkingMax,
			ProgressLabel: "repository brief and shape editing",
		}, "repository_brief_shape", bundleSHA,
	)
	stages := []semanticDiscoveryStageMetrics{briefMetrics}
	if err != nil {
		_ = writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeAttempt), briefAttempt)
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	recoveredBrief, recoveryErr := studymap.RecoverBriefShapeProviderJSON(briefRaw)
	var brief studymap.BriefShapeProposal
	if recoveryErr != nil {
		err = recoveryErr
	} else {
		briefAttempt.RawResponse = ""
		briefAttempt.Response = append(json.RawMessage(nil), recoveredBrief...)
		brief, err = studymap.DecodeBriefShapeProposal(recoveredBrief)
	}
	if err != nil {
		briefMetrics.Status = "rejected"
		briefAttempt.Metrics = briefMetrics
		briefAttempt.ValidationState = briefMetrics.Status
		briefAttempt.FailureReason = semanticDiscoveryReason(err.Error())
		stages[0] = briefMetrics
		_ = writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeAttempt), briefAttempt)
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	briefMetrics.Status = "accepted"
	briefAttempt.Metrics = briefMetrics
	briefAttempt.ValidationState = briefMetrics.Status
	stages[0] = briefMetrics
	if saveErr := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeAttempt), briefAttempt); saveErr != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, saveErr
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeFile), brief); err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}

	directionRaw, directionMetrics, directionAttempt, err := executeStudyMapV32Stage(
		ctx, provider, semanticdiscovery.Prompt{
			Version: semanticdiscovery.StudyCandidatesPromptVersion, System: studyMapV32SystemPrompt,
			User: shared + studyMapDirectionTask, ThinkingProfile: semanticdiscovery.ThinkingMax,
			ProgressLabel: "study direction candidate editing",
		}, "study_direction_candidates", bundleSHA,
	)
	stages = append(stages, directionMetrics)
	if err != nil {
		_ = writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), directionAttempt)
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	recoveredDirections, recoveryErr := studymap.RecoverDirectionProviderJSON(directionRaw)
	var directions studymap.DirectionProposal
	var directionDiagnostics studymap.DirectionProposalDiagnostics
	if recoveryErr != nil {
		err = recoveryErr
	} else {
		directionAttempt.RawResponse = ""
		directionAttempt.Response = append(json.RawMessage(nil), recoveredDirections...)
		directions, directionDiagnostics, err =
			studymap.DecodeDirectionProposalWithDiagnostics(recoveredDirections)
	}
	if directionDiagnostics.Received > 0 {
		directionAttempt.DirectionDiagnostics = &directionDiagnostics
	}
	if err != nil {
		directionMetrics.Status = "rejected"
		directionAttempt.Metrics = directionMetrics
		directionAttempt.ValidationState = directionMetrics.Status
		directionAttempt.FailureReason = semanticDiscoveryReason(err.Error())
		stages[len(stages)-1] = directionMetrics
		_ = writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), directionAttempt)
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	directions, err = studymap.ResolveDirectionProposalReferences(bundle, directions)
	if err != nil {
		directionMetrics.Status = "rejected"
		directionAttempt.Metrics = directionMetrics
		directionAttempt.ValidationState = directionMetrics.Status
		directionAttempt.FailureReason = semanticDiscoveryReason(err.Error())
		stages[len(stages)-1] = directionMetrics
		_ = writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), directionAttempt)
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	directionMetrics.Status = "accepted"
	directionAttempt.Metrics = directionMetrics
	directionAttempt.ValidationState = directionMetrics.Status
	stages[len(stages)-1] = directionMetrics
	if saveErr := writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), directionAttempt); saveErr != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, saveErr
	}
	if err := writeNormalizedDirectionProposal(
		filepath.Join(runDir, studyMapDirectionsFile),
		directions,
	); err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}

	reviews, summaries, reviewStages, preparationIssues, err := reviewStudyMapDirections(
		ctx, runDir, bundle, directions, bundleSHA, provider,
	)
	stages = append(stages, reviewStages...)
	if err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	record, reduction, err := studymap.BuildReviewedRecord(bundle, brief, directions, reviews)
	reduction.Issues = append(reduction.Issues, preparationIssues...)
	artifact := studyMapReviewArtifact{
		Version: studyMapReviewArtifactVersion, Reviews: reviews,
		Reduction: reduction, Attempts: summaries,
	}
	if saveErr := writeGoldenJSON(filepath.Join(runDir, studyMapReviewsFile), artifact); saveErr != nil {
		return studymap.Record{}, reduction, stages, saveErr
	}
	if err != nil {
		return studymap.Record{}, reduction, stages, err
	}
	return record, reduction, stages, nil
}

func reviewSavedStudyMapV32(
	ctx context.Context,
	runDir string,
	record studymap.Record,
	provider semanticDiscoveryEditor,
) (studymap.Record, studymap.ReviewReduction, []semanticDiscoveryStageMetrics, error) {
	brief, directions, inputErr := studyMapV32InputsFromRecord(record)
	if inputErr != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, inputErr
	}
	bundleSHA, err := studymap.BundleHash(record.Bundle)
	if err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapBriefShapeFile), brief); err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, err
	}
	if err := writeNormalizedDirectionProposal(
		filepath.Join(runDir, studyMapDirectionsFile),
		directions,
	); err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, nil, err
	}
	reviews, summaries, stages, preparationIssues, err := reviewStudyMapDirections(
		ctx, runDir, record.Bundle, directions, bundleSHA, provider,
	)
	if err != nil {
		return studymap.Record{}, studymap.ReviewReduction{}, stages, err
	}
	reviewed, reduction, buildErr := studymap.BuildReviewedRecord(record.Bundle, brief, directions, reviews)
	reduction.Issues = append(reduction.Issues, preparationIssues...)
	artifact := studyMapReviewArtifact{
		Version: studyMapReviewArtifactVersion, Reviews: reviews,
		Reduction: reduction, Attempts: summaries,
	}
	if saveErr := writeGoldenJSON(filepath.Join(runDir, studyMapReviewsFile), artifact); saveErr != nil {
		return studymap.Record{}, reduction, stages, saveErr
	}
	return reviewed, reduction, stages, buildErr
}

func executeStudyMapV32Stage(
	ctx context.Context,
	provider semanticDiscoveryEditor,
	prompt semanticdiscovery.Prompt,
	stage string,
	bundleSHA string,
) ([]byte, semanticDiscoveryStageMetrics, studyMapV32StageAttempt, error) {
	plan, err := newSemanticDiscoveryStagePlan(provider, prompt, stage)
	if err != nil {
		return nil, semanticDiscoveryStageMetrics{}, studyMapV32StageAttempt{}, err
	}
	metrics := semanticDiscoveryStageMetrics{
		Stage: plan.name, PromptVersion: prompt.Version, RequestBytes: len(plan.request), ProviderCall: true,
	}
	attempt := studyMapV32StageAttempt{
		Version: 1, PromptVersion: prompt.Version, BundleSHA256: bundleSHA,
		ValidationState: "started", Metrics: metrics,
	}
	started := time.Now()
	result, callErr := provider.DiscoverSemanticsMeasured(ctx, prompt)
	metrics.addResponse(result, time.Since(started))
	attempt.Metrics = metrics
	if ctxErr := ctx.Err(); ctxErr != nil {
		metrics.Status = "canceled"
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(ctxErr.Error())
		return nil, metrics, attempt, ctxErr
	}
	if callErr != nil {
		metrics.Status = "failed_provider"
		attempt.Metrics = metrics
		attempt.ValidationState = metrics.Status
		attempt.FailureReason = semanticDiscoveryReason(callErr.Error())
		return nil, metrics, attempt, fmt.Errorf("study map: %s provider call: %w", stage, callErr)
	}
	if json.Valid(result.Content) {
		attempt.Response = append(json.RawMessage(nil), result.Content...)
	}
	metrics.Status = "accepted_transport"
	attempt.Metrics = metrics
	attempt.ValidationState = metrics.Status
	return append([]byte(nil), result.Content...), metrics, attempt, nil
}

func reviewStudyMapDirections(
	ctx context.Context,
	runDir string,
	bundle studymap.Bundle,
	directions studymap.DirectionProposal,
	bundleSHA string,
	provider semanticDiscoveryEditor,
) (
	[]studymap.ReviewProposal,
	[]studyMapReviewSummary,
	[]semanticDiscoveryStageMetrics,
	[]studymap.ReviewIssue,
	error,
) {
	if err := os.Remove(filepath.Join(runDir, studyMapReviewsFile)); err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, nil, fmt.Errorf("study map: clear saved reviews: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(runDir, studyMapReviewAttemptsDir)); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("study map: clear review attempts: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, studyMapReviewAttemptsDir), 0o700); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("study map: create review attempts: %w", err)
	}
	tasks := make([]studyMapReviewTaskInput, 0, len(directions.Directions))
	ordered := make([]studyMapReviewCompletion, 0, len(directions.Directions))
	for index, direction := range directions.Directions {
		stage := "reading_pack_review/" + direction.DirectionID
		reviewBundle, err := studymap.BuildReviewBundle(bundle, direction)
		if err != nil {
			ordered = append(ordered, rejectedStudyMapReviewPreparation(
				studyMapReviewPreparationFailure{
					index:       index,
					directionID: direction.DirectionID,
					bundleSHA:   bundleSHA,
					stage:       stage,
					issueCode:   "review_bundle_build_failed",
					cause:       err,
				},
			))
			continue
		}
		raw, err := json.Marshal(reviewBundle)
		if err != nil {
			ordered = append(ordered, rejectedStudyMapReviewPreparation(
				studyMapReviewPreparationFailure{
					index:       index,
					directionID: direction.DirectionID,
					bundleSHA:   bundleSHA,
					stage:       stage,
					issueCode:   "review_bundle_encode_failed",
					bundle:      &reviewBundle,
					cause:       err,
				},
			))
			continue
		}
		prompt := semanticdiscovery.Prompt{
			Version: semanticdiscovery.ReadingPackReviewPromptVersion,
			System:  studyMapReviewSystemPrompt, User: studyMapReviewTask + string(raw),
			ThinkingProfile: semanticdiscovery.ThinkingHigh,
			ProgressLabel:   "reading pack review " + reviewBundle.DirectionID,
		}
		plan, err := newSemanticDiscoveryStagePlan(provider, prompt, stage)
		if err != nil {
			ordered = append(ordered, rejectedStudyMapReviewPreparation(
				studyMapReviewPreparationFailure{
					index:       index,
					directionID: direction.DirectionID,
					bundleSHA:   bundleSHA,
					stage:       stage,
					issueCode:   "review_request_plan_failed",
					bundle:      &reviewBundle,
					cause:       err,
				},
			))
			continue
		}
		tasks = append(tasks, studyMapReviewTaskInput{index: index, bundle: reviewBundle, plan: plan})
	}
	completions := make(chan studyMapReviewCompletion, len(tasks))
	semaphore := make(chan struct{}, studyMapV32ReviewConcurrency)
	var wait sync.WaitGroup
	for _, task := range tasks {
		task := task
		wait.Go(func() {
			completion := executeStudyMapReview(ctx, provider, task, bundleSHA, semaphore)
			completions <- completion
		})
	}
	wait.Wait()
	close(completions)
	for completion := range completions {
		ordered = append(ordered, completion)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	reviews := make([]studymap.ReviewProposal, 0, len(ordered))
	summaries := make([]studyMapReviewSummary, 0, len(ordered))
	stages := make([]semanticDiscoveryStageMetrics, 0, len(ordered))
	issues := make([]studymap.ReviewIssue, 0, len(ordered))
	for _, completion := range ordered {
		attemptPath := filepath.Join(runDir, studyMapReviewAttemptsDir, completion.directionID+".json")
		if err := writeGoldenJSON(attemptPath, completion.attempt); err != nil {
			return nil, nil, stages, issues, err
		}
		stages = append(stages, completion.attempt.Metrics)
		summaries = append(summaries, studyMapReviewSummary{
			DirectionID:     completion.directionID,
			ValidationState: completion.attempt.ValidationState,
			IssueCode:       completion.attempt.IssueCode,
			FailureReason:   completion.attempt.FailureReason,
			Metrics:         completion.attempt.Metrics,
		})
		if completion.issue.Code != "" {
			issues = append(issues, completion.issue)
		}
		if completion.valid {
			reviews = append(reviews, completion.proposal)
		}
	}
	if err := ctx.Err(); err != nil {
		return reviews, summaries, stages, issues, err
	}
	return reviews, summaries, stages, issues, nil
}

func rejectedStudyMapReviewPreparation(
	failure studyMapReviewPreparationFailure,
) studyMapReviewCompletion {
	metrics := semanticDiscoveryStageMetrics{
		Stage:         failure.stage,
		PromptVersion: semanticdiscovery.ReadingPackReviewPromptVersion,
		Status:        "rejected",
	}
	attempt := studyMapReviewAttempt{
		Version:         1,
		PromptVersion:   semanticdiscovery.ReadingPackReviewPromptVersion,
		BundleSHA256:    failure.bundleSHA,
		DirectionID:     failure.directionID,
		ValidationState: metrics.Status,
		IssueCode:       failure.issueCode,
		FailureReason:   semanticDiscoveryReason(failure.cause.Error()),
		Metrics:         metrics,
		Bundle:          failure.bundle,
	}
	return studyMapReviewCompletion{
		index:       failure.index,
		directionID: failure.directionID,
		attempt:     attempt,
		issue: studymap.ReviewIssue{
			DirectionID: failure.directionID,
			Code:        failure.issueCode,
			Detail:      semanticDiscoveryReason(failure.cause.Error()),
		},
	}
}

func executeStudyMapReview(
	ctx context.Context,
	provider semanticDiscoveryEditor,
	task studyMapReviewTaskInput,
	bundleSHA string,
	semaphore chan struct{},
) studyMapReviewCompletion {
	metrics := semanticDiscoveryStageMetrics{
		Stage: task.plan.name, PromptVersion: task.plan.prompt.Version,
		RequestBytes: len(task.plan.request), ProviderCall: true,
	}
	attempt := studyMapReviewAttempt{
		Version: 1, PromptVersion: task.plan.prompt.Version, BundleSHA256: bundleSHA,
		DirectionID: task.bundle.DirectionID, ValidationState: "started",
		Metrics: metrics, Bundle: &task.bundle,
	}
	completion := studyMapReviewCompletion{
		index: task.index, directionID: task.bundle.DirectionID,
		bundle: task.bundle, attempt: attempt,
	}
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		metrics.Status = "canceled"
		completion.attempt.Metrics = metrics
		completion.attempt.ValidationState = metrics.Status
		completion.attempt.FailureReason = semanticDiscoveryReason(ctx.Err().Error())
		return completion
	}
	started := time.Now()
	result, callErr := provider.DiscoverSemanticsMeasured(ctx, task.plan.prompt)
	metrics.addResponse(result, time.Since(started))
	completion.attempt.Metrics = metrics
	if ctxErr := ctx.Err(); ctxErr != nil {
		metrics.Status = "canceled"
		completion.attempt.Metrics = metrics
		completion.attempt.ValidationState = metrics.Status
		completion.attempt.FailureReason = semanticDiscoveryReason(ctxErr.Error())
		return completion
	}
	if callErr != nil {
		metrics.Status = "failed_provider"
		completion.attempt.Metrics = metrics
		completion.attempt.ValidationState = metrics.Status
		completion.attempt.FailureReason = semanticDiscoveryReason(callErr.Error())
		return completion
	}
	if json.Valid(result.Content) {
		completion.attempt.Response = append(json.RawMessage(nil), result.Content...)
	} else {
		completion.attempt.RawResponse = string(result.Content)
	}
	proposal, err := studymap.DecodeReviewProposal(result.Content)
	if err != nil || proposal.DirectionID != task.bundle.DirectionID {
		metrics.Status = "rejected"
		completion.attempt.Metrics = metrics
		completion.attempt.ValidationState = metrics.Status
		if err != nil {
			completion.attempt.FailureReason = semanticDiscoveryReason(err.Error())
		} else {
			completion.attempt.FailureReason = "review_direction_mismatch"
		}
		return completion
	}
	metrics.Status = "accepted"
	completion.attempt.Metrics = metrics
	completion.attempt.ValidationState = metrics.Status
	completion.proposal = proposal
	completion.valid = true
	return completion
}

func studyMapV32InputsFromRecord(
	record studymap.Record,
) (studymap.BriefShapeProposal, studymap.DirectionProposal, error) {
	brief := studymap.BriefShapeProposal{
		Version: studymap.BriefShapeProposalVersion, RepositoryType: record.RepositoryType,
		Brief: record.Brief, ShapeAreaIDs: append([]string(nil), record.ShapeAreaIDs...),
	}
	directions := studymap.DirectionProposal{Version: studymap.DirectionProposalVersion}
	for _, direction := range record.Directions {
		directions.Directions = append(directions.Directions, studymap.DirectionCandidate{
			Question:     direction.Question,
			WhyItMatters: direction.WhyItMatters, LearningOutcome: direction.LearningOutcome,
			TargetJob: direction.TargetJob, LearningStage: direction.LearningStage,
			AnchorIDs:   append([]string(nil), direction.AnchorIDs...),
			DocumentIDs: append([]string(nil), direction.DocumentIDs...),
			AreaIDs:     append([]string(nil), direction.AreaIDs...), MechanismID: direction.MechanismID,
			ReadingAnchors: append([]studymap.ReadingAnchor(nil), direction.ReadingAnchors...),
			SearchQueries:  append([]string(nil), direction.SearchQueries...),
		})
	}
	normalized, err := studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	return brief, normalized, nil
}

func aggregateStudyMapMetrics(
	stages []semanticDiscoveryStageMetrics,
	outcomeErr error,
) semanticDiscoveryStageMetrics {
	total := semanticDiscoveryStageMetrics{Stage: "repository_study_map_v32", PromptVersion: "repository-study-map-split-v2"}
	statuses := make(map[string]struct{}, len(stages))
	accepted := 0
	for _, stage := range stages {
		total.ProviderCall = total.ProviderCall || stage.ProviderCall
		total.RequestBytes += stage.RequestBytes
		total.ResponseBytes += stage.ResponseBytes
		total.InputTokens += stage.InputTokens
		total.OutputTokens += stage.OutputTokens
		total.PromptCacheHitTokens += stage.PromptCacheHitTokens
		total.PromptCacheMissTokens += stage.PromptCacheMissTokens
		total.LatencyMillis += stage.LatencyMillis
		statuses[stage.Status] = struct{}{}
		if stage.Status == "accepted" {
			accepted++
		}
	}
	switch {
	case len(stages) == 0:
		total.Status = "not_run"
	case outcomeErr != nil:
		total.Status = failedStudyMapAggregateStatus(statuses)
	case accepted == len(stages):
		total.Status = "accepted"
	case len(statuses) == 1:
		for status := range statuses {
			total.Status = status
		}
	default:
		total.Status = "partial"
	}
	return total
}

func failedStudyMapAggregateStatus(statuses map[string]struct{}) string {
	if _, canceled := statuses["canceled"]; canceled {
		return "canceled"
	}
	if _, providerFailed := statuses["failed_provider"]; providerFailed {
		return "failed_provider"
	}
	return "rejected"
}

func studyMapV32ProposalFromRecord(record studymap.Record) studymap.Proposal {
	proposal := studymap.Proposal{
		Version: studymap.ProposalVersion, RepositoryType: record.RepositoryType,
		Brief: record.Brief, ShapeAreaIDs: append([]string(nil), record.ShapeAreaIDs...),
	}
	for _, direction := range record.Directions {
		proposal.Candidates = append(proposal.Candidates, studymap.Candidate{
			Question: direction.Question, WhyItMatters: direction.WhyItMatters,
			LearningOutcome: direction.LearningOutcome, TargetJob: direction.TargetJob,
			LearningStage: direction.LearningStage,
			AnchorIDs:     append([]string(nil), direction.AnchorIDs...),
			DocumentIDs:   append([]string(nil), direction.DocumentIDs...),
			AreaIDs:       append([]string(nil), direction.AreaIDs...), MechanismID: direction.MechanismID,
			ReadingAnchors: append([]studymap.ReadingAnchor(nil), direction.ReadingAnchors...),
			SearchQueries:  append([]string(nil), direction.SearchQueries...),
		})
	}
	return proposal
}
