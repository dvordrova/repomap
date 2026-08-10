package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/freshness"
)

func TestLoadEntryCallReportProjectionProjectsAcceptedExactTarget(t *testing.T) {
	runDir := t.TempDir()
	target := reportAnalysisTargetFixture(t)
	repositorySHA256 := strings.Repeat("c", 64)
	writeEntryCallReportArtifacts(t, runDir, target, repositorySHA256, nil, nil)

	data := &ReportData{AnalysisTarget: target}
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err != nil {
		t.Fatalf("loadEntryCallReportProjection: %v", err)
	}
	collectOpenablePaths(data)
	if data.EntryCall == nil || data.EntryCall.Version != EntryCallReportProjectionVersion ||
		len(data.EntryCall.Families) != 1 {
		t.Fatalf("entry call projection = %#v", data.EntryCall)
	}
	family := data.EntryCall.Families[0]
	if family.RootDeclaration.Path != target.Roots[0].Path ||
		family.RootDeclaration.Line != target.Roots[0].Line ||
		family.CallerLabel != "main · main" || family.CalleeLabel != "product · run" ||
		family.Invocation != entrycall.InvocationSynchronous || family.WitnessCount != 2 ||
		len(family.Callsites) != 1 || len(data.OpenablePaths) != 1 ||
		data.OpenablePaths[0] != family.Callsites[0].Path {
		t.Fatalf("entry call family/openable paths = %#v / %#v", family, data.OpenablePaths)
	}
	if data.entryCallArtifactBinding == nil || data.entryCallArtifactBinding.statusSHA256 == "" ||
		data.entryCallArtifactBinding.resultSHA256 == "" {
		t.Fatalf("entry call artifact binding = %#v", data.entryCallArtifactBinding)
	}
}

