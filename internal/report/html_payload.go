package report

import (
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
