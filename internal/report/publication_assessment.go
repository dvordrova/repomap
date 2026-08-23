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
	bundleIdentity, err := inspectPublishedHTML(
		filepath.Join(runDir, "report.html"), reportJSON, manifest,
	)
	if err != nil {
		return FailedPublicationAssessment(), err
	}
	if bundleIdentity != nil {
		if err := verifyPublishedStandaloneTargetAuthority(runDir, manifest, *bundleIdentity); err != nil {
			return FailedPublicationAssessment(), err
		}
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

func verifyPublishedStandaloneTargetAuthority(
	runDir string,
	manifest RunManifest,
	identity StandaloneTargetBundleIdentity,
) error {
	container, portfolio, err := manifestStandaloneTargetAuthority(runDir, manifest)
	if err != nil {
		return err
	}
	if identity.TargetRunContainerSHA256 != container.SHA256 ||
		identity.TargetPagePortfolioSHA256 != portfolio.SHA256 {
		return fmt.Errorf("published standalone target bundle does not match manifest-bound authority")
	}
	return nil
}

func inspectPublishedHTML(
	path string,
	reportJSON []byte,
	manifest RunManifest,
) (*StandaloneTargetBundleIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("published report html: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, fmt.Errorf("published report html is not a bounded regular file")
	}
	bundleIdentity, found, bundleErr := InspectStandaloneTargetBundleHTML(path)
	if found {
		if bundleErr != nil {
			return nil, fmt.Errorf("published standalone target bundle: %w", bundleErr)
		}
		bundleIdentity, bundleErr = VerifyStandaloneTargetBundleHTML(
			path, filepath.Dir(path), manifest,
		)
		if bundleErr != nil {
			return nil, fmt.Errorf("published standalone target bundle authority: %w", bundleErr)
		}
		return &bundleIdentity, nil
	}
	if bundleErr != nil {
		return nil, fmt.Errorf("inspect published report html: %w", bundleErr)
	}
	if info.Size() > MaxOrdinaryReportHTMLBytes {
		return nil, fmt.Errorf("published report html exceeds the size limit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read published report html: %w", err)
	}
	var navigation *TargetNavigationPortfolio
	if manifest.MaterialInputs.TargetPagePortfolioSHA256 != "" {
		navigation, err = LoadManifestTargetNavigation(filepath.Dir(path), manifest)
		if err != nil {
			return nil, fmt.Errorf("restore published target navigation: %w", err)
		}
	}
	artifactsDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("resolve published report directory: %w", err)
	}
	authority := OrdinaryReportHTMLAuthority{
		TargetNavigation: navigation,
		StandaloneSource: manifest.StandaloneSource,
		ArtifactsDir:     artifactsDir,
		AnalysisRoot:     manifest.AnalysisRoot,
		RepositoryRoot:   manifest.RepositoryState.Identity,
	}
	if err := VerifyOrdinaryReportHTMLPayload(raw, reportJSON, authority); err != nil {
		return nil, fmt.Errorf("verify published report html payload: %w", err)
	}
	return nil, nil
}
