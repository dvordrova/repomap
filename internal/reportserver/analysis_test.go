package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/report"
)

type recordingLocationResolver struct {
	requests   []analysis.LocationRequest
	resolution analysis.LocationResolution
	err        error
}

type recordingExactAnalyzer struct {
	requests []analysis.ExactSymbolRequest
	graph    evidence.Graph
	err      error
}

type recordingReferenceFinder struct {
	repoPaths []string
	locations []evidence.Location
	result    evidence.LocationSet
	err       error
}

func (a *recordingExactAnalyzer) AnalyzeExactSymbol(_ context.Context, request analysis.ExactSymbolRequest) (evidence.Graph, error) {
	a.requests = append(a.requests, request)
	return a.graph, a.err
}

func (f *recordingReferenceFinder) References(
	_ context.Context,
	repoPath string,
	location evidence.Location,
) (evidence.LocationSet, error) {
	f.repoPaths = append(f.repoPaths, repoPath)
	f.locations = append(f.locations, location)
	return f.result, f.err
}

func (r *recordingLocationResolver) ResolveLocation(_ context.Context, request analysis.LocationRequest) (analysis.LocationResolution, error) {
	r.requests = append(r.requests, request)
	return r.resolution, r.err
}

func TestSymbolsEndpointUsesManifestAuthorityAndReturnsOnlyCallableCandidates(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	resolver := &recordingLocationResolver{resolution: analysis.LocationResolution{
		Location: evidence.Location{Path: "batch.go", Line: 395},
		Candidates: []analysis.LocationCandidate{
			{
				Entity:       evidence.Entity{Kind: evidence.EntityType, Name: "batchInternal", Location: &evidence.Location{Path: "batch.go", Line: 310}},
				Match:        "preceding_declaration",
				Certainty:    evidence.CertaintyPossible,
				Investigable: false,
			},
			{
				Entity:       evidence.Entity{Kind: evidence.EntityMethod, Name: "(*Batch).Commit", Location: &evidence.Location{Path: "batch.go", Line: 1571, Column: 1}},
				Match:        "file_callable",
				Certainty:    evidence.CertaintyPossible,
				Distance:     1176,
				Investigable: true,
				RankReasons:  []string{"component term 'batch'", "component term 'commit'"},
			},
			{
				Entity:       evidence.Entity{Kind: evidence.EntityFunction, Name: "other", Location: &evidence.Location{Path: "other.go", Line: 8}},
				Match:        "file_callable",
				Certainty:    evidence.CertaintyPossible,
				Investigable: true,
			},
		},
		Certainty: evidence.CertaintyPossible,
		Provenance: evidence.Provenance{
			Provider:  "gopls",
			Version:   "fixture",
			Operation: "document_symbols",
			Detail:    repo,
		},
	}}
	handler, err := NewHandler(Options{
		RunsDir:          runsDir,
		Capability:       testCapability,
		LocationResolver: resolver,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	response := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var result symbolsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.CandidateSetID == "" || len(result.Candidates) != 1 {
		t.Fatalf("symbols response = %#v", result)
	}
	candidate := result.Candidates[0]
	if candidate.Name != "(*Batch).Commit" || candidate.Path != "batch.go" || candidate.Line != 1571 || candidate.ID == "" {
		t.Fatalf("candidate = %#v", candidate)
	}
	if result.Provenance.Provider != "gopls" || strings.Contains(response.Body.String(), repo) {
		t.Fatalf("response leaked repository path or lost provenance: %s", response.Body.String())
	}
	if len(resolver.requests) != 1 {
		t.Fatalf("resolver calls = %d", len(resolver.requests))
	}
	request := resolver.requests[0]
	if request.RepoPath != repo || request.Location.Path != "batch.go" || request.Location.Line != 395 || request.MaxCandidates != 20 {
		t.Fatalf("resolver request = %#v", request)
	}
	for _, want := range []string{"batch", "commit", "wal"} {
		if !containsString(request.RankTerms, want) {
			t.Fatalf("rank terms %v do not contain %q", request.RankTerms, want)
		}
	}
}

func TestSymbolsEndpointRejectsStaleOrUnauthorizedRequestsBeforeGopls(t *testing.T) {
	_, runsDir, state := writeAnalysisRun(t)
	tests := []struct {
		name       string
		request    symbolsRequest
		capture    freshness.RepositoryState
		wantStatus int
	}{
		{
			name:    "stale repository",
			request: symbolsRequest{RunID: "20260711-220000-pebble", ComponentID: "component-batch", AnchorID: "anchor-batch", Line: 395},
			capture: func() freshness.RepositoryState {
				changed := state
				changed.Dirty = append([]freshness.DirtyFile(nil), state.Dirty...)
				changed.Dirty[0].ContentSHA256 = strings.Repeat("1", 64)
				return changed
			}(),
			wantStatus: http.StatusConflict,
		},
		{
			name:       "line not present in manifest",
			request:    symbolsRequest{RunID: "20260711-220000-pebble", ComponentID: "component-batch", AnchorID: "anchor-batch", Line: 396},
			capture:    state,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "unknown anchor id",
			request:    symbolsRequest{RunID: "20260711-220000-pebble", ComponentID: "component-batch", AnchorID: "invented", Line: 395},
			capture:    state,
			wantStatus: http.StatusForbidden,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &recordingLocationResolver{}
			handler, err := NewHandler(Options{
				RunsDir:          runsDir,
				Capability:       testCapability,
				LocationResolver: resolver,
				CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
					return test.capture, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			response := performSymbolsRequest(t, handler, test.request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if len(resolver.requests) != 0 {
				t.Fatalf("resolver was called for rejected request: %#v", resolver.requests)
			}
		})
	}
}

func TestSymbolsEndpointMapsExactSourceChangeToExistingStaleResponse(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	resolver := &recordingLocationResolver{}
	handler, err := NewHandler(Options{
		RunsDir:          runsDir,
		Capability:       testCapability,
		LocationResolver: resolver,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "report is stale") ||
		strings.Contains(response.Body.String(), repo) || len(resolver.requests) != 0 {
		t.Fatalf("changed-source response=%d body=%s calls=%d", response.Code, response.Body.String(), len(resolver.requests))
	}
}

func TestInspectSymbolEndpointAcceptsCanonicalSlashIdentityAndReturnsBoundedLocalEvidence(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	rewriteRunReportName(t, runsDir, "20260711-220000-pebble", "example.com/cockroachdb/pebble")
	target := evidence.Entity{
		ID:       "function:batch.go:3:1:Commit",
		Kind:     evidence.EntityFunction,
		Name:     "Commit",
		Language: "go",
		Location: &evidence.Location{Path: "batch.go", Line: 3, Column: 1},
	}
	resolver := &recordingLocationResolver{resolution: analysis.LocationResolution{
		Location: evidence.Location{Path: "batch.go", Line: 395},
		Candidates: []analysis.LocationCandidate{{
			Entity:       target,
			Match:        "file_callable",
			Certainty:    evidence.CertaintyPossible,
			Distance:     392,
			Investigable: true,
		}},
		Certainty:  evidence.CertaintyPossible,
		Provenance: evidence.Provenance{Provider: "gopls", Operation: "document_symbols"},
	}}
	exact := &recordingExactAnalyzer{graph: exactGraphFixture(repo, target)}
	handler, err := NewHandler(Options{
		RunsDir:             runsDir,
		Capability:          testCapability,
		LocationResolver:    resolver,
		ExactSymbolAnalyzer: exact,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
		CaptureFactContext: func(context.Context, freshness.RepositoryState, string) (freshness.FactContext, error) {
			return browserTestFactContext(state), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lookup := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, body=%s", lookup.Code, lookup.Body.String())
	}
	var candidates symbolsResponse
	if err := json.Unmarshal(lookup.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	inspect := performInspectRequest(t, handler, inspectSymbolRequest{
		RunID:          "20260711-220000-pebble",
		CandidateSetID: candidates.CandidateSetID,
		CandidateID:    candidates.Candidates[0].ID,
	})
	if inspect.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body=%s", inspect.Code, inspect.Body.String())
	}
	var result inspectSymbolResponse
	if err := json.Unmarshal(inspect.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.EvidenceLevel != evidence.CertaintyStatic || result.Target.Name != "Commit" {
		t.Fatalf("inspect response = %#v", result)
	}
	if result.Source.Path != "batch.go" || result.Source.StartLine != 3 || len(result.Source.Lines) == 0 || len(result.Source.Lines) > 20 {
		t.Fatalf("source = %#v", result.Source)
	}
	if len(result.IncomingCalls) != 1 || result.IncomingCalls[0].Symbol.Name != "caller" ||
		len(result.OutgoingCalls) != 1 || result.OutgoingCalls[0].Symbol.Name != "writeWAL" {
		t.Fatalf("calls incoming=%#v outgoing=%#v", result.IncomingCalls, result.OutgoingCalls)
	}
	if strings.Contains(inspect.Body.String(), repo) || strings.Contains(inspect.Body.String(), "working_dir") {
		t.Fatalf("inspect response leaks local analysis context: %s", inspect.Body.String())
	}
	if len(exact.requests) != 1 || !reflect.DeepEqual(exact.requests[0].Symbol, target) {
		t.Fatalf("exact requests = %#v", exact.requests)
	}
}

func TestVersion3SymbolListAndExactInspectPreserveEndpointParity(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.Version = 3
	})
	target := evidence.Entity{
		ID:       "function:batch.go:3:1:Commit",
		Kind:     evidence.EntityFunction,
		Name:     "Commit",
		Language: "go",
		Location: &evidence.Location{Path: "batch.go", Line: 3, Column: 1},
	}
	resolver := &recordingLocationResolver{resolution: analysis.LocationResolution{
		Location: evidence.Location{Path: "batch.go", Line: 395},
		Candidates: []analysis.LocationCandidate{{
			Entity: target, Match: "file_callable", Certainty: evidence.CertaintyPossible, Investigable: true,
		}},
		Certainty:  evidence.CertaintyPossible,
		Provenance: evidence.Provenance{Provider: "gopls", Operation: "document_symbols"},
	}}
	exact := &recordingExactAnalyzer{graph: exactGraphFixture(repo, target)}
	handler, err := NewHandler(Options{
		RunsDir:             runsDir,
		Capability:          testCapability,
		LocationResolver:    resolver,
		ExactSymbolAnalyzer: exact,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
		CaptureFactContext: func(context.Context, freshness.RepositoryState, string) (freshness.FactContext, error) {
			return browserTestFactContext(state), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	lookup := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if lookup.Code != http.StatusOK {
		t.Fatalf("v3 lookup status = %d, body=%s", lookup.Code, lookup.Body.String())
	}
	var candidates symbolsResponse
	if err := json.Unmarshal(lookup.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if candidates.Status != "ok" || len(candidates.Candidates) != 1 ||
		candidates.Candidates[0].Name != target.Name {
		t.Fatalf("v3 candidates = %#v", candidates)
	}
	inspect := performInspectRequest(t, handler, inspectSymbolRequest{
		RunID:          "20260711-220000-pebble",
		CandidateSetID: candidates.CandidateSetID,
		CandidateID:    candidates.Candidates[0].ID,
	})
	if inspect.Code != http.StatusOK {
		t.Fatalf("v3 inspect status = %d, body=%s", inspect.Code, inspect.Body.String())
	}
	var result inspectSymbolResponse
	if err := json.Unmarshal(inspect.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Target.Name != target.Name ||
		result.Source.Path != "batch.go" || len(result.IncomingCalls) != 1 ||
		len(result.OutgoingCalls) != 1 || strings.Contains(inspect.Body.String(), repo) {
		t.Fatalf("v3 inspect response = %#v body=%s", result, inspect.Body.String())
	}
}

func TestVersion3SourceOpenKeepsLegacyOpaqueIDBehavior(t *testing.T) {
	repo, runsDir, _ := writeAnalysisRun(t)
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.Version = 3
	})
	sourceID := testSourceID(t, runsDir, "20260711-220000-pebble", "batch.go")
	var opened string
	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(_ context.Context, absolutePath string, _, _ int) error {
			opened = absolutePath
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(
		t,
		server.URL+capabilityURLPrefix(testCapability),
		openRequest{
			RunID:    "20260711-220000-pebble",
			SourceID: sourceID,
			Line:     3,
		},
		true,
	)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || opened != filepath.Join(repo, "batch.go") {
		t.Fatalf("v3 source open status=%d path=%q", response.StatusCode, opened)
	}
}

func TestCurrentManifestCatalogFailureDisablesAnalysisWithoutLeakingDiagnostic(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.CapturedInputs[0].Kind = freshness.FileMissing
		manifest.CapturedInputs[0].Mode = ""
		manifest.CapturedInputs[0].ContentSHA256 = ""
		digest, err := freshness.CapturedInputsDigest(manifest.CapturedInputs)
		if err != nil {
			t.Fatal(err)
		}
		manifest.CapturedInputsSHA256 = digest
	})
	resolver := &recordingLocationResolver{}
	var logs []string
	internal := &handler{
		runsDir: runsDir,
		analysis: newSymbolAnalysis(Options{
			LocationResolver: resolver,
		}),
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	runs, err := internal.loadRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Manifest == nil || runs[0].Report == nil ||
		runs[0].SourceCatalog != nil || runs[0].AnalysisAvailable {
		t.Fatalf("catalog-failed run = %#v", runs)
	}
	diagnostics := 0
	for _, log := range logs {
		if strings.Contains(log, "source catalog unavailable") {
			diagnostics++
		}
		if strings.Contains(log, repo) || strings.Contains(log, "captured input") {
			t.Fatalf("catalog diagnostic leaked local/error detail: %q", log)
		}
	}
	if diagnostics != 1 {
		t.Fatalf("catalog diagnostics = %d, logs=%v", diagnostics, logs)
	}

	handler, err := NewHandler(Options{
		RunsDir:          runsDir,
		Capability:       testCapability,
		LocationResolver: resolver,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), repo) ||
		strings.Contains(response.Body.String(), "catalog") || len(resolver.requests) != 0 {
		t.Fatalf("catalog-failed response=%d body=%s calls=%d", response.Code, response.Body.String(), len(resolver.requests))
	}
}

func TestInspectSymbolEndpointDelegatesCapturedHashAuthorityToInspectionService(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	target := evidence.Entity{
		ID:       "function:batch.go:3:1:Commit",
		Kind:     evidence.EntityFunction,
		Name:     "Commit",
		Language: "go",
		Location: &evidence.Location{Path: "batch.go", Line: 3, Column: 1},
	}
	resolver := &recordingLocationResolver{resolution: analysis.LocationResolution{
		Location: evidence.Location{Path: "batch.go", Line: 395},
		Candidates: []analysis.LocationCandidate{{
			Entity: target, Match: "file_callable", Certainty: evidence.CertaintyPossible, Investigable: true,
		}},
		Certainty:  evidence.CertaintyPossible,
		Provenance: evidence.Provenance{Provider: "gopls", Operation: "document_symbols"},
	}}
	exact := &recordingExactAnalyzer{graph: exactGraphFixture(repo, target)}
	handler, err := NewHandler(Options{
		RunsDir:             runsDir,
		Capability:          testCapability,
		LocationResolver:    resolver,
		ExactSymbolAnalyzer: exact,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
		CaptureFactContext: func(context.Context, freshness.RepositoryState, string) (freshness.FactContext, error) {
			t.Fatal("fact capture must not run after an exact source hash mismatch")
			return freshness.FactContext{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	lookup := performSymbolsRequest(t, handler, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, body=%s", lookup.Code, lookup.Body.String())
	}
	var candidates symbolsResponse
	if err := json.Unmarshal(lookup.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspect := performInspectRequest(t, handler, inspectSymbolRequest{
		RunID:          "20260711-220000-pebble",
		CandidateSetID: candidates.CandidateSetID,
		CandidateID:    candidates.Candidates[0].ID,
	})
	if inspect.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(inspect.Body.String(), "could not read a bounded source window") ||
		strings.Contains(inspect.Body.String(), repo) {
		t.Fatalf("inspect status=%d body=%s", inspect.Code, inspect.Body.String())
	}
	sessionPath := filepath.Join(
		runsDir,
		"20260711-220000-pebble",
		investigationDirectory,
		memory.SessionFileName,
	)
	if _, err := os.Lstat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected checkpoint after source mismatch: %v", err)
	}
}

func TestInvestigationCheckpointResumesAfterHandlerRestartAndFindsTests(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	target := evidence.Entity{
		ID:       "function:batch.go:3:1:Commit",
		Kind:     evidence.EntityFunction,
		Name:     "Commit",
		Language: "go",
		Location: &evidence.Location{Path: "batch.go", Line: 3, Column: 1},
	}
	resolver := &recordingLocationResolver{resolution: analysis.LocationResolution{
		Location: evidence.Location{Path: "batch.go", Line: 395},
		Candidates: []analysis.LocationCandidate{{
			Entity:       target,
			Match:        "file_callable",
			Certainty:    evidence.CertaintyPossible,
			Distance:     392,
			Investigable: true,
		}},
		Certainty:  evidence.CertaintyPossible,
		Provenance: evidence.Provenance{Provider: "gopls", Operation: "document_symbols"},
	}}
	exact := &recordingExactAnalyzer{graph: exactGraphFixture(repo, target)}
	captureRepository := func(context.Context, string) (freshness.RepositoryState, error) {
		return state, nil
	}
	captureFacts := func(context.Context, freshness.RepositoryState, string) (freshness.FactContext, error) {
		return browserTestFactContext(state), nil
	}
	first, err := NewHandler(Options{
		RunsDir:             runsDir,
		Capability:          testCapability,
		LocationResolver:    resolver,
		ExactSymbolAnalyzer: exact,
		CaptureRepository:   captureRepository,
		CaptureFactContext:  captureFacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	missing := performInvestigationRequest(t, first, "latest", "resume-investigation", "20260711-220000-pebble")
	if missing.Code != http.StatusNoContent {
		t.Fatalf("missing latest status = %d, body=%s", missing.Code, missing.Body.String())
	}
	lookup := performSymbolsRequest(t, first, symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, body=%s", lookup.Code, lookup.Body.String())
	}
	var candidates symbolsResponse
	if err := json.Unmarshal(lookup.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	inspect := performInspectRequest(t, first, inspectSymbolRequest{
		RunID:          "20260711-220000-pebble",
		CandidateSetID: candidates.CandidateSetID,
		CandidateID:    candidates.Candidates[0].ID,
	})
	if inspect.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body=%s", inspect.Code, inspect.Body.String())
	}
	var sourceReady inspectSymbolResponse
	if err := json.Unmarshal(inspect.Body.Bytes(), &sourceReady); err != nil {
		t.Fatal(err)
	}
	if sourceReady.InvestigationStatus != "source_ready" || !sourceReady.CanFindTestReferences ||
		sourceReady.ComponentID != "component-batch" || sourceReady.AnchorID != "anchor-batch" {
		t.Fatalf("source-ready response = %#v", sourceReady)
	}
	checkpoint := filepath.Join(runsDir, "20260711-220000-pebble", investigationDirectory, "investigation_session.json")
	if info, err := os.Lstat(checkpoint); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("checkpoint info = %#v, err=%v", info, err)
	}

	finder := &recordingReferenceFinder{result: evidence.LocationSet{
		Locations:  []evidence.Location{{Path: "batch_test.go", Line: 12, Column: 2}},
		Certainty:  evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "gopls", Operation: "references"}},
		Scenarios:  []evidence.Scenario{{ID: "build", Name: "active build"}},
	}}
	restarted, err := NewHandler(Options{
		RunsDir:            runsDir,
		Capability:         testCapability,
		LocationResolver:   &recordingLocationResolver{},
		ReferenceFinder:    finder,
		CaptureRepository:  captureRepository,
		CaptureFactContext: captureFacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	latest := performInvestigationRequest(t, restarted, "latest", "resume-investigation", "20260711-220000-pebble")
	if latest.Code != http.StatusOK {
		t.Fatalf("latest status = %d, body=%s", latest.Code, latest.Body.String())
	}
	var resumed inspectSymbolResponse
	if err := json.Unmarshal(latest.Body.Bytes(), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.InvestigationStatus != "source_ready" || resumed.Target.Name != target.Name {
		t.Fatalf("resumed response = %#v", resumed)
	}
	tests := performInvestigationRequest(t, restarted, "target-tests", "find-test-references", "20260711-220000-pebble")
	if tests.Code != http.StatusOK {
		t.Fatalf("target-tests status = %d, body=%s", tests.Code, tests.Body.String())
	}
	var testsReady inspectSymbolResponse
	if err := json.Unmarshal(tests.Body.Bytes(), &testsReady); err != nil {
		t.Fatal(err)
	}
	if testsReady.InvestigationStatus != "tests_ready" || testsReady.CanFindTestReferences ||
		len(testsReady.TestReferences) != 1 || testsReady.TestReferences[0].Path != "batch_test.go" ||
		len(finder.locations) != 1 || !reflect.DeepEqual(finder.locations[0], *target.Location) {
		t.Fatalf("tests-ready response = %#v; finder locations=%#v", testsReady, finder.locations)
	}
	if strings.Contains(tests.Body.String(), repo) || strings.Contains(tests.Body.String(), "step-") ||
		strings.Contains(tests.Body.String(), "report_sha256") {
		t.Fatalf("tests response leaked internal state: %s", tests.Body.String())
	}
	repeated := performInvestigationRequest(t, restarted, "target-tests", "find-test-references", "20260711-220000-pebble")
	if repeated.Code != http.StatusOK || len(finder.locations) != 1 {
		t.Fatalf("repeated status = %d, calls=%d, body=%s", repeated.Code, len(finder.locations), repeated.Body.String())
	}
}

func performSymbolsRequest(t *testing.T, handler http.Handler, request symbolsRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	prefix := capabilityURLPrefix(testCapability)
	httpRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+prefix+"/api/symbols", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Origin", "http://127.0.0.1")
	httpRequest.Header.Set("X-Repomap-Action", "list-symbols")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func performInspectRequest(t *testing.T, handler http.Handler, request inspectSymbolRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	prefix := capabilityURLPrefix(testCapability)
	httpRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+prefix+"/api/symbol", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Origin", "http://127.0.0.1")
	httpRequest.Header.Set("X-Repomap-Action", "inspect-symbol")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func performInvestigationRequest(
	t *testing.T,
	handler http.Handler,
	endpoint string,
	action string,
	runID string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(investigationRequest{RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	prefix := capabilityURLPrefix(testCapability)
	httpRequest := httptest.NewRequest(
		http.MethodPost,
		"http://127.0.0.1"+prefix+"/api/investigation/"+endpoint,
		bytes.NewReader(body),
	)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Origin", "http://127.0.0.1")
	httpRequest.Header.Set("X-Repomap-Action", action)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func writeAnalysisRun(t *testing.T) (string, string, freshness.RepositoryState) {
	t.Helper()
	repo := t.TempDir()
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	source := `package pebble

func Commit() {
	writeWAL()
}

func writeWAL() {}

func caller() {
	Commit()
}
`
	sourceBytes := []byte(source)
	sourceSHA256 := fmt.Sprintf("%x", sha256.Sum256(sourceBytes))
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	runID := "20260711-220000-pebble"
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), []byte("report"), 0o600); err != nil {
		t.Fatal(err)
	}
	component := report.Component{
		ID:           "component-batch",
		Name:         "Batch Operations",
		ModelPurpose: "commit writes through the WAL",
		AnchorGroups: []report.AnchorGroup{{
			ID:             "anchor-batch",
			Path:           "batch.go",
			Grounding:      "model_evidence",
			Locations:      []evidence.Location{{Path: "batch.go", Line: 395}},
			ModelNotes:     []string{"fsync during commit"},
			CanListSymbols: true,
		}},
	}
	reportData := report.ReportData{
		FormatVersion: report.CurrentFormatVersion,
		RepoName:      "pebble",
		OpenablePaths: []string{"batch.go"},
		Components:    []report.Component{component},
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: filepath.Clean(repo),
		Head:     strings.Repeat("0", 40),
		Dirty: []freshness.DirtyFile{{
			Status: "modified", Path: "batch.go", Kind: freshness.FileRegular,
			ContentSHA256: sourceSHA256,
		}},
	}
	digest, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	inputs := []freshness.CapturedInput{{
		Version: freshness.CapturedInputVersion, ID: strings.Repeat("b", 64), Path: "batch.go",
		Kind: freshness.FileRegular, Mode: "file", ContentSHA256: sourceSHA256,
		Stages: []string{"report_evidence"},
	}}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest := report.RunManifest{
		Version:               report.CurrentRunManifestVersion,
		RepositoryState:       state,
		AnalysisRoot:          filepath.Clean(repo),
		RepositoryStateSHA256: digest,
		ReportSHA256:          fmt.Sprintf("%x", sha256.Sum256(reportJSON)),
		ReportFormatVersion:   report.CurrentFormatVersion,
		OpenablePaths:         []string{"batch.go"},
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		Freshness:             freshness.NewFreshnessResult(freshness.FreshnessFresh),
		MaterialInputs: report.MaterialInputs{
			SelectedRevision: state.Head, InputPolicyVersion: "captured-inputs-v1",
			ArchitectureContract: 1, ReportContract: report.CurrentFormatVersion,
		},
		Components: []report.ComponentAuthority{{
			ID: "component-batch",
			Anchors: []report.AnchorAuthority{{
				ID:             "anchor-batch",
				Path:           "batch.go",
				AllowedLines:   []int{395},
				CanListSymbols: true,
			}},
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, report.RunManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	return repo, runsDir, state
}

func rewriteAnalysisManifest(
	t *testing.T,
	runsDir string,
	mutate func(*report.RunManifest),
) {
	t.Helper()
	path := filepath.Join(runsDir, "20260711-220000-pebble", report.RunManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := report.DecodeRunManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func exactGraphFixture(repo string, target evidence.Entity) evidence.Graph {
	graph := evidence.NewGraph(repo, target.Name)
	graph.Build = evidence.BuildContext{GOOS: "darwin", GOARCH: "amd64"}
	graph.Scenarios = []evidence.Scenario{{ID: "build", Name: "active build", WorkingDir: repo, Build: graph.Build}}
	query := evidence.Entity{ID: "query:commit", Kind: evidence.EntityQuery, Name: target.Name}
	caller := evidence.Entity{ID: "function:batch.go:9:1:caller", Kind: evidence.EntityFunction, Name: "caller", Language: "go", Location: &evidence.Location{Path: "batch.go", Line: 9, Column: 1}}
	callee := evidence.Entity{ID: "function:batch.go:7:1:writeWAL", Kind: evidence.EntityFunction, Name: "writeWAL", Language: "go", Location: &evidence.Location{Path: "batch.go", Line: 7, Column: 1}}
	for _, entity := range []evidence.Entity{query, target, caller, callee} {
		graph.AddEntity(entity)
	}
	provenance := func(path string, line int) []evidence.Provenance {
		return []evidence.Provenance{{Provider: "gopls", Operation: "call_hierarchy", Location: &evidence.Location{Path: path, Line: line, Column: 1}}}
	}
	graph.AddRelation(evidence.Relation{From: query.ID, To: target.ID, Kind: evidence.RelationMatchesQuery, Certainty: evidence.CertaintyStatic, Provenance: provenance("batch.go", 3), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: query.ID, To: target.ID, Kind: evidence.RelationResolvesTo, Certainty: evidence.CertaintyStatic, Provenance: provenance("batch.go", 3), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: caller.ID, To: target.ID, Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic, Provenance: provenance("batch.go", 10), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: target.ID, To: callee.ID, Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic, Provenance: provenance("batch.go", 4), Scenarios: []string{"build"}})
	graph.Sort()
	return graph
}

func browserTestFactContext(state freshness.RepositoryState) freshness.FactContext {
	return freshness.FactContext{
		Version:          freshness.FactContextVersion,
		Repository:       state,
		GoVersion:        "go1.24.0",
		Analyzer:         "gopls",
		AnalyzerVersion:  "v0.19.0",
		Collector:        "browser-investigation",
		CollectorVersion: "fixture-v1",
		InputsSHA256:     strings.Repeat("d", 64),
		Build: evidence.BuildContext{
			GOOS:   "darwin",
			GOARCH: "amd64",
		},
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
