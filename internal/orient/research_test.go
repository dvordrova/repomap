package orient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestObtainOrientationRefetchesInvalidCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"fresh\":true}"}}]}`)
	}))
	defer server.Close()

	client := &deepseek.Client{
		HTTPClient: server.Client(),
		Model:      "fixture-model",
		MaxTokens:  128,
		Endpoint:   server.URL,
		Auth:       "none",
	}
	baseDir := t.TempDir()
	writer, err := debugdump.NewWriter(baseDir, "invalid-cache", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	repository := modelresearch.RepositoryContext{
		Identity: "fixture", Revision: "abc", Scenario: "go-default",
	}
	policy := modelresearch.DefaultPolicy()
	bundleJSON := []byte(`{"bounded":"evidence"}`)
	requestJSON := []byte(`{"provider":"request"}`)
	bundleHash := modelresearch.SHA256(bundleJSON)
	fingerprint := modelresearch.FingerprintInput{
		Repository: repository, Stage: "orientation",
		PromptVersion: deepseek.OrientationPromptVersionJSON,
		Profile:       "test", Model: client.Model,
		EvidenceBundleHash: bundleHash, PolicyVersion: policy.Version,
	}
	cacheKey, err := modelresearch.CacheKey(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(baseDir, ".model-research")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, cacheKey+".json"), []byte(`{"version":0}`), 0o600); err != nil {
		t.Fatal(err)
	}

	call, err := obtainOrientation(
		context.Background(), client, writer, policy, repository, "test", bundleJSON, requestJSON, true,
	)
	if err != nil {
		t.Fatalf("obtainOrientation() error = %v", err)
	}
	if requests != 1 || call.Metrics.CacheHit || string(call.Raw) != `{"fresh":true}` {
		t.Fatalf("refetched orientation = requests %d, cache hit %t, raw %q", requests, call.Metrics.CacheHit, call.Raw)
	}
	if err := saveOrientationResponse(call); err != nil {
		t.Fatal(err)
	}
	replayed, err := obtainOrientation(
		context.Background(), client, writer, policy, repository, "test", bundleJSON, requestJSON, true,
	)
	if err != nil {
		t.Fatalf("cached obtainOrientation() error = %v", err)
	}
	if requests != 1 || !replayed.Metrics.CacheHit || string(replayed.Raw) != `{"fresh":true}` {
		t.Fatalf("replayed orientation = requests %d, cache hit %t, raw %q", requests, replayed.Metrics.CacheHit, replayed.Raw)
	}

	uncached, err := obtainOrientation(
		context.Background(), client, writer, policy, repository, "test", bundleJSON, requestJSON, false,
	)
	if err != nil {
		t.Fatalf("uncached obtainOrientation() error = %v", err)
	}
	if requests != 2 || uncached.Metrics.CacheHit || uncached.SaveCache || string(uncached.Raw) != `{"fresh":true}` {
		t.Fatalf(
			"uncached orientation = requests %d, cache hit %t, save cache %t, raw %q",
			requests, uncached.Metrics.CacheHit, uncached.SaveCache, uncached.Raw,
		)
	}
}

func TestObtainOrientationDoesNotCacheRecoveredCompletionUnderBaseRequest(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests%2 == 1 {
			_, _ = io.WriteString(w, `{
				"choices":[{"finish_reason":"length","message":{"content":"{\"cut\":"}}],
				"usage":{"completion_tokens":128}
			}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],
			"usage":{"completion_tokens":8}
		}`)
	}))
	defer server.Close()

	client := &deepseek.Client{
		HTTPClient: server.Client(), Model: "fixture-model",
		MaxTokens: 128, Endpoint: server.URL, Auth: "none",
	}
	baseDir := t.TempDir()
	writer, err := debugdump.NewWriter(baseDir, "recovered-cache", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	bundleJSON := []byte(`{"bounded":"evidence"}`)
	requestJSON, err := client.OrientPromptJSON(bundleJSON)
	if err != nil {
		t.Fatal(err)
	}
	repository := modelresearch.RepositoryContext{
		Identity: "fixture", Revision: "abc", Scenario: "go-default",
	}
	policy := modelresearch.DefaultPolicy()

	run := func() orientationCall {
		t.Helper()
		call, callErr := obtainOrientation(
			context.Background(), client, writer, policy, repository, "test",
			bundleJSON, requestJSON, true,
		)
		if callErr != nil {
			t.Fatal(callErr)
		}
		if saveErr := saveOrientationResponse(call); saveErr != nil {
			t.Fatal(saveErr)
		}
		return call
	}
	first := run()
	second := run()
	if first.SaveCache || second.SaveCache || first.Metrics.CacheHit || second.Metrics.CacheHit {
		t.Fatalf("recovered cache state = first %#v, second %#v", first, second)
	}
	if requests != 4 || first.Metrics.SemanticCalls != 1 || first.Metrics.RetryCount != 1 ||
		first.Metrics.RequestBytes <= len(requestJSON) {
		t.Fatalf("requests/metrics = %d/%#v", requests, first.Metrics)
	}
}

func TestObtainOrientationCacheReusesCanonicalEnglishAcrossPresentationLocales(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"choices":[{"message":{"content":"{\"language\":\"en\"}"}}]}`,
		)
	}))
	defer server.Close()

	client := &deepseek.Client{
		HTTPClient: server.Client(),
		Model:      "fixture-model",
		MaxTokens:  128,
		Endpoint:   server.URL,
		Auth:       "none",
	}
	baseDir := t.TempDir()
	writer, err := debugdump.NewWriter(baseDir, "language-cache", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	repository := modelresearch.RepositoryContext{
		Identity: "fixture", Revision: "abc", Scenario: "go-default",
	}
	policy := modelresearch.DefaultPolicy()
	bundleJSON := []byte(`{"bounded":"evidence"}`)

	run := func(requestJSON []byte) orientationCall {
		t.Helper()
		call, callErr := obtainOrientation(
			context.Background(),
			client,
			writer,
			policy,
			repository,
			"test",
			bundleJSON,
			requestJSON,
			true,
		)
		if callErr != nil {
			t.Fatal(callErr)
		}
		if call.SaveCache {
			if saveErr := saveOrientationResponse(call); saveErr != nil {
				t.Fatal(saveErr)
			}
		}
		return call
	}

	canonicalRequest := []byte(`{"provider":"request","language":"en"}`)
	first := run(canonicalRequest)
	replayedForEnglish := run(canonicalRequest)
	replayedForRussian := run(canonicalRequest)

	if first.Metrics.CacheHit ||
		!replayedForEnglish.Metrics.CacheHit || !replayedForRussian.Metrics.CacheHit ||
		requests != 1 {
		t.Fatalf(
			"cache hits = %t/%t/%t, requests = %d",
			first.Metrics.CacheHit,
			replayedForEnglish.Metrics.CacheHit,
			replayedForRussian.Metrics.CacheHit,
			requests,
		)
	}
	if string(replayedForEnglish.Raw) != `{"language":"en"}` ||
		string(replayedForRussian.Raw) != `{"language":"en"}` {
		t.Fatalf(
			"replayed language responses = %q / %q",
			replayedForEnglish.Raw,
			replayedForRussian.Raw,
		)
	}
}

