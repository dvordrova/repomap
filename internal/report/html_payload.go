package report

import (
	"bytes"
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

// VerifyOrdinaryReportHTMLPayload reprojects the strict canonical report and
// requires the HTML to carry exactly one matching repository payload and one
// matching opaque chunk for the current analyzed target.
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
		return fmt.Errorf("report: ordinary html is missing the report application shell")
	}
	if err := validateTargetNavigation(&data, authority.TargetNavigation); err != nil {
		return fmt.Errorf("report: expected target navigation: %w", err)
	}
	expectedGitHub, expectedGitLab, err := ordinaryHTMLSourceLinks(data.CapturedRevision, authority)
	if err != nil {
		return err
	}
	data.GitHubSourceLinks = expectedGitHub
	data.GitLabSourceLinks = expectedGitLab
	localRoots := []string{authority.ArtifactsDir}
	if expectedGitHub != nil || expectedGitLab != nil {
		localRoots = append(localRoots, authority.AnalysisRoot, authority.RepositoryRoot)
	}
	expected, err := buildOrdinaryBrowserTransportV4(
		&data, authority.TargetNavigation, localRoots,
	)
	if err != nil {
		return fmt.Errorf("report: project expected browser transport: %w", err)
	}
	actual, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		return fmt.Errorf("report: extract ordinary browser transport: %w", err)
	}
	if len(actual.Index.Targets) != 1 || len(actual.TargetChunks) != 1 ||
		actual.Index.Targets[0].State != standaloneBundleTransportTargetAnalyzed ||
		actual.Index.Targets[0].Chunk == nil {
		return fmt.Errorf("report: ordinary browser transport must contain exactly one analyzed target")
	}
	repositoryPayload, err := DecodeBrowserRepositoryPayload(actual.RepositoryPayload)
	if err != nil {
		return fmt.Errorf("report: decode embedded repository payload: %w", err)
	}
	canonicalRepository, err := EncodeBrowserRepositoryPayload(repositoryPayload)
	if err != nil || !bytes.Equal(canonicalRepository, actual.RepositoryPayload) {
		return fmt.Errorf("report: embedded repository payload is not canonical")
	}
	if !bytes.Equal(actual.IndexJSON, expected.IndexJSON) {
		return fmt.Errorf("report: embedded browser transport index does not match report authority")
	}
	if !bytes.Equal(actual.RepositoryPayload, expected.RepositoryPayload) {
		return fmt.Errorf("report: embedded repository payload does not match report.json")
	}
	actualTargetRaw, err := decodeStandaloneBundleTargetChunkV4(
		actual.TargetChunks[0].Ref, actual.TargetChunks[0].Base64,
	)
	if err != nil {
		return fmt.Errorf("report: decode ordinary target chunk: %w", err)
	}
	targetPayload, err := DecodeBrowserTargetPayload(actualTargetRaw)
	if err != nil {
		return fmt.Errorf("report: decode embedded target payload: %w", err)
	}
	canonicalTarget, err := EncodeBrowserTargetPayload(targetPayload)
	if err != nil || !bytes.Equal(canonicalTarget, actualTargetRaw) {
		return fmt.Errorf("report: embedded target payload is not canonical")
	}
	expectedTargetRaw, err := decodeStandaloneBundleTargetChunkV4(
		expected.TargetChunks[0].Ref, expected.TargetChunks[0].Base64,
	)
	if err != nil {
		return fmt.Errorf("report: decode expected target chunk: %w", err)
	}
	if !bytes.Equal(actualTargetRaw, expectedTargetRaw) {
		return fmt.Errorf("report: embedded target payload does not match report.json")
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
