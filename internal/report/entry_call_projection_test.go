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
	"github.com/dvordrova/repomap/internal/snapshot"
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

func TestLoadEntryCallReportProjectionRestoresSeparateSemanticSurfaces(t *testing.T) {
	runDir := t.TempDir()
	target := reportAnalysisTargetFixture(t)
	repositorySHA256 := strings.Repeat("c", 64)
	writeEntryCallReportArtifacts(
		t, runDir, target, repositorySHA256,
		func(result *entrycall.Result) { appendEntryCallReportSurfaceFixtures(result, target) },
		nil,
	)

	data := &ReportData{
		AnalysisTarget: target,
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "local-route-kept-separate", Kind: "http_route",
		}}},
	}
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err != nil {
		t.Fatalf("loadEntryCallReportProjection: %v", err)
	}
	collectOpenablePaths(data)
	if err := validateEntryCallReportProjection(target, data.EntryCall, data.OpenablePaths); err != nil {
		t.Fatalf("validateEntryCallReportProjection: %v", err)
	}
	if data.EntryCall == nil || data.EntryCall.Version != 2 || len(data.EntryCall.Surfaces) != 2 ||
		data.EntryCall.SurfaceCoverage.SelectedProposals != 2 ||
		data.EntryCall.SurfaceCoverage.AdvertisedCandidates != 2 ||
		len(data.DiscoveredSurfaces.Triggers) != 1 {
		t.Fatalf("separate surface projection/catalog = %#v / %#v", data.EntryCall, data.DiscoveredSurfaces)
	}
	httpSurface := data.EntryCall.Surfaces[1]
	cliSurface := data.EntryCall.Surfaces[0]
	if httpSurface.Kind != entrycall.SurfaceKindHTTPRoute || httpSurface.Method == nil ||
		httpSurface.Method.Text != "GET" || httpSurface.Path == nil || httpSurface.Path.Text != "/account/:id" ||
		httpSurface.Handler == nil || httpSurface.Handler.Text != "fixture.getAccount" ||
		httpSurface.Site.Line != target.Roots[0].Line+2 ||
		httpSurface.State != EntryCallSurfaceStateExactRegistration ||
		httpSurface.Origin != EntryCallSurfaceOriginModelAssisted {
		t.Fatalf("HTTP surface = %#v", httpSurface)
	}
	if cliSurface.Kind != entrycall.SurfaceKindCLICommand || cliSurface.Identity == nil ||
		cliSurface.Identity.Text != "serve   [flags]" || cliSurface.Handler == nil ||
		cliSurface.Handler.Text != "fixture.runServe" ||
		cliSurface.State != EntryCallSurfaceStateDeclaredDescriptor ||
		cliSurface.RuntimeReachability != EntryCallSurfaceRuntimeReachabilityUnknown {
		t.Fatalf("CLI surface = %#v", cliSurface)
	}
	wire, err := json.Marshal(data.EntryCall)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"candidate_ref"`) || strings.Contains(string(wire), `"root_ref"`) ||
		strings.Contains(string(wire), "c1") || strings.Contains(string(wire), "r1") {
		t.Fatalf("request-local refs leaked into report projection: %s", wire)
	}
}

func TestLoadEntryCallReportProjectionFallsBackToExactLegacyV2Pair(t *testing.T) {
	runDir := t.TempDir()
	target := reportAnalysisTargetFixture(t)
	repositorySHA256 := strings.Repeat("c", 64)
	writeLegacyV2EntryCallReportArtifacts(t, runDir, target, repositorySHA256)

	data := &ReportData{AnalysisTarget: target}
	if err := loadEntryCallReportProjection(runDir, data, repositorySHA256); err != nil {
		t.Fatalf("load legacy v2 projection: %v", err)
	}
	if data.EntryCall == nil || data.EntryCall.Version != EntryCallReportProjectionVersion ||
		len(data.EntryCall.Families) != 1 || data.EntryCall.Surfaces == nil ||
		len(data.EntryCall.Surfaces) != 0 || data.EntryCall.SurfaceCoverage != (EntryCallReportSurfaceCoverage{}) {
		t.Fatalf("legacy v2 current projection = %#v", data.EntryCall)
	}

	// A current partial pair must fail closed rather than silently mixing or
	// downgrading to the complete legacy pair.
	currentStatus, _ := entryCallReportArtifactBytes(t, target, repositorySHA256, nil, nil)
	writeEntryCallArtifact(t, runDir, entrycall.StatusArtifactFilename, currentStatus)
	err := loadEntryCallReportProjection(runDir, &ReportData{AnalysisTarget: target}, repositorySHA256)
	if err == nil || !strings.Contains(err.Error(), "accepted status requires result") {
		t.Fatalf("current/legacy mixed-pair error = %v", err)
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
	report.EntryCall.Surfaces[1].Path.Text = "/tampered"
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

func TestGenerateAuthorizedPublishesAcceptedEntryCallAfterSnapshotCredentialRedaction(t *testing.T) {
	fixture := newTargetPageManifestFixture(t)
	var target *analysistarget.Target
	for _, projection := range fixture.container.Targets {
		if projection.Target.Ref == fixture.appRef {
			owned := projection.Target.Snapshot()
			target = &owned
			break
		}
	}
	if target == nil {
		t.Fatal("fixture target is absent from container")
	}

	repository := newRunManifestRepository(t)
	if err := os.MkdirAll(filepath.Join(repository, filepath.Dir(target.Roots[0].Path)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repository, target.Roots[0].Path, "package main\n\nfunc main() {}\n")
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
	const redactedSnapshot = "[redacted: credential assignment detected]\n"
	writeTestFile(t, runDir, "snapshot.json", redactedSnapshot)
	writeTargetPageManifestArtifact(
		t,
		runDir,
		snapshot.TargetRunContainerArtifactFilename,
		fixture.singleContainerRaw,
	)
	metadata, err := json.Marshal(map[string]any{
		"repo_name":                    "gitlab.com/formatic/syn",
		"analysis_target_ref":          target.Ref,
		"analysis_target_kind":         target.Kind,
		"analysis_target_module":       target.ModulePath,
		"analysis_target_display_path": target.DisplayPath(),
		"analysis_target_package":      target.PackagePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "metadata.json", string(metadata))
	writeEntryCallReportArtifacts(t, runDir, target, repositorySHA256, nil, nil)

	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted ReportData
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.RepoName != "gitlab.com/formatic/syn" ||
		persisted.AnalysisTarget == nil || persisted.AnalysisTarget.Ref != target.Ref ||
		persisted.EntryCall == nil || len(persisted.EntryCall.Families) != 1 {
		t.Fatalf(
			"restored repo/target/entry-call = %q / %#v / %#v",
			persisted.RepoName, persisted.AnalysisTarget, persisted.EntryCall,
		)
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyEntryCallArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyEntryCallArtifacts: %v", err)
	}
	snapshotRaw, err := os.ReadFile(filepath.Join(runDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(snapshotRaw) != redactedSnapshot {
		t.Fatalf("mandatory snapshot redaction changed: %q", snapshotRaw)
	}
	var driftedMetadata map[string]any
	if err := json.Unmarshal(metadata, &driftedMetadata); err != nil {
		t.Fatal(err)
	}
	driftedMetadata["analysis_target_package"] = target.PackagePath + "/other"
	driftedRaw, err := json.Marshal(driftedMetadata)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "metadata.json", string(driftedRaw))
	if _, err := ReadRunDir(runDir); err == nil ||
		!strings.Contains(err.Error(), "metadata/container binding mismatch") {
		t.Fatalf("metadata/container drift error = %v", err)
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
	writeEntryCallReportArtifacts(
		t, runDir, target, repositorySHA256,
		func(result *entrycall.Result) { appendEntryCallReportSurfaceFixtures(result, target) },
		nil,
	)
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
		SurfaceProposals:         []entrycall.ResultSurfaceProposal{},
		RejectedSurfaceProposals: []entrycall.RejectedSurfaceProposal{},
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
		AdvertisedSurfaceCandidates: result.SurfaceCandidateCoverage.AdvertisedCandidates,
		SelectedSurfaces:            result.SelectedSurfaceCount(),
		RejectedSurfaces:            result.RejectedSurfaceCount(),
		SurfaceCandidateCoverage:    result.SurfaceCandidateCoverage,
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

func writeLegacyV2EntryCallReportArtifacts(
	t *testing.T,
	runDir string,
	target *analysistarget.Target,
	repositorySHA256 string,
) {
	t.Helper()
	_, currentResultRaw := entryCallReportArtifactBytes(t, target, repositorySHA256, nil, nil)
	currentResult, err := entrycall.DecodeResult(currentResultRaw)
	if err != nil {
		t.Fatal(err)
	}
	const legacyPromptVersion = "entry-call-compression-prompt-c53df8bbacc9"
	legacyResult := struct {
		Version               int                     `json:"version"`
		PromptVersion         string                  `json:"prompt_version"`
		RequestRef            string                  `json:"request_ref"`
		RequestSHA256         string                  `json:"request_sha256"`
		SubstrateSHA256       string                  `json:"substrate_sha256"`
		RepositoryStateSHA256 string                  `json:"repository_state_sha256,omitempty"`
		Entries               []entrycall.ResultEntry `json:"entries"`
	}{
		Version: 2, PromptVersion: legacyPromptVersion,
		RequestRef: currentResult.RequestRef, RequestSHA256: currentResult.RequestSHA256,
		SubstrateSHA256:       currentResult.SubstrateSHA256,
		RepositoryStateSHA256: currentResult.RepositoryStateSHA256,
		Entries:               currentResult.Entries,
	}
	legacyResultRaw, err := json.Marshal(legacyResult)
	if err != nil {
		t.Fatal(err)
	}
	legacyStatus := struct {
		Version               int                   `json:"version"`
		State                 entrycall.StatusState `json:"state"`
		PromptVersion         string                `json:"prompt_version"`
		RequestRef            string                `json:"request_ref"`
		RequestSHA256         string                `json:"request_sha256"`
		SubstrateSHA256       string                `json:"substrate_sha256"`
		RepositoryStateSHA256 string                `json:"repository_state_sha256"`
		ResultSHA256          string                `json:"result_sha256"`
		AdvertisedFamilies    int                   `json:"advertised_families"`
		SelectedFamilies      int                   `json:"selected_families"`
		RejectedFamilies      int                   `json:"rejected_families"`
	}{
		Version: 2, State: entrycall.StatusAccepted, PromptVersion: legacyPromptVersion,
		RequestRef: currentResult.RequestRef, RequestSHA256: currentResult.RequestSHA256,
		SubstrateSHA256:       currentResult.SubstrateSHA256,
		RepositoryStateSHA256: currentResult.RepositoryStateSHA256,
		ResultSHA256:          manifestSHA256(legacyResultRaw),
		AdvertisedFamilies:    1,
		SelectedFamilies:      currentResult.SelectedFamilyCount(),
		RejectedFamilies:      currentResult.RejectedFamilyCount(),
	}
	legacyStatusRaw, err := json.Marshal(legacyStatus)
	if err != nil {
		t.Fatal(err)
	}
	writeEntryCallArtifact(t, runDir, entrycall.LegacyV2ResultArtifactFilename, legacyResultRaw)
	writeEntryCallArtifact(t, runDir, entrycall.LegacyV2StatusArtifactFilename, legacyStatusRaw)
}

func appendEntryCallReportSurfaceFixtures(
	result *entrycall.Result,
	target *analysistarget.Target,
) {
	root := target.Roots[0]
	value := func(kind entrycall.SurfaceFactKind, text string, line int) *entrycall.ResultSurfaceValue {
		location := entrycall.Location{Path: root.Path, Line: line, Column: 2}
		return &entrycall.ResultSurfaceValue{Kind: kind, Text: text, Location: &location}
	}
	result.SurfaceProposals = []entrycall.ResultSurfaceProposal{
		{
			ID: "model-surface-111111111111111111111111", CandidateRef: "c1", RootRef: "r1",
			Kind: entrycall.SurfaceKindHTTPRoute, Role: entrycall.SurfaceRoleEntrySurface,
			Form:    entrycall.SurfaceCandidateDirectCall,
			Site:    entrycall.Location{Path: root.Path, Line: root.Line + 2, Column: 2},
			Method:  value(entrycall.SurfaceFactToken, "GET", root.Line+2),
			Path:    value(entrycall.SurfaceFactString, "/account/:id", root.Line+2),
			Handler: value(entrycall.SurfaceFactCallable, "fixture.getAccount", root.Line+3),
		},
		{
			ID: "model-surface-222222222222222222222222", CandidateRef: "c2", RootRef: "r1",
			Kind: entrycall.SurfaceKindCLICommand, Role: entrycall.SurfaceRoleDescriptor,
			Form:     entrycall.SurfaceCandidateKeyedComposite,
			Site:     entrycall.Location{Path: root.Path, Line: root.Line + 4, Column: 2},
			Identity: value(entrycall.SurfaceFactString, "serve   [flags]", root.Line+4),
			Handler:  value(entrycall.SurfaceFactCallable, "fixture.runServe", root.Line+5),
		},
	}
	result.SurfaceCandidateCoverage = entrycall.SurfaceCandidateCoverage{
		ConsideredCandidates: 2, AdvertisedCandidates: 2,
		ConsideredFacts: 5, AdvertisedFacts: 5,
	}
}
