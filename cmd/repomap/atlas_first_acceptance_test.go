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
)

type atlasFirstAcceptanceStage string

const (
	atlasFirstStageNavigator    atlasFirstAcceptanceStage = "navigator"
	atlasFirstStageArchitecture atlasFirstAcceptanceStage = "architecture"
	atlasFirstStageStudy        atlasFirstAcceptanceStage = "atlas_study"
)

type atlasFirstAcceptanceProvider struct {
	t                      *testing.T
	repositoryType         atlasstudy.RepositoryType
	rejectArchitecture     bool
	rejectNavigator        bool
	failArchitectureCall   bool
	failNavigatorCall      bool
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
		atlasFirstStageStudy,
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
	assertAtlasFirstAcceptedStudy(t, data, 1)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 3, map[string]string{
		debugdump.SemanticStageNavigator:    "accepted",
		debugdump.SemanticStageArchitecture: "accepted",
		debugdump.SemanticStageAtlasStudy:   "accepted",
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

	resultBytes, err := os.ReadFile(filepath.Join(runDir, atlasstudy.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	result, err := atlasstudy.DecodeResultRecord(resultBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Diagnostics.DirectionsReceived != 2 ||
		result.Diagnostics.DirectionsAccepted != 1 ||
		result.Diagnostics.DirectionsRejected != 1 ||
		len(result.Diagnostics.Issues) != 1 ||
		result.Diagnostics.Issues[0].Code != atlasstudy.IssueUnknownRef {
		t.Fatalf("invalid sibling did not preserve valid Brief/route: %#v", result.Diagnostics)
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

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudy)
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
	assertAtlasFirstAcceptedStudy(t, data, 1)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, 2, map[string]string{
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

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudy)
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
	assertAtlasFirstAcceptedStudy(t, data, 1)
	assertAtlasFirstDiagnostics(t, runDir, 2, map[string]string{
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
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
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
		atlasFirstStageStudy,
	)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateFailed ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("failed Navigator report = %#v", data.Navigator)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data, 1)
	assertAtlasFirstDiagnostics(t, runDir, 3, map[string]string{
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
		atlasFirstStageStudy,
	)
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateFailed ||
		data.Navigator.FailureCode != navigator.FailureProvider ||
		data.Navigator.Recommendation != nil {
		t.Fatalf("provider-failed Navigator report = %#v", data.Navigator)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data, 1)
	assertAtlasFirstDiagnostics(t, runDir, 3, map[string]string{
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

	provider.assertStages(t, atlasFirstStageNavigator, atlasFirstStageArchitecture)
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
	if data.AtlasStudy == nil ||
		data.AtlasStudy.State != atlasstudy.ProductStateUnavailable ||
		data.AtlasStudy.UnavailableCode != report.AtlasStudyUnavailableInsufficientCatalog ||
		data.StudyMap != nil {
		t.Fatalf("provider-failed Architecture Study state = %#v / %#v", data.AtlasStudy, data.StudyMap)
	}
	assertAtlasFirstDiagnostics(t, runDir, 2, map[string]string{
		debugdump.SemanticStageNavigator:    "accepted",
		debugdump.SemanticStageArchitecture: "failed",
		debugdump.SemanticStageAtlasStudy:   "unavailable",
	})
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
	case atlasFirstStageStudy:
		response, err = atlasFirstAcceptanceStudyResponse(
			combined,
			provider.repositoryType,
			provider.includeBadStudySibling,
		)
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
	case strings.Contains(combined, "Use member, anchor, and flow refs as opaque request-local typed values.") &&
		strings.Contains(combined, "Bounded candidate request:\n"):
		return atlasFirstStageArchitecture, combined, nil
	case strings.Contains(combined, "Catalog JSON:\n") &&
		strings.Contains(combined, "\"reading_targets\"") &&
		strings.Contains(combined, "Response schema: {\"repository_type\""):
		return atlasFirstStageStudy, combined, nil
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

func atlasFirstAcceptanceStudyResponse(
	combined string,
	repositoryType atlasstudy.RepositoryType,
	includeBadSibling bool,
) ([]byte, error) {
	const startMarker = "Catalog JSON:\n"
	const endMarker = "\n\nResponse schema:"
	start := strings.LastIndex(combined, startMarker)
	if start < 0 {
		return nil, fmt.Errorf("Atlas Study catalog marker is absent")
	}
	start += len(startMarker)
	end := strings.Index(combined[start:], endMarker)
	if end < 0 {
		return nil, fmt.Errorf("Atlas Study schema marker is absent")
	}
	var wire struct {
		Components []struct {
			Ref               string   `json:"ref"`
			ReadingTargetRefs []string `json:"reading_target_refs"`
		} `json:"components"`
		Surfaces []struct {
			Ref               string   `json:"ref"`
			ReadingTargetRefs []string `json:"reading_target_refs"`
		} `json:"surfaces"`
		ReadingTargets []struct {
			Ref           string   `json:"ref"`
			Path          string   `json:"path"`
			PrincipalRefs []string `json:"principal_refs"`
		} `json:"reading_targets"`
	}
	if err := json.Unmarshal([]byte(combined[start:start+end]), &wire); err != nil {
		return nil, fmt.Errorf("decode Atlas Study wire: %w", err)
	}
	if len(wire.Components) == 0 {
		return nil, fmt.Errorf("Atlas Study wire has no component")
	}
	componentRef := wire.Components[0].Ref
	knownPrincipals := make(map[string]struct{}, len(wire.Components)+len(wire.Surfaces))
	for _, component := range wire.Components {
		knownPrincipals[component.Ref] = struct{}{}
	}
	for _, surface := range wire.Surfaces {
		knownPrincipals[surface.Ref] = struct{}{}
	}
	var targetRefs []string
	var principalRefs []string
	principalSet := make(map[string]struct{})
	addPrincipal := func(ref string) {
		if _, duplicate := principalSet[ref]; duplicate {
			return
		}
		principalSet[ref] = struct{}{}
		principalRefs = append(principalRefs, ref)
	}
	addPrincipal(componentRef)
	for _, target := range wire.ReadingTargets {
		if target.Path == "a_package.go" {
			return nil, fmt.Errorf("package-declaration-only source became a Study reading target")
		}
		targetPrincipal := ""
		for _, principal := range target.PrincipalRefs {
			if _, known := knownPrincipals[principal]; known {
				targetPrincipal = principal
				break
			}
		}
		if targetPrincipal == "" {
			continue
		}
		if len(targetRefs) == 3 {
			break
		}
		targetRefs = append(targetRefs, target.Ref)
		addPrincipal(targetPrincipal)
	}
	if len(targetRefs) < 3 {
		return nil, fmt.Errorf(
			"Atlas Study wire has %d exact reading targets across %d components and %d surfaces",
			len(targetRefs), len(wire.Components), len(wire.Surfaces),
		)
	}
	if len(principalRefs) > 5 {
		return nil, fmt.Errorf("Atlas Study fixture requires too many principals")
	}
	brief := map[string]any{
		"what_it_is": map[string]any{
			"text":         "A repository described by an accepted conceptual component.",
			"support_refs": []string{componentRef},
		},
		"problem": map[string]any{
			"text":         "It provides a bounded codebase for repository work.",
			"support_refs": []string{componentRef},
		},
		"main_input": map[string]any{
			"text":         "Repository work is represented by exact local facts.",
			"support_refs": []string{componentRef},
		},
		"central_responsibility": map[string]any{
			"text":         "The accepted component groups related repository responsibilities.",
			"support_refs": []string{componentRef},
		},
		"observable_result": map[string]any{
			"text":         "The report exposes source-backed reading targets.",
			"support_refs": []string{componentRef},
		},
	}
	valid := map[string]any{
		"question":         "Which exact sources explain the accepted repository component?",
		"why_it_matters":   "The route keeps the conceptual component tied to exact local reading targets.",
		"learning_outcome": "The reader can identify three source-backed responsibilities.",
		"target_job":       string(atlasstudy.JobFirstContact),
		"learning_stage":   string(atlasstudy.StageOrientation),
		"principal_refs":   principalRefs,
		"reading": []any{
			map[string]any{"target_ref": targetRefs[0], "label": string(atlasstudy.ReadingStart), "what_to_look_for": "Inspect the first exact responsibility."},
			map[string]any{"target_ref": targetRefs[1], "label": string(atlasstudy.ReadingConnect), "what_to_look_for": "Inspect the related local responsibility."},
			map[string]any{"target_ref": targetRefs[2], "label": string(atlasstudy.ReadingVerify), "what_to_look_for": "Confirm the third source-backed responsibility."},
		},
	}
	directions := []any{valid}
	if includeBadSibling {
		directions = append(directions, map[string]any{
			"question":         "Which invalid route must remain isolated?",
			"why_it_matters":   "This deliberately invalid sibling proves item-local rejection.",
			"learning_outcome": "The valid Brief and route remain available.",
			"target_job":       string(atlasstudy.JobFirstContact),
			"learning_stage":   string(atlasstudy.StageOrientation),
			"principal_refs":   principalRefs,
			"reading": []any{
				map[string]any{"target_ref": targetRefs[0], "label": string(atlasstudy.ReadingStart), "what_to_look_for": "Inspect the exact source."},
				map[string]any{"target_ref": targetRefs[1], "label": string(atlasstudy.ReadingConnect), "what_to_look_for": "Inspect the related source."},
				map[string]any{"target_ref": "r999999", "label": string(atlasstudy.ReadingVerify), "what_to_look_for": "This ref is deliberately unknown."},
			},
		})
	}
	content, err := json.Marshal(map[string]any{
		"repository_type": string(repositoryType),
		"brief":           brief,
		"directions":      directions,
	})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 377, 211), nil
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

func assertAtlasFirstAcceptedStudy(t *testing.T, data *report.ReportData, wantDirections int) {
	t.Helper()
	if data.AtlasStudy == nil || data.AtlasStudy.State != atlasstudy.ProductStateAccepted ||
		data.AtlasStudy.DirectionCount != wantDirections || data.StudyMap == nil ||
		len(data.StudyMap.Directions) != wantDirections || len(data.StudyMap.Brief.WhatItIs) == 0 {
		t.Fatalf("accepted Atlas Study = %#v / %#v", data.AtlasStudy, data.StudyMap)
	}
	for _, direction := range data.StudyMap.Directions {
		if len(direction.ReadingAnchors) < 3 {
			t.Fatalf("Study route has fewer than three exact reading anchors: %#v", direction)
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
	wantCandidates := append([]componentmap.Candidate(nil), input.CandidateBundle.Candidates...)
	sort.Slice(wantCandidates, func(i, j int) bool {
		if wantCandidates[i].ID.Kind != wantCandidates[j].ID.Kind {
			return wantCandidates[i].ID.Kind < wantCandidates[j].ID.Kind
		}
		return wantCandidates[i].ID.Value < wantCandidates[j].ID.Value
	})
	if !reflect.DeepEqual(gotCandidates, wantCandidates) {
		t.Fatalf("model grouping changed exact local candidates\ngot:  %#v\nwant: %#v", gotCandidates, wantCandidates)
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
	// ceiling on future independently approved Atlas questions.
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
		if attempt.ProviderCallCount < 0 || attempt.ProviderCallCount > 1 {
			t.Fatalf("Atlas-first stage call count is not diagnostic one-call state: %#v", attempt)
		}
	}
}

func assertAtlasFirstSemanticStages(t *testing.T, runDir string, want ...string) {
	t.Helper()
	entries := readSemanticJournalEntries(t, runDir)
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
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
	for _, name := range []string{
		navigator.StatusArtifactFilename,
		navigator.RecordArtifactFilename,
		report.ArchitectureSynthesisFile,
		report.ArchitectureSynthesisStatusFile,
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
		"report.json",
		"report.html",
		report.RunManifestFilename,
	} {
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
		inputs.AtlasStudyRequestSHA256 == "" ||
		inputs.AtlasStudyResultSHA256 == "" ||
		inputs.AtlasStudyStatusSHA256 == "" ||
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