func TestAddResearchFocusLocationsUsesLocalEvidencePriority(t *testing.T) {
	t.Parallel()

	const filePath = "internal/server/server.go"
	serverStart := surfacediscovery.Location{Path: filePath, Line: 71}
	descriptor := surfacediscovery.Location{Path: filePath, Line: 72}
	surfaceResult := &surfacediscovery.Result{
		Catalog: surfacediscovery.TriggerCatalog{Triggers: []surfacediscovery.TriggerRecord{{
			RegistrationSite:  surfacediscovery.Location{Path: filePath, Line: 70},
			ServerStartSite:   &serverStart,
			DescriptorSite:    &descriptor,
			ProcessEntrypoint: surfacediscovery.Symbol{Location: surfacediscovery.Location{Path: filePath, Line: 73}},
		}}},
		Grounding: surfacediscovery.ArchitectureGrounding{Anchors: []surfacediscovery.BehaviorAnchor{{
			Location: surfacediscovery.Location{Path: filePath, Line: 50},
		}}},
	}
	report := &orientationPart{CandidateFlows: []flowexplain.CandidateFlow{{
		LocalProof: &flowproof.Session{Proof: flowproof.Proof{
			Anchors: []flowproof.Anchor{{
				ID: "handler", Location: &evidence.Location{Path: filePath, Line: 62},
			}},
			Transitions: []flowproof.Transition{
				{ID: "frontier", Resolution: evidence.ResolutionUnresolved, Evidence: evidence.Location{Path: filePath, Line: 60}},
				{ID: "resolved", Resolution: evidence.ResolutionStatic, Evidence: evidence.Location{Path: filePath, Line: 61}},
			},
			Slots: []flowproof.Slot{{
				Status: flowproof.SlotUnresolved, EvidenceIDs: []string{"frontier"},
			}},
		}},
	}}}
	snap := snapshot.Snapshot{GoFacts: &gofacts.Facts{CommandTraces: []gofacts.CommandTrace{{
		Steps: []gofacts.CommandTraceStep{{
			CallsiteLocation: &evidence.Location{Path: filePath, Line: 41},
			TargetLocation:   evidence.Location{Path: filePath, Line: 39},
		}},
		HandlerCalls: []gofacts.CommandTraceCall{{
			Path: filePath, Line: 40, Resolved: true, TargetPath: filePath, TargetLine: 38,
		}},
	}}}}

	candidates := addResearchFocusLocations(
		[]modelresearch.FileCandidate{{ID: "server", Path: filePath}},
		report,
		surfaceResult,
		snap,
		[]sourcesignals.Signal{{Path: filePath, Line: 30}},
	)
	got := make([]int, 0, len(candidates[0].FocusLocations))
	for _, location := range candidates[0].FocusLocations {
		got = append(got, location.Line)
	}
	want := []int{70, 71, 72, 73, 60, 61, 62, 50, 40, 41, 38, 39, 30}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("focus priority = %v, want %v", got, want)
	}
}

func TestSavedFlowProofIDsExcludeEvidenceOnlyDirections(t *testing.T) {
	t.Parallel()

	flows := []flowexplain.CandidateFlow{
		{Name: "Evidence only", Disposition: flowexplain.DirectionAccepted},
		{
			Name: "Saved trace", Disposition: flowexplain.DirectionAccepted,
			LocalProof: &flowproof.Session{Proof: flowproof.Proof{ID: "saved-trace"}},
		},
	}
	if got := savedFlowProofIDs(flows); !reflect.DeepEqual(got, []string{"saved-trace"}) {
		t.Fatalf("savedFlowProofIDs() = %v", got)
	}
}
