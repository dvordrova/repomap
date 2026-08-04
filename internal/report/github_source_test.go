package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeGitHubRepositoryURL(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		raw  string
		want string
	}{
		{
			raw:  " https://github.com/devodev/go-office365.git/ ",
			want: "https://github.com/devodev/go-office365",
		},
		{
			raw:  "http://github.example.test:8443/platform/service",
			want: "http://github.example.test:8443/platform/service",
		},
	} {
		got, err := NormalizeGitHubRepositoryURL(test.raw)
		if err != nil {
			t.Fatalf("NormalizeGitHubRepositoryURL(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("NormalizeGitHubRepositoryURL(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestNormalizeGitHubRepositoryURLRejectsUnsafeOrNonRootURLs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"git@github.com:team/project.git",
		"ssh://git@github.com/team/project.git",
		"https://token@github.com/team/project",
		"https://github.com/team/project?token=secret",
		"https://github.com/team/project#main",
		"https://github.com",
		"https://github.com/project",
		"https://github.com/team/project/blob/main/main.go",
		"https://github.com/team/project/pull/1",
		"https://github.com/team%2Fother/project",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeGitHubRepositoryURL(raw); err == nil {
				t.Fatalf("NormalizeGitHubRepositoryURL(%q) succeeded", raw)
			}
		})
	}
}

func TestResolveGitHubRepositoryURLInfersRepositoryFromOrigin(t *testing.T) {
	t.Parallel()

	got, err := ResolveGitHubRepositoryURL(
		"https://github.com",
		"github.com/devodev/go-office365",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/devodev/go-office365" {
		t.Fatalf("ResolveGitHubRepositoryURL() = %q", got)
	}
	if _, err := ResolveGitHubRepositoryURL(
		"https://github.com",
		"gitlab.com/devodev/go-office365",
	); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched origin error = %v", err)
	}
}

func TestGitHubPresentationIsHTMLOnly(t *testing.T) {
	data := ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "github-fixture",
		GitHubSourceLinks: &GitHubSourceLinks{
			RepositoryURL: "https://github.com/team/project",
			Revision:      strings.Repeat("a", 40),
		},
	}
	html, err := RenderHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte(`"github_source_links"`)) ||
		!bytes.Contains(html, []byte(`"repository_url":"https://github.com/team/project"`)) {
		t.Fatalf("standalone HTML missing GitHub routing: %s", html)
	}

	jsonPath := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReportJSON(&data, jsonPath); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte("github_source_links")) {
		t.Fatalf("canonical report contains HTML-only GitHub routing: %s", persisted)
	}
}
