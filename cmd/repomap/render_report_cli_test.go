package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

func TestRunRenderReportCLIWithAuthorizedRepository(t *testing.T) {
	runDir := t.TempDir()
	repoDir := t.TempDir()
	captures := 0
	plainCalls := 0
	authorizedCalls := 0
	var stdout bytes.Buffer
	err := runRenderReportCLIWith(
		[]string{runDir, "--repo", repoDir},
		&stdout,
		renderReportDependencies{
			captureRepo: func(_ context.Context, gotRepo string) (freshness.RepositoryState, error) {
				if gotRepo != repoDir {
					t.Fatalf("capture repo = %q", gotRepo)
				}
				captures++
				return freshness.RepositoryState{Identity: repoDir, Head: "revision"}, nil
			},
			readAuthoritySeed: func(gotRun string) (report.RunManifestAuthoritySeed, error) {
				if gotRun != runDir {
					t.Fatalf("seed run = %q", gotRun)
				}
				return report.RunManifestAuthoritySeed{
					RepositoryIdentity: repoDir,
					AnalysisRoot:       repoDir,
					SelectedRevision:   "revision",
					CapturedInputPaths: []string{"go.mod"},
				}, nil
			},
			confirmAuthorityScoped: func(_ context.Context, gotRoot string, _, _ freshness.RepositoryState, paths []string, strict bool) (report.RunAuthority, error) {
				if gotRoot != repoDir {
					t.Fatalf("analysis root = %q", gotRoot)
				}
				if len(paths) != 1 || paths[0] != "go.mod" || !strict {
					t.Fatalf("paths=%v strict=%v", paths, strict)
				}
				return report.RunAuthority{}, nil
			},
			generate: func(string) error {
				plainCalls++
				return nil
			},
			generateAuthorized: func(gotRun string, _ report.RunAuthority) error {
				if gotRun != runDir {
					t.Fatalf("authorized run = %q", gotRun)
				}
				authorizedCalls++
				return nil
			},
			resolveAnalysisRoot: func(gotRepo string) (string, error) {
				if gotRepo != repoDir {
					t.Fatalf("resolve repo = %q", gotRepo)
				}
				return repoDir, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("runRenderReportCLIWith: %v", err)
	}
	if captures != 2 || plainCalls != 0 || authorizedCalls != 1 {
		t.Fatalf("captures=%d plain=%d authorized=%d", captures, plainCalls, authorizedCalls)
	}
	if !strings.Contains(stdout.String(), runDir+"/report.html") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRenderReportCLIWithPlainAndFailClosedPaths(t *testing.T) {
	dependencies := renderReportDependencies{
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			return freshness.RepositoryState{}, errors.New("capture must not run")
		},
		readAuthoritySeed: func(string) (report.RunManifestAuthoritySeed, error) {
			return report.RunManifestAuthoritySeed{}, errors.New("seed must not run")
		},
		confirmAuthorityScoped: func(context.Context, string, freshness.RepositoryState, freshness.RepositoryState, []string, bool) (report.RunAuthority, error) {
			return report.RunAuthority{}, errors.New("confirm must not run")
		},
		generate: func(string) error { return nil },
		generateAuthorized: func(string, report.RunAuthority) error {
			return errors.New("authorized render must not run")
		},
		resolveAnalysisRoot: func(string) (string, error) {
			return "", errors.New("resolve must not run")
		},
	}
	var stdout bytes.Buffer
	if err := runRenderReportCLIWith([]string{t.TempDir()}, &stdout, dependencies); err != nil {
		t.Fatalf("plain render: %v", err)
	}
	for _, args := range [][]string{
		nil,
		{t.TempDir(), "--repo"},
		{t.TempDir(), "--repo", ""},
		{t.TempDir(), "--unknown", t.TempDir()},
	} {
		if err := runRenderReportCLIWith(args, &bytes.Buffer{}, dependencies); err == nil ||
			!strings.Contains(err.Error(), "usage:") {
			t.Fatalf("args %q error = %v", args, err)
		}
	}
}

func TestRunRenderReportCLIWithDoesNotRenderAfterSecondCaptureFailure(t *testing.T) {
	captures := 0
	rendered := false
	err := runRenderReportCLIWith(
		[]string{t.TempDir(), "--repo", t.TempDir()},
		&bytes.Buffer{},
		renderReportDependencies{
			captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
				captures++
				if captures == 2 {
					return freshness.RepositoryState{}, errors.New("changed while capturing")
				}
				return freshness.RepositoryState{}, nil
			},
			readAuthoritySeed: func(string) (report.RunManifestAuthoritySeed, error) {
				repo := t.TempDir()
				return report.RunManifestAuthoritySeed{RepositoryIdentity: repo, AnalysisRoot: repo}, nil
			},
			confirmAuthorityScoped: func(context.Context, string, freshness.RepositoryState, freshness.RepositoryState, []string, bool) (report.RunAuthority, error) {
				return report.RunAuthority{}, nil
			},
			generate: func(string) error { return nil },
			generateAuthorized: func(string, report.RunAuthority) error {
				rendered = true
				return nil
			},
			resolveAnalysisRoot: func(repo string) (string, error) { return repo, nil },
		},
	)
	if err == nil || !strings.Contains(err.Error(), "capture repository after authority confirmation") {
		t.Fatalf("error = %v", err)
	}
	if rendered {
		t.Fatal("authorized render ran after failed second capture")
	}
}
