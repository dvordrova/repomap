package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
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

func TestReviewedRootLinkKeepsNestedDirectory(t *testing.T) {
	for _, prefix := range []string{"/blob/", "/-/blob/"} {
		links := pageLinks{repositoryURL: "https://example.test/owner/repo", blobPrefix: prefix, revision: "abc", pathPrefix: "examples/tutorial game"}
		want := "https://example.test/owner/repo" + strings.Replace(prefix, "blob", "tree", 1) + "abc/examples/tutorial%20game"
		if got := links.rootURL(); got != want {
			t.Fatalf("root URL = %q, want %q", got, want)
		}
		links.pathPrefix = ""
		if links.rootURL() != links.repositoryURL {
			t.Fatal("a full repository should link to its root")
		}
	}
}

// A declaration's code link covers all of its lines (owner, 2026-09-28):
// GitHub writes #L1250-L1360, GitLab #L1250-1360; one line or no static
// link has none.
func TestADeclarationsCodeLinkCoversItsLines(t *testing.T) {
	github := pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}
	if got := github.rangeLink("redis.c", 1250, 1360); got != "https://github.com/o/r/blob/abc/redis.c#L1250-L1360" {
		t.Fatalf("GitHub: %s", got)
	}
	gitlab := pageLinks{repositoryURL: "https://gitlab.com/o/r", blobPrefix: "/-/blob/", revision: "abc"}
	if got := gitlab.rangeLink("redis.c", 1250, 1360); got != "https://gitlab.com/o/r/-/blob/abc/redis.c#L1250-1360" {
		t.Fatalf("GitLab: %s", got)
	}
	if github.rangeLink("redis.c", 12, 12) != "" || (pageLinks{}).rangeLink("redis.c", 1, 9) != "" {
		t.Fatal("one line, or no static links, has no range")
	}
	builder := &pageBuilder{links: github, data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Objects: []programindex.Object{
		{ID: "n1", Name: "serverCron", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: 1250, Column: 5}, EndLine: 1360}}}}}}}
	_, anchor := builder.subjectDisplay(groupindex.Subject{ID: "n1", Object: &groupindex.ObjectFacts{Name: "serverCron", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: 1250, Column: 5}}})
	if anchor.Code != "https://github.com/o/r/blob/abc/redis.c#L1250-L1360" || anchor.Href != "https://github.com/o/r/blob/abc/redis.c#L1250" {
		t.Fatalf("the declaration's anchor: %+v", anchor)
	}
}
