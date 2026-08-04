package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenMechanismProbeAttemptFile      = "golden_mechanism_probe_attempt.json"
	goldenMechanismProjectionAttemptFile = "golden_mechanism_projection_attempt.json"
	goldenMechanismRejectedResponseFile  = "golden_mechanism_response_attempt.json"
	goldenMechanismResponseAttemptFile   = "golden_mechanism_response_attempt_v2.json"
	goldenMechanismStatusFile            = "golden_mechanism_status.json"
	maxGoldenSavedFileBytes              = 32 << 20
)

type goldenMechanismStatus struct {
	Version             int                                     `json:"version"`
	State               string                                  `json:"state"`
	FailureClass        string                                  `json:"failure_class,omitempty"`
	FailureReason       string                                  `json:"failure_reason,omitempty"`
	CandidateID         string                                  `json:"candidate_id"`
	Question            string                                  `json:"question"`
	RepositoryRevision  string                                  `json:"repository_revision,omitempty"`
	BaseArtifactCount   int                                     `json:"base_artifact_count"`
	ProbeSHA256         string                                  `json:"probe_sha256,omitempty"`
	ProbePartial        bool                                    `json:"probe_partial,omitempty"`
	ProbeStopReason     goldenmechanism.StopReason              `json:"probe_stop_reason,omitempty"`
	ProbeElapsedMillis  int64                                   `json:"probe_elapsed_ms,omitempty"`
	ProbeBudget         goldenmechanism.BudgetStats             `json:"probe_budget,omitempty"`
	CapabilityContract  *semanticdiscovery.CapabilityContract   `json:"capability_contract,omitempty"`
	RequiredAspects     []semanticdiscovery.AnswerAspect        `json:"required_answer_aspects,omitempty"`
	Synthesis           *semanticDiscoveryStageMetrics          `json:"synthesis,omitempty"`
	Reduction           *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	Artifact            *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	CanonicalFactsFile  string                                  `json:"canonical_facts_file,omitempty"`
	CanonicalRecordFile string                                  `json:"canonical_record_file,omitempty"`
	ReportFile          string                                  `json:"report_file,omitempty"`
}

type goldenMechanismArtifactSummary struct {
	ID                     string                    `json:"id"`
	ContentSHA256          string                    `json:"content_sha256"`
	Title                  string                    `json:"title"`
	Summary                string                    `json:"summary"`
	Verdict                semanticdiscovery.Verdict `json:"verdict"`
	SupportedSteps         int                       `json:"supported_steps"`
	RequiredAnswerAspects  []string                  `json:"required_answer_aspects"`
	CoveredAnswerAspects   []string                  `json:"covered_answer_aspects"`
	UncoveredAnswerAspects []string                  `json:"uncovered_answer_aspects"`
	LocalRubricScore       int                       `json:"local_rubric_score"`
}

type goldenMechanismResponseAttempt struct {
	Version          int                                     `json:"version"`
	CandidateID      string                                  `json:"candidate_id"`
	PromptVersion    string                                  `json:"prompt_version"`
	ValidationStatus string                                  `json:"validation_status,omitempty"`
	FailureClass     string                                  `json:"failure_class,omitempty"`
	Reduction        *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	Content          string                                  `json:"content"`
}

type goldenMechanismRun struct {
	runDir        string
	analysisRoot  string
	manifest      report.RunManifest
	current       freshness.RepositoryState
	data          *report.ReportData
	bundle        semanticdiscovery.Bundle
	record        semanticdiscovery.Record
	candidate     semanticdiscovery.OpportunityCandidate
	baseArtifacts []semanticdiscovery.Artifact
}

type goldenMechanismSynthesis struct {
	RecordBytes []byte
	FanIn       semanticdiscovery.FanInArtifact
	Artifacts   []semanticdiscovery.Artifact
	Metrics     semanticDiscoveryStageMetrics
	Reduction   semanticdiscovery.FanInReductionReport
	RawResponse []byte
}

