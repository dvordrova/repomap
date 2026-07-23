package main

import (
	"context"
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
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenMechanismV02StatusFile       = "golden_mechanism_v02_status.json"
	goldenMechanismV02ReplayStatusFile = "golden_mechanism_v02_replay_status.json"
	goldenMechanismProjectionV2File    = "golden_mechanism_projection_v2.json"
	goldenMechanismResponseV3File      = "golden_mechanism_response_attempt_v3.json"
	goldenMechanismV3ReservationFile   = "golden_mechanism_v3_call_reservation.json"
	goldenMechanismProjectionV2SHA256  = "231aafd80c37e62ff8c79320ae346bf9ab49e97e465e74f062b3be43781cad4e"
	goldenMechanismSequenceFactID      = "gmf-35af91a5e369db6fb6d79030"

	goldenBrowseBranchObservationID = "gm-obs-2f31f008b0ea723b444bb731"
	goldenBrowseCallObservationID   = "gm-obs-f50524e5dc2cc1c0321ec063"
)

type goldenMechanismV02Mode string

const (
	goldenMechanismV02Live    goldenMechanismV02Mode = "live"
	goldenMechanismV02Prepare goldenMechanismV02Mode = "prepare"
	goldenMechanismV02Replay  goldenMechanismV02Mode = "replay"
)

type goldenMechanismLocalSequenceSummary struct {
	Proven              bool                            `json:"proven"`
	Scope               string                          `json:"scope"`
	FunctionID          string                          `json:"function_id"`
	BranchObservationID string                          `json:"branch_observation_id"`
	CallObservationID   string                          `json:"call_observation_id"`
	SequenceFactID      string                          `json:"sequence_fact_id"`
	SequenceFactSHA256  string                          `json:"sequence_fact_sha256"`
	Evidence            []semanticdiscovery.EvidenceRef `json:"evidence"`
	Capabilities        []semanticdiscovery.Capability  `json:"capabilities"`
}

type goldenMechanismV02Status struct {
	Version               int                                     `json:"version"`
	State                 string                                  `json:"state"`
	FailureClass          string                                  `json:"failure_class,omitempty"`
	FailureReason         string                                  `json:"failure_reason,omitempty"`
	CandidateID           string                                  `json:"candidate_id"`
	Question              string                                  `json:"question"`
	RepositoryRevision    string                                  `json:"repository_revision,omitempty"`
	BaseArtifactCount     int                                     `json:"base_artifact_count"`
	FixedProjectionSHA256 string                                  `json:"fixed_projection_sha256"`
	FixedProbeSHA256      string                                  `json:"fixed_probe_sha256"`
	ProjectionV2File      string                                  `json:"projection_v2_file,omitempty"`
	ProjectionV2SHA256    string                                  `json:"projection_v2_sha256,omitempty"`
	LocalSequence         *goldenMechanismLocalSequenceSummary    `json:"local_sequence,omitempty"`
	Synthesis             *semanticDiscoveryStageMetrics          `json:"synthesis,omitempty"`
	Reduction             *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	ProviderCalls         int                                     `json:"provider_calls"`
	ReservationFile       string                                  `json:"reservation_file,omitempty"`
	ResponseAttemptFile   string                                  `json:"response_attempt_file,omitempty"`
	Artifact              *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	CanonicalFactsFile    string                                  `json:"canonical_facts_file,omitempty"`
	CanonicalRecordFile   string                                  `json:"canonical_record_file,omitempty"`
	ReportFile            string                                  `json:"report_file,omitempty"`
}

type goldenMechanismV02ReplayStatus struct {
	Version               int    `json:"version"`
	State                 string `json:"state"`
	ModelCalls            int    `json:"model_calls"`
	RepositoryAnalyzers   int    `json:"repository_analyzers"`
	TargetedProbeCalls    int    `json:"targeted_probe_calls"`
	SupplementalFactCount int    `json:"supplemental_fact_count"`
	SequenceFactID        string `json:"sequence_fact_id"`
	ArtifactID            string `json:"artifact_id"`
	ArtifactSHA256        string `json:"artifact_sha256"`
	ReportFile            string `json:"report_file"`
}

type goldenMechanismV02Input struct {
	loaded           goldenMechanismRun
	projection       goldenProjection
	projectionBytes  []byte
	projectionSHA256 string
	sequenceFact     semanticdiscovery.Fact
	sequenceProof    goldenmechanism.LocalSequenceProof
	supplement       report.SemanticSupplement
	bundle           semanticdiscovery.Bundle
	proposal         semanticdiscovery.OpportunityProposal
	leaf             semanticdiscovery.LeafResult
}

