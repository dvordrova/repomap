package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenMechanismV03StatusFile       = "golden_mechanism_v03_status.json"
	goldenMechanismV03ReplayStatusFile = "golden_mechanism_v03_replay_status.json"
	goldenMechanismV03ResponseSHA256   = "4b96c0a8992bd8f84f2ec345458fdf13393f26076f616d47caf310e91fd52e4f"
	goldenMechanismV03ContentSHA256    = "3730052e1527acc558a5805f602bf16614cb8623f719e1c577c1cc478435c1e8"
)

type goldenMechanismV03Status struct {
	Version               int                                        `json:"version"`
	State                 string                                     `json:"state"`
	FailureReason         string                                     `json:"failure_reason,omitempty"`
	CandidateID           string                                     `json:"candidate_id"`
	Question              string                                     `json:"question"`
	RepositoryRevision    string                                     `json:"repository_revision,omitempty"`
	ProjectionSHA256      string                                     `json:"projection_sha256"`
	ResponseSHA256        string                                     `json:"response_sha256"`
	ResponseContentSHA256 string                                     `json:"response_content_sha256"`
	ClaimCoverage         *semanticdiscovery.ClaimCoverageAssessment `json:"claim_coverage,omitempty"`
	Artifact              *goldenMechanismArtifactSummary            `json:"artifact,omitempty"`
	ModelCalls            int                                        `json:"model_calls"`
	RepositoryAnalyzers   int                                        `json:"repository_analyzers"`
	TargetedProbeCalls    int                                        `json:"targeted_probe_calls"`
	CanonicalFactsFile    string                                     `json:"canonical_facts_file,omitempty"`
	CanonicalRecordFile   string                                     `json:"canonical_record_file,omitempty"`
	ReportFile            string                                     `json:"report_file,omitempty"`
}

type goldenMechanismV03ReplayStatus struct {
	Version                int      `json:"version"`
	State                  string   `json:"state"`
	ModelCalls             int      `json:"model_calls"`
	RepositoryAnalyzers    int      `json:"repository_analyzers"`
	TargetedProbeCalls     int      `json:"targeted_probe_calls"`
	SupplementalFactCount  int      `json:"supplemental_fact_count"`
	UsedFactIDs            []string `json:"used_fact_ids"`
	UnusedAvailableFactIDs []string `json:"unused_available_fact_ids"`
	CoveredAspectIDs       []string `json:"covered_answer_aspects"`
	UncoveredAspectIDs     []string `json:"uncovered_answer_aspects"`
	ArtifactID             string   `json:"artifact_id"`
	ArtifactSHA256         string   `json:"artifact_sha256"`
	SearchIndexed          bool     `json:"search_indexed"`
	HTMLContainsArtifact   bool     `json:"html_contains_artifact"`
	EvidenceCount          int      `json:"evidence_count"`
	ReportFile             string   `json:"report_file"`
}