func runGoldenMechanismCLI(args []string, stdout, stderr io.Writer) error {
	runDir, probeOnly, err := parseGoldenMechanismArgs(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runGoldenMechanism(ctx, runDir, probeOnly, stdout, stderr)
}

func parseGoldenMechanismArgs(args []string) (string, bool, error) {
	runDir := ""
	probeOnly := false
	for _, arg := range args {
		switch arg {
		case "--probe-only":
			probeOnly = true
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", false, fmt.Errorf(
					"Usage: repomap dev golden-mechanism <run-dir> [--probe-only]",
				)
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", false, fmt.Errorf(
			"Usage: repomap dev golden-mechanism <run-dir> [--probe-only]",
		)
	}
	return runDir, probeOnly, nil
}

func runGoldenMechanism(
	ctx context.Context,
	runDir string,
	probeOnly bool,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("golden mechanism: resolve run directory: %w", err)
	}
	status := goldenMechanismStatus{
		Version: 1, State: "started",
		CandidateID: goldenDirectoryListingCandidateID,
		Question:    "How does the file server generate and sort directory listings?",
	}
	defer func() {
		if returnErr != nil && status.State != "published" && status.State != "probe_validated" {
			status.State = "failed"
			if status.FailureClass == "" {
				status.FailureClass = "local_stage_failure"
			}
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		if err := writeGoldenJSON(
			filepath.Join(absDir, goldenMechanismStatusFile),
			status,
		); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	loaded, err := loadGoldenMechanismRun(ctx, absDir)
	if err != nil {
		status.FailureClass = "saved_run_unavailable"
		return err
	}
	status.RepositoryRevision = loaded.manifest.RepositoryState.Head
	status.BaseArtifactCount = len(loaded.baseArtifacts)
	fmt.Fprintf(
		stderr,
		"repomap: golden mechanism using saved Caddy facts at revision %s\n",
		loaded.manifest.RepositoryState.Head,
	)

	plan, err := caddyDirectoryListingPlan(loaded.bundle)
	if err != nil {
		status.FailureClass = "probe_did_not_find_required_facts"
		return err
	}
	fmt.Fprintf(
		stderr,
		"repomap: probing one mechanism from %d saved source anchors (max %d files, %d functions, %d retained bytes)\n",
		len(plan.Seeds), plan.Limits.MaxFiles, plan.Limits.MaxFunctions, plan.Limits.MaxSourceBytes,
	)
	probe, err := goldenmechanism.Probe(ctx, loaded.analysisRoot, plan)
	if err != nil {
		status.FailureClass = "probe_did_not_find_required_facts"
		return err
	}
	if err := ctx.Err(); err != nil {
		status.FailureClass = "canceled"
		return err
	}
	status.ProbeElapsedMillis = probe.Budget.ElapsedMillis
	status.ProbeBudget = probe.Budget
	status.ProbePartial = probe.Partial
	status.ProbeStopReason = probe.StopReason
	stableProbe := probe
	stableProbe.Budget.ElapsedMillis = 0
	probeBytes, err := marshalGoldenJSON(stableProbe)
	if err != nil {
		status.FailureClass = "probe_artifact_invalid"
		return err
	}
	if kind, sensitive := secretscan.Detect(string(probeBytes)); sensitive {
		status.FailureClass = "probe_artifact_sensitive"
		return fmt.Errorf("golden mechanism: probe artifact contains an obvious %s", kind)
	}
	probeDigest := sha256.Sum256(probeBytes)
	status.ProbeSHA256 = hex.EncodeToString(probeDigest[:])
	probePath := filepath.Join(absDir, goldenMechanismProbeAttemptFile)
	if err := writeAtomicFile(probePath, probeBytes, 0o600); err != nil {
		status.FailureClass = "probe_artifact_write_failed"
		return err
	}

	projection, err := projectCaddyDirectoryListing(
		loaded.bundle,
		loaded.candidate,
		probe,
	)
	if err != nil {
		status.FailureClass = "probe_did_not_find_required_facts"
		return err
	}
	supplement, enrichedBundle, err := report.PrepareSemanticSupplement(
		loaded.data,
		projection.Candidate.ID,
		status.ProbeSHA256,
		projection.Facts,
	)
	if err != nil {
		status.FailureClass = "probe_facts_lost_in_bundling"
		return fmt.Errorf("golden mechanism: prepare local fact supplement: %w", err)
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(enrichedBundle, proposal); err != nil {
		status.FailureClass = "probe_facts_lost_in_bundling"
		return err
	}
	leaf, err := buildGoldenMechanismLeaf(enrichedBundle, projection.Candidate)
	if err != nil {
		status.FailureClass = "probe_facts_lost_in_bundling"
		return err
	}
	projection.Leaf = leaf
	projectionPath := filepath.Join(absDir, goldenMechanismProjectionAttemptFile)
	if err := writeGoldenJSON(projectionPath, projection); err != nil {
		status.FailureClass = "probe_artifact_write_failed"
		return err
	}
	status.CapabilityContract = projection.Candidate.CapabilityContract
	status.RequiredAspects = append(
		[]semanticdiscovery.AnswerAspect(nil),
		projection.Candidate.IntentContract.RequiredAnswerAspects...,
	)
	fmt.Fprintf(
		stderr,
		"repomap: probe retained %d functions and projected %d locally checked mechanism facts (partial=%t, stop=%s)\n",
		probe.Budget.FunctionsIncluded,
		len(projection.Facts),
		probe.Partial,
		probe.StopReason,
	)
	if probeOnly {
		status.State = "probe_validated"
		fmt.Fprintf(stdout, "Probe: %s\nProjection: %s\n", probePath, projectionPath)
		return nil
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		status.FailureClass = "provider_unavailable"
		return fmt.Errorf("golden mechanism: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	fmt.Fprintln(stderr, "repomap: editing one bounded directory-listing mechanism from verified local facts")
	synthesis, err := executeGoldenMechanismSynthesis(
		ctx,
		enrichedBundle,
		proposal,
		leaf,
		client,
	)
	status.Synthesis = &synthesis.Metrics
	status.Reduction = &synthesis.Reduction
	if len(synthesis.RawResponse) > 0 {
		responsePath := filepath.Join(absDir, goldenMechanismResponseAttemptFile)
		validationStatus := "accepted"
		failureClass := ""
		if err != nil {
			validationStatus = "rejected"
			failureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		}
		response := goldenMechanismResponseAttempt{
			Version: 1, CandidateID: projection.Candidate.ID,
			PromptVersion:    semanticdiscovery.GoldenMechanismPromptVersion,
			ValidationStatus: validationStatus,
			FailureClass:     failureClass,
			Reduction:        &synthesis.Reduction,
			Content:          string(synthesis.RawResponse),
		}
		if saveErr := writeGoldenJSON(responsePath, response); saveErr != nil {
			status.FailureClass = "response_artifact_write_failed"
			return errors.Join(err, saveErr)
		}
	}
	if err != nil {
		status.FailureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		return err
	}
	artifact := synthesis.Artifacts[0]
	summary, err := summarizeGoldenMechanismArtifact(projection.Candidate, artifact)
	if err != nil {
		status.FailureClass = "validator_passed_irrelevant_artifact"
		return err
	}
	status.Artifact = &summary

	supplementBytes, err := marshalGoldenJSON(supplement)
	if err != nil {
		status.FailureClass = "local_publish_failure"
		return err
	}
	recordBytes := append(append([]byte(nil), synthesis.RecordBytes...), '\n')
	if err := publishGoldenMechanism(
		ctx,
		loaded,
		supplementBytes,
		recordBytes,
		artifact,
	); err != nil {
		status.FailureClass = "report_distorted_good_artifact"
		return err
	}
	status.State = "published"
	status.CanonicalFactsFile = filepath.Join(absDir, report.GoldenMechanismFactsFile)
	status.CanonicalRecordFile = filepath.Join(absDir, report.GoldenMechanismRecordFile)
	status.ReportFile = filepath.Join(absDir, "report.html")
	fmt.Fprintf(
		stderr,
		"repomap: golden mechanism accepted with %d supported step(s), %d covered aspect(s), score %d/4\n",
		summary.SupportedSteps,
		len(summary.CoveredAnswerAspects),
		summary.LocalRubricScore,
	)
	fmt.Fprintf(
		stdout,
		"Golden mechanism: %s\nReport: %s\n",
		status.CanonicalRecordFile,
		status.ReportFile,
	)
	return nil
}

func loadGoldenMechanismRun(
	ctx context.Context,
	runDir string,
) (goldenMechanismRun, error) {
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: verify saved run manifest: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return goldenMechanismRun{}, err
	}
	current, err := freshness.CaptureRepository(ctx, analysisRoot)
	if err != nil {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: capture repository state: %w", err)
	}
	if current.Head != manifest.RepositoryState.Head {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: repository revision changed")
	}
	if err := manifest.VerifyRepositoryState(current); err != nil {
		return goldenMechanismRun{}, err
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: read saved report: %w", err)
	}
	data.SemanticSupplementalFacts = nil
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: rebuild saved semantic bundle: %w", err)
	}
	raw, err := readBoundedRegularFile(
		filepath.Join(runDir, semanticdiscovery.RecordFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismRun{}, err
	}
	record, err := semanticdiscovery.DecodeRecord(raw)
	if err != nil {
		return goldenMechanismRun{}, err
	}
	baseArtifacts, err := semanticdiscovery.ReplayRecord(bundle, raw)
	if err != nil {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: replay base semantic record: %w", err)
	}
	var candidate semanticdiscovery.OpportunityCandidate
	for _, item := range record.Opportunity.Candidates {
		if item.ID == goldenDirectoryListingCandidateID {
			candidate = item
			break
		}
	}
	if candidate.ID == "" || candidate.Kind != semanticdiscovery.ArtifactMechanism {
		return goldenMechanismRun{}, fmt.Errorf("golden mechanism: saved directory-listing candidate is unavailable")
	}
	return goldenMechanismRun{
		runDir: runDir, analysisRoot: analysisRoot,
		manifest: manifest, current: current, data: data, bundle: bundle,
		record: record, candidate: candidate, baseArtifacts: baseArtifacts,
	}, nil
}

func executeGoldenMechanismSynthesis(
	ctx context.Context,
	bundle semanticdiscovery.Bundle,
	proposal semanticdiscovery.OpportunityProposal,
	leaf semanticdiscovery.LeafResult,
	provider semanticDiscoveryEditor,
) (result goldenMechanismSynthesis, returnErr error) {
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		return result, err
	}
	plan, err := newSemanticDiscoveryStagePlan(
		provider,
		prompt,
		"golden_mechanism_synthesis",
	)
	if err != nil {
		return result, err
	}
	result.Metrics, returnErr = executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		&semanticDiscoveryBudget{},
		func(raw []byte) error {
			evaluated, err := evaluateGoldenMechanismResponse(
				bundle,
				proposal,
				leaf,
				raw,
			)
			result.RecordBytes = evaluated.RecordBytes
			result.FanIn = evaluated.FanIn
			result.Artifacts = evaluated.Artifacts
			result.Reduction = evaluated.Reduction
			result.RawResponse = evaluated.RawResponse
			return err
		},
	)
	return result, returnErr
}

func evaluateGoldenMechanismResponse(
	bundle semanticdiscovery.Bundle,
	proposal semanticdiscovery.OpportunityProposal,
	leaf semanticdiscovery.LeafResult,
	raw []byte,
) (result goldenMechanismSynthesis, returnErr error) {
	result.RawResponse = append([]byte(nil), raw...)
	if kind, sensitive := secretscan.Detect(string(raw)); sensitive {
		return result, fmt.Errorf("golden mechanism: response contains an obvious %s", kind)
	}
	parsed, err := semanticdiscovery.ParseFanInArtifact(raw)
	if err != nil {
		return result, err
	}
	parsed = semanticdiscovery.NormalizeFanInArtifact(parsed)
	if len(parsed.Artifacts) != 1 {
		return result, fmt.Errorf("golden mechanism: synthesis returned %d artifacts, want one", len(parsed.Artifacts))
	}
	reduced, reduction, err := semanticdiscovery.ReduceFanInArtifact(
		bundle,
		[]semanticdiscovery.LeafResult{leaf},
		parsed,
	)
	result.Reduction = reduction
	if err != nil {
		return result, err
	}
	if reduction.DroppedArtifacts != 0 || len(reduced.Artifacts) != 1 {
		return result, fmt.Errorf("golden mechanism: synthesis did not preserve exactly one valid proposal")
	}
	artifacts, err := semanticdiscovery.MaterializePartialArtifacts(
		bundle,
		[]semanticdiscovery.LeafResult{leaf},
		reduced,
	)
	if err != nil {
		return result, err
	}
	if len(artifacts) != 1 {
		return result, fmt.Errorf("golden mechanism: synthesis materialized %d artifacts", len(artifacts))
	}
	if _, err := summarizeGoldenMechanismArtifact(
		proposal.Candidates[0],
		artifacts[0],
	); err != nil {
		return result, err
	}
	record, err := semanticdiscovery.EncodeRecord(
		bundle,
		proposal,
		[]semanticdiscovery.OpportunityCandidate{proposal.Candidates[0]},
		[]semanticdiscovery.LeafResult{leaf},
		reduced,
	)
	if err != nil {
		return result, err
	}
	result.RecordBytes = record
	result.FanIn = reduced
	result.Artifacts = artifacts
	return result, nil
}

func classifyGoldenMechanismValidationFailure(
	reduction semanticdiscovery.FanInReductionReport,
) string {
	found := false
	for _, issue := range reduction.Issues {
		for _, reason := range issue.Reasons {
			found = true
			switch reason.Code {
			case semanticdiscovery.FanInReasonUnknownRepositoryReference,
				semanticdiscovery.FanInReasonUnsupportedSequence,
				semanticdiscovery.FanInReasonLimitationNotExplicit:
			default:
				return "llm_ignored_sufficient_facts"
			}
		}
	}
	if found {
		return "prompt_validator_contract_mismatch"
	}
	return "llm_ignored_sufficient_facts"
}

func summarizeGoldenMechanismArtifact(
	candidate semanticdiscovery.OpportunityCandidate,
	artifact semanticdiscovery.Artifact,
) (goldenMechanismArtifactSummary, error) {
	if artifact.CandidateID != candidate.ID || artifact.Question != candidate.QuestionAnswered {
		return goldenMechanismArtifactSummary{}, fmt.Errorf("golden mechanism: artifact lost its original identity or question")
	}
	if len(artifact.Steps) < 3 || len(artifact.Steps) > 7 {
		return goldenMechanismArtifactSummary{}, fmt.Errorf("golden mechanism: artifact step count is outside 3..7")
	}
	supported := 0
	for _, statement := range artifact.Statements {
		if statement.Basis != semanticdiscovery.ClaimUnresolved {
			supported++
		}
	}
	if supported < 3 {
		return goldenMechanismArtifactSummary{}, fmt.Errorf("golden mechanism: artifact has fewer than three supported steps")
	}
	covered := make(map[string]struct{}, len(artifact.CoveredAspectIDs))
	for _, id := range artifact.CoveredAspectIDs {
		covered[id] = struct{}{}
	}
	stronglyCovered := make(map[string]struct{})
	for _, statement := range artifact.Statements {
		if statement.Basis != semanticdiscovery.ClaimDirect &&
			statement.Basis != semanticdiscovery.ClaimCompositional {
			continue
		}
		for _, aspectID := range statement.AspectIDs {
			stronglyCovered[aspectID] = struct{}{}
		}
	}
	keyCovered := 0
	keyCount := 0
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		if aspect.Key {
			keyCount++
			if _, exists := stronglyCovered[aspect.ID]; exists {
				keyCovered++
			}
		}
	}
	if len(covered) < candidate.IntentContract.MinCovered ||
		keyCovered < candidate.IntentContract.MinKeyCovered {
		return goldenMechanismArtifactSummary{}, fmt.Errorf("golden mechanism: artifact misses the fixed intent threshold")
	}
	if keyCovered != keyCount {
		return goldenMechanismArtifactSummary{}, fmt.Errorf(
			"golden mechanism: artifact does not directly or compositionally cover every key answer aspect",
		)
	}
	if len(artifact.UncoveredAspectIDs) > 0 && len(artifact.Unknowns) == 0 {
		return goldenMechanismArtifactSummary{}, fmt.Errorf(
			"golden mechanism: artifact hides its uncovered answer aspects",
		)
	}
	if artifact.Verdict == semanticdiscovery.VerdictInsufficientEvidence {
		return goldenMechanismArtifactSummary{}, fmt.Errorf("golden mechanism: artifact remained insufficient")
	}
	contentSHA256, err := goldenMechanismArtifactSHA256(artifact)
	if err != nil {
		return goldenMechanismArtifactSummary{}, err
	}
	score := 3
	if len(covered) >= 7 && supported >= 5 && len(artifact.Unknowns) > 0 {
		score = 4
	}
	return goldenMechanismArtifactSummary{
		ID: artifact.ID, ContentSHA256: contentSHA256,
		Title: artifact.Title, Summary: artifact.Summary,
		Verdict: artifact.Verdict, SupportedSteps: supported,
		RequiredAnswerAspects:  append([]string(nil), artifact.RequiredAspectIDs...),
		CoveredAnswerAspects:   append([]string(nil), artifact.CoveredAspectIDs...),
		UncoveredAnswerAspects: append([]string(nil), artifact.UncoveredAspectIDs...),
		LocalRubricScore:       score,
	}, nil
}

