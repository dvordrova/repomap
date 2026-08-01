package orient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llmbundle"
)

func TestRunDumpsInspectableRequestBeforeProviderFailure(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/trial\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "trial"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cmd", "trial", "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "cmd/trial/main.go")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported response_format"}`))
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "company-test-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "128")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	debugDir := t.TempDir()
	runID := "failed-provider"
	_, err := Run(context.Background(), Options{
		RepoPath:            repo,
		OutputJSON:          true,
		RunID:               runID,
		DebugDir:            debugDir,
		RequireArtifacts:    true,
		DumpRedacted:        true,
		MaxReadmeBytes:      1024,
		MaxReadmeLLMBytes:   512,
		MaxTreeLines:        50,
		MaxInterestingFiles: 50,
		MaxGoPkgs:           50,
		MaxGoEdges:          50,
		MaxLLMEntrypoints:   10,
		MaxLLMModules:       10,
		MaxLLMFiles:         50,
		MaxLLMEdges:         50,
		EffectiveOptions: debugdump.EffectiveOptions{
			FlowCount:        2,
			DiscoverSurfaces: true,
			OutputJSON:       true,
			NoOpen:           true,
			Port:             59769,
			DebugEnabled:     true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("Run() error = %v", err)
	}

	runDir := filepath.Join(debugDir, runID)
	request, err := os.ReadFile(filepath.Join(runDir, "llm_request.redacted.json"))
	if err != nil {
		t.Fatalf("read request artifact: %v", err)
	}
	if !strings.Contains(string(request), `"model":"company-test-model"`) || !strings.Contains(string(request), `"json_object"`) {
		t.Fatalf("request artifact does not describe the attempted request: %s", request)
	}
	semanticRecords := readOrientationSemanticRecords(t, runDir)
	if len(semanticRecords) != 1 ||
		semanticRecords[0].Stage != debugdump.SemanticStageOrientation ||
		semanticRecords[0].State != debugdump.SemanticStateProviderFailed ||
		semanticRecords[0].ValidationCode != debugdump.SemanticValidationProvider ||
		semanticRecords[0].RequestProvenance != debugdump.SemanticRequestPrepared ||
		semanticRecords[0].SemanticCalls != 1 ||
		semanticRecords[0].TransportAttempts != 1 ||
		semanticRecords[0].Response.Storage != "raw_unavailable" {
		t.Fatalf("failed orientation semantic exchange = %#v", semanticRecords)
	}

	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata artifact: %v", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Model != "company-test-model" || metadata.Endpoint != server.URL ||
		metadata.PromptVersion != deepseek.OrientationPromptVersionJSON {
		t.Fatalf("metadata model/endpoint/prompt = %q / %q / %q", metadata.Model, metadata.Endpoint, metadata.PromptVersion)
	}
	if metadata.CompactContextBytes <= 0 || metadata.ExternalRequestBytes <= metadata.CompactContextBytes {
		t.Fatalf(
			"metadata compact/external bytes = %d / %d",
			metadata.CompactContextBytes,
			metadata.ExternalRequestBytes,
		)
	}
	if metadata.ProviderRequestCount != 1 {
		t.Fatalf("metadata provider request count = %d, want 1", metadata.ProviderRequestCount)
	}
	if metadata.ProviderLatencyMillis == nil || *metadata.ProviderLatencyMillis < 0 {
		t.Fatalf("metadata provider latency = %v", metadata.ProviderLatencyMillis)
	}
	if metadata.AuthMode != "none" || metadata.TimeoutMillis != 5000 || metadata.MaxTokens != 128 {
		t.Fatalf(
			"metadata auth/timeout/tokens = %q / %d / %d",
			metadata.AuthMode,
			metadata.TimeoutMillis,
			metadata.MaxTokens,
		)
	}
	if metadata.EffectiveOptions.FlowCount != 2 || !metadata.EffectiveOptions.DiscoverSurfaces ||
		metadata.EffectiveOptions.DumpLLM || !metadata.EffectiveOptions.OutputJSON ||
		!metadata.EffectiveOptions.NoOpen || metadata.EffectiveOptions.Port != 59769 ||
		!metadata.EffectiveOptions.DebugEnabled {
		t.Fatalf("metadata effective options = %#v", metadata.EffectiveOptions)
	}
	if len(metadata.RequestAttempts) != 1 {
		t.Fatalf("metadata request attempts = %#v, want one", metadata.RequestAttempts)
	}
	attempt := metadata.RequestAttempts[0]
	if attempt.Stage != "orientation" || attempt.State != "failed" ||
		attempt.RequestBytes != metadata.ExternalRequestBytes || attempt.ProviderCallCount != 1 ||
		attempt.LatencyMillis == nil {
		t.Fatalf("metadata request attempt = %#v", attempt)
	}
	if strings.Contains(string(metadataBytes), "Authorization") || strings.Contains(string(metadataBytes), "REPOMAP_LLM_API_KEY") {
		t.Fatalf("metadata contains secret-bearing configuration names: %s", metadataBytes)
	}
}

func TestRunDumpLLMAbortsBeforeNetworkWhenDebugWriterFails(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/dump-failure\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "company-test-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	blockedDebugDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedDebugDir, []byte("file blocks mkdir"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		RepoPath:     repo,
		OutputJSON:   true,
		RunID:        "must-not-call-provider",
		DebugDir:     blockedDebugDir,
		DumpLLM:      true,
		DumpRedacted: true,
	})
	if err == nil || !strings.Contains(err.Error(), "create required debug writer") {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("provider requests = %d, want 0", requests)
	}

	_, err = Run(context.Background(), Options{
		RepoPath:         repo,
		OutputJSON:       true,
		RunID:            "required-browser-artifacts",
		DebugDir:         blockedDebugDir,
		DumpRedacted:     true,
		RequireArtifacts: true,
	})
	if err == nil || !strings.Contains(err.Error(), "create required debug writer") {
		t.Fatalf("Run() required-artifact error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("required-artifact failure made %d provider request(s), want 0", requests)
	}
}

