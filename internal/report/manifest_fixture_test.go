package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/freshness"
)

func TestRunManifestRetainsRepositoryStateBeyondFormerDirtyEntryThreshold(t *testing.T) {
	manifest := validRunManifestFixture(t)
	manifest.RepositoryState.Dirty = make([]freshness.DirtyFile, maxManifestRepositoryDirtyFiles+1)
	for index := range manifest.RepositoryState.Dirty {
		manifest.RepositoryState.Dirty[index] = freshness.DirtyFile{
			Status: "modified", Path: fmt.Sprintf("dirty/%05d.go", index),
			Kind: freshness.FileRegular, ContentSHA256: strings.Repeat("d", 64),
		}
	}
	digest, err := manifest.RepositoryState.Digest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.RepositoryStateSHA256 = digest
	if err := manifest.Validate(); err != nil {
		t.Fatalf("manifest above former dirty-entry threshold: %v", err)
	}
	warnings := RunManifestScaleWarnings(manifest)
	found := false
	for _, warning := range warnings {
		found = found || warning.Kind == ReportScaleWarningDirtyEntries &&
			warning.Retained == len(manifest.RepositoryState.Dirty)
	}
	if !found {
		t.Fatalf("manifest scale warnings = %#v", warnings)
	}
}

func validRunManifestFixture(t *testing.T) RunManifest {
	t.Helper()
	repository := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: "/repo",
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
	digest, err := repository.Digest()
	if err != nil {
		t.Fatal(err)
	}
	inputs := []freshness.CapturedInput{{
		Version:       freshness.CapturedInputVersion,
		ID:            strings.Repeat("c", 64),
		Path:          "batch.go",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("d", 64),
		Stages:        []string{"report_evidence"},
	}}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		t.Fatal(err)
	}
	return RunManifest{
		Version:               CurrentRunManifestVersion,
		RepositoryState:       repository,
		AnalysisRoot:          "/repo",
		RepositoryStateSHA256: digest,
		SnapshotSHA256:        strings.Repeat("e", 64),
		ReportSHA256:          strings.Repeat("b", 64),
		ReportFormatVersion:   CurrentFormatVersion,
		OpenablePaths:         []string{"batch.go"},
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		MaterialInputs: MaterialInputs{
			SelectedRevision:             repository.Head,
			ProgramTargetID:              "pt-fixture",
			ProgramTargetSHA256:          strings.Repeat("8", 64),
			ProgramIndexSetSHA256:        strings.Repeat("9", 64),
			ProgramPagePortfolioSHA256:   strings.Repeat("3", 64),
			TargetOutcomePortfolioSHA256: strings.Repeat("4", 64),
			DependencyCatalogSHA256:      strings.Repeat("5", 64),
			ReducedDocumentationSHA256:   strings.Repeat("6", 64),
			GroupsIndexSHA256:            strings.Repeat("7", 64),
			InputPolicyVersion:           "captured-inputs-v1",
			ReportContract:               CurrentFormatVersion,
		},
	}
}

func TestRunManifestStandaloneSourceAuthorityIsClosedAndCanonical(t *testing.T) {
	manifest := validRunManifestFixture(t)
	manifest.StandaloneSource = &StandaloneSourceAuthority{
		Host:          "GitHub",
		RepositoryURL: "https://github.com/example/project",
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("canonical standalone source authority: %v", err)
	}

	noncanonical := manifest
	noncanonical.StandaloneSource = &StandaloneSourceAuthority{
		Host:          "GitHub",
		RepositoryURL: "https://github.com/example/project/",
	}
	if err := noncanonical.Validate(); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical standalone source authority error = %v", err)
	}

	unknownHost := manifest
	unknownHost.StandaloneSource = &StandaloneSourceAuthority{
		Host:          "Bitbucket",
		RepositoryURL: "https://bitbucket.org/example/project",
	}
	if err := unknownHost.Validate(); err == nil || !strings.Contains(err.Error(), "host is invalid") {
		t.Fatalf("unknown standalone source host error = %v", err)
	}
}

func newRunManifestRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	writeTestFile(t, repository, "batch.go", "package fixture\n\nfunc Commit() {}\n")
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "batch.go")
	runManifestGit(t, repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	state := captureRunManifestRepositoryState(t, repository)
	return state.Identity
}

func captureRunManifestRepositoryState(t *testing.T, repository string) freshness.RepositoryState {
	t.Helper()
	repositoryCorpus, err := corpus.Open(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer repositoryCorpus.Close()
	state, err := freshness.CaptureRepository(
		context.Background(), repository, repositoryCorpus,
	)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func writeRunManifestMetadata(t *testing.T, runDir, repository string) {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"repo_name": "manifest-fixture",
		"repo_path": repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runManifestGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestFile(%s): %v", name, err)
	}
}

func writeTargetPageManifestArtifact(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdirAll(%s): %v", path, err)
	}
}