func publishGoldenMechanism(
	ctx context.Context,
	loaded goldenMechanismRun,
	supplementBytes []byte,
	recordBytes []byte,
	want semanticdiscovery.Artifact,
) error {
	return publishGoldenMechanismArtifacts(
		ctx,
		loaded,
		supplementBytes,
		recordBytes,
		[]semanticdiscovery.Artifact{want},
	)
}

func publishGoldenMechanismArtifacts(
	ctx context.Context,
	loaded goldenMechanismRun,
	supplementBytes []byte,
	recordBytes []byte,
	wants []semanticdiscovery.Artifact,
) (returnErr error) {
	if err := validateGoldenArtifactExpectations(wants); err != nil {
		return err
	}
	paths := []string{
		filepath.Join(loaded.runDir, report.GoldenMechanismRecordFile),
		filepath.Join(loaded.runDir, report.GoldenMechanismFactsFile),
		filepath.Join(loaded.runDir, "report.json"),
		filepath.Join(loaded.runDir, "report.html"),
		filepath.Join(loaded.runDir, report.RunManifestFilename),
	}
	backups, err := backupGoldenFiles(paths)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := restoreGoldenFiles(backups); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("golden mechanism: rollback: %w", err))
		}
	}()

	if err := writeAtomicFile(paths[0], recordBytes, 0o600); err != nil {
		return err
	}
	if err := writeAtomicFile(paths[1], supplementBytes, 0o600); err != nil {
		return err
	}
	replayed, err := report.ReadRunDir(loaded.runDir)
	if err != nil {
		return err
	}
	for _, warning := range replayed.Warnings {
		if strings.HasPrefix(warning, "golden mechanism unavailable:") {
			return fmt.Errorf(
				"golden mechanism: report replay rejected canonical pair: %s",
				warning,
			)
		}
	}
	if err := requireGoldenArtifacts(replayed.SemanticArtifacts, wants); err != nil {
		return err
	}
	current, err := freshness.CaptureRepository(ctx, loaded.analysisRoot)
	if err != nil {
		return err
	}
	if err := loaded.manifest.VerifyRepositoryState(current); err != nil {
		return err
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx,
		loaded.analysisRoot,
		loaded.manifest.RepositoryState,
		current,
		report.CapturedInputPaths(replayed),
		true,
	)
	if err != nil {
		return err
	}
	if err := report.GenerateAuthorized(loaded.runDir, authority); err != nil {
		return fmt.Errorf("golden mechanism: generate authorized report: %w", err)
	}
	if _, err := report.ReadRunManifest(loaded.runDir); err != nil {
		return fmt.Errorf("golden mechanism: verify regenerated report manifest: %w", err)
	}
	finalData, err := report.ReadRunDir(loaded.runDir)
	if err != nil {
		return err
	}
	if err := requireGoldenArtifacts(finalData.SemanticArtifacts, wants); err != nil {
		return err
	}
	committed = true
	return nil
}

