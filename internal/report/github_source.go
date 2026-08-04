package report

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode"
)

// GitHubSourceLinks turns repository locations in a standalone report into
// ordinary GitHub links pinned to the exact analyzed revision.
type GitHubSourceLinks struct {
	RepositoryURL    string   `json:"repository_url"`
	Revision         string   `json:"revision"`
	PathPrefix       string   `json:"path_prefix,omitempty"`
	WorkingTreeDirty bool     `json:"working_tree_dirty,omitempty"`
	WorkingTreePaths []string `json:"working_tree_paths,omitempty"`
}

// ResolveGitHubRepositoryURL accepts either a complete GitHub repository URL
// or a host-only URL completed from the credential-free origin identity.
func ResolveGitHubRepositoryURL(raw, remoteIdentity string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return NormalizeGitHubRepositoryURL(raw)
	}
	if strings.Trim(parsed.EscapedPath(), "/") != "" {
		return NormalizeGitHubRepositoryURL(raw)
	}

	remoteHost, remoteProject, found := strings.Cut(strings.TrimSpace(remoteIdentity), "/")
	if !found || remoteHost == "" || remoteProject == "" {
		return "", fmt.Errorf("--github-url host-only form requires a repository-local origin remote with a project path")
	}
	if !strings.EqualFold(parsed.Hostname(), remoteHost) {
		return "", fmt.Errorf(
			"--github-url host %q does not match repository remote host; supply the complete GitHub repository URL",
			parsed.Hostname(),
		)
	}
	parsed.Path = "/" + remoteProject
	parsed.RawPath = ""
	return NormalizeGitHubRepositoryURL(parsed.String())
}

// NormalizeGitHubRepositoryURL accepts a GitHub repository root URL and
// rejects credentials and page URLs that are unsafe to persist in HTML.
func NormalizeGitHubRepositoryURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("--github-url must be a valid http(s) repository URL")
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("--github-url must be an absolute http(s) repository URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("--github-url must not contain credentials, query parameters, or a fragment")
	}
	escapedRepositoryPath := parsed.EscapedPath()
	escapedLower := strings.ToLower(escapedRepositoryPath)
	if strings.Contains(escapedLower, "%2f") || strings.Contains(escapedLower, "%5c") {
		return "", fmt.Errorf("--github-url contains an invalid repository path")
	}
	repositoryPath := strings.TrimSuffix(strings.TrimSuffix(escapedRepositoryPath, "/"), ".git")
	if repositoryPath == "" || repositoryPath == "/" {
		return "", fmt.Errorf("--github-url must identify a GitHub repository")
	}
	decodedPath, err := url.PathUnescape(repositoryPath)
	if err != nil || strings.Contains(decodedPath, "\\") {
		return "", fmt.Errorf("--github-url contains an invalid repository path")
	}
	cleaned := path.Clean(decodedPath)
	if cleaned != decodedPath || cleaned == "." || cleaned == "/" || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") || !strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("--github-url contains an invalid repository path")
	}
	repositorySegments := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")
	if len(repositorySegments) != 2 {
		return "", fmt.Errorf("--github-url must identify a GitHub repository root")
	}
	for _, segment := range repositorySegments {
		if segment == "" {
			return "", fmt.Errorf("--github-url contains an invalid repository path")
		}
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("--github-url contains an invalid repository path")
		}
	}
	parsed.Path = cleaned
	parsed.RawPath = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func newGitHubSourceLinks(repositoryURL, revision, pathPrefix string) (*GitHubSourceLinks, error) {
	normalizedURL, err := NormalizeGitHubRepositoryURL(repositoryURL)
	if err != nil {
		return nil, err
	}
	if normalizedURL == "" {
		return nil, fmt.Errorf("report: GitHub repository URL is required")
	}
	revision = strings.TrimSpace(revision)
	if !validGitRevision(revision) {
		return nil, fmt.Errorf("report: GitHub source revision must be a 40- or 64-character Git object ID")
	}
	pathPrefix = strings.Trim(strings.TrimSpace(pathPrefix), "/")
	if pathPrefix != "" {
		if err := validateManifestPath(pathPrefix); err != nil {
			return nil, fmt.Errorf("report: GitHub source path prefix is invalid")
		}
	}
	return &GitHubSourceLinks{
		RepositoryURL: normalizedURL,
		Revision:      strings.ToLower(revision),
		PathPrefix:    pathPrefix,
	}, nil
}

func (links *GitHubSourceLinks) validate() error {
	if links == nil {
		return nil
	}
	normalized, err := newGitHubSourceLinks(links.RepositoryURL, links.Revision, links.PathPrefix)
	if err != nil {
		return err
	}
	if normalized.RepositoryURL != links.RepositoryURL ||
		normalized.Revision != links.Revision ||
		normalized.PathPrefix != links.PathPrefix {
		return fmt.Errorf("report: GitHub source links are not canonical")
	}
	return validateWorkingTreeSourceLinks(
		links.WorkingTreeDirty,
		links.WorkingTreePaths,
		"GitHub",
	)
}
