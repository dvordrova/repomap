package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/studymap"
)

const navigatorAcceptanceFixtureDir = "testdata/navigator_acceptance"

func TestRunDefaultAtlasFirstAcceptsOneSavedNavigatorResponse(t *testing.T) {
	clearLLMEnv(t)
	repo := navigatorAcceptanceRepository(t)
	runsDir := t.TempDir()
	server, provider := navigatorAcceptanceProvider(t, "provider_selected.json")
	defer server.Close()
	configureNavigatorAcceptanceProvider(t, server.URL)

	var stderr bytes.Buffer
	if err := runDefaultWithDeps(
		repo,
		[]string{
			"--debug-dir", runsDir,
			"--lang", "ru",
			"--no-cache",
			"--no-open",
			"--no-serve",
		},
		defaultRunDeps{
			ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		},
	); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want exactly one Navigator call", got)
	}

	runDir := navigatorAcceptanceRunDir(t, runsDir)
	manifest, data := readNavigatorAcceptanceRun(t, runDir)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateSelected ||
		data.Navigator.Recommendation == nil {
		t.Fatalf("Navigator report = %#v, want one selected recommendation", data.Navigator)
	}
	assertNavigatorAcceptanceSemanticMinimum(t, data)
	assertNavigatorAcceptanceRequestArtifact(t, runDir)
	assertNavigatorAcceptanceManifestBindings(t, manifest, true)
	assertNavigatorAcceptanceJournal(
		t, runDir, provider, "provider_selected.json",
		debugdump.SemanticStateAccepted, debugdump.SemanticValidationAccepted,
	)
	assertNoSupersededSemanticProducts(t, data)
	assertNoSupersededSemanticArtifacts(t, runDir)
	assertNoLegacyOrientationWarning(t, data)
}