func TestLoadEntryCallReportProjectionFailsClosedOnAcceptedArtifactDrift(t *testing.T) {
	target := reportAnalysisTargetFixture(t)
	repositorySHA256 := strings.Repeat("c", 64)
	tests := []struct {
		name         string
		mutateResult func(*entrycall.Result)
		mutateStatus func(*entrycall.Status)
		expectedSHA  string
		removeResult bool
		want         string
	}{
		{
			name: "root outside target",
			mutateResult: func(result *entrycall.Result) {
				result.Entries[0].Declaration.Path = "cmd/other/main.go"
			},
			want: "outside analysis target",
		},
		{
			name: "selected count mismatch",
			mutateStatus: func(status *entrycall.Status) {
				status.SelectedFamilies = 0
			},
			want: "family counts mismatch",
		},
		{
			name:        "repository mismatch",
			expectedSHA: strings.Repeat("d", 64),
			want:        "repository state binding mismatch",
		},
		{
			name:         "missing accepted result",
			removeResult: true,
			want:         "accepted status requires result",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := t.TempDir()
			writeEntryCallReportArtifacts(
				t, runDir, target, repositorySHA256, test.mutateResult, test.mutateStatus,
			)
			if test.removeResult {
				if err := os.Remove(filepath.Join(runDir, entrycall.ResultArtifactFilename)); err != nil {
					t.Fatal(err)
				}
			}
			expectedSHA := test.expectedSHA
			if expectedSHA == "" {
				expectedSHA = repositorySHA256
			}
			err := loadEntryCallReportProjection(runDir, &ReportData{AnalysisTarget: target}, expectedSHA)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadEntryCallReportProjectionOmitsClosedStatus(t *testing.T) {
	runDir := t.TempDir()
	repositorySHA256 := strings.Repeat("c", 64)
	status := entrycall.Status{
		Version: entrycall.StatusVersion, State: entrycall.StatusRejected,
		Reason: entrycall.ReasonResponseRejected, PromptVersion: entrycall.PromptVersion,
		RequestRef: "q-0123456789abcdef", RequestSHA256: strings.Repeat("a", 64),
		SubstrateSHA256: strings.Repeat("b", 64), RepositoryStateSHA256: repositorySHA256,
		AdvertisedFamilies: 1,
	}
	statusRaw, err := entrycall.EncodeStatus(status)
	if err != nil {
		t.Fatal(err)
	}
	writeEntryCallArtifact(t, runDir, entrycall.StatusArtifactFilename, statusRaw)
	data := &ReportData{AnalysisTarget: reportAnalysisTargetFixture(t)}
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err != nil {
		t.Fatalf("load rejected status: %v", err)
	}
	if data.EntryCall != nil || data.entryCallArtifactBinding != nil {
		t.Fatalf("closed status leaked projection/binding = %#v / %#v", data.EntryCall, data.entryCallArtifactBinding)
	}

	_, resultRaw := entryCallReportArtifactBytes(t, data.AnalysisTarget, repositorySHA256, nil, nil)
	writeEntryCallArtifact(t, runDir, entrycall.ResultArtifactFilename, resultRaw)
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err == nil ||
		!strings.Contains(err.Error(), "unexpected result") {
		t.Fatalf("closed status with result error = %v", err)
	}
}

func TestRunManifestBindsAndReplaysEntryCallProjection(t *testing.T) {
	manifest, reportJSON, runDir := authorizedEntryCallReportFixture(t)
	if manifest.MaterialInputs.EntryCallStatusSHA256 == "" ||
		manifest.MaterialInputs.EntryCallResultSHA256 == "" {
		t.Fatalf("entry call material = %#v", manifest.MaterialInputs)
	}
	if err := manifest.VerifyEntryCallArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyEntryCallArtifacts: %v", err)
	}

	var report ReportData
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		t.Fatal(err)
	}
	report.EntryCall.Families[0].CallerLabel = "main · tampered"
	tampered, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyEntryCallArtifacts(runDir, tampered); err == nil ||
		!strings.Contains(err.Error(), "do not match report") {
		t.Fatalf("tampered projection error = %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(runDir, entrycall.ResultArtifactFilename),
		append([]byte(nil), reportJSON...),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyEntryCallArtifacts(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered result error = %v", err)
	}
}

func authorizedEntryCallReportFixture(t *testing.T) (RunManifest, []byte, string) {
	t.Helper()
	repository := newRunManifestRepository(t)
	if err := os.MkdirAll(filepath.Join(repository, "cmd/product"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repository, "cmd/product/main.go", "package main\n\nfunc main() {}\n")
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, state, state)
	if err != nil {
		t.Fatal(err)
	}
	repositorySHA256, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	target := reportAnalysisTargetFixture(t)
	writeRunManifestSnapshot(t, runDir, "fixture")
	writeEntryCallReportArtifacts(t, runDir, target, repositorySHA256, nil, nil)
	data := &ReportData{
		FormatVersion:  CurrentFormatVersion,
		RepoName:       "fixture",
		AnalysisTarget: target,
	}
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err != nil {
		t.Fatal(err)
	}
	collectOpenablePaths(data)
	reportPath := filepath.Join(runDir, "report.json")
	if err := WriteReportJSON(data, reportPath); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAuthorizedRunManifest(runDir, data, reportJSON, authority); err != nil {
		t.Fatalf("writeAuthorizedRunManifest: %v", err)
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	return manifest, reportJSON, runDir
}

func writeEntryCallReportArtifacts(
	t *testing.T,
	runDir string,
	target *analysistarget.Target,
	repositorySHA256 string,
	mutateResult func(*entrycall.Result),
	mutateStatus func(*entrycall.Status),
) {
	t.Helper()
	statusRaw, resultRaw := entryCallReportArtifactBytes(
		t, target, repositorySHA256, mutateResult, mutateStatus,
	)
	writeEntryCallArtifact(t, runDir, entrycall.ResultArtifactFilename, resultRaw)
	writeEntryCallArtifact(t, runDir, entrycall.StatusArtifactFilename, statusRaw)
}

func entryCallReportArtifactBytes(
	t *testing.T,
	target *analysistarget.Target,
	repositorySHA256 string,
	mutateResult func(*entrycall.Result),
	mutateStatus func(*entrycall.Status),
) ([]byte, []byte) {
	t.Helper()
	result := entrycall.Result{
		Version: entrycall.ResultVersion, PromptVersion: entrycall.PromptVersion,
		RequestRef: "q-0123456789abcdef", RequestSHA256: strings.Repeat("a", 64),
		SubstrateSHA256: strings.Repeat("b", 64), RepositoryStateSHA256: repositorySHA256,
		Entries: []entrycall.ResultEntry{{
			RootRef: "r1", Label: "main · main",
			Declaration: entrycall.Location{Path: target.Roots[0].Path, Line: target.Roots[0].Line, Column: 1},
			Families: []entrycall.ResultFamily{{
				Ref: "f1", CallerLabel: "main · main", CalleeLabel: "product · run",
				Invocation: entrycall.InvocationSynchronous, WitnessCount: 2,
				Callsites: []entrycall.Location{{Path: target.Roots[0].Path, Line: target.Roots[0].Line + 1, Column: 2}},
			}},
			RejectedFamilies: []entrycall.RejectedResultFamily{},
			Frontier:         []entrycall.RequestFrontier{},
		}},
	}
	if mutateResult != nil {
		mutateResult(&result)
	}
	resultRaw, err := entrycall.EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	status := entrycall.Status{
		Version: entrycall.StatusVersion, State: entrycall.StatusAccepted,
		PromptVersion: entrycall.PromptVersion,
		RequestRef:    result.RequestRef, RequestSHA256: result.RequestSHA256,
		SubstrateSHA256: result.SubstrateSHA256, RepositoryStateSHA256: result.RepositoryStateSHA256,
		ResultSHA256: manifestSHA256(resultRaw), AdvertisedFamilies: 1,
		SelectedFamilies: result.SelectedFamilyCount(), RejectedFamilies: result.RejectedFamilyCount(),
	}
	if mutateStatus != nil {
		mutateStatus(&status)
	}
	statusRaw, err := entrycall.EncodeStatus(status)
	if err != nil {
		t.Fatalf("EncodeStatus: %v", err)
	}
	return statusRaw, resultRaw
}

func writeEntryCallArtifact(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
