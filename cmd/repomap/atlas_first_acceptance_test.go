package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/studymap"
	"github.com/dvordrova/repomap/internal/themestudy"
)

type atlasFirstAcceptanceStage string

const (
	atlasFirstStageNavigator         atlasFirstAcceptanceStage = "navigator"
	atlasFirstStageArchitecture      atlasFirstAcceptanceStage = "architecture"
	atlasFirstStageStudyScout        atlasFirstAcceptanceStage = "theme_scout"
	atlasFirstStageStudyAdjudication atlasFirstAcceptanceStage = "theme_adjudication"
	atlasFirstStageStudy             atlasFirstAcceptanceStage = "atlas_study"
)

type atlasFirstAcceptanceProvider struct {
	t                      *testing.T
	repositoryType         atlasstudy.RepositoryType
	rejectArchitecture     bool
	rejectNavigator        bool
	failArchitectureCall   bool
	failNavigatorCall      bool
	lengthArchitectureCall bool
	includeBadStudySibling bool

	mu     sync.Mutex
	stages []atlasFirstAcceptanceStage
	bodies map[atlasFirstAcceptanceStage][][]byte
}

func TestRunDefaultAtlasFirstPublishesNavigatorArchitectureAndStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		includeBadStudySibling: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateSelected ||
		data.Navigator.Recommendation == nil {
		t.Fatalf("Navigator report = %#v, want selected recommendation", data.Navigator)
	}
	assertNavigatorAcceptanceSemanticMinimum(t, data)
	assertAtlasFirstNavigatorRequestArtifact(t, runDir, data)
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.PackageEdges) != 1 {
		t.Fatalf(
			"top-level Atlas-first exact package edges = %#v, want one preserved edge",
			data.RepositoryGraph,
		)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 4, map[string]string{
		debugdump.SemanticStageNavigator:    "accepted",
		debugdump.SemanticStageArchitecture: "accepted",
		debugdump.SemanticStageAtlasStudy:   "accepted_partial",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageNavigator,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
	assertAtlasFirstAcceptedArtifacts(t, runDir, true)
	assertAtlasFirstAcceptedManifest(t, manifest, true)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)
	assertNoLegacyOrientationWarning(t, data)

	scoutBytes, err := os.ReadFile(filepath.Join(runDir, themestudy.ScoutResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	scout, err := themestudy.DecodeScoutResult(scoutBytes)
	if err != nil {
		t.Fatal(err)
	}
	if scout.Version != themestudy.ScoutResultVersion || scout.State != "accepted_partial" ||
		scout.Status.Received != 2 || scout.Status.Accepted != 1 || scout.Status.Rejected != 1 ||
		len(scout.Status.Issues) != 1 || scout.Status.Issues[0].Code != themestudy.ScoutIssueUnknownRef {
		t.Fatalf("invalid sibling did not preserve the valid theme candidate: %#v", scout.Status)
	}
	themesBytes, err := os.ReadFile(filepath.Join(runDir, themestudy.StudyThemesArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	themes, err := themestudy.DecodeStudyThemes(themesBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(themes.Cards) == 0 {
		t.Fatalf("theme shelf = %#v, want at least one published card", themes)
	}
	// The Scout rejection is recorded in the Scout result (accepted_partial);
	// the published shelf is the surviving candidate. The report projection
	// carries the final partial state.
	if data.AtlasStudy.State != atlasstudy.ProductStateAcceptedPartial {
		t.Fatalf("study state = %q, want accepted_partial after a rejected Scout sibling", data.AtlasStudy.State)
	}
}

func assertAtlasFirstNavigatorRequestArtifact(
	t *testing.T,
	runDir string,
	data *report.ReportData,
) {
	t.Helper()
	if data == nil || data.RepositoryAtlas == nil || data.Navigator == nil ||
		data.Navigator.Recommendation == nil {
		t.Fatal("selected Navigator report substrate is absent")
	}
	raw, err := os.ReadFile(filepath.Join(runDir, navigator.RequestArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	record, err := navigator.DecodeRequestRecord(raw)
	if err != nil {
		t.Fatalf("DecodeRequestRecord: %v", err)
	}
	if err := navigator.ValidateRequestRecordAgainstAtlas(record, *data.RepositoryAtlas); err != nil {
		t.Fatalf("Navigator request does not match the exact persisted Atlas: %v", err)
	}
	found := false
	for _, action := range record.Actions {
		if action.Key == data.Navigator.Recommendation.Key {
			found = reflect.DeepEqual(action, *data.Navigator.Recommendation)
			break
		}
	}
	if !found {
		t.Fatalf("selected recommendation is absent from exact request artifact: %#v", record.Actions)
	}
}

func TestRunDefaultAtlasFirstEmptyNavigatorLibraryStillPublishesArchitectureAndStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_library")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryLibrary,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudyScout, atlasFirstStageStudyAdjudication)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateEmpty ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("library Navigator report = %#v, want explicit empty result", data.Navigator)
	}
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Packages) != 1 {
		t.Fatalf("library package graph = %#v, want exactly one package", data.RepositoryGraph)
	}
	if data.RepositoryAtlas == nil || len(data.RepositoryAtlas.Relations) != 0 {
		t.Fatalf("library Atlas relations = %#v, want no synthetic startup relation", data.RepositoryAtlas)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 3, map[string]string{
		debugdump.SemanticStageNavigator:    "empty",
		debugdump.SemanticStageArchitecture: "accepted",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
	assertAtlasFirstAcceptedArtifacts(t, runDir, false)
	assertAtlasFirstAcceptedManifest(t, manifest, false)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)
}

func TestRunDefaultAtlasFirstRejectedArchitectureKeepsLocalCanvasAndCallsStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_library")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		rejectArchitecture: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudyScout, atlasFirstStageStudyAdjudication)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		!data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.ProposalAccepted {
		t.Fatalf("rejected Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalAnchors &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages) ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("rejected enrichment erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 3, map[string]string{
		debugdump.SemanticStageNavigator:    "empty",
		debugdump.SemanticStageArchitecture: "rejected",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
	if _, err := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected Architecture persisted enrichment: %v", err)
	}
	for _, name := range []string{
		navigator.StatusArtifactFilename,
		navigator.RecordArtifactFilename,
		report.ArchitectureSynthesisStatusFile,
		themestudy.ScoutRequestArtifactFilename,
		themestudy.ScoutResultArtifactFilename,
		themestudy.ScoutStatusArtifactFilename,
		themestudy.ExpansionArtifactFilename,
		themestudy.AdjudicationRequestArtifactFilename,
		themestudy.AdjudicationResultArtifactFilename,
		themestudy.AdjudicationStatusArtifactFilename,
		themestudy.StudyThemesArtifactFilename,
		"report.json",
		"report.html",
		report.RunManifestFilename,
	} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("rejected enrichment run missing %s: %v", name, err)
		}
	}
	assertAtlasFirstAcceptedManifest(t, manifest, false)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)
}

func TestRunDefaultAtlasFirstNavigatorFailureDoesNotGateArchitectureOrStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		rejectNavigator: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateFailed ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("failed Navigator report = %#v", data.Navigator)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 4, map[string]string{
		debugdump.SemanticStageNavigator:    "failed",
		debugdump.SemanticStageArchitecture: "accepted",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageNavigator,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
}

func TestRunDefaultAtlasFirstNavigatorProviderFailureDoesNotGateArchitectureOrStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		failNavigatorCall: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateFailed ||
		data.Navigator.FailureCode != navigator.FailureProvider ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("provider-failed Navigator report = %#v", data.Navigator)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 4, map[string]string{
		debugdump.SemanticStageNavigator:    "failed",
		debugdump.SemanticStageArchitecture: "accepted",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
}

func TestRunDefaultAtlasFirstArchitectureProviderFailureKeepsLocalCanvas(t *testing.T) {
	repo := navigatorAcceptanceRepository(t)
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		failArchitectureCall: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ProposalAccepted ||
		data.ArchitectureSynthesis.ProposalRejected {
		t.Fatalf("provider-failed Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalAnchors &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages) ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("provider failure erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 4, map[string]string{
		debugdump.SemanticStageNavigator:    "accepted",
		debugdump.SemanticStageArchitecture: "failed",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
}

// TestRunDefaultAtlasFirstArchitectureOutputExhaustionPublishesLocalProduct
// is the Decision 215 continuation proof: an attempted Architecture provider
// output exhaustion (finish_reason=length with a partial repeated-ref
// response) records a durable failed status and accounting, keeps the partial
// response diagnostic-only, and the run continues through both D213 semantic
// stages and the published report bound to the canonical local Canvas.
func TestRunDefaultAtlasFirstArchitectureOutputExhaustionPublishesLocalProduct(t *testing.T) {
	repo := navigatorAcceptanceRepository(t)
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		lengthArchitectureCall: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit ||
		data.ArchitectureSynthesis.FinishReason != "length" ||
		data.ArchitectureSynthesis.ResponseComplete ||
		data.ArchitectureSynthesis.ConfiguredMaxTokens <= 0 ||
		data.ArchitectureSynthesis.ObservedOutputTokens <= 0 {
		t.Fatalf("output-exhausted Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureSynthesis.ProposalAccepted || data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.ProposalPartial || data.ArchitectureSynthesis.MembershipCounted ||
		data.ArchitectureSynthesis.ArchitectureSource != "" ||
		len(data.ArchitectureSynthesis.ValidationCodes) != 0 {
		t.Fatalf("output-exhausted Architecture status published partial response evidence: %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("output exhaustion erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 4, map[string]string{
		debugdump.SemanticStageNavigator:    "accepted",
		debugdump.SemanticStageArchitecture: "resource_exhausted",
		debugdump.SemanticStageAtlasStudy:   "accepted",
	})
	if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output exhaustion published a synthesis record: %v", err)
	}
	// The manifest binds the failed Architecture status alongside the local
	// Canvas.
	statusData, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatalf("failed Architecture status missing: %v", err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatal(err)
	}
	if status.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit {
		t.Fatalf("manifest-bound failed status = %#v", status)
	}
}

// TestEtcdArchitectureOutputExhaustionReplayPublishesCompleteReport is the
// Decision 215 etcd acceptance replay: the real CLI runs against the local
// etcd repository with a deterministic provider that serves Navigator, the
// exact etcd-shaped Architecture output exhaustion (one subsystem, one
// component, an open member_refs repeating a bounded package-ref block,
// finish_reason=length), and valid D213 Scout + Adjudication fixtures. The
// run must complete: failed Architecture status and accounting durable, no
// synthesis record, Study accepted, report + manifest published, and the
// canonical local Canvas bound.
func TestEtcdArchitectureOutputExhaustionReplayPublishesCompleteReport(t *testing.T) {
	repo := "/Users/dvordrova/git/etcd"
	if info, err := os.Stat(repo); err != nil || !info.IsDir() {
		t.Skipf("local etcd repository unavailable: %v", err)
	}
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryLibrary,
		lengthArchitectureCall: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageNavigator,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit ||
		data.ArchitectureSynthesis.FinishReason != "length" ||
		data.ArchitectureSynthesis.ResponseComplete ||
		data.ArchitectureSynthesis.ConfiguredMaxTokens != 64_000 ||
		data.ArchitectureSynthesis.ObservedOutputTokens != 64_000 {
		t.Fatalf("etcd output-exhausted Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("etcd output exhaustion erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	// Study must have executed (accepted or an honest zero-candidate failure);
	// it must not be absent from a run that continued past Architecture.
	if data.AtlasStudy == nil && data.StudyPublication == nil {
		t.Fatalf("etcd continuation did not execute Study")
	}
	if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("etcd output exhaustion published a synthesis record: %v", err)
	}
	// The run manifest binds the published report, which carries the failed
	// Architecture status alongside the local Canvas.
	if manifest.ReportSHA256 == "" {
		t.Fatalf("etcd manifest missing report binding")
	}
	// The published report.json binds the failed status and the local Canvas.
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatalf("read etcd report.json: %v", err)
	}
	if !bytes.Contains(reportJSON, []byte(`"error_code": "provider_output_limit"`)) &&
		!bytes.Contains(reportJSON, []byte(`"error_code":"provider_output_limit"`)) {
		t.Logf("architecture_synthesis in report.json: %s", string(bytesBetween(reportJSON, []byte(`"architecture_synthesis":`))))
		t.Fatalf("etcd report.json missing provider_output_limit binding")
	}
}

func bytesBetween(data, marker []byte) []byte {
	index := bytes.Index(data, marker)
	if index < 0 {
		return []byte("MARKER ABSENT")
	}
	end := index + len(marker) + 400
	if end > len(data) {
		end = len(data)
	}
	return data[index:end]
}

func runAtlasFirstAcceptance(
	t *testing.T,
	repo string,
	provider *atlasFirstAcceptanceProvider,
) (string, report.RunManifest, *report.ReportData) {
	t.Helper()
	clearLLMEnv(t)
	runsDir := t.TempDir()
	server := httptest.NewServer(provider)
	defer server.Close()
	configureNavigatorAcceptanceProvider(t, server.URL)

	var stderr bytes.Buffer
	err := runDefaultWithDeps(
		repo,
		[]string{
			"--debug-dir", runsDir,
			"--lang", "en",
			"--no-cache",
			"--no-open",
			"--no-serve",
		},
		defaultRunDeps{ctx: context.Background(), stdout: io.Discard, stderr: &stderr},
	)
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	runDir := navigatorAcceptanceRunDir(t, runsDir)
	manifest, data := readNavigatorAcceptanceRun(t, runDir)
	return runDir, manifest, data
}

func (provider *atlasFirstAcceptanceProvider) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request,
) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		provider.t.Errorf("read provider request: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if request.Method != http.MethodPost || request.Header.Get("Authorization") != "" ||
		!strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		provider.t.Errorf("invalid local provider request method/header")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	stage, combined, err := atlasFirstAcceptanceRequestStage(body)
	if err != nil {
		provider.t.Errorf("unrecognized semantic request: %v\n%s", err, body)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.stages = append(provider.stages, stage)
	if provider.bodies == nil {
		provider.bodies = make(map[atlasFirstAcceptanceStage][][]byte)
	}
	provider.bodies[stage] = append(provider.bodies[stage], append([]byte(nil), body...))
	provider.mu.Unlock()
	if (stage == atlasFirstStageNavigator && provider.failNavigatorCall) ||
		(stage == atlasFirstStageArchitecture && provider.failArchitectureCall) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"message":"fixture provider call failed"}}`))
		return
	}
	if stage == atlasFirstStageArchitecture && provider.lengthArchitectureCall {
		// Decision 215: the provider exhausts its output budget with a partial
		// response. Like the etcd incident, it emits one subsystem, one
		// component, an open member_refs array, and then repeats the same
		// bounded package-ref block many times with no closing JSON. The block
		// is generated deterministically, not stored as a large fixture.
		writer.Header().Set("Content-Type", "application/json")
		var partial strings.Builder
		partial.WriteString(`{"records":[{"kind":"subsystem","ref":"g1","name":"Fixture core","description":"Fixture grouping"},{"kind":"component","ref":"c1","subsystem_ref":"g1","name":"Fixture component","description":"Fixture component grouping","member_refs":[`)
		block := []string{
			`{"kind":"package","ref":"p1"}`,
			`{"kind":"package","ref":"p2"}`,
			`{"kind":"package","ref":"p3"}`,
			`{"kind":"package","ref":"p4"}`,
			`{"kind":"package","ref":"p5"}`,
			`{"kind":"package","ref":"p6"}`,
			`{"kind":"package","ref":"p7"}`,
			`{"kind":"package","ref":"p8"}`,
		}
		for index := 0; index < 128; index++ {
			if index > 0 {
				partial.WriteByte(',')
			}
			partial.WriteString(block[index%len(block)])
		}
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "length",
				"message": map[string]any{
					"role": "assistant", "content": partial.String(),
				},
			}},
			"usage": map[string]any{
				"prompt_tokens": 42197, "completion_tokens": 64000,
			},
		}); err != nil {
			provider.t.Errorf("encode length-ended architecture response: %v", err)
		}
		return
	}

	var response []byte
	switch stage {
	case atlasFirstStageNavigator:
		if provider.rejectNavigator {
			response, err = atlasFirstAcceptanceRejectedNavigator(combined)
		} else {
			response, err = atlasFirstAcceptanceSelectedNavigator(combined)
		}
	case atlasFirstStageArchitecture:
		response, err = atlasFirstAcceptanceArchitectureResponse(combined, provider.rejectArchitecture)
	case atlasFirstStageStudyScout:
		response, err = atlasFirstAcceptanceScoutResponse(
			combined,
			provider.includeBadStudySibling,
		)
	case atlasFirstStageStudyAdjudication:
		response, err = atlasFirstAcceptanceAdjudicationResponse(combined)
	default:
		err = fmt.Errorf("unsupported stage %q", stage)
	}
	if err != nil {
		provider.t.Errorf("build %s response: %v", stage, err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(response)
}

func atlasFirstAcceptanceSelectedNavigator(combined string) ([]byte, error) {
	const marker = "Answer the exact product question using only this request-local projection:\n"
	index := strings.LastIndex(combined, marker)
	if index < 0 {
		return nil, fmt.Errorf("Navigator request marker is absent")
	}
	var wire struct {
		Version      int    `json:"version"`
		CatalogRef   string `json:"catalog_ref"`
		DirectTrails []struct {
			Ref          string   `json:"ref"`
			SourceRef    string   `json:"source_ref"`
			TargetRef    string   `json:"target_ref"`
			EvidenceRefs []string `json:"evidence_refs"`
		} `json:"direct_trails"`
		Actions []struct {
			Ref       string `json:"ref"`
			TargetRef string `json:"target_ref"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(combined[index+len(marker):]), &wire); err != nil {
		return nil, fmt.Errorf("decode Navigator wire: %w", err)
	}
	if len(wire.Actions) == 0 {
		return nil, fmt.Errorf("Navigator wire has no advertised action")
	}
	action := wire.Actions[0]
	for _, trail := range wire.DirectTrails {
		if trail.SourceRef != action.TargetRef {
			continue
		}
		content, err := json.Marshal(map[string]any{
			"version":           wire.Version,
			"catalog_ref":       wire.CatalogRef,
			"entity_refs":       []string{trail.SourceRef, trail.TargetRef},
			"trail_refs":        []string{trail.Ref},
			"intersection_refs": []string{},
			"evidence_refs":     trail.EvidenceRefs,
			"gap_refs":          []string{},
			"action_refs":       []string{action.Ref},
		})
		if err != nil {
			return nil, err
		}
		return atlasFirstAcceptanceCompletion(content, 211, 31), nil
	}
	return nil, fmt.Errorf("Navigator action %q has no matching direct trail", action.Ref)
}

func atlasFirstAcceptanceRequestStage(
	body []byte,
) (atlasFirstAcceptanceStage, string, error) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "", "", err
	}
	if len(request.Messages) != 2 {
		return "", "", fmt.Errorf("message count = %d", len(request.Messages))
	}
	combined := request.Messages[0].Content + "\n" + request.Messages[1].Content
	switch {
	case strings.Contains(combined, "atlas-navigator-startup-json-v1"):
		return atlasFirstStageNavigator, combined, nil
	case strings.Contains(combined, "Use conceptual member, anchor, and flow refs as opaque request-local typed values.") &&
		strings.Contains(combined, "Refs under structural_context are read-only locator context") &&
		strings.Contains(combined, "Bounded candidate request:\n"):
		return atlasFirstStageArchitecture, combined, nil
	case strings.Contains(combined, "theme_kind is one of: user_journey") &&
		strings.Contains(combined, "Request bundle JSON:\n"):
		return atlasFirstStageStudyScout, combined, nil
	case strings.Contains(combined, "fit is one of: direct, supporting") &&
		strings.Contains(combined, "Request bundle JSON:\n"):
		return atlasFirstStageStudyAdjudication, combined, nil
	default:
		return "", combined, fmt.Errorf("request is not a current Atlas-first stage")
	}
}

func atlasFirstAcceptanceArchitectureResponse(
	combined string,
	reject bool,
) ([]byte, error) {
	const marker = "Bounded candidate request:\n"
	requestJSON := combined[strings.LastIndex(combined, marker)+len(marker):]
	var request componentmap.SynthesisRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode Architecture request: %w", err)
	}
	if len(request.Candidates) == 0 {
		return nil, fmt.Errorf("Architecture request has no candidates")
	}
	members := make([]componentmap.SynthesisMemberRef, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		members = append(members, candidate.Ref)
	}
	if reject {
		members[0].Ref += "-unknown-provider-member"
	}
	anchors := make([]componentmap.SynthesisAnchorRef, 0, len(request.BehaviorAnchors))
	for _, anchor := range request.BehaviorAnchors {
		anchors = append(anchors, anchor.Ref)
	}
	type architectureWireComponent struct {
		Kind         string                            `json:"kind"`
		SubsystemRef string                            `json:"subsystem_ref"`
		Name         string                            `json:"name"`
		Description  string                            `json:"description"`
		MemberRefs   []componentmap.SynthesisMemberRef `json:"member_refs"`
		AnchorRefs   []componentmap.SynthesisAnchorRef `json:"anchor_refs"`
		Hypothesis   bool                              `json:"hypothesis"`
	}
	type architectureWireSubsystem struct {
		Kind        string `json:"kind"`
		Ref         string `json:"ref"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	proposal := struct {
		Records []any `json:"records"`
	}{
		Records: []any{
			architectureWireSubsystem{
				Kind: "subsystem", Ref: "g1",
				Name: "Repository system", Description: "Conceptual grouping over exact local facts.",
			},
			architectureWireComponent{
				Kind: "component", SubsystemRef: "g1",
				Name: "Repository core", Description: "Groups the supplied local responsibilities.",
				MemberRefs: members, AnchorRefs: anchors, Hypothesis: len(anchors) == 0,
			},
		},
	}
	content, err := json.Marshal(proposal)
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 73), nil
}

// atlasFirstAcceptanceWireBundle extracts the model-visible request bundle
// JSON that BuildScoutPrompt/BuildAdjudicationPrompt embed after the
// "Request bundle JSON:" marker.
func atlasFirstAcceptanceWireBundle(combined string) ([]byte, error) {
	const marker = "Request bundle JSON:\n"
	index := strings.LastIndex(combined, marker)
	if index < 0 {
		return nil, fmt.Errorf("theme request bundle marker is absent")
	}
	return []byte(combined[index+len(marker):]), nil
}

// atlasFirstAcceptanceScoutResponse builds a valid Theme Scout response from
// the request bundle: candidate themes over the exact a* anchors and f*
// expansion refs, with one deliberately unknown-ref sibling when requested so
// the run exercises item-local rejection (accepted_partial).
func atlasFirstAcceptanceScoutResponse(combined string, includeBadSibling bool) ([]byte, error) {
	raw, err := atlasFirstAcceptanceWireBundle(combined)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Vocabulary struct {
			Files []struct {
				Ref string `json:"ref"`
			} `json:"files"`
		} `json:"vocabulary"`
		SeedPacks struct {
			Packs []struct {
				Seed struct {
					Ref string `json:"ref"`
				} `json:"seed"`
			} `json:"packs"`
		} `json:"seed_packs"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode Theme Scout wire: %w", err)
	}
	var anchorRefs []string
	for _, pack := range wire.SeedPacks.Packs {
		if pack.Seed.Ref != "" {
			anchorRefs = append(anchorRefs, pack.Seed.Ref)
		}
	}
	if len(anchorRefs) == 0 {
		return nil, fmt.Errorf("Theme Scout wire has no a* seed anchor")
	}
	var fileRefs []string
	for _, file := range wire.Vocabulary.Files {
		if file.Ref != "" {
			fileRefs = append(fileRefs, file.Ref)
		}
	}
	expansionRef := ""
	if len(fileRefs) > 0 {
		expansionRef = fileRefs[0]
	}
	theme := func(title, question string, refs []string) map[string]any {
		return map[string]any{
			"title":               title,
			"question":            question,
			"theme_kind":          string(themestudy.KindSiblingImplementationFamily),
			"anchor_refs":         refs,
			"expansion_file_refs": []string{expansionRef},
			"why_it_matters":      "The accepted anchors participate in one bounded editorial responsibility.",
			"expected_learning":   "The reader can inspect the exact anchors and their bounded source.",
			"relation_claim":      "editorial_only",
			"focused":             false,
		}
	}
	// The candidate contract bounds each theme to MaxThemeAnchors anchors and
	// rejects duplicate normalized question/learning pairs, so large
	// repositories (etcd) are covered by several bounded candidates with
	// distinct questions instead of one oversized theme.
	var themes []any
	for offset := 0; offset < len(anchorRefs); offset += themestudy.MaxThemeAnchors {
		end := min(offset+themestudy.MaxThemeAnchors, len(anchorRefs))
		themes = append(themes, theme(
			fmt.Sprintf("Shared responsibilities across exact anchors %d", offset/themestudy.MaxThemeAnchors+1),
			fmt.Sprintf("How do anchors %d through %d work together in this repository?", offset+1, end),
			anchorRefs[offset:end],
		))
	}
	if includeBadSibling {
		themes = append(themes, map[string]any{
			"title":               "Deliberately invalid sibling",
			"question":            "This candidate references an unknown anchor to prove item-local rejection.",
			"theme_kind":          string(themestudy.KindCrossCuttingPolicy),
			"anchor_refs":         []string{"a999999"},
			"expansion_file_refs": []string{expansionRef},
			"why_it_matters":      "The unknown ref must reject only this candidate.",
			"expected_learning":   "The valid sibling survives.",
			"relation_claim":      "editorial_only",
			"focused":             true,
		})
	}
	content, err := json.Marshal(map[string]any{"themes": themes})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 31), nil
}

// atlasFirstAcceptanceAdjudicationResponse builds a valid Theme Adjudication
// response from the request bundle: for every t* candidate, a direct
// assessment over its own a* anchors with a supported observation and reading
// order.
func atlasFirstAcceptanceAdjudicationResponse(combined string) ([]byte, error) {
	raw, err := atlasFirstAcceptanceWireBundle(combined)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Candidates []struct {
			Ref        string   `json:"ref"`
			AnchorRefs []string `json:"anchor_refs"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode Theme Adjudication wire: %w", err)
	}
	if len(wire.Candidates) == 0 {
		return nil, fmt.Errorf("Theme Adjudication wire has no candidate")
	}
	themes := make([]any, 0, len(wire.Candidates))
	for _, candidate := range wire.Candidates {
		if len(candidate.AnchorRefs) == 0 {
			return nil, fmt.Errorf("Theme Adjudication candidate %s has no anchor", candidate.Ref)
		}
		assessments := make([]any, 0, len(candidate.AnchorRefs))
		readingOrder := make([]string, 0, len(candidate.AnchorRefs))
		for index, anchor := range candidate.AnchorRefs {
			role := "supporting"
			if index == 0 {
				role = "public_entry"
			}
			assessments = append(assessments, map[string]any{
				"anchor_ref":            anchor,
				"fit":                   "direct",
				"role":                  role,
				"supported_observation": "The exact source pack shows this anchor participating in the theme.",
			})
			readingOrder = append(readingOrder, anchor)
		}
		themes = append(themes, map[string]any{
			"candidate_ref":      candidate.Ref,
			"final_title":        "Accepted theme for " + candidate.Ref,
			"final_question":     "What shared responsibility do the exact anchors implement?",
			"anchor_assessments": assessments,
			"reading_order":      readingOrder,
			"unknowns":           []string{},
		})
	}
	content, err := json.Marshal(map[string]any{"themes": themes})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 73), nil
}

func atlasFirstAcceptanceRejectedNavigator(combined string) ([]byte, error) {
	const marker = "Answer the exact product question using only this request-local projection:\n"
	index := strings.LastIndex(combined, marker)
	if index < 0 {
		return nil, fmt.Errorf("Navigator request marker is absent")
	}
	var wire struct {
		Version    int    `json:"version"`
		CatalogRef string `json:"catalog_ref"`
	}
	if err := json.Unmarshal([]byte(combined[index+len(marker):]), &wire); err != nil {
		return nil, fmt.Errorf("decode Navigator wire: %w", err)
	}
	content, err := json.Marshal(map[string]any{
		"version":           wire.Version,
		"catalog_ref":       wire.CatalogRef,
		"entity_refs":       []string{},
		"trail_refs":        []string{},
		"intersection_refs": []string{},
		"evidence_refs":     []string{},
		"gap_refs":          []string{},
		"action_refs":       []string{"a999"},
	})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 31), nil
}

func atlasFirstAcceptanceCompletion(content []byte, inputTokens, outputTokens int) []byte {
	envelope, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": string(content)},
		}},
		"usage": map[string]any{
			"prompt_tokens":            inputTokens,
			"completion_tokens":        outputTokens,
			"prompt_cache_hit_tokens":  0,
			"prompt_cache_miss_tokens": inputTokens,
		},
	})
	return envelope
}

func (provider *atlasFirstAcceptanceProvider) assertStages(
	t *testing.T,
	want ...atlasFirstAcceptanceStage,
) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if !reflect.DeepEqual(provider.stages, want) {
		t.Fatalf("provider stage order = %v, want %v", provider.stages, want)
	}
	for stage, bodies := range provider.bodies {
		if len(bodies) != 1 {
			t.Fatalf("provider stage %s request count = %d, want one", stage, len(bodies))
		}
	}
}

func atlasFirstAcceptanceRepository(t *testing.T, fixtureDir string) string {
	t.Helper()
	repo := t.TempDir()
	var names []string
	err := filepath.WalkDir(fixtureDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(repo, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	runGit(t, repo, "init", "--quiet")
	args := append([]string{"add", "--"}, names...)
	runGit(t, repo, args...)
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	commitTestRepository(t, repo)
	return repo
}

func assertAtlasFirstAcceptedArchitecture(t *testing.T, data *report.ReportData) {
	t.Helper()
	if data.ArchitectureSynthesis == nil ||
		(data.ArchitectureSynthesis.State != report.ArchitectureSynthesisSucceeded &&
			data.ArchitectureSynthesis.State != report.ArchitectureSynthesisCached) ||
		!data.ArchitectureSynthesis.ProposalAccepted ||
		data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.FallbackSelected ||
		data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceValidatedModel &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceNormalizedModel) {
		t.Fatalf("accepted Architecture = %#v / %#v", data.ArchitectureSynthesis, data.ArchitectureCanvas)
	}
}

func assertAtlasFirstAcceptedStudy(t *testing.T, data *report.ReportData) {
	t.Helper()
	if data.AtlasStudy == nil || data.AtlasStudy.Version != themestudy.ScoutResultVersion ||
		data.AtlasStudy.Themes == nil || len(data.AtlasStudy.Themes.Cards) == 0 {
		t.Fatalf("accepted theme Study = %#v", data.AtlasStudy)
	}
	// D213 admits exactly the accepted state pairs: a fully accepted shelf or
	// a partial acceptance with rejected siblings uncovered. Anything else —
	// including a third state value — is a failure.
	switch data.AtlasStudy.State {
	case atlasstudy.ProductStateAccepted, atlasstudy.ProductStateAcceptedPartial:
	default:
		t.Fatalf("accepted theme Study = %#v", data.AtlasStudy)
	}
	for _, card := range data.AtlasStudy.Themes.Cards {
		if len(card.Readings) == 0 {
			t.Fatalf("theme card has no exact reading: %#v", card)
		}
	}
}

func assertAtlasFirstLocalSubstrateUnchanged(t *testing.T, data *report.ReportData) {
	t.Helper()
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	local, err := report.ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.ArchitectureCanvas.BehaviorAnchors, local.BehaviorAnchors) ||
		!reflect.DeepEqual(data.ArchitectureCanvas.StructuralFacts, local.StructuralFacts) {
		t.Fatalf("model grouping changed exact local anchors/relations")
	}
	gotCandidates := make([]componentmap.Candidate, 0)
	for _, component := range data.ArchitectureCanvas.Components {
		gotCandidates = append(gotCandidates, component.Members...)
	}
	sort.Slice(gotCandidates, func(i, j int) bool {
		if gotCandidates[i].ID.Kind != gotCandidates[j].ID.Kind {
			return gotCandidates[i].ID.Kind < gotCandidates[j].ID.Kind
		}
		return gotCandidates[i].ID.Value < gotCandidates[j].ID.Value
	})
	wantCandidates := make([]componentmap.Candidate, 0, len(input.CandidateBundle.Candidates))
	wantStructuralLocators := make([]componentmap.Candidate, 0, len(input.CandidateBundle.Candidates))
	for _, candidate := range input.CandidateBundle.Candidates {
		if candidate.Role == componentmap.CandidateRoleStructuralLocator {
			wantStructuralLocators = append(wantStructuralLocators, candidate)
			continue
		}
		wantCandidates = append(wantCandidates, candidate)
	}
	sort.Slice(wantCandidates, func(i, j int) bool {
		if wantCandidates[i].ID.Kind != wantCandidates[j].ID.Kind {
			return wantCandidates[i].ID.Kind < wantCandidates[j].ID.Kind
		}
		return wantCandidates[i].ID.Value < wantCandidates[j].ID.Value
	})
	if !reflect.DeepEqual(gotCandidates, wantCandidates) {
		t.Fatalf("model grouping changed exact local candidates\ngot:  %#v\nwant: %#v", gotCandidates, wantCandidates)
	}
	gotStructuralLocators := make([]componentmap.Candidate, 0, len(data.ArchitectureCanvas.StructuralLocators))
	for _, locator := range data.ArchitectureCanvas.StructuralLocators {
		gotStructuralLocators = append(gotStructuralLocators, locator.Locator)
	}
	sort.Slice(gotStructuralLocators, func(i, j int) bool {
		if gotStructuralLocators[i].ID.Kind != gotStructuralLocators[j].ID.Kind {
			return gotStructuralLocators[i].ID.Kind < gotStructuralLocators[j].ID.Kind
		}
		return gotStructuralLocators[i].ID.Value < gotStructuralLocators[j].ID.Value
	})
	sort.Slice(wantStructuralLocators, func(i, j int) bool {
		if wantStructuralLocators[i].ID.Kind != wantStructuralLocators[j].ID.Kind {
			return wantStructuralLocators[i].ID.Kind < wantStructuralLocators[j].ID.Kind
		}
		return wantStructuralLocators[i].ID.Value < wantStructuralLocators[j].ID.Value
	})
	if !reflect.DeepEqual(gotStructuralLocators, wantStructuralLocators) {
		t.Fatalf("model grouping changed exact structural locators\ngot:  %#v\nwant: %#v", gotStructuralLocators, wantStructuralLocators)
	}
}

func assertAtlasFirstDiagnostics(
	t *testing.T,
	runDir string,
	wantCalls int,
	wantStates map[string]string,
) {
	t.Helper()
	// Exact call totals describe this first Atlas-backed implementation. They
	// diagnose accidental legacy fan-out; they are not a permanent product
	// ceiling on future independently approved Atlas questions. D213's Study
	// stage makes exactly two semantic calls (Theme Scout + Theme
	// Adjudication); other stages make one.
	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.ProviderAccountingComplete || metadata.ProviderRequestCount != wantCalls ||
		len(metadata.RequestAttempts) != 3 {
		t.Fatalf("Atlas-first diagnostic totals = %#v", metadata)
	}
	for _, attempt := range metadata.RequestAttempts {
		wantState, ok := wantStates[attempt.Stage]
		if !ok || attempt.State != wantState {
			t.Fatalf("Atlas-first stage diagnostic = %#v, want states %#v", attempt, wantStates)
		}
		maxCalls := 1
		if attempt.Stage == debugdump.SemanticStageAtlasStudy {
			maxCalls = 2 // D213: exactly Theme Scout + Theme Adjudication
		}
		if attempt.ProviderCallCount < 0 || attempt.ProviderCallCount > maxCalls {
			t.Fatalf("Atlas-first stage call count is not diagnostic one/two-call state: %#v", attempt)
		}
	}
}

func assertAtlasFirstSemanticStages(t *testing.T, runDir string, want ...string) {
	t.Helper()
	entries := readSemanticJournalEntries(t, runDir)
	seen := make(map[string]struct{}, len(entries))
	var got []string
	for _, entry := range entries {
		if _, dup := seen[entry.record.Stage]; dup {
			continue
		}
		seen[entry.record.Stage] = struct{}{}
		got = append(got, entry.record.Stage)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("semantic stages = %v, want current Atlas-first stages %v", got, want)
	}
}

func assertAtlasFirstAcceptedArtifacts(t *testing.T, runDir string, navigatorSelected bool) {
	t.Helper()
	artifactNames := []string{
		navigator.StatusArtifactFilename,
		navigator.RecordArtifactFilename,
		report.ArchitectureSynthesisFile,
		report.ArchitectureSynthesisStatusFile,
		themestudy.ScoutRequestArtifactFilename,
		themestudy.ScoutResultArtifactFilename,
		themestudy.ScoutStatusArtifactFilename,
		themestudy.ExpansionArtifactFilename,
		themestudy.AdjudicationRequestArtifactFilename,
		themestudy.AdjudicationResultArtifactFilename,
		themestudy.AdjudicationStatusArtifactFilename,
		themestudy.StudyThemesArtifactFilename,
		"report.json",
		"report.html",
		report.RunManifestFilename,
	}
	for _, name := range artifactNames {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("accepted Atlas-first run missing %s: %v", name, err)
		}
	}
	_, err := os.Stat(filepath.Join(runDir, navigator.RequestArtifactFilename))
	if navigatorSelected && err != nil {
		t.Fatalf("selected Navigator missing request: %v", err)
	}
	if !navigatorSelected && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty Navigator unexpectedly persisted a provider request: %v", err)
	}
}

func assertAtlasFirstAcceptedManifest(
	t *testing.T,
	manifest report.RunManifest,
	navigatorSelected bool,
) {
	t.Helper()
	inputs := manifest.MaterialInputs
	if inputs.RepositoryAtlasSHA256 == "" ||
		inputs.NavigatorStatusSHA256 == "" ||
		(inputs.NavigatorRequestSHA256 != "") != navigatorSelected ||
		inputs.NavigatorResultSHA256 == "" ||
		inputs.ThemeScoutRequestSHA256 == "" ||
		inputs.ThemeScoutResultSHA256 == "" ||
		inputs.ThemeScoutStatusSHA256 == "" ||
		inputs.ThemeSourceExpansionSHA256 == "" ||
		inputs.ThemeAdjudicationRequestSHA256 == "" ||
		inputs.ThemeAdjudicationResultSHA256 == "" ||
		inputs.ThemeAdjudicationStatusSHA256 == "" ||
		inputs.StudyThemesSHA256 == "" ||
		inputs.ModelBundleSHA256 != "" ||
		inputs.OrientationContextSelectionSHA256 != "" {
		t.Fatalf("Atlas-first manifest bindings = %#v", inputs)
	}
}

func assertNoLegacyAtlasFirstArtifacts(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{
		"orientation_report.json",
		"llm_bundle.json",
		guidedTourBundleFile,
		guidedTourMonolithicFile,
		studymap.RecordFile,
		studymap.BundleFile,
		studyMapBriefShapeFile,
		studyMapDirectionsFile,
		studyMapReviewsFile,
		"flows",
	} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Atlas-first run retained legacy artifact %s: %v", name, err)
		}
	}
}