func TestRunDefaultAtlasFirstOfflineIsExplicitAndProviderFree(t *testing.T) {
	clearLLMEnv(t)
	repo := navigatorAcceptanceRepository(t)
	runsDir := t.TempDir()
	server, provider := navigatorAcceptanceProvider(t, "provider_selected.json")
	defer server.Close()
	configureNavigatorAcceptanceProvider(t, server.URL)

	var stderr bytes.Buffer
	if err := runDefaultWithDeps(
		repo,
		[]string{
			"--debug-dir", runsDir,
			"--offline",
			"--lang", "ru",
			"--no-cache",
			"--no-open",
			"--no-serve",
		},
		defaultRunDeps{
			ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		},
	); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("offline provider calls = %d, want zero", got)
	}

	runDir := navigatorAcceptanceRunDir(t, runsDir)
	manifest, data := readNavigatorAcceptanceRun(t, runDir)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateUnavailable ||
		data.Navigator.UnavailableCode != navigator.UnavailableOffline ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("offline Navigator report = %#v", data.Navigator)
	}
	assertNavigatorAcceptanceAtlasMinimum(t, data)
	assertNavigatorAcceptanceRequestArtifact(t, runDir)
	assertNavigatorAcceptanceManifestBindings(t, manifest, false)
	if _, err := os.Stat(filepath.Join(runDir, navigator.RecordArtifactFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("offline Navigator result artifact stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, debugdump.SemanticExchangesDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("offline semantic exchange directory stat error = %v, want not exist", err)
	}
	assertNoSupersededSemanticProducts(t, data)
	assertNoSupersededSemanticArtifacts(t, runDir)
	assertNoLegacyOrientationWarning(t, data)
}

func TestRunDefaultAtlasFirstResourceFailurePublishesNoReportAuthority(t *testing.T) {
	clearLLMEnv(t)
	repo := navigatorAcceptanceRepository(t)
	runsDir := t.TempDir()
	server, provider := navigatorAcceptanceProvider(t, "provider_resource.json")
	defer server.Close()
	configureNavigatorAcceptanceProvider(t, server.URL)

	var stderr bytes.Buffer
	err := runDefaultWithDeps(
		repo,
		[]string{
			"--debug-dir", runsDir,
			"--lang", "ru",
			"--no-cache",
			"--no-open",
			"--no-serve",
		},
		defaultRunDeps{
			ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		},
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != "navigator" ||
		limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.ConfiguredMaxTokens != 64_000 || limitErr.Limit != 64_000 ||
		limitErr.OutputTokens != 64_000 || limitErr.FinishReason != "length" {
		t.Fatalf("run resource error = %#v / %v\nstderr:\n%s", limitErr, err, stderr.String())
	}
	if got := defaultRunExitCode(err); got != 1 {
		t.Fatalf("terminal resource exit code = %d, want 1", got)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("resource-limited provider calls = %d, want one", got)
	}

	runDir := navigatorAcceptanceRunDir(t, runsDir)
	for _, name := range []string{"report.json", "report.html", report.RunManifestFilename} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("terminal resource failure published %s: %v", name, statErr)
		}
	}
	if _, statErr := os.Lstat(filepath.Join(runsDir, "latest")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("terminal resource failure published latest: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, navigator.RecordArtifactFilename)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("terminal resource failure published Navigator result: %v", statErr)
	}
	statusJSON, readErr := os.ReadFile(filepath.Join(runDir, navigator.StatusArtifactFilename))
	if readErr != nil {
		t.Fatal(readErr)
	}
	status, decodeErr := navigator.DecodeStatus(statusJSON)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if status.State != navigator.ProductStateFailed || status.FailureCode != navigator.FailureResource {
		t.Fatalf("terminal Navigator status = %#v", status)
	}
	atlasJSON, readErr := os.ReadFile(filepath.Join(runDir, repositoryatlas.ArtifactFilename))
	if readErr != nil {
		t.Fatal(readErr)
	}
	atlas, decodeErr := repositoryatlas.DecodeCanonicalJSON(atlasJSON)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if err := navigator.ValidateStatusAgainstAtlas(status, atlas); err != nil {
		t.Fatalf("terminal Navigator status is not bound to the saved Atlas: %v", err)
	}
	assertNavigatorAcceptanceRequestArtifact(t, runDir)
	assertNavigatorAcceptanceJournal(
		t, runDir, provider, "provider_resource.json",
		debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider,
	)
}

type navigatorAcceptanceProviderState struct {
	calls  atomic.Int32
	mu     sync.Mutex
	bodies [][]byte
}

func navigatorAcceptanceProvider(
	t *testing.T,
	responseName string,
) (*httptest.Server, *navigatorAcceptanceProviderState) {
	t.Helper()
	response := navigatorAcceptanceFixture(t, responseName)
	requestFixture := decodeNavigatorAcceptanceRequest(t)
	state := &navigatorAcceptanceProviderState{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		state.calls.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if request.Header.Get("Authorization") != "" {
			t.Errorf("Navigator fixture received Authorization header")
		}
		if request.Method != http.MethodPost {
			t.Errorf("Navigator fixture request method = %q, want POST", request.Method)
		}
		if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			t.Errorf("Navigator fixture Content-Type = %q, want application/json", contentType)
		}
		state.mu.Lock()
		state.bodies = append(state.bodies, append([]byte(nil), body...))
		state.mu.Unlock()
		assertNavigatorAcceptanceProviderRequest(t, body, requestFixture.WireJSON)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(response)
	}))
	return server, state
}

func (state *navigatorAcceptanceProviderState) onlyBody(t *testing.T) []byte {
	t.Helper()
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.bodies) != 1 {
		t.Fatalf("captured provider request bodies = %d, want exactly one", len(state.bodies))
	}
	return append([]byte(nil), state.bodies[0]...)
}

