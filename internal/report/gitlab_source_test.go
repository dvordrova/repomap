package report

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestNormalizeGitLabRepositoryURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "gitlab dot com clone URL",
			raw:  " https://gitlab.com/group/project.git/ ",
			want: "https://gitlab.com/group/project",
		},
		{
			name: "self hosted subgroup and port",
			raw:  "http://gitlab.example.test:8443/root/platform/project",
			want: "http://gitlab.example.test:8443/root/platform/project",
		},
		{
			name: "escaped project name",
			raw:  "https://gitlab.example.test/team/project%20name.git",
			want: "https://gitlab.example.test/team/project%20name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeGitLabRepositoryURL(test.raw)
			if err != nil {
				t.Fatalf("NormalizeGitLabRepositoryURL(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("NormalizeGitLabRepositoryURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestNormalizeGitLabRepositoryURLRejectsUnsafeOrIncompleteURLs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"git@gitlab.example.test:team/project.git",
		"ssh://git@gitlab.example.test/team/project.git",
		"https://token@gitlab.example.test/team/project",
		"https://gitlab.example.test/team/project?private_token=secret",
		"https://gitlab.example.test/team/project#main",
		"https://gitlab.example.test",
		"https://gitlab.example.test/project",
		"https://gitlab.example.test/team/project/-/tree/main",
		"https://gitlab.example.test/../project",
		"https://gitlab.example.test/team//project",
		"https://gitlab.example.test/team%2Fother/project",
		"https://gitlab.example.test/team/project%5Cother",
		"https://gitlab.example.test/team/project%0Aother",
		"https://gitlab.example.test/team/project\\other",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeGitLabRepositoryURL(raw); err == nil {
				t.Fatalf("NormalizeGitLabRepositoryURL(%q) succeeded", raw)
			}
		})
	}
}

