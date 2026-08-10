package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
)

// publicationReadiness reports publication health, not whether every bounded
// semantic frontier was exhausted. Accepted partial results retain exact local
// remainder and coverage disclosure and are ready; missing, failed,
// unavailable, empty, or interrupted product stages are degraded. Integrity
// failures remain failed.
type publicationReadiness string

const (
	publicationReady    publicationReadiness = "READY"
	publicationDegraded publicationReadiness = "DEGRADED"
	publicationFailed   publicationReadiness = "FAILED"
)

type publicationReason string

const (
	publicationReasonArtifactsInvalid        publicationReason = "artifacts_missing_or_invalid"
	publicationReasonArchitectureMissing     publicationReason = "architecture_missing"
	publicationReasonArchitectureEmpty       publicationReason = "architecture_empty"
	publicationReasonArchitecturePartial     publicationReason = "architecture_partial"
	publicationReasonArchitectureFailed      publicationReason = "architecture_failed"
	publicationReasonArchitectureUnavailable publicationReason = "architecture_unavailable"
	publicationReasonArchitectureIncomplete  publicationReason = "architecture_incomplete"
	publicationReasonStudyMissing            publicationReason = "study_missing"
	publicationReasonStudyEmpty              publicationReason = "study_empty"
	publicationReasonStudyPartial            publicationReason = "study_partial"
	publicationReasonStudyFailed             publicationReason = "study_failed"
	publicationReasonStudyUnavailable        publicationReason = "study_unavailable"
	publicationReasonStudyIncomplete         publicationReason = "study_incomplete"
	publicationReasonInvestigationPartial    publicationReason = "study_investigation_partial"
	publicationReasonInvestigationFailed     publicationReason = "study_investigation_failed"
)

const maxPublicationHTMLBytes = 32 << 20

type publicationAssessment struct {
	Status  publicationReadiness `json:"status"`
	Reasons []publicationReason  `json:"reasons,omitempty"`
}

// assessRunPublication verifies the current manifest-bound report before
// deriving readiness. A report file existing on disk is never enough to earn
// READY or DEGRADED.
func assessRunPublication(runDir string) (publicationAssessment, error) {
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return failedPublication(), err
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		return failedPublication(), fmt.Errorf("read report: %w", err)
	}
	// ReadRunManifest already performed the complete artifact verification.
	// Recheck the bytes used below so a concurrent rewrite cannot change the
	// readiness input after that verification.
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return failedPublication(), err
	}
	bundleIdentity, err := inspectPublishedHTML(filepath.Join(runDir, "report.html"))
	if err != nil {
		return failedPublication(), err
	}
	if bundleIdentity != nil {
		containerRaw, found, readErr := readTargetPageRunFile(
			runDir, snapshot.TargetRunContainerArtifactFilename, snapshot.MaxTargetRunContainerBytes,
		)
		if readErr != nil {
			return failedPublication(), fmt.Errorf("read standalone target bundle container authority: %w", readErr)
		}
		if !found {
			return failedPublication(), fmt.Errorf("standalone target bundle container authority is missing")
		}
		container, decodeErr := snapshot.DecodeTargetRunContainer(containerRaw)
		if decodeErr != nil {
			return failedPublication(), decodeErr
		}
		portfolioRaw, found, readErr := readTargetPageRunFile(
			runDir, snapshot.TargetPagePortfolioArtifactFilename, snapshot.MaxTargetPagePortfolioBytes,
		)
		if readErr != nil {
			return failedPublication(), fmt.Errorf("read standalone target bundle portfolio authority: %w", readErr)
		}
		if !found {
			return failedPublication(), fmt.Errorf("standalone target bundle portfolio authority is missing")
		}
		portfolio, decodeErr := snapshot.DecodeTargetPagePortfolio(portfolioRaw)
		if decodeErr != nil {
			return failedPublication(), decodeErr
		}
		if bundleIdentity.TargetRunContainerSHA256 != container.SHA256 ||
			bundleIdentity.TargetPagePortfolioSHA256 != portfolio.SHA256 {
			return failedPublication(), fmt.Errorf("published standalone target bundle does not match manifest-bound authority")
		}
	}
	var data report.ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		return failedPublication(), fmt.Errorf("decode verified report: %w", err)
	}
	assessment := assessPublication(&data)
	investigationReasons, err := studyInvestigationPublicationReasons(runDir, manifest.MaterialInputs)
	if err != nil {
		return failedPublication(), err
	}
	if len(investigationReasons) != 0 {
		assessment.Status = publicationDegraded
		assessment.Reasons = append(assessment.Reasons, investigationReasons...)
	}
	return assessment, nil
}