func assertNavigatorAcceptanceJournal(
	t *testing.T,
	runDir string,
	provider *navigatorAcceptanceProviderState,
	responseName string,
	wantState string,
	wantValidation string,
) {
	t.Helper()
	entries := readSemanticJournalEntries(t, runDir)
	if len(entries) != 1 {
		t.Fatalf("semantic journal entries = %d, want exactly one Navigator exchange", len(entries))
	}
	entry := entries[0]
	if entry.record.Stage != debugdump.SemanticStageNavigator ||
		entry.record.State != wantState || entry.record.ValidationCode != wantValidation ||
		entry.record.SemanticCalls != 1 || entry.record.TransportAttempts != 1 ||
		entry.record.RequestProvenance != debugdump.SemanticRequestExactSent {
		t.Fatalf("Navigator semantic journal metadata = %#v", entry.record)
	}
	if want := provider.onlyBody(t); !bytes.Equal(entry.request, want) {
		t.Fatalf("journaled Navigator request differs from exact HTTP body\njournal: %s\nHTTP:    %s", entry.request, want)
	}
	if want := navigatorAcceptanceProviderContent(t, responseName); !bytes.Equal(entry.response, want) {
		t.Fatalf("journaled Navigator response differs from exact provider content\njournal: %s\nprovider: %s", entry.response, want)
	}
}

func navigatorAcceptanceProviderContent(t *testing.T, name string) []byte {
	t.Helper()
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(navigatorAcceptanceFixture(t, name), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Choices) != 1 || envelope.Choices[0].Message.Content == "" {
		t.Fatalf("provider fixture %s does not contain exactly one response", name)
	}
	return []byte(envelope.Choices[0].Message.Content)
}

func assertNavigatorAcceptanceProviderRequest(t *testing.T, body []byte, wireJSON string) {
	t.Helper()
	var request struct {
		Model          string `json:"model"`
		MaxTokens      int    `json:"max_tokens"`
		ResponseFormat *struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Errorf("decode provider request: %v", err)
		return
	}
	if request.Model != "fixture-navigator-model" || request.MaxTokens != 64_000 ||
		request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" ||
		len(request.Messages) != 2 {
		t.Errorf("provider request shape = %#v", request)
		return
	}
	combined := request.Messages[0].Content + "\n" + request.Messages[1].Content
	for _, want := range []string{
		"atlas-navigator-startup-json-v1",
		navigator.ProductQuestion,
		wireJSON,
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("provider request is missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"Orientation facts bundle JSON",
		"source_signals",
		"file_tree",
		"internal_edges",
		"allowed_paths",
		"main.go",
		"trigger-ed9e5f67825f43d40c3f6028",
		"operation-f32ee0565fe429746fb6c2c7",
		"relation-4dfe6eef63793b759ffe9d4f",
		"evidence-51e48b24d651beaf4a12028d",
	} {
		if strings.Contains(combined, forbidden) {
			t.Errorf("provider request leaked superseded/raw value %q", forbidden)
		}
	}
}

func configureNavigatorAcceptanceProvider(t *testing.T, endpoint string) {
	t.Helper()
	t.Setenv("REPOMAP_LLM_ENDPOINT", endpoint)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-navigator-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "64000")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")
}

func navigatorAcceptanceRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, name := range []string{"go.mod", "main.go"} {
		data := navigatorAcceptanceFixture(t, name)
		if err := os.WriteFile(filepath.Join(repo, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	commitTestRepository(t, repo)
	return repo
}

func navigatorAcceptanceRunDir(t *testing.T, runsDir string) string {
	t.Helper()
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(runsDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "metadata.json")); err == nil {
			runDirs = append(runDirs, candidate)
		}
	}
	if len(runDirs) != 1 {
		t.Fatalf("run directories = %v, want exactly one", runDirs)
	}
	return runDirs[0]
}

func readNavigatorAcceptanceRun(
	t *testing.T,
	runDir string,
) (report.RunManifest, *report.ReportData) {
	t.Helper()
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest(%s): %v", runDir, err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != report.CurrentRunManifestVersion ||
		manifest.ReportFormatVersion != report.CurrentFormatVersion ||
		data.FormatVersion != report.CurrentFormatVersion {
		t.Fatalf(
			"Atlas-first versions manifest/report = %d/%d/%d",
			manifest.Version, manifest.ReportFormatVersion, data.FormatVersion,
		)
	}
	return manifest, &data
}