func runGoldenMechanismV02CLI(args []string, stdout, stderr io.Writer) error {
	runDir, mode, err := parseGoldenMechanismV02Args(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == goldenMechanismV02Replay {
		return replayGoldenMechanismV02(ctx, runDir, stdout, stderr)
	}
	return runGoldenMechanismV02(ctx, runDir, mode, stdout, stderr)
}

func parseGoldenMechanismV02Args(
	args []string,
) (string, goldenMechanismV02Mode, error) {
	runDir := ""
	mode := goldenMechanismV02Live
	for _, arg := range args {
		switch arg {
		case "--prepare":
			if mode != goldenMechanismV02Live {
				return "", "", goldenMechanismV02Usage()
			}
			mode = goldenMechanismV02Prepare
		case "--replay":
			if mode != goldenMechanismV02Live {
				return "", "", goldenMechanismV02Usage()
			}
			mode = goldenMechanismV02Replay
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", "", goldenMechanismV02Usage()
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", "", goldenMechanismV02Usage()
	}
	return runDir, mode, nil
}

func goldenMechanismV02Usage() error {
	return fmt.Errorf(
		"Usage: repomap dev golden-mechanism-v02 <run-dir> [--prepare | --replay]",
	)
}

func runGoldenMechanismV02(
	ctx context.Context,
	runDir string,
	mode goldenMechanismV02Mode,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("golden mechanism v0.2: resolve run directory: %w", err)
	}
	status := goldenMechanismV02Status{
		Version: 2, State: "started",
		CandidateID:           goldenDirectoryListingCandidateID,
		Question:              goldenMechanismV01Question,
		FixedProjectionSHA256: goldenMechanismFixedProjectionSHA256,
		FixedProbeSHA256:      goldenMechanismFixedProbeSHA256,
	}
	defer func() {
		if returnErr != nil && status.State != "sequence_proven" && status.State != "published" {
			status.State = "failed"
			if status.FailureClass == "" {
				status.FailureClass = "local_stage_failure"
			}
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		if err := writeGoldenJSON(filepath.Join(absDir, goldenMechanismV02StatusFile), status); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	input, err := loadGoldenMechanismV02Input(ctx, absDir)
	if err != nil {
		status.FailureClass = "local_sequence_not_proven"
		return err
	}
	status.RepositoryRevision = input.loaded.manifest.RepositoryState.Head
	status.BaseArtifactCount = len(input.loaded.baseArtifacts)
	status.ProjectionV2File = filepath.Join(absDir, goldenMechanismProjectionV2File)
	status.ProjectionV2SHA256 = input.projectionSHA256
	status.LocalSequence = summarizeGoldenLocalSequence(input.sequenceFact, input.sequenceProof)

	if mode == goldenMechanismV02Prepare {
		if err := writeAtomicFile(status.ProjectionV2File, input.projectionBytes, 0o600); err != nil {
			status.FailureClass = "seven_fact_fixture_write_failed"
			return err
		}
		status.State = "sequence_proven"
		fmt.Fprintf(
			stderr,
			"repomap: proved %s local sequence from the saved probe; no provider, probe, or repository analyzer ran\n",
			input.sequenceProof.Scope,
		)
		fmt.Fprintf(
			stdout,
			"Seven-fact fixture: %s\nSHA-256: %s\n",
			status.ProjectionV2File,
			status.ProjectionV2SHA256,
		)
		return nil
	}
	prepared, err := readBoundedRegularFile(status.ProjectionV2File, maxGoldenSavedFileBytes)
	if err != nil || !reflect.DeepEqual(prepared, input.projectionBytes) {
		status.FailureClass = "seven_fact_fixture_not_prepared"
		return fmt.Errorf(
			"golden mechanism v0.2: prepared seven-fact fixture is missing or changed; run --prepare before the one live call",
		)
	}

	return runGoldenMechanismV02Live(ctx, absDir, input, &status, stdout, stderr)
}

func runGoldenMechanismV02Live(
	ctx context.Context,
	runDir string,
	input goldenMechanismV02Input,
	status *goldenMechanismV02Status,
	stdout io.Writer,
	stderr io.Writer,
) error {
	client, err := deepseek.NewFromEnv()
	if err != nil {
		status.FailureClass = "provider_unavailable"
		return fmt.Errorf("golden mechanism v0.2: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	reservationPath := filepath.Join(runDir, goldenMechanismV3ReservationFile)
	if err := reserveGoldenMechanismV3Call(
		reservationPath,
		input.projectionSHA256,
		input.sequenceFact.ID,
	); err != nil {
		status.FailureClass = "fresh_synthesis_already_reserved"
		return err
	}
	status.ReservationFile = reservationPath
	fmt.Fprintln(
		stderr,
		"repomap: running the single allowed v3 synthesis call over six fixed facts plus one local sequence fact",
	)

	counted := &countingSemanticDiscoveryEditor{delegate: client}
	synthesis, err := executeGoldenMechanismV02Synthesis(ctx, input, counted)
	status.ProviderCalls = counted.calls
	status.Synthesis = &synthesis.Metrics
	status.Reduction = &synthesis.Reduction
	responsePath := filepath.Join(runDir, goldenMechanismResponseV3File)
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
		return errors.Join(
			err,
			fmt.Errorf("golden mechanism v0.2: provider calls = %d, want exactly one", counted.calls),
		)
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
	status.CanonicalFactsFile = filepath.Join(runDir, report.GoldenMechanismFactsFile)
	status.CanonicalRecordFile = filepath.Join(runDir, report.GoldenMechanismRecordFile)
	status.ReportFile = filepath.Join(runDir, "report.html")
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

func loadGoldenMechanismV02Input(
	ctx context.Context,
	runDir string,
) (goldenMechanismV02Input, error) {
	loaded, err := loadGoldenMechanismRun(ctx, runDir)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	projectionRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismProjectionAttemptFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	if digestSHA256(projectionRaw) != goldenMechanismFixedProjectionSHA256 {
		return goldenMechanismV02Input{}, fmt.Errorf(
			"golden mechanism v0.2: fixed six-fact projection changed",
		)
	}
	var original goldenProjection
	if err := decodeGoldenFixture(projectionRaw, &original); err != nil {
		return goldenMechanismV02Input{}, err
	}
	if err := validateGoldenMechanismFixedProjection(loaded, original); err != nil {
		return goldenMechanismV02Input{}, err
	}

	probeRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismProbeAttemptFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	if digestSHA256(probeRaw) != goldenMechanismFixedProbeSHA256 {
		return goldenMechanismV02Input{}, fmt.Errorf(
			"golden mechanism v0.2: fixed saved probe changed",
		)
	}
	var probe goldenmechanism.Result
	if err := decodeGoldenFixture(probeRaw, &probe); err != nil {
		return goldenMechanismV02Input{}, err
	}
	projection, sequenceFact, proof, err := addCaddyDirectoryListingLocalSequence(
		original,
		probe,
	)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	if proof.BranchObservation.ID != goldenBrowseBranchObservationID ||
		proof.CallObservation.ID != goldenBrowseCallObservationID {
		return goldenMechanismV02Input{}, fmt.Errorf(
			"golden mechanism v0.2: saved sequence observations changed",
		)
	}

	supplement, enrichedBundle, err := report.PrepareSemanticSupplement(
		loaded.data,
		projection.Candidate.ID,
		goldenMechanismFixedProbeSHA256,
		projection.Facts,
	)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(enrichedBundle, proposal); err != nil {
		return goldenMechanismV02Input{}, err
	}
	leaf, err := buildGoldenMechanismLeaf(enrichedBundle, projection.Candidate)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	projection.Leaf = leaf
	if err := validateGoldenMechanismV02Projection(
		original,
		projection,
		sequenceFact,
	); err != nil {
		return goldenMechanismV02Input{}, err
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(enrichedBundle, leaf)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	if prompt.Version != semanticdiscovery.GoldenMechanismPromptVersion {
		return goldenMechanismV02Input{}, fmt.Errorf(
			"golden mechanism v0.2: unexpected prompt version %q",
			prompt.Version,
		)
	}
	projectionBytes, err := marshalGoldenJSON(projection)
	if err != nil {
		return goldenMechanismV02Input{}, err
	}
	projectionSHA256 := digestSHA256(projectionBytes)
	if projectionSHA256 != goldenMechanismProjectionV2SHA256 ||
		sequenceFact.ID != goldenMechanismSequenceFactID {
		return goldenMechanismV02Input{}, fmt.Errorf(
			"golden mechanism v0.2: fixed seven-fact fixture identity changed",
		)
	}
	return goldenMechanismV02Input{
		loaded: loaded, projection: projection,
		projectionBytes: projectionBytes, projectionSHA256: projectionSHA256,
		sequenceFact: sequenceFact, sequenceProof: proof,
		supplement: supplement, bundle: enrichedBundle, proposal: proposal, leaf: leaf,
	}, nil
}

func validateGoldenMechanismV02Projection(
	original goldenProjection,
	updated goldenProjection,
	sequenceFact semanticdiscovery.Fact,
) error {
	if len(original.Facts) != 6 || len(updated.Facts) != 7 ||
		!reflect.DeepEqual(original.Facts, updated.Facts[:6]) ||
		!reflect.DeepEqual(updated.Facts[6], sequenceFact) {
		return fmt.Errorf(
			"golden mechanism v0.2: the seven-fact fixture changed an old fact",
		)
	}
	originalCandidate := original.Candidate
	updatedCandidate := updated.Candidate
	originalCandidate.EnrichmentSupportIDs = nil
	updatedCandidate.EnrichmentSupportIDs = nil
	if !reflect.DeepEqual(originalCandidate, updatedCandidate) {
		return fmt.Errorf(
			"golden mechanism v0.2: fixed candidate, question, or rubric changed",
		)
	}
	wantIDs := append(append([]string(nil), goldenMechanismFixedFactIDs...), sequenceFact.ID)
	slices.Sort(wantIDs)
	if !slices.Equal(updated.Candidate.EnrichmentSupportIDs, wantIDs) {
		return fmt.Errorf("golden mechanism v0.2: seven-fact identity is invalid")
	}
	if sequenceFact.ID == goldenDirectoryListingEntryFactID ||
		!slices.Contains(sequenceFact.Capabilities, semanticdiscovery.CapabilitySequence) {
		return fmt.Errorf("golden mechanism v0.2: separate sequence fact is invalid")
	}
	entrySourceGroup := ""
	for _, fact := range original.Facts {
		if fact.ID == goldenDirectoryListingEntryFactID {
			entrySourceGroup = fact.SourceGroup
			break
		}
	}
	if sequenceFact.SourceGroup != entrySourceGroup {
		return fmt.Errorf(
			"golden mechanism v0.2: local sequence fact invented source independence",
		)
	}
	if err := updated.Leaf.Task.Validate(); err != nil {
		return err
	}
	return semanticdiscovery.ValidateLeafArtifact(updated.Leaf.Task, updated.Leaf.Artifact)
}

func summarizeGoldenLocalSequence(
	fact semanticdiscovery.Fact,
	proof goldenmechanism.LocalSequenceProof,
) *goldenMechanismLocalSequenceSummary {
	factBytes, _ := marshalGoldenJSON(fact)
	return &goldenMechanismLocalSequenceSummary{
		Proven: true, Scope: proof.Scope, FunctionID: proof.FunctionID,
		BranchObservationID: proof.BranchObservation.ID,
		CallObservationID:   proof.CallObservation.ID,
		SequenceFactID:      fact.ID,
		SequenceFactSHA256:  digestSHA256(factBytes),
		Evidence:            append([]semanticdiscovery.EvidenceRef(nil), fact.Evidence...),
		Capabilities:        append([]semanticdiscovery.Capability(nil), fact.Capabilities...),
	}
}

func executeGoldenMechanismV02Synthesis(
	ctx context.Context,
	input goldenMechanismV02Input,
	provider semanticDiscoveryEditor,
) (goldenMechanismSynthesis, error) {
	result, err := executeGoldenMechanismSynthesis(
		ctx,
		input.bundle,
		input.proposal,
		input.leaf,
		provider,
	)
	if err != nil {
		return result, err
	}
	if len(result.FanIn.Artifacts) != 1 {
		return result, fmt.Errorf(
			"golden mechanism v0.2: accepted fan-in lost its proposal",
		)
	}
	if err := semanticdiscovery.ValidateLocalSequenceClaims(
		input.bundle,
		result.FanIn.Artifacts[0],
		goldenDirectoryListingEntryFactID,
		input.sequenceFact.ID,
	); err != nil {
		addGoldenLocalSequenceRejection(&result.Reduction, input.sequenceFact.ID, err)
		result.RecordBytes = nil
		result.Artifacts = nil
		return result, err
	}
	return result, nil
}

func addGoldenLocalSequenceRejection(
	reduction *semanticdiscovery.FanInReductionReport,
	sequenceFactID string,
	validationErr error,
) {
	reduction.DroppedArtifacts = 1
	reduction.KeptArtifacts = 0
	reason := semanticdiscovery.FanInReductionReason{
		Code:       semanticdiscovery.FanInReasonLocalSequenceScope,
		Field:      "artifact.local_sequence",
		SupportIDs: []string{sequenceFactID},
	}
	var scopeErr *semanticdiscovery.LocalSequenceClaimError
	if errors.As(validationErr, &scopeErr) && scopeErr.ClaimIndex >= 0 {
		index := scopeErr.ClaimIndex
		reason.Field = "claim.text"
		reason.ClaimIndex = &index
	}
	reduction.Issues = append(reduction.Issues, semanticdiscovery.FanInReductionIssue{
		ArtifactIndex: 0,
		Code:          "invalid_proposal",
		Reasons:       []semanticdiscovery.FanInReductionReason{reason},
	})
}

func reserveGoldenMechanismV3Call(
	path string,
	projectionSHA256 string,
	sequenceFactID string,
) error {
	reservation := map[string]any{
		"version":           1,
		"state":             "reserved",
		"candidate_id":      goldenDirectoryListingCandidateID,
		"prompt_version":    semanticdiscovery.GoldenMechanismPromptVersion,
		"projection_sha256": projectionSHA256,
		"probe_sha256":      goldenMechanismFixedProbeSHA256,
		"sequence_fact_id":  sequenceFactID,
	}
	encoded, err := marshalGoldenJSON(reservation)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(
				"golden mechanism v0.2: the single v3 synthesis call is already reserved",
			)
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

func replayGoldenMechanismV02(
	_ context.Context,
	runDir string,
	stdout io.Writer,
	_ io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := goldenMechanismV02ReplayStatus{
		Version:    2,
		State:      "started",
		ReportFile: filepath.Join(absDir, "report.html"),
	}
	defer func() {
		if returnErr != nil {
			status.State = "failed"
		}
		if err := writeGoldenJSON(
			filepath.Join(absDir, goldenMechanismV02ReplayStatusFile),
			status,
		); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	before, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	artifact, err := requiredGoldenReplayArtifact(before.SemanticArtifacts)
	if err != nil {
		return err
	}
	sequenceFactID, err := requiredGoldenReplaySequenceFact(
		before.SemanticSupplementalFacts,
	)
	if err != nil {
		return err
	}
	beforeSHA, err := goldenMechanismArtifactSHA256(artifact)
	if err != nil {
		return err
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	after, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	if err := requireGoldenArtifact(after.SemanticArtifacts, artifact); err != nil {
		return err
	}
	status.State = "replayed"
	status.SupplementalFactCount = len(after.SemanticSupplementalFacts)
	status.SequenceFactID = sequenceFactID
	status.ArtifactID = artifact.ID
	status.ArtifactSHA256 = beforeSHA
	fmt.Fprintf(
		stdout,
		"No-model replay: %s\nArtifact: %s\nReport: %s\n",
		status.State,
		status.ArtifactID,
		status.ReportFile,
	)
	return nil
}

func requiredGoldenReplayArtifact(
	artifacts []semanticdiscovery.Artifact,
) (semanticdiscovery.Artifact, error) {
	var matched semanticdiscovery.Artifact
	for _, artifact := range artifacts {
		if artifact.CandidateID != goldenDirectoryListingCandidateID {
			continue
		}
		if matched.ID != "" {
			return semanticdiscovery.Artifact{}, fmt.Errorf(
				"golden mechanism v0.2 replay: duplicate canonical candidate",
			)
		}
		matched = artifact
	}
	if matched.ID == "" {
		return semanticdiscovery.Artifact{}, fmt.Errorf(
			"golden mechanism v0.2 replay: canonical artifact is unavailable",
		)
	}
	return matched, nil
}

func requiredGoldenReplaySequenceFact(
	facts []semanticdiscovery.Fact,
) (string, error) {
	if len(facts) != 7 {
		return "", fmt.Errorf(
			"golden mechanism v0.2 replay: supplemental fact count = %d, want 7",
			len(facts),
		)
	}
	var sequenceID string
	for _, fact := range facts {
		if slices.Contains(goldenMechanismFixedFactIDs, fact.ID) {
			continue
		}
		if sequenceID != "" ||
			!slices.Contains(fact.Capabilities, semanticdiscovery.CapabilitySequence) ||
			!slices.Contains(fact.Capabilities, semanticdiscovery.CapabilityLimitation) {
			return "", fmt.Errorf(
				"golden mechanism v0.2 replay: separate local sequence fact is invalid",
			)
		}
		sequenceID = fact.ID
	}
	if sequenceID == "" {
		return "", fmt.Errorf(
			"golden mechanism v0.2 replay: local sequence fact is unavailable",
		)
	}
	return sequenceID, nil
}
