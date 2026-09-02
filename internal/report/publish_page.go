package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/programpage"
)

// AdvisoryStandaloneTargetBundlePayloadBytes is the size above which one
// published page is reported as unusually large. It is a warning only and
// never truncates or rejects a page.
const AdvisoryStandaloneTargetBundlePayloadBytes int64 = 24 * 1024 * 1024

// StandaloneTargetBundleResourceLimitError is the terminal outcome of a page
// that cannot be represented. It exposes byte counts only, never content.
type StandaloneTargetBundleResourceLimitError struct {
	LimitBytes  int64
	ActualBytes int64
}

func (err *StandaloneTargetBundleResourceLimitError) Error() string {
	return fmt.Sprintf(
		"report: published page needs %d bytes and the limit is %d",
		err.ActualBytes, err.LimitBytes,
	)
}

// PublishProgramPageBundleFromVerifiedRunsAtomic writes the one physical
// report page of a repository run. Every analyzed target already persisted
// its own verified report.json; the owner run holds the complete matched
// graph, the fact layer, and the target inventory, so the page is projected
// from the owner's data alone and never merges sibling HTML.
func PublishProgramPageBundleFromVerifiedRunsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
	receipts []VerifiedRunReceipt,
) (PublicationAssessment, error) {
	if err := portfolio.Validate(); err != nil {
		return FailedPublicationAssessment(), err
	}
	if len(receipts) == 0 {
		return FailedPublicationAssessment(), fmt.Errorf("report: publication requires at least one verified run")
	}
	rendered, err := renderPublishedPage(runDir)
	if err != nil {
		return FailedPublicationAssessment(), err
	}
	if err := writePublishedPageAtomic(runDir, rendered); err != nil {
		return FailedPublicationAssessment(), err
	}
	return PublicationAssessment{Status: PublicationReady}, nil
}

// renderPublishedPage projects the page from the run's own persisted
// report.json, so the published HTML can only show data the manifest binds.
func renderPublishedPage(runDir string) ([]byte, error) {
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		return nil, fmt.Errorf("report: read published report data: %w", err)
	}
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return nil, fmt.Errorf("report: decode published report data: %w", err)
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("report: read published run manifest: %w", err)
	}
	github, gitlab, err := ordinaryHTMLSourceLinks(data.CapturedRevision, OrdinaryReportHTMLAuthority{
		StandaloneSource: manifest.StandaloneSource,
		RepositoryRoot:   manifest.RepositoryState.Identity,
		AnalysisRoot:     manifest.AnalysisRoot,
		ArtifactsDir:     runDir,
	})
	if err != nil {
		return nil, err
	}
	data.GitHubSourceLinks = github
	data.GitLabSourceLinks = gitlab
	data.ArtifactsDir = runDir
	digest := sha256.Sum256(reportJSON)
	localRoots := renderPayloadLocalRoots(&data, []string{
		runDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity,
	})
	rendered, err := executeProgramReport(&data, hex.EncodeToString(digest[:]), localRoots)
	if err != nil {
		return nil, err
	}
	if int64(len(rendered)) > MaxOrdinaryReportHTMLBytes {
		return nil, &StandaloneTargetBundleResourceLimitError{
			LimitBytes: MaxOrdinaryReportHTMLBytes, ActualBytes: int64(len(rendered)),
		}
	}
	return rendered, nil
}

func writePublishedPageAtomic(runDir string, rendered []byte) error {
	final := filepath.Join(runDir, "report.html")
	staged, err := os.CreateTemp(runDir, ".report.html.*")
	if err != nil {
		return fmt.Errorf("report: stage published page: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if _, err := staged.Write(rendered); err != nil {
		staged.Close()
		return fmt.Errorf("report: write published page: %w", err)
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return fmt.Errorf("report: sync published page: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("report: close published page: %w", err)
	}
	if err := os.Chmod(stagedPath, 0o644); err != nil {
		return fmt.Errorf("report: set published page mode: %w", err)
	}
	if err := os.Rename(stagedPath, final); err != nil {
		return fmt.Errorf("report: install published page: %w", err)
	}
	return nil
}