func assertNavigatorAcceptanceRequestArtifact(t *testing.T, runDir string) {
	t.Helper()
	want := navigatorAcceptanceFixture(t, navigator.RequestArtifactFilename)
	got, err := os.ReadFile(filepath.Join(runDir, navigator.RequestArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Navigator request artifact changed\ngot:  %s\nwant: %s", got, want)
	}
}

func assertNavigatorAcceptanceManifestBindings(
	t *testing.T,
	manifest report.RunManifest,
	wantResult bool,
) {
	t.Helper()
	inputs := manifest.MaterialInputs
	if inputs.RepositoryAtlasSHA256 == "" || inputs.NavigatorRequestSHA256 == "" ||
		inputs.NavigatorStatusSHA256 == "" || (inputs.NavigatorResultSHA256 != "") != wantResult {
		t.Fatalf("Atlas-first manifest material inputs = %#v", inputs)
	}
	if inputs.ModelBundleSHA256 != "" || inputs.OrientationContextSelectionSHA256 != "" {
		t.Fatalf("Atlas-first manifest retained superseded Orientation inputs = %#v", inputs)
	}
}

func assertNavigatorAcceptanceSemanticMinimum(t *testing.T, data *report.ReportData) {
	t.Helper()
	assertNavigatorAcceptanceAtlasMinimum(t, data)
	recommendation := data.Navigator.Recommendation
	relation := navigatorAcceptanceStartupRelation(t, data.RepositoryAtlas)
	if recommendation.Operation != navigator.StartupActionOperation ||
		recommendation.Surface != relation.Source ||
		recommendation.Application != relation.Target ||
		recommendation.RelationID != relation.ID ||
		len(recommendation.EvidenceIDs) != 1 ||
		recommendation.EvidenceIDs[0] != relation.EvidenceRefs[0] {
		t.Fatalf("Navigator recommendation does not restore the exact Atlas vertical: %#v", recommendation)
	}
}

func assertNavigatorAcceptanceAtlasMinimum(t *testing.T, data *report.ReportData) {
	t.Helper()
	if data.RepositoryAtlas == nil {
		t.Fatal("report has no Repository Atlas")
	}
	atlas := data.RepositoryAtlas
	if len(atlas.Units) < 4 || len(atlas.Entities) < 2 ||
		len(atlas.Observations) < 2 || len(atlas.Evidence) < 1 || len(atlas.Relations) < 1 {
		t.Fatalf(
			"semantic Atlas = units:%d entities:%d observations:%d evidence:%d relations:%d",
			len(atlas.Units), len(atlas.Entities), len(atlas.Observations),
			len(atlas.Evidence), len(atlas.Relations),
		)
	}
	unitKinds := map[repositoryatlas.UnitKind]int{}
	units := map[string]repositoryatlas.Unit{}
	for _, unit := range atlas.Units {
		unitKinds[unit.Kind]++
		units[unit.ID] = unit
	}
	for _, kind := range []repositoryatlas.UnitKind{
		repositoryatlas.UnitRepository,
		repositoryatlas.UnitModule,
		repositoryatlas.UnitApp,
		repositoryatlas.UnitPackage,
	} {
		if unitKinds[kind] < 1 {
			t.Fatalf("Atlas has no unit kind %q", kind)
		}
	}
	entities := map[string]repositoryatlas.Entity{}
	for _, entity := range atlas.Entities {
		entities[entity.ID] = entity
	}
	relation := navigatorAcceptanceStartupRelation(t, atlas)
	if len(relation.EvidenceRefs) != 1 ||
		entities[relation.Source.ID].UnitID != relation.UnitID ||
		entities[relation.Target.ID].UnitID != relation.UnitID ||
		units[relation.UnitID].Kind != repositoryatlas.UnitApp {
		t.Fatalf("Atlas startup relation = %#v", relation)
	}
	app := units[relation.UnitID]
	module := units[app.ParentID]
	repository := units[module.ParentID]
	if module.Kind != repositoryatlas.UnitModule || repository.Kind != repositoryatlas.UnitRepository ||
		repository.ParentID != "" {
		t.Fatalf("Atlas app/module/repository ownership chain = %#v / %#v / %#v", app, module, repository)
	}
	evidence, ok := navigatorAcceptanceEvidence(atlas, relation.EvidenceRefs[0])
	if !ok {
		t.Fatalf("Atlas startup relation evidence %q is absent", relation.EvidenceRefs[0])
	}
	if evidence.ID != relation.EvidenceRefs[0] || evidence.UnitID != relation.UnitID ||
		evidence.Location.Path != "main.go" || evidence.Location.Line != 3 {
		t.Fatalf("Atlas startup evidence = %#v", evidence)
	}
	for _, endpoint := range []repositoryatlas.EntityRef{relation.Source, relation.Target} {
		if !navigatorAcceptanceHasObservation(atlas, relation.UnitID, endpoint, evidence.ID) {
			t.Fatalf("Atlas endpoint %#v has no exact observation citing %q", endpoint, evidence.ID)
		}
	}
	if !navigatorAcceptanceHasSource(data, "main.go", 3) {
		t.Fatalf("report has no saved exact source excerpt for main.go:3: %#v", data.UserSources)
	}
}

func navigatorAcceptanceStartupRelation(
	t *testing.T,
	atlas *repositoryatlas.Atlas,
) repositoryatlas.Relation {
	t.Helper()
	for _, relation := range atlas.Relations {
		if relation.Kind == repositoryatlas.RelationExposes &&
			relation.Phase == repositoryatlas.PhaseStartup &&
			relation.Authority == repositoryatlas.AuthorityResolved &&
			relation.Source.Kind == repositoryatlas.EntitySurface &&
			relation.Target.Kind == repositoryatlas.EntityOperation {
			return relation
		}
	}
	t.Fatal("Atlas has no resolved Surface -> Operation startup relation")
	return repositoryatlas.Relation{}
}

func navigatorAcceptanceEvidence(
	atlas *repositoryatlas.Atlas,
	id string,
) (repositoryatlas.Evidence, bool) {
	for _, item := range atlas.Evidence {
		if item.ID == id {
			return item, true
		}
	}
	return repositoryatlas.Evidence{}, false
}

func navigatorAcceptanceHasObservation(
	atlas *repositoryatlas.Atlas,
	unitID string,
	subject repositoryatlas.EntityRef,
	evidenceID string,
) bool {
	for _, observation := range atlas.Observations {
		if observation.UnitID == unitID && observation.Subject == subject &&
			len(observation.EvidenceRefs) == 1 && observation.EvidenceRefs[0] == evidenceID {
			return true
		}
	}
	return false
}

func navigatorAcceptanceHasSource(
	data *report.ReportData,
	path string,
	line int,
) bool {
	for _, source := range data.UserSources {
		if source.Path != path || source.StartLine > line || source.EndLine < line || source.Content == "" {
			continue
		}
		for _, item := range source.Lines {
			if item.Line == line && strings.Contains(item.Text, "func main") {
				return true
			}
		}
	}
	return false
}

func assertNoSupersededSemanticProducts(t *testing.T, data *report.ReportData) {
	t.Helper()
	if len(data.CandidateDirections) != 0 || len(data.Flows) != 0 ||
		data.GuidedTour != nil || data.StudyMap != nil || len(data.UserMechanisms) != 0 {
		t.Fatalf(
			"Atlas-first run published superseded semantic products: directions=%d flows=%d guided=%t study=%t mechanisms=%d",
			len(data.CandidateDirections), len(data.Flows), data.GuidedTour != nil,
			data.StudyMap != nil, len(data.UserMechanisms),
		)
	}
}

func assertNoSupersededSemanticArtifacts(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{
		"orientation_report.json",
		"llm_bundle.json",
		"architecture_synthesis.json",
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
			t.Fatalf("Atlas-first run retained superseded semantic artifact %s: %v", name, err)
		}
	}
}

func assertNoLegacyOrientationWarning(t *testing.T, data *report.ReportData) {
	t.Helper()
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "orientation_report.json") ||
			strings.HasPrefix(strings.TrimSpace(warning), "orientation:") {
			t.Fatalf("Atlas-first report retained legacy Orientation warning %q", warning)
		}
	}
}

func decodeNavigatorAcceptanceRequest(t *testing.T) navigator.RequestRecord {
	t.Helper()
	record, err := navigator.DecodeRequestRecord(
		navigatorAcceptanceFixture(t, navigator.RequestArtifactFilename),
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func navigatorAcceptanceFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(navigatorAcceptanceFixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