func TestResolveGitLabRepositoryURLInfersProjectFromRemoteIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		raw            string
		remoteIdentity string
		want           string
	}{
		{
			name:           "host only",
			raw:            "https://gitlab.example.test",
			remoteIdentity: "gitlab.example.test/team/project",
			want:           "https://gitlab.example.test/team/project",
		},
		{
			name:           "host only with web port and subgroup",
			raw:            "https://gitlab.example.test:8443/",
			remoteIdentity: "gitlab.example.test/platform/services/project",
			want:           "https://gitlab.example.test:8443/platform/services/project",
		},
		{
			name:           "complete URL does not need remote",
			raw:            "https://gitlab.example.test/other/project.git",
			remoteIdentity: "",
			want:           "https://gitlab.example.test/other/project",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveGitLabRepositoryURL(test.raw, test.remoteIdentity)
			if err != nil {
				t.Fatalf("ResolveGitLabRepositoryURL(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("ResolveGitLabRepositoryURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestResolveGitLabRepositoryURLRejectsMissingOrMismatchedRemote(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		remoteIdentity string
		want           string
	}{
		{name: "missing", want: "repository-local origin remote"},
		{
			name:           "different host",
			remoteIdentity: "github.com/team/project",
			want:           "does not match repository remote host",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveGitLabRepositoryURL("https://gitlab.example.test", test.remoteIdentity)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveGitLabRepositoryURL() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveGitLabRepositoryURLErrorDoesNotEchoRemoteCredentials(t *testing.T) {
	t.Parallel()

	const credential = "secret-token"
	_, err := ResolveGitLabRepositoryURL(
		"https://gitlab.example.test",
		credential+"@attacker.example/team/project",
	)
	if err == nil {
		t.Fatal("ResolveGitLabRepositoryURL() unexpectedly accepted unsafe remote identity")
	}
	if strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), "attacker.example") {
		t.Fatalf("ResolveGitLabRepositoryURL() echoed remote identity: %v", err)
	}
}

func TestNewGitLabSourceLinksValidatesRevisionAndPathPrefix(t *testing.T) {
	t.Parallel()

	links, err := newGitLabSourceLinks(
		"https://gitlab.example.test/team/project.git",
		strings.Repeat("A", 40),
		"services/api",
	)
	if err != nil {
		t.Fatal(err)
	}
	if links.RepositoryURL != "https://gitlab.example.test/team/project" ||
		links.Revision != strings.Repeat("a", 40) ||
		links.PathPrefix != "services/api" {
		t.Fatalf("links = %#v", links)
	}
	for _, test := range []struct {
		revision string
		prefix   string
	}{
		{revision: "main"},
		{revision: strings.Repeat("g", 40)},
		{revision: strings.Repeat("a", 40), prefix: "../service"},
		{revision: strings.Repeat("a", 40), prefix: "service//api"},
		{revision: strings.Repeat("a", 40), prefix: `service\api`},
	} {
		if _, err := newGitLabSourceLinks(
			"https://gitlab.example.test/team/project",
			test.revision,
			test.prefix,
		); err == nil {
			t.Fatalf("newGitLabSourceLinks(%q, %q) succeeded", test.revision, test.prefix)
		}
	}
}

func TestGitLabAuthorityAllowsCapturedDirtyStateAndRejectsAnalyzedSubmodules(t *testing.T) {
	repository := newRunManifestRepository(t)
	writeTestFile(t, repository, "batch.go", "package fixture\n\nfunc Commit() { panic(\"dirty\") }\n")
	dirty := captureRunManifestRepositoryState(t, repository)
	dirtyAuthority, err := ConfirmRunAuthorityScoped(
		context.Background(), repository, dirty, []string{"batch.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGitLabAuthority(dirtyAuthority); err != nil {
		t.Fatalf("stable dirty authority rejected: %v", err)
	}

	cleanRepository := newRunManifestRepository(t)
	clean := captureRunManifestRepositoryState(t, cleanRepository)
	submoduleAuthority, err := ConfirmRunAuthorityScoped(
		context.Background(), cleanRepository, clean, []string{"batch.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	submoduleAuthority.repository.Submodules = []freshness.SubmoduleState{{
		Path:               "third_party/library",
		IncludedInAnalysis: true,
		RecordedGitlink:    strings.Repeat("b", 40),
		CurrentHead:        strings.Repeat("b", 40),
		Availability:       freshness.SubmoduleClean,
	}}
	if err := GenerateAuthorizedGitLab(
		t.TempDir(),
		submoduleAuthority,
		"https://gitlab.example.test/team/project",
	); err == nil || !strings.Contains(err.Error(), "does not support analyzed submodule source") {
		t.Fatalf("analyzed submodule authority error = %v", err)
	}
}

func TestGitLabSourcePathPrefix(t *testing.T) {
	t.Parallel()

	root := filepath.Join(string(filepath.Separator), "work", "repository")
	for _, test := range []struct {
		name         string
		analysisRoot string
		want         string
		wantErr      bool
	}{
		{name: "repository root", analysisRoot: root},
		{name: "nested analysis root", analysisRoot: filepath.Join(root, "services", "api"), want: "services/api"},
		{name: "outside repository", analysisRoot: filepath.Dir(root), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := standaloneSourcePathPrefix(root, test.analysisRoot)
			if (err != nil) != test.wantErr {
				t.Fatalf("standaloneSourcePathPrefix() error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("standaloneSourcePathPrefix() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMarshalHTMLPayloadKeepsSourceContent(t *testing.T) {
	const sentinel = "UNCLASSIFIED_SOURCE_SIGNAL_SENTINEL"
	data, err := marshalHTMLPayloadWithLocalRoots(map[string]any{
		"gitlab_source_links": map[string]any{
			"repository_url": "https://gitlab.example.test/team/project",
			"revision":       strings.Repeat("a", 40),
		},
		"component": map[string]any{
			"path":    "internal/service.go",
			"line":    7,
			"snippet": sentinel,
			"reason":  "exact location without a scanner category",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(sentinel)) || !bytes.Contains(data, []byte(`"snippet"`)) {
		t.Fatalf("browser payload lost source content: %s", data)
	}
	if !bytes.Contains(data, []byte(`"path":"internal/service.go"`)) {
		t.Fatalf("standalone payload lost source location: %s", data)
	}
}

func TestMarshalStandaloneHTMLPayloadScrubsLocalRoots(t *testing.T) {
	const localRoot = "/tmp/repomap-private-run"
	data, err := marshalHTMLPayloadWithLocalRoots(&ReportData{
		GitLabSourceLinks: &GitLabSourceLinks{
			RepositoryURL: "https://gitlab.example.test/team/project",
			Revision:      strings.Repeat("a", 40),
		},
		Warnings: []string{"orientation: open " + localRoot + "/orientation_report.json"},
	}, []string{localRoot})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(localRoot)) {
		t.Fatalf("standalone payload retained local root: %s", data)
	}
	if !bytes.Contains(data, []byte(`orientation: open [local path]/orientation_report.json`)) {
		t.Fatalf("standalone payload lost useful warning context: %s", data)
	}
}
