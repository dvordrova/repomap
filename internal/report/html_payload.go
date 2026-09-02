package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// OrdinaryReportHTMLAuthority carries the local rendering authority that is
// deliberately absent from canonical report.json.
type OrdinaryReportHTMLAuthority struct {
	TargetNavigation *TargetNavigationPortfolio
	StandaloneSource *StandaloneSourceAuthority
	ArtifactsDir     string
	AnalysisRoot     string
	RepositoryRoot   string
}

// VerifyOrdinaryReportHTMLPayload proves that one published page was
// rendered from exactly the report.json the manifest binds. The page carries
// no analysis payload of its own, so the check is the stamped digest of those
// bytes plus the page shell; content equality follows from the digest.
func VerifyOrdinaryReportHTMLPayload(
	htmlBytes, reportJSON []byte,
	authority OrdinaryReportHTMLAuthority,
) error {
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return fmt.Errorf("report: verify html report data: %w", err)
	}
	if err := validateProgramPresentation(&data); err != nil {
		return fmt.Errorf("report: verify html report data: %w", err)
	}
	if !bytes.Contains(bytes.ToLower(htmlBytes), []byte("<html")) {
		return fmt.Errorf("report: ordinary html is missing the report page")
	}
	if err := validateTargetNavigation(&data, authority.TargetNavigation); err != nil {
		return fmt.Errorf("report: expected target navigation: %w", err)
	}
	digest := sha256.Sum256(reportJSON)
	stamped := fmt.Sprintf(
		`<meta name="repomap-report-sha256" content="%s">`, hex.EncodeToString(digest[:]),
	)
	if !bytes.Contains(htmlBytes, []byte(stamped)) {
		return fmt.Errorf("report: published page is not stamped with its report data digest")
	}
	version := fmt.Sprintf(
		`<meta name="repomap-format-version" content="%d">`, data.FormatVersion,
	)
	if !bytes.Contains(htmlBytes, []byte(version)) {
		return fmt.Errorf("report: published page does not carry the report format version")
	}
	return nil
}

func ordinaryHTMLSourceLinks(
	revision string,
	authority OrdinaryReportHTMLAuthority,
) (*GitHubSourceLinks, *GitLabSourceLinks, error) {
	if authority.StandaloneSource == nil {
		return nil, nil, nil
	}
	if err := authority.StandaloneSource.validate(); err != nil {
		return nil, nil, err
	}
	prefix, err := standaloneSourcePathPrefix(authority.RepositoryRoot, authority.AnalysisRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("report: embedded source path authority: %w", err)
	}
	switch authority.StandaloneSource.Host {
	case "GitHub":
		links, err := newGitHubSourceLinks(
			authority.StandaloneSource.RepositoryURL, revision, prefix,
		)
		return links, nil, err
	case "GitLab":
		links, err := newGitLabSourceLinks(
			authority.StandaloneSource.RepositoryURL, revision, prefix,
		)
		return nil, links, err
	default:
		return nil, nil, fmt.Errorf("report: unsupported manifest source host")
	}
}
