package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

func TestGenerateWritesVerifiedRunManifestAndRejectsReportTampering(t *testing.T) {
	repository := newRunManifestRepository(t)
	initialState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	writeTestFile(t, runDir, "snapshot.json", `{"repo_name":"manifest-fixture"}`)
	writeRunManifestMetadata(t, runDir, repository)
	writeTestFile(t, runDir, "llm_bundle.json", `{
		"allowed_paths":["batch.go"],
		"source_signals":[{"path":"batch.go","line":3,"category":"request_handler","snippet":"func Commit() {}","reason":"fixture"}]
	}`)
	writeTestFile(t, runDir, "orientation_report.json", `{
		"project_guess":"batch fixture",
		"high_level_map":[{
			"name":"Batch Operations",
			"evidence":["batch.go:3 defines Commit"],
			"why_it_matters":"groups writes"
		}],
		"candidate_flows":[{
			"name":"Write Batch",
			"trigger":"Commit is called",
			"likely_entrypoint":"batch.go",
			"likely_files":["batch.go"],
			"why_interesting":"primary write path",
			"evidence":["batch.go:3 defines Commit"],
			"confidence":0.9
		}],
		"warnings":[]
	}`)

	currentState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initialState, currentState)
	if err != nil {
		t.Fatalf("ConfirmRunAuthority: %v", err)
	}
	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	if manifest.Version != CurrentRunManifestVersion || manifest.ReportFormatVersion != CurrentFormatVersion {
		t.Fatalf("manifest versions = %d/%d", manifest.Version, manifest.ReportFormatVersion)
	}
	if manifest.RepositoryState.Identity != repository {
		t.Fatalf("repository identity = %q, want %q", manifest.RepositoryState.Identity, repository)
	}
	if manifest.AnalysisRoot != repository {
		t.Fatalf("analysis root = %q, want %q", manifest.AnalysisRoot, repository)
	}
	if resolved, err := manifest.ResolveAnalysisRoot(); err != nil || resolved != repository {
		t.Fatalf("ResolveAnalysisRoot() = %q, %v, want %q", resolved, err, repository)
	}
	if err := manifest.VerifyRepositoryState(manifest.RepositoryState); err != nil {
		t.Fatalf("VerifyRepositoryState: %v", err)
	}
	if len(manifest.OpenablePaths) != 1 || manifest.OpenablePaths[0] != "batch.go" {
		t.Fatalf("openable paths = %#v", manifest.OpenablePaths)
	}
	if len(manifest.Components) != 1 || len(manifest.Components[0].Anchors) != 1 {
		t.Fatalf("component authority = %#v", manifest.Components)
	}
	component := manifest.Components[0]
	anchor := component.Anchors[0]
	if len(component.RelatedFlowIDs) != 1 || component.RelatedFlowIDs[0] != "write-batch" {
		t.Fatalf("related flow ids = %#v", component.RelatedFlowIDs)
	}
	if anchor.Path != "batch.go" || !anchor.CanListSymbols || len(anchor.AllowedLines) != 1 || anchor.AllowedLines[0] != 3 {
		t.Fatalf("anchor authority = %#v", anchor)
	}
	info, err := os.Stat(filepath.Join(runDir, RunManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest permissions = %o, want 600", info.Mode().Perm())
	}
	writeTestFile(t, repository, "batch.go", "package fixture\n\nfunc Commit() { panic(\"changed\") }\n")
	currentRepository, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRepositoryState(currentRepository); err == nil || !strings.Contains(err.Error(), "analyzed inputs are partially_stale") {
		t.Fatalf("VerifyRepositoryState after source change error = %v", err)
	}

	reportPath := filepath.Join(runDir, "report.json")
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(reportJSON, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRunManifest(runDir); err == nil || !strings.Contains(err.Error(), "report sha256 mismatch") {
		t.Fatalf("ReadRunManifest after report tamper error = %v", err)
	}
}

func TestGenerateWithoutConfirmedAuthorityLeavesRunViewOnly(t *testing.T) {
	runDir := t.TempDir()
	writeTestFile(t, runDir, "snapshot.json", `{"repo_name":"legacy"}`)
	writeTestFile(t, runDir, "metadata.json", `{"repo_path":"../relative/repository"}`)
	writeTestFile(t, runDir, "orientation_report.json", `{"project_guess":"legacy report","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, runDir, RunManifestFilename, `{"stale":true}`)

	if err := Generate(runDir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, RunManifestFilename)); !os.IsNotExist(err) {
		t.Fatalf("view-only run manifest stat error = %v, want not exist", err)
	}
}

func TestConfirmRunAuthorityRejectsRepositoryChange(t *testing.T) {
	repository := newRunManifestRepository(t)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repository, "batch.go", "package fixture\n\nfunc Commit() { panic(\"changed\") }\n")
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmRunAuthority(repository, initial, current); err == nil || !strings.Contains(err.Error(), "repository changed during orientation") {
		t.Fatalf("ConfirmRunAuthority error = %v", err)
	}
}

func TestConfirmRunAuthorityPreservesSubdirectoryAnalysisRoot(t *testing.T) {
	repository := newRunManifestRepository(t)
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := freshness.CaptureRepository(context.Background(), analysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(analysisRoot, state, state)
	if err != nil {
		t.Fatalf("ConfirmRunAuthority: %v", err)
	}
	if authority.analysisRoot != analysisRoot {
		t.Fatalf("analysis root = %q, want %q", authority.analysisRoot, analysisRoot)
	}
	if authority.repository.Identity != repository {
		t.Fatalf("repository identity = %q, want %q", authority.repository.Identity, repository)
	}
}

func TestRunManifestValidateRejectsUnsafeOrAmbiguousAuthority(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RunManifest)
		want   string
	}{
		{
			name: "invalid openable path",
			mutate: func(manifest *RunManifest) {
				manifest.OpenablePaths[0] = "../secret"
			},
			want: "clean repository-relative slash path",
		},
		{
			name: "duplicate component id",
			mutate: func(manifest *RunManifest) {
				duplicate := manifest.Components[0]
				duplicate.Anchors = nil
				manifest.Components = append(manifest.Components, duplicate)
			},
			want: "duplicate component id",
		},
		{
			name: "duplicate anchor id",
			mutate: func(manifest *RunManifest) {
				manifest.OpenablePaths = append(manifest.OpenablePaths, "wal/wal.go")
				manifest.Components = append(manifest.Components, ComponentAuthority{
					ID: "component-wal",
					Anchors: []AnchorAuthority{{
						ID:             manifest.Components[0].Anchors[0].ID,
						Path:           "wal/wal.go",
						AllowedLines:   []int{12},
						CanListSymbols: true,
					}},
				})
			},
			want: "duplicate anchor id",
		},
		{
			name: "analysis root outside repository",
			mutate: func(manifest *RunManifest) {
				manifest.AnalysisRoot = "/other/repository"
			},
			want: "must be inside repository identity",
		},
		{
			name: "repository digest mismatch",
			mutate: func(manifest *RunManifest) {
				manifest.RepositoryStateSHA256 = strings.Repeat("0", 64)
			},
			want: "repository state sha256 mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validRunManifestFixture(t)
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunManifestSourceCatalogPreservesCurrentSourceScopeAndJSON(t *testing.T) {
	t.Parallel()

	manifest := validRunManifestFixture(t)
	before, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := manifest.SourceCatalog()
	if err != nil {
		t.Fatalf("SourceCatalog: %v", err)
	}
	if catalog.AnalysisRoot() != manifest.AnalysisRoot {
		t.Fatalf("catalog analysis root = %q, want %q", catalog.AnalysisRoot(), manifest.AnalysisRoot)
	}
	if got := catalog.Paths(); len(got) != 1 || got[0] != manifest.OpenablePaths[0] {
		t.Fatalf("catalog paths = %#v, want %#v", got, manifest.OpenablePaths)
	}
	source, ok := catalog.Lookup("batch.go")
	if !ok || source.RepositoryPath != manifest.CapturedInputs[0].Path ||
		source.ContentSHA256 != manifest.CapturedInputs[0].ContentSHA256 {
		t.Fatalf("catalog source = %#v, %t; input = %#v", source, ok, manifest.CapturedInputs[0])
	}
	after, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("SourceCatalog changed manifest JSON:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRunManifestWorkspaceSnapshotPreservesV3V4AuthorityAndJSON(t *testing.T) {
	t.Parallel()

	for _, version := range []int{3, CurrentRunManifestVersion} {
		t.Run(fmt.Sprintf("version %d", version), func(t *testing.T) {
			t.Parallel()

			manifest := validRunManifestFixture(t)
			manifest.Version = version
			before, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := manifest.WorkspaceSnapshot()
			if err != nil {
				t.Fatalf("WorkspaceSnapshot: %v", err)
			}
			if snapshot.RepositoryRoot() != manifest.RepositoryState.Identity ||
				snapshot.AnalysisRoot() != manifest.AnalysisRoot ||
				snapshot.Revision() != manifest.RepositoryState.Head ||
				snapshot.RepositoryDigest() != manifest.RepositoryStateSHA256 ||
				snapshot.CapturedInputsDigest() != manifest.CapturedInputsSHA256 {
				t.Fatalf("snapshot identity does not match manifest: %#v", snapshot)
			}
			wantCatalog, err := manifest.SourceCatalog()
			if err != nil {
				t.Fatalf("SourceCatalog: %v", err)
			}
			assertCatalogParity(t, snapshot.Catalog(), wantCatalog)

			for _, current := range []freshness.RepositoryState{
				manifest.RepositoryState,
				reportRepositoryWithDirty(
					manifest.RepositoryState,
					"notes.txt",
					strings.Repeat("e", 64),
				),
				reportRepositoryWithDirty(
					manifest.RepositoryState,
					"batch.go",
					strings.Repeat("f", 64),
				),
				func() freshness.RepositoryState {
					state := manifest.RepositoryState
					state.Identity = "/other"
					return state
				}(),
			} {
				want := manifest.CurrentFreshness(current)
				got := snapshot.Assess(current)
				want.ComparedAt = ""
				got.ComparedAt = ""
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("Assess parity:\n got: %#v\nwant: %#v", got, want)
				}
				wantAllowed := manifest.VerifyRepositoryState(current) == nil
				if gotAllowed := snapshot.Verify(current) == nil; gotAllowed != wantAllowed {
					t.Fatalf("Verify allowed = %t, want %t", gotAllowed, wantAllowed)
				}
			}

			after, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("WorkspaceSnapshot changed manifest JSON:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestRunManifestWorkspaceSnapshotExcludesReportBindings(t *testing.T) {
	t.Parallel()

	first := validRunManifestFixture(t)
	second := validRunManifestFixture(t)
	second.ReportSHA256 = strings.Repeat("e", 64)
	second.Components = nil
	second.Freshness = freshness.NewFreshnessResult(freshness.FreshnessUnrelatedChanges)

	firstSnapshot, err := first.WorkspaceSnapshot()
	if err != nil {
		t.Fatalf("first WorkspaceSnapshot: %v", err)
	}
	secondSnapshot, err := second.WorkspaceSnapshot()
	if err != nil {
		t.Fatalf("second WorkspaceSnapshot: %v", err)
	}
	if firstSnapshot.RepositoryDigest() != secondSnapshot.RepositoryDigest() ||
		firstSnapshot.CapturedInputsDigest() != secondSnapshot.CapturedInputsDigest() ||
		firstSnapshot.AnalysisRoot() != secondSnapshot.AnalysisRoot() ||
		!reflect.DeepEqual(firstSnapshot.Catalog().Paths(), secondSnapshot.Catalog().Paths()) {
		t.Fatalf("report-only fields changed neutral authority")
	}

	invalid := first
	invalid.ReportSHA256 = "invalid"
	if _, err := invalid.WorkspaceSnapshot(); err == nil || !strings.Contains(err.Error(), "report sha256 is invalid") {
		t.Fatalf("WorkspaceSnapshot validation error = %v", err)
	}
}

func TestRunManifestSourceCatalogPreservesSubdirectoryMapping(t *testing.T) {
	t.Parallel()

	manifest := validRunManifestFixture(t)
	manifest.AnalysisRoot = "/repo/service"
	manifest.CapturedInputs[0].Path = "service/batch.go"
	rootAlias := manifest.CapturedInputs[0]
	rootAlias.ID = strings.Repeat("d", 64)
	rootAlias.Path = "batch.go"
	rootAlias.ContentSHA256 = strings.Repeat("e", 64)
	manifest.CapturedInputs = append(manifest.CapturedInputs, rootAlias)
	inputsDigest, err := freshness.CapturedInputsDigest(manifest.CapturedInputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest.CapturedInputsSHA256 = inputsDigest

	catalog, err := manifest.SourceCatalog()
	if err != nil {
		t.Fatalf("SourceCatalog: %v", err)
	}
	source, ok := catalog.Lookup("batch.go")
	if !ok || source.Path != "batch.go" || source.RepositoryPath != "service/batch.go" ||
		source.ContentSHA256 != manifest.CapturedInputs[0].ContentSHA256 {
		t.Fatalf("subdirectory source = %#v, %t", source, ok)
	}
	snapshot, err := manifest.WorkspaceSnapshot()
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	assertCatalogParity(t, snapshot.Catalog(), catalog)
}

func TestRunManifestSourceCatalogDoesNotChangeLegacyValidation(t *testing.T) {
	t.Parallel()

	version3 := validRunManifestFixture(t)
	version3.Version = 3
	if _, err := version3.SourceCatalog(); err != nil {
		t.Fatalf("v3 SourceCatalog: %v", err)
	}

	version2 := validRunManifestFixture(t)
	version2.Version = 2
	version2.RepositoryState.Version = 1
	version2.CapturedInputs = nil
	version2.CapturedInputsSHA256 = ""
	version2.Freshness = freshness.FreshnessResult{}
	version2.MaterialInputs = MaterialInputs{}
	digest, err := version2.RepositoryState.Digest()
	if err != nil {
		t.Fatal(err)
	}
	version2.RepositoryStateSHA256 = digest
	encoded, err := json.Marshal(version2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRunManifest(encoded); err != nil {
		t.Fatalf("DecodeRunManifest(v2): %v", err)
	}
	if _, err := version2.SourceCatalog(); err == nil || !strings.Contains(err.Error(), "has no captured input") {
		t.Fatalf("v2 SourceCatalog error = %v", err)
	}
	if _, err := version2.WorkspaceSnapshot(); err == nil ||
		!strings.Contains(err.Error(), "workspace snapshot is unavailable for version 2") {
		t.Fatalf("v2 WorkspaceSnapshot error = %v", err)
	}
	if err := version2.VerifyRepositoryState(version2.RepositoryState); err != nil {
		t.Fatalf("v2 VerifyRepositoryState: %v", err)
	}
	if got := version2.CurrentFreshness(version2.RepositoryState).State; got != freshness.FreshnessLegacyUnknown {
		t.Fatalf("v2 CurrentFreshness = %s", got)
	}
}

func TestDecodeRunManifestRejectsUnknownFields(t *testing.T) {
	manifest := validRunManifestFixture(t)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)
	if _, err := DecodeRunManifest(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeRunManifest error = %v", err)
	}
}

func TestDecodeRunManifestReopensLegacyManifestWithUnknownFreshness(t *testing.T) {
	t.Parallel()

	manifest := validRunManifestFixture(t)
	manifest.Version = 2
	manifest.RepositoryState.Version = 1
	manifest.CapturedInputs = nil
	manifest.CapturedInputsSHA256 = ""
	manifest.Freshness = freshness.FreshnessResult{}
	manifest.MaterialInputs = MaterialInputs{}
	digest, err := manifest.RepositoryState.Digest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.RepositoryStateSHA256 = digest
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRunManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != 2 || decoded.CurrentFreshness(decoded.RepositoryState).State != freshness.FreshnessLegacyUnknown {
		t.Fatalf("legacy manifest = %#v", decoded)
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
		Version: freshness.CapturedInputVersion, ID: strings.Repeat("c", 64), Path: "batch.go",
		Kind: freshness.FileRegular, Mode: "100644", ContentSHA256: strings.Repeat("d", 64),
		Stages: []string{"report_evidence"},
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
		ReportSHA256:          strings.Repeat("b", 64),
		ReportFormatVersion:   CurrentFormatVersion,
		OpenablePaths:         []string{"batch.go"},
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		Freshness:             freshness.NewFreshnessResult(freshness.FreshnessFresh),
		MaterialInputs: MaterialInputs{
			SelectedRevision: repository.Head, InputPolicyVersion: "captured-inputs-v1",
			ArchitectureContract: 1, ReportContract: CurrentFormatVersion,
		},
		Components: []ComponentAuthority{{
			ID:             "component-batch",
			RelatedFlowIDs: []string{"write-batch"},
			Anchors: []AnchorAuthority{{
				ID:             "anchor-batch",
				Path:           "batch.go",
				AllowedLines:   []int{3},
				CanListSymbols: true,
			}},
		}},
	}
}

func assertCatalogParity(t *testing.T, got, want interface {
	AnalysisRoot() string
	Paths() []string
	Lookup(string) (sourcecatalog.Source, bool)
}) {
	t.Helper()
	if got.AnalysisRoot() != want.AnalysisRoot() || !reflect.DeepEqual(got.Paths(), want.Paths()) {
		t.Fatalf("catalog roots/paths differ: got=%q %#v want=%q %#v",
			got.AnalysisRoot(), got.Paths(), want.AnalysisRoot(), want.Paths())
	}
	for _, path := range want.Paths() {
		gotSource, gotOK := got.Lookup(path)
		wantSource, wantOK := want.Lookup(path)
		if gotOK != wantOK || !reflect.DeepEqual(gotSource, wantSource) {
			t.Fatalf("catalog source %q differs: got=%#v,%t want=%#v,%t",
				path, gotSource, gotOK, wantSource, wantOK)
		}
	}
}

func reportRepositoryWithDirty(
	repository freshness.RepositoryState,
	path, digest string,
) freshness.RepositoryState {
	repository.Dirty = []freshness.DirtyFile{{
		Status:        "modified",
		Path:          path,
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: digest,
	}}
	return repository
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
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	return state.Identity
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
