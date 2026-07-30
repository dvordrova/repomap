package report

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/freshness"
)

// GitLabSourceLinks turns repository locations in a standalone report into
// ordinary links to the exact analyzed Git revision. RepositoryURL never
// contains credentials, query parameters, or a fragment.
type GitLabSourceLinks struct {
	RepositoryURL string `json:"repository_url"`
	Revision      string `json:"revision"`
	PathPrefix    string `json:"path_prefix,omitempty"`
}

// NormalizeGitLabRepositoryURL accepts a GitLab project URL, removes the
// optional clone suffix, and rejects URL features that could accidentally
// persist credentials in a shareable report.
func NormalizeGitLabRepositoryURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("--gitlab-url must be a valid http(s) project URL")
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("--gitlab-url must be an absolute http(s) project URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("--gitlab-url must not contain credentials, query parameters, or a fragment")
	}
	escapedProjectPath := parsed.EscapedPath()
	escapedLower := strings.ToLower(escapedProjectPath)
	if strings.Contains(escapedLower, "%2f") || strings.Contains(escapedLower, "%5c") {
		return "", fmt.Errorf("--gitlab-url contains an invalid project path")
	}
	projectPath := strings.TrimSuffix(strings.TrimSuffix(escapedProjectPath, "/"), ".git")
	if projectPath == "" || projectPath == "/" {
		return "", fmt.Errorf("--gitlab-url must identify a GitLab project")
	}
	decodedPath, err := url.PathUnescape(projectPath)
	if err != nil || strings.Contains(decodedPath, "\\") {
		return "", fmt.Errorf("--gitlab-url contains an invalid project path")
	}
	cleaned := path.Clean(decodedPath)
	if cleaned != decodedPath || cleaned == "." || cleaned == "/" || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") || !strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("--gitlab-url contains an invalid project path")
	}
	projectSegments := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")
	if len(projectSegments) < 2 || strings.Contains(cleaned, "/-/") {
		return "", fmt.Errorf("--gitlab-url must identify a GitLab project root")
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("--gitlab-url contains an invalid project path")
		}
	}
	parsed.Path = cleaned
	parsed.RawPath = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func newGitLabSourceLinks(repositoryURL, revision, pathPrefix string) (*GitLabSourceLinks, error) {
	normalizedURL, err := NormalizeGitLabRepositoryURL(repositoryURL)
	if err != nil {
		return nil, err
	}
	if normalizedURL == "" {
		return nil, fmt.Errorf("report: GitLab repository URL is required")
	}
	revision = strings.TrimSpace(revision)
	if !validGitRevision(revision) {
		return nil, fmt.Errorf("report: GitLab source revision must be a 40- or 64-character Git object ID")
	}
	pathPrefix = strings.Trim(strings.TrimSpace(pathPrefix), "/")
	if pathPrefix != "" {
		if err := validateManifestPath(pathPrefix); err != nil {
			return nil, fmt.Errorf("report: GitLab source path prefix is invalid")
		}
	}
	return &GitLabSourceLinks{
		RepositoryURL: normalizedURL,
		Revision:      strings.ToLower(revision),
		PathPrefix:    pathPrefix,
	}, nil
}

func (links *GitLabSourceLinks) validate() error {
	if links == nil {
		return nil
	}
	normalized, err := newGitLabSourceLinks(links.RepositoryURL, links.Revision, links.PathPrefix)
	if err != nil {
		return err
	}
	if normalized.RepositoryURL != links.RepositoryURL ||
		normalized.Revision != links.Revision ||
		normalized.PathPrefix != links.PathPrefix {
		return fmt.Errorf("report: GitLab source links are not canonical")
	}
	return nil
}

func validGitRevision(revision string) bool {
	if len(revision) != 40 && len(revision) != 64 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

func validateGitLabAuthority(authority RunAuthority) error {
	if err := authority.validate(); err != nil {
		return err
	}
	if len(authority.repository.Dirty) != 0 ||
		authority.freshness.State != freshness.FreshnessFresh {
		return fmt.Errorf("report: standalone GitLab report requires a clean, unchanged repository")
	}
	for _, submodule := range authority.repository.Submodules {
		if submodule.IncludedInAnalysis {
			return fmt.Errorf("report: standalone GitLab report does not support analyzed submodule source")
		}
	}
	return nil
}