func runGoldenMechanismV03CLI(args []string, stdout, stderr io.Writer) error {
	runDir, replay, err := parseGoldenMechanismV03Args(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if replay {
		return replayGoldenMechanismV03(ctx, runDir, stdout, stderr)
	}
	return runGoldenMechanismV03(ctx, runDir, stdout, stderr)
}

func parseGoldenMechanismV03Args(args []string) (string, bool, error) {
	runDir := ""
	replay := false
	for _, arg := range args {
		switch arg {
		case "--replay":
			if replay {
				return "", false, goldenMechanismV03Usage()
			}
			replay = true
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", false, goldenMechanismV03Usage()
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", false, goldenMechanismV03Usage()
	}
	return runDir, replay, nil
}

func goldenMechanismV03Usage() error {
	return fmt.Errorf(
		"Usage: repomap dev golden-mechanism-v03 <run-dir> [--replay]",
	)
}

func runGoldenMechanismV03(
	ctx context.Context,
	runDir string,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := goldenMechanismV03Status{
		Version:               3,
		State:                 "started",
		CandidateID:           goldenDirectoryListingCandidateID,
		Question:              goldenMechanismV01Question,
		ProjectionSHA256:      goldenMechanismProjectionV2SHA256,
		ResponseSHA256:        goldenMechanismV03ResponseSHA256,
		ResponseContentSHA256: goldenMechanismV03ContentSHA256,
	}
	defer func() {
		if returnErr != nil {
			status.State = "failed"
			status.FailureReason = semanticDiscoveryReason(returnErr.Error())
		}
		if err := writeGoldenJSON(
			filepath.Join(absDir, goldenMechanismV03StatusFile),
			status,
		); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	input, err := loadGoldenMechanismV02Input(ctx, absDir)
	if err != nil {
		return err
	}
	status.RepositoryRevision = input.loaded.manifest.RepositoryState.Head
	prepared, err := readBoundedRegularFile(
		filepath.Join(absDir, goldenMechanismProjectionV2File),
		maxGoldenSavedFileBytes,
	)
	if err != nil || digestSHA256(prepared) != goldenMechanismProjectionV2SHA256 ||
		!bytes.Equal(prepared, input.projectionBytes) {
		return fmt.Errorf("golden mechanism v0.3: saved seven-fact fixture changed")
	}

	rawResponse, err := readGoldenMechanismV03Response(absDir)
	if err != nil {
		return err
	}
	evaluated, err := evaluateGoldenMechanismResponse(
		input.bundle,
		input.proposal,
		input.leaf,
		rawResponse,
	)
	if err != nil {
		return fmt.Errorf("golden mechanism v0.3: fixed response validation: %w", err)
	}
	proposal := evaluated.FanIn.Artifacts[0]
	if err := semanticdiscovery.ValidateLocalSequenceClaims(
		input.bundle,
		proposal,
		goldenDirectoryListingEntryFactID,
		input.sequenceFact.ID,
	); err != nil {
		return fmt.Errorf("golden mechanism v0.3: local sequence validation: %w", err)
	}
	assessment, err := semanticdiscovery.AssessClaimCoverage(
		input.bundle,
		[]semanticdiscovery.LeafResult{input.leaf},
		proposal,
	)
	if err != nil {
		return fmt.Errorf("golden mechanism v0.3: claim coverage: %w", err)
	}
	artifact := evaluated.Artifacts[0]
	if err := validateGoldenMechanismV03Assessment(
		input.projection.Candidate,
		input.sequenceFact.ID,
		assessment,
		artifact,
	); err != nil {
		return err
	}
	status.ClaimCoverage = &assessment
	summary, err := summarizeGoldenMechanismArtifact(input.projection.Candidate, artifact)
	if err != nil {
		return err
	}
	status.Artifact = &summary
	supplementBytes, err := marshalGoldenJSON(input.supplement)
	if err != nil {
		return err
	}
	recordBytes := append(append([]byte(nil), evaluated.RecordBytes...), '\n')
	if err := publishGoldenMechanism(
		ctx,
		input.loaded,
		supplementBytes,
		recordBytes,
		artifact,
	); err != nil {
		return err
	}
	status.State = "published"
	status.CanonicalFactsFile = filepath.Join(absDir, report.GoldenMechanismFactsFile)
	status.CanonicalRecordFile = filepath.Join(absDir, report.GoldenMechanismRecordFile)
	status.ReportFile = filepath.Join(absDir, "report.html")
	fmt.Fprintf(
		stderr,
		"repomap: accepted saved v0.2 response by claim support and coverage; %d used, %d unused available fact(s), no model or probe call\n",
		len(assessment.UsedFactIDs),
		len(assessment.UnusedAvailableFactIDs),
	)
	fmt.Fprintf(
		stdout,
		"Golden mechanism: %s\nReport: %s\n",
		status.CanonicalRecordFile,
		status.ReportFile,
	)
	return nil
}

func readGoldenMechanismV03Response(runDir string) ([]byte, error) {
	path := filepath.Join(runDir, goldenMechanismResponseV3File)
	wrapperRaw, err := readBoundedRegularFile(path, maxGoldenSavedFileBytes)
	if err != nil {
		return nil, err
	}
	if digestSHA256(wrapperRaw) != goldenMechanismV03ResponseSHA256 {
		return nil, fmt.Errorf("golden mechanism v0.3: fixed response wrapper changed")
	}
	var wrapper goldenMechanismResponseAttempt
	if err := decodeGoldenFixture(wrapperRaw, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Version != 1 ||
		wrapper.CandidateID != goldenDirectoryListingCandidateID ||
		wrapper.PromptVersion != semanticdiscovery.GoldenMechanismPromptVersionV3 ||
		wrapper.Content == "" {
		return nil, fmt.Errorf("golden mechanism v0.3: fixed response identity changed")
	}
	raw := []byte(wrapper.Content)
	if digestSHA256(raw) != goldenMechanismV03ContentSHA256 {
		return nil, fmt.Errorf("golden mechanism v0.3: fixed response content changed")
	}
	return raw, nil
}

func validateGoldenMechanismV03Assessment(
	candidate semanticdiscovery.OpportunityCandidate,
	sequenceFactID string,
	assessment semanticdiscovery.ClaimCoverageAssessment,
	artifact semanticdiscovery.Artifact,
) error {
	if assessment.CandidateID != candidate.ID ||
		len(assessment.AvailableFactIDs) != 7 ||
		len(assessment.UsedFactIDs) != 6 ||
		!slices.Equal(
			assessment.UnusedAvailableFactIDs,
			[]string{sequenceFactID},
		) {
		return fmt.Errorf("golden mechanism v0.3: fixed fact-use partition changed")
	}
	if !slices.Equal(artifact.UsedFactIDs, assessment.UsedFactIDs) ||
		!slices.Equal(
			artifact.UnusedAvailableFactIDs,
			assessment.UnusedAvailableFactIDs,
		) ||
		!slices.Equal(artifact.CoveredAspectIDs, assessment.CoveredAspectIDs) ||
		!slices.Equal(artifact.UncoveredAspectIDs, assessment.UncoveredAspectIDs) {
		return fmt.Errorf("golden mechanism v0.3: canonical usage or coverage metadata diverged")
	}
	keyCount := 0
	strongKeyCount := 0
	for _, aspect := range assessment.AnswerAspects {
		if !aspect.Key {
			continue
		}
		keyCount++
		if aspect.Status == semanticdiscovery.AspectCoveredDirectly ||
			aspect.Status == semanticdiscovery.AspectCoveredCompositionally {
			strongKeyCount++
		}
	}
	if keyCount == 0 || strongKeyCount != keyCount {
		return fmt.Errorf("golden mechanism v0.3: not every key answer aspect has strong coverage")
	}
	if !slices.Equal(assessment.UncoveredAspectIDs, []string{"known_unknowns"}) {
		return fmt.Errorf("golden mechanism v0.3: fixed uncovered-aspect contract changed")
	}
	return nil
}

func replayGoldenMechanismV03(
	_ context.Context,
	runDir string,
	stdout io.Writer,
	_ io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(absDir, report.RunManifestFilename)
	if _, err := os.Lstat(manifestPath); err == nil {
		return fmt.Errorf(
			"golden mechanism v0.3 replay refuses an authorized run; copy it and remove %s first",
			report.RunManifestFilename,
		)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("golden mechanism v0.3 replay: inspect run authority: %w", err)
	}
	status := goldenMechanismV03ReplayStatus{
		Version:    3,
		State:      "started",
		ReportFile: filepath.Join(absDir, "report.html"),
	}
	defer func() {
		if returnErr != nil {
			status.State = "failed"
		}
		if err := writeGoldenJSON(
			filepath.Join(absDir, goldenMechanismV03ReplayStatusFile),
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
	sequenceFactID, err := requiredGoldenReplaySequenceFact(before.SemanticSupplementalFacts)
	if err != nil {
		return err
	}
	bundle, err := report.BuildSemanticDiscoveryBundle(before)
	if err != nil {
		return err
	}
	recordRaw, err := readBoundedRegularFile(
		filepath.Join(absDir, report.GoldenMechanismRecordFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return err
	}
	record, err := semanticdiscovery.DecodeRecord(recordRaw)
	if err != nil {
		return err
	}
	proposal, candidate, leaf, err := goldenMechanismV03RecordParts(record)
	if err != nil {
		return err
	}
	if err := semanticdiscovery.ValidateLocalSequenceClaims(
		bundle,
		proposal,
		goldenDirectoryListingEntryFactID,
		sequenceFactID,
	); err != nil {
		return err
	}
	assessment, err := semanticdiscovery.AssessClaimCoverage(
		bundle,
		[]semanticdiscovery.LeafResult{leaf},
		proposal,
	)
	if err != nil {
		return err
	}
	if err := validateGoldenMechanismV03Assessment(
		candidate,
		sequenceFactID,
		assessment,
		artifact,
	); err != nil {
		return err
	}
	if _, err := summarizeGoldenMechanismArtifact(candidate, artifact); err != nil {
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
	status.UsedFactIDs = append([]string(nil), assessment.UsedFactIDs...)
	status.UnusedAvailableFactIDs = append(
		[]string(nil),
		assessment.UnusedAvailableFactIDs...,
	)
	status.CoveredAspectIDs = append([]string(nil), assessment.CoveredAspectIDs...)
	status.UncoveredAspectIDs = append([]string(nil), assessment.UncoveredAspectIDs...)
	status.ArtifactID = artifact.ID
	status.ArtifactSHA256 = beforeSHA
	status.SearchIndexed = semanticSearchContainsArtifact(after, artifact.ID)
	status.EvidenceCount = len(artifact.Evidence)
	html, err := readBoundedRegularFile(status.ReportFile, maxGoldenSavedFileBytes)
	if err != nil {
		return err
	}
	status.HTMLContainsArtifact = bytes.Contains(html, []byte(artifact.ID))
	if !status.SearchIndexed || !status.HTMLContainsArtifact || status.EvidenceCount == 0 {
		return fmt.Errorf("golden mechanism v0.3 replay: report projection is incomplete")
	}
	fmt.Fprintf(
		stdout,
		"No-model replay: %s\nArtifact: %s\nReport: %s\n",
		status.State,
		status.ArtifactID,
		status.ReportFile,
	)
	return nil
}

func goldenMechanismV03RecordParts(
	record semanticdiscovery.Record,
) (
	semanticdiscovery.ArtifactProposal,
	semanticdiscovery.OpportunityCandidate,
	semanticdiscovery.LeafResult,
	error,
) {
	if len(record.FanIn.Artifacts) != 1 || len(record.Leaves) != 1 {
		return semanticdiscovery.ArtifactProposal{},
			semanticdiscovery.OpportunityCandidate{},
			semanticdiscovery.LeafResult{},
			fmt.Errorf("golden mechanism v0.3 replay: canonical record shape changed")
	}
	proposal := record.FanIn.Artifacts[0]
	for _, candidate := range record.Opportunity.Candidates {
		if candidate.ID == proposal.CandidateID {
			return proposal, candidate, record.Leaves[0], nil
		}
	}
	return semanticdiscovery.ArtifactProposal{},
		semanticdiscovery.OpportunityCandidate{},
		semanticdiscovery.LeafResult{},
		fmt.Errorf("golden mechanism v0.3 replay: candidate is unavailable")
}

func semanticSearchContainsArtifact(data *report.ReportData, artifactID string) bool {
	if data == nil || data.SemanticSearch == nil {
		return false
	}
	for _, item := range data.SemanticSearch.Items {
		if item.Target.Kind == report.SemanticSearchTargetArtifact &&
			item.Target.ArtifactID == artifactID {
			return true
		}
	}
	return false
}