// studyInvestigationPublicationReasons keeps optional enrichment failure in
// the console/corpus publication state without copying its status or failure
// codes into report JSON/HTML. The current manifest has already verified the
// family; decoding it again binds the exact bytes used for this assessment.
func studyInvestigationPublicationReasons(
	runDir string,
	inputs report.MaterialInputs,
) ([]publicationReason, error) {
	digests := []string{
		inputs.StudyInvestigationFactsSHA256,
		inputs.StudyInvestigationCandidatesSHA256,
		inputs.StudyInvestigationResultSHA256,
		inputs.StudyInvestigationStatusSHA256,
	}
	bound := 0
	for _, digest := range digests {
		if digest != "" {
			bound++
		}
	}
	if bound == 0 {
		return nil, nil
	}
	if bound != len(digests) {
		return nil, fmt.Errorf("Study investigation manifest identity is incomplete")
	}
	facts, err := readStudyInvestigationArtifact(
		runDir, mechanismstudy.FactsArtifactFilename, mechanismstudy.MaxFactsArtifactBytes,
	)
	if err != nil {
		return nil, err
	}
	candidates, err := readStudyInvestigationArtifact(
		runDir, mechanismstudy.CandidatesArtifactFilename, mechanismstudy.MaxCandidatesArtifactBytes,
	)
	if err != nil {
		return nil, err
	}
	result, err := readStudyInvestigationArtifact(
		runDir, mechanismstudy.ResultArtifactFilename, mechanismstudy.MaxResultArtifactBytes,
	)
	if err != nil {
		return nil, err
	}
	statusRaw, err := readStudyInvestigationArtifact(
		runDir, mechanismstudy.StatusArtifactFilename, mechanismstudy.MaxStatusArtifactBytes,
	)
	if err != nil {
		return nil, err
	}
	for position, raw := range [][]byte{facts, candidates, result, statusRaw} {
		if modelresearch.SHA256(raw) != digests[position] {
			return nil, fmt.Errorf("Study investigation publication bytes changed after manifest verification")
		}
	}
	status, err := mechanismstudy.DecodeStatus(facts, candidates, result, statusRaw)
	if err != nil {
		return nil, fmt.Errorf("decode Study investigation publication status: %w", err)
	}
	return studyInvestigationStatusReasons(status)
}

func studyInvestigationStatusReasons(status mechanismstudy.Status) ([]publicationReason, error) {
	switch status.State {
	case mechanismstudy.StatusComplete:
		return nil, nil
	case mechanismstudy.StatusPartial:
		return []publicationReason{publicationReasonInvestigationPartial}, nil
	case mechanismstudy.StatusFailed:
		return []publicationReason{publicationReasonInvestigationFailed}, nil
	default:
		return nil, fmt.Errorf("unsupported Study investigation publication state %q", status.State)
	}
}

func verifyPublishedHTML(path string) error {
	_, err := inspectPublishedHTML(path)
	return err
}

func inspectPublishedHTML(path string) (*report.StandaloneTargetBundleIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("published report html: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, fmt.Errorf("published report html is not a bounded regular file")
	}
	bundleIdentity, bundleFound, bundleErr := report.InspectStandaloneTargetBundleHTML(path)
	if bundleFound {
		if bundleErr != nil {
			return nil, fmt.Errorf("published standalone target bundle: %w", bundleErr)
		}
		return &bundleIdentity, nil
	}
	if bundleErr != nil {
		return nil, fmt.Errorf("inspect published report html: %w", bundleErr)
	}
	if info.Size() > maxPublicationHTMLBytes {
		return nil, fmt.Errorf("published report html is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read published report html: %w", err)
	}
	lower := strings.ToLower(string(raw))
	if !strings.Contains(lower, "<html") || !strings.Contains(lower, `id="rm-report-data"`) {
		return nil, fmt.Errorf("published report html is missing the report application shell")
	}
	return nil, nil
}

func failedPublication() publicationAssessment {
	return publicationAssessment{
		Status:  publicationFailed,
		Reasons: []publicationReason{publicationReasonArtifactsInvalid},
	}
}

func assessPublication(data *report.ReportData) publicationAssessment {
	if data == nil {
		return failedPublication()
	}
	reasons := architecturePublicationReasons(data)
	reasons = append(reasons, studyPublicationReasons(data)...)
	if len(reasons) == 0 {
		return publicationAssessment{Status: publicationReady}
	}
	return publicationAssessment{Status: publicationDegraded, Reasons: reasons}
}