func requireGoldenArtifact(
	artifacts []semanticdiscovery.Artifact,
	want semanticdiscovery.Artifact,
) error {
	return requireGoldenArtifacts(artifacts, []semanticdiscovery.Artifact{want})
}

func requireGoldenArtifacts(
	artifacts []semanticdiscovery.Artifact,
	wants []semanticdiscovery.Artifact,
) error {
	if err := validateGoldenArtifactExpectations(wants); err != nil {
		return err
	}
	for _, want := range wants {
		matches := make([]semanticdiscovery.Artifact, 0, 1)
		for _, artifact := range artifacts {
			if artifact.CandidateID == want.CandidateID {
				matches = append(matches, artifact)
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("golden mechanism: report replay omitted the accepted artifact")
		}
		if len(matches) > 1 {
			return fmt.Errorf(
				"golden mechanism: report replay duplicated the accepted candidate %q",
				want.CandidateID,
			)
		}
		gotSHA256, err := goldenMechanismArtifactSHA256(matches[0])
		if err != nil {
			return err
		}
		wantSHA256, err := goldenMechanismArtifactSHA256(want)
		if err != nil {
			return err
		}
		if gotSHA256 != wantSHA256 {
			return fmt.Errorf(
				"golden mechanism: report replay changed accepted artifact fields: %s",
				strings.Join(goldenArtifactChangedFields(matches[0], want), ", "),
			)
		}
	}
	return nil
}

func validateGoldenArtifactExpectations(wants []semanticdiscovery.Artifact) error {
	if len(wants) == 0 || len(wants) > semanticdiscovery.MaxSelectedCandidates {
		return fmt.Errorf(
			"golden mechanism: expected artifact count must be between 1 and %d",
			semanticdiscovery.MaxSelectedCandidates,
		)
	}
	candidateIDs := make(map[string]struct{}, len(wants))
	artifactIDs := make(map[string]struct{}, len(wants))
	for _, want := range wants {
		if want.CandidateID == "" || want.ID == "" {
			return fmt.Errorf("golden mechanism: expected artifact identity is incomplete")
		}
		if _, duplicate := candidateIDs[want.CandidateID]; duplicate {
			return fmt.Errorf(
				"golden mechanism: duplicate expected candidate %q",
				want.CandidateID,
			)
		}
		candidateIDs[want.CandidateID] = struct{}{}
		if _, duplicate := artifactIDs[want.ID]; duplicate {
			return fmt.Errorf(
				"golden mechanism: duplicate expected artifact %q",
				want.ID,
			)
		}
		artifactIDs[want.ID] = struct{}{}
	}
	return nil
}

func goldenArtifactChangedFields(
	got semanticdiscovery.Artifact,
	want semanticdiscovery.Artifact,
) []string {
	gotValue := reflect.ValueOf(got)
	wantValue := reflect.ValueOf(want)
	artifactType := gotValue.Type()
	changed := make([]string, 0, artifactType.NumField())
	for index := range artifactType.NumField() {
		if reflect.DeepEqual(
			gotValue.Field(index).Interface(),
			wantValue.Field(index).Interface(),
		) {
			continue
		}
		name := strings.Split(artifactType.Field(index).Tag.Get("json"), ",")[0]
		if name == "" {
			name = artifactType.Field(index).Name
		}
		changed = append(changed, name)
	}
	return changed
}

func goldenMechanismArtifactSHA256(artifact semanticdiscovery.Artifact) (string, error) {
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return "", fmt.Errorf("golden mechanism: encode accepted artifact hash: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

type goldenFileBackup struct {
	path   string
	exists bool
	mode   os.FileMode
	data   []byte
}

func backupGoldenFiles(paths []string) ([]goldenFileBackup, error) {
	backups := make([]goldenFileBackup, 0, len(paths))
	for _, path := range paths {
		backup := goldenFileBackup{path: path}
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				backups = append(backups, backup)
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxGoldenSavedFileBytes {
			return nil, fmt.Errorf("golden mechanism: cannot transactionally back up %s", filepath.Base(path))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		backup.exists = true
		backup.mode = info.Mode().Perm()
		backup.data = data
		backups = append(backups, backup)
	}
	return backups, nil
}

func restoreGoldenFiles(backups []goldenFileBackup) error {
	var result error
	for _, backup := range backups {
		if !backup.exists {
			if err := os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
				result = errors.Join(result, err)
			}
			continue
		}
		if err := writeAtomicFile(backup.path, backup.data, backup.mode); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("golden mechanism: %s is not a bounded regular file", filepath.Base(path))
	}
	return os.ReadFile(path)
}

func marshalGoldenJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("golden mechanism: encode artifact: %w", err)
	}
	return append(encoded, '\n'), nil
}

func writeGoldenJSON(path string, value any) error {
	encoded, err := marshalGoldenJSON(value)
	if err != nil {
		return err
	}
	if kind, sensitive := secretscan.Detect(string(encoded)); sensitive {
		return fmt.Errorf("golden mechanism: refuse to save %s containing an obvious %s", filepath.Base(path), kind)
	}
	return writeAtomicFile(path, encoded, 0o600)
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".repomap-golden-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}
