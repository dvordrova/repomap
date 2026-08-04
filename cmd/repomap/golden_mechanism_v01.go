package main

import (
	"bytes"
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
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenMechanismV01StatusFile         = "golden_mechanism_v01_status.json"
	goldenMechanismOldReplayFile         = "golden_mechanism_v1_replay_diagnostics.json"
	goldenMechanismV2ReservationFile     = "golden_mechanism_v2_call_reservation.json"
	goldenMechanismFixedProjectionSHA256 = "2241c0171ce8bdc3b0aea539b6fa9e5fda2b0bd62ee203036d54e0670a4404ed"
	goldenMechanismRejectedWrapperSHA256 = "c089d18eadd7c2a0691b40e7107c97895953303fbf868ee350e113197d71e199"
	goldenMechanismFixedProbeSHA256      = "5a8c93ed65dc166e52fea16613d9a3a0587700dfc8b83e8d952a092e64b9ba48"
	goldenMechanismRejectedPromptVersion = "semantic-golden-mechanism-json-v1"
	goldenMechanismV01Question           = "How does the file server generate and sort directory listings?"
	goldenMechanismV01Title              = "File Server Directory Listing"
)

var goldenMechanismFixedFactIDs = []string{
	"gmf-0ec8ef0974e6bca4e91a0355",
	"gmf-63fee9ff4e4ab91b356962ee",
	"gmf-7afe2d8279da85ce2346ac3a",
	"gmf-b774c86f6b35d386bcb95299",
	"gmf-e7c6e36ae4486ba9ea1213df",
	"gmf-fc86a4e7f0832a9377f2085c",
}

type goldenMechanismOldReplay struct {
	Version        int                                    `json:"version"`
	State          string                                 `json:"state"`
	FailureClass   string                                 `json:"failure_class"`
	PromptVersion  string                                 `json:"prompt_version"`
	ResponseSHA256 string                                 `json:"response_sha256"`
	Reduction      semanticdiscovery.FanInReductionReport `json:"reduction"`
}

type goldenMechanismV01Status struct {
	Version               int                                     `json:"version"`
	State                 string                                  `json:"state"`
	FailureClass          string                                  `json:"failure_class,omitempty"`
	FailureReason         string                                  `json:"failure_reason,omitempty"`
	CandidateID           string                                  `json:"candidate_id"`
	Question              string                                  `json:"question"`
	RepositoryRevision    string                                  `json:"repository_revision,omitempty"`
	BaseArtifactCount     int                                     `json:"base_artifact_count"`
	FixedProjectionSHA256 string                                  `json:"fixed_projection_sha256,omitempty"`
	FixedProbeSHA256      string                                  `json:"fixed_probe_sha256,omitempty"`
	OldReplay             *goldenMechanismOldReplay               `json:"old_response_replay,omitempty"`
	Synthesis             *semanticDiscoveryStageMetrics          `json:"synthesis,omitempty"`
	Reduction             *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	ProviderCalls         int                                     `json:"provider_calls"`
	ReservationFile       string                                  `json:"reservation_file,omitempty"`
	ResponseAttemptFile   string                                  `json:"response_attempt_file,omitempty"`
	Artifact              *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	CanonicalFactsFile    string                                  `json:"canonical_facts_file,omitempty"`
	CanonicalRecordFile   string                                  `json:"canonical_record_file,omitempty"`
	ReportFile            string                                  `json:"report_file,omitempty"`
	CapabilityContract    *semanticdiscovery.CapabilityContract   `json:"capability_contract,omitempty"`
	RequiredAnswerAspects []semanticdiscovery.AnswerAspect        `json:"required_answer_aspects,omitempty"`
}

type goldenMechanismV01Input struct {
	loaded     goldenMechanismRun
	projection goldenProjection
	supplement report.SemanticSupplement
	bundle     semanticdiscovery.Bundle
	proposal   semanticdiscovery.OpportunityProposal
	leaf       semanticdiscovery.LeafResult
	prompt     semanticdiscovery.Prompt
}

type goldenMechanismCallReservation struct {
	Version          int    `json:"version"`
	State            string `json:"state"`
	CandidateID      string `json:"candidate_id"`
	PromptVersion    string `json:"prompt_version"`
	ProjectionSHA256 string `json:"projection_sha256"`
	ProbeSHA256      string `json:"probe_sha256"`
}

