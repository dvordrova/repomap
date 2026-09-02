package report

import (
	"fmt"
	"os"
	"path/filepath"
)

// PublicationReadiness is the closed publication verification state exposed
// to the ordinary run boundary.
type PublicationReadiness string

const (
	PublicationReady  PublicationReadiness = "READY"
	PublicationFailed PublicationReadiness = "FAILED"
)

// PublicationReason is a closed reason emitted when publication verification
// fails.
type PublicationReason string

const PublicationReasonArtifactsInvalid PublicationReason = "artifacts_missing_or_invalid"

// PublicationAssessment is the verified state of one published run.
type PublicationAssessment struct {
	Status  PublicationReadiness `json:"status"`
	Reasons []PublicationReason  `json:"reasons,omitempty"`

	scaleWarnings       []ReportInputScaleWarning
	targetScaleWarnings []TargetReportScaleWarning
}

// ScaleWarnings returns transient measurements captured by the successful
// live publication transaction. Recovery assessment deliberately does not
// reread or decompress the report merely to reconstruct diagnostics.
func (assessment PublicationAssessment) ScaleWarnings() []ReportInputScaleWarning {
	return append([]ReportInputScaleWarning(nil), assessment.scaleWarnings...)
}

// TargetScaleWarnings returns exact target-bound measurements captured during
// this live publication transaction, including measurements completed before
// a later sibling or atomic-install failure.
func (assessment PublicationAssessment) TargetScaleWarnings() []TargetReportScaleWarning {
	return append([]TargetReportScaleWarning(nil), assessment.targetScaleWarnings...)
}

func completedPublicationAssessment(
	publicationErr error,
	scaleWarnings []ReportInputScaleWarning,
	targetScaleWarnings []TargetReportScaleWarning,
) PublicationAssessment {
	if publicationErr != nil {
		assessment := FailedPublicationAssessment()
		assessment.scaleWarnings = append([]ReportInputScaleWarning(nil), scaleWarnings...)
		assessment.targetScaleWarnings = append(
			[]TargetReportScaleWarning(nil), targetScaleWarnings...,
		)
		return assessment
	}
	return PublicationAssessment{
		Status:        PublicationReady,
		scaleWarnings: append([]ReportInputScaleWarning(nil), scaleWarnings...),
		targetScaleWarnings: append(
			[]TargetReportScaleWarning(nil), targetScaleWarnings...,
		),
	}
}

// AssessRunPublication uses ReadRunManifest as the one semantic and artifact
// authority. It adds only verification of the published HTML payload (and the
// optional standalone multi-target document), which is outside the manifest's
// JSON/artifact reconstruction contract.
func AssessRunPublication(runDir string) (PublicationAssessment, error) {
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return FailedPublicationAssessment(), err
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		return FailedPublicationAssessment(), fmt.Errorf("read report: %w", err)
	}
	if err := inspectPublishedHTML(
		filepath.Join(runDir, "report.html"), reportJSON, manifest,
	); err != nil {
		return FailedPublicationAssessment(), err
	}
	return PublicationAssessment{Status: PublicationReady}, nil
}

// FailedPublicationAssessment returns the single integrity failure state used
// when publication cannot complete before AssessRunPublication is reached.
func FailedPublicationAssessment() PublicationAssessment {
	return PublicationAssessment{
		Status:  PublicationFailed,
		Reasons: []PublicationReason{PublicationReasonArtifactsInvalid},
	}
}

// inspectPublishedHTML proves the one physical page on disk was rendered from
// the run's own verified report data.
func inspectPublishedHTML(
	path string,
	reportJSON []byte,
	manifest RunManifest,
) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("published report html: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("published report html is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read published report html: %w", err)
	}
	navigation, err := LoadManifestTargetNavigation(filepath.Dir(path), manifest)
	if err != nil {
		return fmt.Errorf("restore published target navigation: %w", err)
	}
	artifactsDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("resolve published report directory: %w", err)
	}
	authority := OrdinaryReportHTMLAuthority{
		TargetNavigation: navigation,
		StandaloneSource: manifest.StandaloneSource,
		ArtifactsDir:     artifactsDir,
		AnalysisRoot:     manifest.AnalysisRoot,
		RepositoryRoot:   manifest.RepositoryState.Identity,
	}
	if err := VerifyOrdinaryReportHTMLPayload(raw, reportJSON, authority); err != nil {
		return fmt.Errorf("verify published report html payload: %w", err)
	}
	return nil
}