func TestRunOfflineRespectsZeroFlowExpansion(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/offline\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	output, err := Run(context.Background(), Options{
		RepoPath:   repo,
		Offline:    true,
		OutputJSON: true,
		FlowCount:  0,
	})
	if err != nil {
		t.Fatal(err)
	}
	var report combinedReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.ExplainedFlows) != 0 {
		t.Fatalf("explained flows = %d, want 0", len(report.ExplainedFlows))
	}
}

func TestRunOfflineWritesExactBoundedOrientationContextSelection(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/selection\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(`package main

import "time"

func main() {
	time.NewTicker(time.Second)
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "private"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "private", "blob.bin"), []byte("must-not-leak-source-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go", "private/blob.bin")

	debugDir := t.TempDir()
	runID := "selection-manifest"
	if _, err := Run(context.Background(), Options{
		RepoPath:         repo,
		Offline:          true,
		OutputJSON:       true,
		FlowCount:        0,
		RunID:            runID,
		DebugDir:         debugDir,
		RequireArtifacts: true,
		MaxLLMFiles:      8,
		MaxLLMEdges:      8,
	}); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(debugDir, runID)
	manifestBytes, err := os.ReadFile(filepath.Join(runDir, llmbundle.OrientationContextSelectionFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := llmbundle.DecodeOrientationContextSelection(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := os.ReadFile(filepath.Join(runDir, "llm_bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle llmbundle.Bundle
	if err := json.Unmarshal(bundleBytes, &bundle); err != nil {
		t.Fatal(err)
	}
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	typedWire, err := buildOrientationWireBundle(bundle, catalog)
	if err != nil {
		t.Fatal(err)
	}
	bundleDigest := sha256.Sum256(bundleBytes)
	wireDigest := sha256.Sum256(typedWire)
	if manifest.CanonicalBundleSHA256 != hex.EncodeToString(bundleDigest[:]) ||
		manifest.CanonicalBundleBytes != len(bundleBytes) ||
		manifest.PersistedBundleSHA256 != hex.EncodeToString(bundleDigest[:]) ||
		manifest.PersistedBundleBytes != len(bundleBytes) ||
		manifest.TypedWireSHA256 != hex.EncodeToString(wireDigest[:]) ||
		manifest.TypedWireBytes != len(typedWire) {
		t.Fatalf("selection identity = %#v", manifest)
	}
	if len(manifest.SelectedCandidates) != len(bundle.CandidateFileIndex) {
		t.Fatalf("selected candidates = %d, bundle = %d", len(manifest.SelectedCandidates), len(bundle.CandidateFileIndex))
	}
	for index, candidate := range bundle.CandidateFileIndex {
		selected := manifest.SelectedCandidates[index]
		if selected.Path != candidate.Path || selected.Kind != candidate.Kind || selected.Score != candidate.Score ||
			strings.Join(selected.Reasons, "\x00") != strings.Join(candidate.Reasons, "\x00") ||
			strings.Join(selected.Signals, "\x00") != strings.Join(candidate.Signals, "\x00") {
			t.Fatalf("selected candidate %d differs: %#v / %#v", index, selected, candidate)
		}
	}
	if strings.Contains(string(manifestBytes), "must-not-leak-source-body") ||
		strings.Contains(string(manifestBytes), `"file_tree"`) {
		t.Fatalf("selection manifest leaked source body or full file tree: %s", manifestBytes)
	}
}

func TestRunWritesLocalEvidenceForEveryDirectionWithoutExtraModelCalls(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/onboarding\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "trial"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cmd", "trial", "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal", "worker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "worker", "worker.go"), []byte("package worker\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "cmd/trial/main.go", "internal/worker/worker.go")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		wire := orientationWireFromHTTPRequest(t, request)
		mainRef, mainEvidence := orientationWireFileRefs(t, wire, "cmd/trial/main.go")
		workerRef, workerEvidence := orientationWireFileRefs(t, wire, "internal/worker/worker.go")
		orientation, err := json.Marshal(orientationProviderResponse{
			ProjectGuess: "tiny worker command", Confidence: 0.9,
			CandidateFlows: []orientationProviderCandidateFlow{
				{
					Name: "Process startup", FlowType: "request", Trigger: "the executable starts",
					LikelyEntrypointRef: mainRef, LikelyFileRefs: []string{mainRef},
					WhyInteresting: "shows process wiring", EvidenceRefs: []string{mainEvidence}, Confidence: 0.9,
				},
				{
					Name: "Worker run", FlowType: "request", Trigger: "the worker is invoked",
					LikelyEntrypointRef: workerRef, LikelyFileRefs: []string{workerRef},
					WhyInteresting: "shows background work", EvidenceRefs: []string{workerEvidence}, Confidence: 0.8,
				},
			},
			Warnings: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": string(orientation)},
			}},
		})
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	debugDir := t.TempDir()
	runID := "onboarding-directions"
	options := Options{
		RepoPath:               repo,
		OutputJSON:             true,
		FlowCount:              0,
		RunID:                  runID,
		DebugDir:               debugDir,
		DumpRedacted:           true,
		MaxReadmeBytes:         1024,
		MaxReadmeLLMBytes:      512,
		MaxTreeLines:           50,
		MaxInterestingFiles:    50,
		MaxGoPkgs:              50,
		MaxGoEdges:             50,
		MaxLLMEntrypoints:      10,
		MaxLLMModules:          10,
		MaxLLMFiles:            10,
		MaxLocalDirectionFiles: 1,
		MaxLLMEdges:            10,
	}
	output, err := Run(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("provider requests = %d, want one orientation request", requests)
	}
	var report combinedReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.ExplainedFlows) != 0 {
		t.Fatalf("model-expanded flows = %d, want 0", len(report.ExplainedFlows))
	}

	runDir := filepath.Join(debugDir, runID)
	semanticRecords := readOrientationSemanticRecords(t, runDir)
	if len(semanticRecords) != 1 ||
		semanticRecords[0].State != debugdump.SemanticStateAccepted ||
		semanticRecords[0].ValidationCode != debugdump.SemanticValidationAccepted ||
		semanticRecords[0].SemanticCalls != 1 ||
		semanticRecords[0].TransportAttempts != 1 ||
		semanticRecords[0].Response.Storage != "raw_content" {
		t.Fatalf("accepted orientation semantic exchange = %#v", semanticRecords)
	}
	options.RunID = runID + "-cached"
	if _, err := Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("validated orientation cache made %d provider requests, want 1 total", requests)
	}
	cachedRecords := readOrientationSemanticRecords(t, filepath.Join(debugDir, options.RunID))
	if len(cachedRecords) != 1 ||
		cachedRecords[0].State != debugdump.SemanticStateCacheHit ||
		cachedRecords[0].ValidationCode != debugdump.SemanticValidationCache ||
		cachedRecords[0].SemanticCalls != 0 || cachedRecords[0].TransportAttempts != 0 ||
		cachedRecords[0].Response.Storage != "raw_content" {
		t.Fatalf("cached orientation semantic exchange = %#v", cachedRecords)
	}
	orientationReportJSON, err := os.ReadFile(filepath.Join(runDir, "orientation_report.json"))
	if err != nil {
		t.Fatal(err)
	}
	warningSidecarJSON, err := os.ReadFile(filepath.Join(
		runDir,
		ConfidenceWarningDiagnosticsFile,
	))
	if err != nil {
		t.Fatal(err)
	}
	warningSidecar, err := DecodeConfidenceWarningDiagnostics(warningSidecarJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !warningSidecar.MatchesOrientationReport(orientationReportJSON) ||
		len(warningSidecar.Diagnostics) != 0 {
		t.Fatalf("orientation warning sidecar = %#v", warningSidecar)
	}
	var savedOrientation orientationPart
	if err := json.Unmarshal(orientationReportJSON, &savedOrientation); err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range warningSidecar.Diagnostics {
		rawWarning, ok := diagnostic.RawWarning()
		if !ok || diagnostic.WarningIndex >= len(savedOrientation.Warnings) ||
			savedOrientation.Warnings[diagnostic.WarningIndex] != rawWarning {
			t.Fatalf("sidecar diagnostic does not address producer warning: %#v", diagnostic)
		}
	}
	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.CandidateDirectionCount != 2 || metadata.ProviderRequestCount != 1 || metadata.CompactContextBytes <= 0 || metadata.ExternalRequestBytes <= 0 {
		t.Fatalf("onboarding metadata = %#v", metadata)
	}
	for _, flowID := range []string{"process-startup", "worker-run"} {
		bundlePath := filepath.Join(runDir, "flows", flowID, "flow_bundle.json")
		bundle, err := os.ReadFile(bundlePath)
		if err != nil {
			t.Fatalf("read %s: %v", bundlePath, err)
		}
		if !strings.Contains(string(bundle), `"flow_seed"`) || !strings.Contains(string(bundle), `"selected_files"`) {
			t.Fatalf("local direction bundle is incomplete: %s", bundle)
		}
		var local struct {
			FlowSeed struct {
				ValidSeedFiles []string `json:"valid_seed_files"`
			} `json:"flow_seed"`
			SelectedFiles []struct {
				Path string `json:"path"`
			} `json:"selected_files"`
			SelectedTests []json.RawMessage `json:"selected_tests"`
			SelectedDocs  []json.RawMessage `json:"selected_docs"`
		}
		if err := json.Unmarshal(bundle, &local); err != nil {
			t.Fatal(err)
		}
		if total := len(local.SelectedFiles) + len(local.SelectedTests) + len(local.SelectedDocs); total > 1 {
			t.Fatalf("local direction bundle selected %d items, want at most 1", total)
		}
		if len(local.FlowSeed.ValidSeedFiles) != 1 || len(local.SelectedFiles) != 1 || local.SelectedFiles[0].Path != local.FlowSeed.ValidSeedFiles[0] {
			t.Fatalf("local direction did not preserve its seed: %#v", local)
		}
		status, err := os.ReadFile(filepath.Join(runDir, "flows", flowID, "flow_status.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(status), `"mode": "local_only"`) {
			t.Fatalf("local direction status = %s", status)
		}
		if _, err := os.Stat(filepath.Join(runDir, "flows", flowID, "flow_report.json")); !os.IsNotExist(err) {
			t.Fatalf("flow %s unexpectedly has a model report: %v", flowID, err)
		}
	}
}

func TestRunKeepsFlowExpansionLocalUnderResearchCallBudget(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/expanded\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOrientGit(t, repo, "init", "--quiet")
	runOrientGit(t, repo, "add", "--", "go.mod", "main.go")

	var requestSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		requestSizes = append(requestSizes, len(body))
		wire := orientationWireFromRequestBytes(t, body)
		fileRef, evidenceRef := orientationWireFileRefs(t, wire, "main.go")
		orientation, err := json.Marshal(orientationProviderResponse{
			ProjectGuess: "tiny command", Confidence: 0.9,
			FirstFilesToOpen: []orientationProviderFileToOpen{{FileRef: fileRef, Reason: "entrypoint"}},
			CandidateFlows: []orientationProviderCandidateFlow{{
				Name: "Process startup", FlowType: "request", Trigger: "the executable starts",
				LikelyEntrypointRef: fileRef, LikelyFileRefs: []string{fileRef},
				WhyInteresting: "shows wiring", EvidenceRefs: []string{evidenceRef}, Confidence: 0.9,
			}},
			Warnings: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": string(orientation)},
			}},
		})
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	debugDir := t.TempDir()
	runID := "expanded-flow"
	_, err := Run(context.Background(), Options{
		RepoPath:          repo,
		OutputJSON:        true,
		FlowCount:         1,
		RunID:             runID,
		DebugDir:          debugDir,
		DumpRedacted:      true,
		RequireArtifacts:  true,
		MaxLLMFiles:       10,
		MaxLLMEdges:       10,
		MaxLLMEntrypoints: 10,
		MaxLLMModules:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requestSizes) != 1 {
		t.Fatalf("provider request count = %d, want only orientation", len(requestSizes))
	}
	runDir := filepath.Join(debugDir, runID)
	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	wantBytes := requestSizes[0]
	if metadata.ProviderRequestCount != 1 || metadata.ExternalRequestBytes != wantBytes {
		t.Fatalf("provider metadata count/bytes = %d/%d, want 1/%d", metadata.ProviderRequestCount, metadata.ExternalRequestBytes, wantBytes)
	}
	status, err := os.ReadFile(filepath.Join(runDir, "flows", "process-startup", "flow_status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), `"mode": "local_only"`) {
		t.Fatalf("expanded flow status = %s", status)
	}
}

func readOrientationSemanticRecords(
	t *testing.T,
	runDir string,
) []debugdump.SemanticExchangeRecord {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	records := make([]debugdump.SemanticExchangeRecord, 0, len(directories))
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(
			runDir,
			debugdump.SemanticExchangesDir,
			directory.Name(),
			debugdump.SemanticExchangeMetaFile,
		))
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func runOrientGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
}