func architecturePublicationReasons(data *report.ReportData) []publicationReason {
	if data.ArchitectureCanvas == nil {
		return []publicationReason{publicationReasonArchitectureMissing}
	}
	reasons := make([]publicationReason, 0, 2)
	if len(data.ArchitectureCanvas.Components) == 0 {
		reasons = append(reasons, publicationReasonArchitectureEmpty)
	}
	status := data.ArchitectureSynthesis
	if status == nil {
		return append(reasons, publicationReasonArchitectureMissing)
	}
	switch status.State {
	case report.ArchitectureSynthesisFailed:
		return append(reasons, publicationReasonArchitectureFailed)
	case report.ArchitectureSynthesisUnavailable:
		return append(reasons, publicationReasonArchitectureUnavailable)
	case report.ArchitectureSynthesisSucceeded, report.ArchitectureSynthesisCached:
		acceptedPartial := status.ProposalAccepted && status.ProposalPartial &&
			!status.ProposalRejected && !status.FallbackSelected &&
			data.ArchitectureCanvas.ArchitectureSource == componentmap.SourcePartialModel &&
			data.ArchitectureCanvas.ValidationOutcome == componentmap.ValidationAcceptedPartial
		if acceptedPartial {
			return reasons
		}
		completeSource := data.ArchitectureCanvas.ArchitectureSource == componentmap.SourceValidatedModel ||
			data.ArchitectureCanvas.ArchitectureSource == componentmap.SourceNormalizedModel
		completeOutcome := data.ArchitectureCanvas.ValidationOutcome == componentmap.ValidationAccepted ||
			data.ArchitectureCanvas.ValidationOutcome == componentmap.ValidationAcceptedNormalized
		if !status.ProposalAccepted || status.ProposalRejected || status.FallbackSelected ||
			!completeSource || !completeOutcome {
			return append(reasons, publicationReasonArchitectureIncomplete)
		}
		return reasons
	default:
		return append(reasons, publicationReasonArchitectureIncomplete)
	}
}

func studyPublicationReasons(data *report.ReportData) []publicationReason {
	status := data.AtlasStudy
	if status == nil {
		return []publicationReason{publicationReasonStudyMissing}
	}
	switch status.State {
	case atlasstudy.ProductStateFailed:
		return []publicationReason{publicationReasonStudyFailed}
	case atlasstudy.ProductStateUnavailable:
		return []publicationReason{publicationReasonStudyUnavailable}
	case atlasstudy.ProductStatePrepared:
		return []publicationReason{publicationReasonStudyIncomplete}
	case atlasstudy.ProductStateAcceptedPartial:
		if status.Themes == nil || len(status.Themes.Cards) == 0 {
			return []publicationReason{publicationReasonStudyPartial, publicationReasonStudyEmpty}
		}
		return nil
	case atlasstudy.ProductStateAccepted:
		if status.Themes == nil || len(status.Themes.Cards) == 0 {
			return []publicationReason{publicationReasonStudyEmpty}
		}
		// These are bounded shelf-coverage facts, not publication-health
		// failures. They remain visible in Study diagnostics.
		return nil
	default:
		return []publicationReason{publicationReasonStudyIncomplete}
	}
}

func (assessment publicationAssessment) consoleState() string {
	return strings.ToLower(string(assessment.Status))
}

func (assessment publicationAssessment) consoleDetails() []string {
	details := make([]string, 0, len(assessment.Reasons))
	for _, reason := range assessment.Reasons {
		switch reason {
		case publicationReasonArchitectureMissing:
			details = append(details, "Architecture: current publication state is missing")
		case publicationReasonArchitectureEmpty:
			details = append(details, "Architecture: no conceptual components were published")
		case publicationReasonArchitecturePartial:
			details = append(details, "Architecture: partial model grouping; the exact local remainder remains available")
		case publicationReasonArchitectureFailed:
			details = append(details, "Architecture: model grouping failed; the exact local Map remains available")
		case publicationReasonArchitectureUnavailable:
			details = append(details, "Architecture: model grouping unavailable; the exact local Map remains available")
		case publicationReasonArchitectureIncomplete:
			details = append(details, "Architecture: grouping is not a complete accepted model result")
		case publicationReasonStudyMissing:
			details = append(details, "Study: current publication state is missing")
		case publicationReasonStudyEmpty:
			details = append(details, "Study: no themes were published")
		case publicationReasonStudyPartial:
			details = append(details, "Study: accepted with incomplete coverage")
		case publicationReasonStudyFailed:
			details = append(details, "Study: model stage failed")
		case publicationReasonStudyUnavailable:
			details = append(details, "Study: unavailable")
		case publicationReasonStudyIncomplete:
			details = append(details, "Study: one or more tracked coverage dimensions are incomplete")
		case publicationReasonInvestigationPartial:
			details = append(details, "Study investigation: some planned mechanism batches were not completed")
		case publicationReasonInvestigationFailed:
			details = append(details, "Study investigation: mechanism enrichment was not completed")
		case publicationReasonArtifactsInvalid:
			details = append(details, "report artifacts are missing or failed integrity checks")
		}
	}
	return details
}
