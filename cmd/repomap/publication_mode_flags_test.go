package main

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"
)

func TestExplicitPortIsRejectedOnlyForStaticReportMode(t *testing.T) {
	if err := validateReportModeFlags(false, true); err != nil {
		t.Fatalf("served explicit port: %v", err)
	}
	if err := validateReportModeFlags(true, false); err != nil {
		t.Fatalf("static report without explicit port: %v", err)
	}
	err := validateReportModeFlags(true, true)
	if err == nil || !strings.Contains(err.Error(), "remove --port") ||
		!strings.Contains(err.Error(), "serve locally") {
		t.Fatalf("static explicit port error = %v", err)
	}
}

func TestDirectCallControlsUseZeroAsUnboundedAndAcceptPositiveNarrowing(t *testing.T) {
	for _, controls := range [][2]int{{0, 0}, {11, 300_000}} {
		if err := validateDirectCallControls(controls[0], controls[1]); err != nil {
			t.Fatalf("controls depth=%d edges=%d: %v", controls[0], controls[1], err)
		}
	}
	for _, controls := range [][2]int{{-1, 0}, {0, -1}} {
		if err := validateDirectCallControls(controls[0], controls[1]); err == nil ||
			!strings.Contains(err.Error(), "0 keeps all") {
			t.Fatalf("negative controls depth=%d edges=%d error = %v", controls[0], controls[1], err)
		}
	}
}

func TestDirectCallExplicitEdgeCeilingGuidanceHasNoAbsoluteMaximum(t *testing.T) {
	err := directCallEdgeCeilingError("example.com/server", 0, 300_000, 12)
	if err == nil {
		t.Fatal("missing explicit edge ceiling error")
	}
	for _, expected := range []string{
		"explicit --edges-limit=300000",
		"all reachable calls",
		"--depth 12",
		"--edges-limit 600000",
		"--edges-limit 0",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("edge ceiling guidance missing %q:\n%s", expected, err)
		}
	}
}

func TestUsageExplainsUnboundedDirectCallDefaults(t *testing.T) {
	var output bytes.Buffer
	printUsageTo(&output)
	for _, expected := range []string{
		"--depth N",
		"target call-graph depth (0 = all; default: 0)",
		"--edges-limit N",
		"maximum exact target call-graph edges (0 = all; default: 0)",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("usage missing %q:\n%s", expected, output.String())
		}
	}
}

func TestBindParsedRepositoryArgumentTracksPositionalAfterFlags(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("github-url", "", "")
	if err := fs.Parse([]string{
		"--github-url", "https://github.com/upstream/project", "../checkout",
	}); err != nil {
		t.Fatal(err)
	}
	repo, omitted, err := bindParsedRepositoryArgument(".", true, fs.Args())
	if err != nil {
		t.Fatal(err)
	}
	if repo != "../checkout" || omitted {
		t.Fatalf("repository binding = %q, omitted %t", repo, omitted)
	}

	if _, _, err := bindParsedRepositoryArgument(".", false, []string{"other"}); err == nil {
		t.Fatal("second repository positional unexpectedly succeeded")
	}
}

func TestValidateImplicitRepositorySourceLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		flagName   string
		url        string
		origin     string
		omitted    bool
		wantError  bool
		credential string
	}{
		{
			name:     "matching GitHub origin",
			flagName: "--github-url", url: "https://github.com:8443/Team/Project",
			origin: "github.com/team/project", omitted: true,
		},
		{
			name:     "matching GitLab subgroup origin",
			flagName: "--gitlab-url", url: "https://gitlab.example.test/group/services/project",
			origin: "gitlab.example.test/group/services/project", omitted: true,
		},
		{
			name:     "missing origin preserves complete URL",
			flagName: "--github-url", url: "https://github.com/upstream/project",
			omitted: true,
		},
		{
			name:     "explicit repository preserves upstream override",
			flagName: "--gitlab-url", url: "https://gitlab.example.test/upstream/project",
			origin: "gitlab.example.test/fork/project", omitted: false,
		},
		{
			name:     "repository conflict",
			flagName: "--github-url", url: "https://github.com/upstream/project",
			origin: "github.com/fork/project", omitted: true, wantError: true,
		},
		{
			name:     "host conflict",
			flagName: "--gitlab-url", url: "https://gitlab.example.test/team/project",
			origin: "other.example.test/team/project", omitted: true, wantError: true,
		},
		{
			name:     "unsafe origin is not echoed",
			flagName: "--github-url", url: "https://github.com/team/project",
			origin: "secret-token@attacker.example/team/project", omitted: true,
			wantError: true, credential: "secret-token",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateImplicitRepositorySourceLink(
				test.flagName,
				test.url,
				test.origin,
				test.omitted,
			)
			if !test.wantError {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.flagName) ||
				!strings.Contains(err.Error(), "does not clone or select a repository") ||
				!strings.Contains(err.Error(), "pass [repo] explicitly") {
				t.Fatalf("source-link conflict error = %v", err)
			}
			if test.credential != "" && strings.Contains(err.Error(), test.credential) {
				t.Fatalf("source-link conflict echoed origin credential: %v", err)
			}
		})
	}
}