type countingSemanticDiscoveryEditor struct {
	delegate semanticDiscoveryEditor
	calls    int
}

func (editor *countingSemanticDiscoveryEditor) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return editor.delegate.SemanticDiscoveryPromptJSON(prompt)
}

func (editor *countingSemanticDiscoveryEditor) DiscoverSemanticsMeasured(
	ctx context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	editor.calls++
	return editor.delegate.DiscoverSemanticsMeasured(ctx, prompt)
}

func runGoldenMechanismV01CLI(args []string, stdout, stderr io.Writer) error {
	runDir, replayOnly, err := parseGoldenMechanismV01Args(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runGoldenMechanismV01(ctx, runDir, replayOnly, stdout, stderr)
}

func parseGoldenMechanismV01Args(args []string) (string, bool, error) {
	runDir := ""
	replayOnly := false
	for _, arg := range args {
		switch arg {
		case "--replay-old":
			replayOnly = true
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", false, fmt.Errorf(
					"Usage: repomap dev golden-mechanism-v01 <run-dir> [--replay-old]",
				)
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", false, fmt.Errorf(
			"Usage: repomap dev golden-mechanism-v01 <run-dir> [--replay-old]",
		)
	}
	return runDir, replayOnly, nil
}

func runGoldenMechanismV01(
	ctx context.Context,
	runDir string,
	replayOnly bool,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("golden mechanism v0.1: resolve run directory: %w", err)
	}
	status := goldenMechanismV01Status{
		Version:     1,
		State:       "started",
		CandidateID: goldenDirectoryListingCandidateID,
		Question:    goldenMechanismV01Question,
	}
	defer func() {
		if returnErr != nil && status.State != "published" && status.State != "old_response_replayed" {
			status.State = "failed"
			if status.FailureClass == "" {
				status.FailureClass = "local_stage_failure"
			}
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		if err := writeGoldenJSON(filepath.Join(absDir, goldenMechanismV01StatusFile), status); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	input, err := loadGoldenMechanismV01Input(ctx, absDir)
	if err != nil {
		status.FailureClass = "fixed_fixture_invalid"
		return err
	}
	status.RepositoryRevision = input.loaded.manifest.RepositoryState.Head
	status.BaseArtifactCount = len(input.loaded.baseArtifacts)
	status.FixedProjectionSHA256 = goldenMechanismFixedProjectionSHA256
	status.FixedProbeSHA256 = goldenMechanismFixedProbeSHA256
	status.CapabilityContract = input.projection.Candidate.CapabilityContract
	status.RequiredAnswerAspects = append(
		[]semanticdiscovery.AnswerAspect(nil),
		input.projection.Candidate.IntentContract.RequiredAnswerAspects...,
	)
	fmt.Fprintln(stderr, "repomap: Golden Mechanism v0.1 loaded the fixed six-fact Caddy fixture; no probe or repository analyzer will run")

	oldReplay, err := replayRejectedGoldenMechanism(input, absDir)
	status.OldReplay = &oldReplay
	if err != nil {
		status.FailureClass = "old_response_replay_failed"
		return err
	}
	fmt.Fprintf(
		stderr,
		"repomap: old v1 response remains rejected with %d precise reason(s) across %d bounded diagnostic issue(s)\n",
		goldenReductionReasonCount(oldReplay.Reduction),
		len(oldReplay.Reduction.Issues),
	)
	if replayOnly {
		status.State = "old_response_replayed"
		fmt.Fprintf(stdout, "Old response diagnostics: %s\n", filepath.Join(absDir, goldenMechanismOldReplayFile))
		return nil
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		status.FailureClass = "provider_unavailable"
		return fmt.Errorf("golden mechanism v0.1: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	reservationPath := filepath.Join(absDir, goldenMechanismV2ReservationFile)
	if err := reserveGoldenMechanismV2Call(reservationPath); err != nil {
		status.FailureClass = "fresh_synthesis_already_reserved"
		return err
	}
	status.ReservationFile = reservationPath
	fmt.Fprintln(stderr, "repomap: running the single allowed v2 synthesis call over the fixed facts")
	counted := &countingSemanticDiscoveryEditor{delegate: client}
	synthesis, err := executeGoldenMechanismSynthesis(
		ctx,
		input.bundle,
		input.proposal,
		input.leaf,
		counted,
	)
	status.ProviderCalls = counted.calls
	status.Synthesis = &synthesis.Metrics
	status.Reduction = &synthesis.Reduction
	responsePath := filepath.Join(absDir, goldenMechanismResponseAttemptFile)
	status.ResponseAttemptFile = responsePath
	if len(synthesis.RawResponse) > 0 {
		validationStatus := "accepted"
		failureClass := ""
		if err != nil {
			validationStatus = "rejected"
			failureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		}
		response := goldenMechanismResponseAttempt{
			Version: 1, CandidateID: goldenDirectoryListingCandidateID,
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
	if counted.calls != 1 {
		status.FailureClass = "fresh_synthesis_call_count_invalid"
		return errors.Join(err, fmt.Errorf("golden mechanism v0.1: provider calls = %d, want exactly one", counted.calls))
	}
	if err != nil {
		status.FailureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		return err
	}
	artifact := synthesis.Artifacts[0]
	summary, err := summarizeGoldenMechanismArtifact(input.projection.Candidate, artifact)
	if err != nil {
		status.FailureClass = "validator_passed_irrelevant_artifact"
		return err
	}
	status.Artifact = &summary

	supplementBytes, err := marshalGoldenJSON(input.supplement)
	if err != nil {
		status.FailureClass = "local_publish_failure"
		return err
	}
	recordBytes := append(append([]byte(nil), synthesis.RecordBytes...), '\n')
	if err := publishGoldenMechanism(
		ctx,
		input.loaded,
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
		"repomap: accepted and published %s with %d supported step(s), %d covered aspect(s), score %d/4\n",
		summary.ID,
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

func loadGoldenMechanismV01Input(
	ctx context.Context,
	runDir string,
) (goldenMechanismV01Input, error) {
	loaded, err := loadGoldenMechanismRun(ctx, runDir)
	if err != nil {
		return goldenMechanismV01Input{}, err
	}
	projectionPath := filepath.Join(runDir, goldenMechanismProjectionAttemptFile)
	projectionRaw, err := readBoundedRegularFile(projectionPath, maxGoldenSavedFileBytes)
	if err != nil {
		return goldenMechanismV01Input{}, err
	}
	if digestSHA256(projectionRaw) != goldenMechanismFixedProjectionSHA256 {
		return goldenMechanismV01Input{}, fmt.Errorf("golden mechanism v0.1: fixed projection hash changed")
	}
	var projection goldenProjection
	if err := decodeGoldenFixture(projectionRaw, &projection); err != nil {
		return goldenMechanismV01Input{}, err
	}
	if err := validateGoldenMechanismFixedProjection(loaded, projection); err != nil {
		return goldenMechanismV01Input{}, err
	}
	probeRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismProbeAttemptFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV01Input{}, err
	}
	if digestSHA256(probeRaw) != goldenMechanismFixedProbeSHA256 {
		return goldenMechanismV01Input{}, fmt.Errorf("golden mechanism v0.1: fixed probe hash changed")
	}

	supplement, enrichedBundle, err := report.PrepareSemanticSupplement(
		loaded.data,
		projection.Candidate.ID,
		goldenMechanismFixedProbeSHA256,
		projection.Facts,
	)
	if err != nil {
		return goldenMechanismV01Input{}, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(enrichedBundle, proposal); err != nil {
		return goldenMechanismV01Input{}, err
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(enrichedBundle, projection.Leaf)
	if err != nil {
		return goldenMechanismV01Input{}, err
	}
	if prompt.Version != semanticdiscovery.GoldenMechanismPromptVersion {
		return goldenMechanismV01Input{}, fmt.Errorf("golden mechanism v0.1: unexpected prompt version %q", prompt.Version)
	}
	return goldenMechanismV01Input{
		loaded: loaded, projection: projection, supplement: supplement,
		bundle: enrichedBundle, proposal: proposal, leaf: projection.Leaf, prompt: prompt,
	}, nil
}

func validateGoldenMechanismFixedProjection(
	loaded goldenMechanismRun,
	projection goldenProjection,
) error {
	candidate := projection.Candidate
	if candidate.ID != goldenDirectoryListingCandidateID ||
		candidate.Title != goldenMechanismV01Title ||
		candidate.QuestionAnswered != goldenMechanismV01Question {
		return fmt.Errorf("golden mechanism v0.1: fixed candidate identity changed")
	}
	if loaded.candidate.ID != candidate.ID || loaded.candidate.Title != candidate.Title ||
		loaded.candidate.QuestionAnswered != candidate.QuestionAnswered ||
		!equalGoldenStrings(loaded.candidate.SupportIDs, candidate.SupportIDs) {
		return fmt.Errorf("golden mechanism v0.1: fixed candidate no longer extends the saved candidate")
	}
	gotFactIDs := make([]string, 0, len(projection.Facts))
	for _, fact := range projection.Facts {
		gotFactIDs = append(gotFactIDs, fact.ID)
	}
	slices.Sort(gotFactIDs)
	if !slices.Equal(gotFactIDs, goldenMechanismFixedFactIDs) ||
		!slices.Equal(candidate.EnrichmentSupportIDs, goldenMechanismFixedFactIDs) {
		return fmt.Errorf("golden mechanism v0.1: fixed six-fact identity changed")
	}
	if !reflect.DeepEqual(projection.Leaf.Task.Candidate, candidate) {
		return fmt.Errorf("golden mechanism v0.1: fixed leaf candidate changed")
	}
	if err := projection.Leaf.Task.Validate(); err != nil {
		return err
	}
	if err := semanticdiscovery.ValidateLeafArtifact(projection.Leaf.Task, projection.Leaf.Artifact); err != nil {
		return err
	}
	return nil
}

func replayRejectedGoldenMechanism(
	input goldenMechanismV01Input,
	runDir string,
) (goldenMechanismOldReplay, error) {
	raw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismRejectedResponseFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismOldReplay{}, err
	}
	if digestSHA256(raw) != goldenMechanismRejectedWrapperSHA256 {
		return goldenMechanismOldReplay{}, fmt.Errorf("golden mechanism v0.1: rejected response fixture hash changed")
	}
	var response goldenMechanismResponseAttempt
	if err := decodeGoldenFixture(raw, &response); err != nil {
		return goldenMechanismOldReplay{}, err
	}
	if response.CandidateID != goldenDirectoryListingCandidateID ||
		response.PromptVersion != goldenMechanismRejectedPromptVersion {
		return goldenMechanismOldReplay{}, fmt.Errorf("golden mechanism v0.1: rejected response identity changed")
	}
	evaluated, validationErr := evaluateGoldenMechanismResponse(
		input.bundle,
		input.proposal,
		input.leaf,
		[]byte(response.Content),
	)
	if validationErr == nil {
		return goldenMechanismOldReplay{}, fmt.Errorf("golden mechanism v0.1: rejected response unexpectedly passed current validation")
	}
	failureClass := classifyGoldenMechanismValidationFailure(evaluated.Reduction)
	replay := goldenMechanismOldReplay{
		Version: 1, State: "rejected", FailureClass: failureClass,
		PromptVersion:  response.PromptVersion,
		ResponseSHA256: digestSHA256([]byte(response.Content)),
		Reduction:      evaluated.Reduction,
	}
	if failureClass != "prompt_validator_contract_mismatch" {
		return replay, fmt.Errorf("golden mechanism v0.1: old response failure class = %q", failureClass)
	}
	if err := writeGoldenJSON(filepath.Join(runDir, goldenMechanismOldReplayFile), replay); err != nil {
		return replay, err
	}
	return replay, nil
}

func reserveGoldenMechanismV2Call(path string) error {
	reservation := goldenMechanismCallReservation{
		Version: 1, State: "reserved",
		CandidateID:      goldenDirectoryListingCandidateID,
		PromptVersion:    semanticdiscovery.GoldenMechanismPromptVersion,
		ProjectionSHA256: goldenMechanismFixedProjectionSHA256,
		ProbeSHA256:      goldenMechanismFixedProbeSHA256,
	}
	encoded, err := marshalGoldenJSON(reservation)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("golden mechanism v0.1: the single v2 synthesis call is already reserved")
		}
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return nil
}

func decodeGoldenFixture(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("golden mechanism v0.1: decode fixed fixture: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("golden mechanism v0.1: fixed fixture has trailing data")
	}
	return nil
}

func digestSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func equalGoldenStrings(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func goldenReductionReasonCount(reduction semanticdiscovery.FanInReductionReport) int {
	count := 0
	for _, issue := range reduction.Issues {
		count += len(issue.Reasons)
	}
	return count
}
