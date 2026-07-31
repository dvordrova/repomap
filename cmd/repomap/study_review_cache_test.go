package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

const studyReviewCacheTestAPIKey = "study-cache-actual-secret-key"

type studyReviewCacheHTTPFixture struct {
	server *httptest.Server
	calls  atomic.Int64
}

type studyReviewCacheRunResult struct {
	recordJSON []byte
	recordHash string
	editor     *studyReviewCachingEditor
}

type studyReviewCacheRunOptions struct {
	runsDir      string
	runName      string
	endpoint     string
	model        string
	disableCache bool
	withoutLive  bool
	mutateEditor func(*studyReviewCachingEditor)
}

func TestStudyReviewCacheHTTPColdWarmAndContentAddressing(t *testing.T) {
	provider := newStudyReviewCacheHTTPFixture(t)
	defer provider.server.Close()

	runsDir := t.TempDir()
	bundleA, directions := studyReviewCacheIndependentFixture(t)
	cold := runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "cold",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	if got, want := provider.calls.Load(), int64(len(directions.Directions)); got != want {
		t.Fatalf("cold review HTTP calls = %d, want %d", got, want)
	}
	assertStudyReviewCacheStats(t, cold.editor, 0, len(directions.Directions), 0, 0)

	cacheFiles := studyReviewCacheFiles(t, runsDir)
	if got, want := len(cacheFiles), len(directions.Directions); got != want {
		t.Fatalf("cold cache files = %d, want %d", got, want)
	}
	for _, path := range cacheFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(studyReviewCacheTestAPIKey)) ||
			bytes.Contains(bytes.ToLower(raw), []byte("authorization")) {
			t.Fatalf("cache record %q persisted transport credentials", filepath.Base(path))
		}
	}

	var warmLiveFactories atomic.Int64
	warm := runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:     runsDir,
		runName:     "warm-without-api-key",
		endpoint:    provider.server.URL,
		model:       "study-cache-model",
		withoutLive: true,
		mutateEditor: func(editor *studyReviewCachingEditor) {
			editor.newLive = func() (semanticDiscoveryEditor, error) {
				warmLiveFactories.Add(1)
				return nil, errors.New("warm cache hit constructed a live provider")
			}
		},
	})
	if got, want := provider.calls.Load(), int64(len(directions.Directions)); got != want {
		t.Fatalf("warm review HTTP calls changed total to %d, want %d", got, want)
	}
	if got := warmLiveFactories.Load(); got != 0 {
		t.Fatalf("warm live provider factories = %d, want 0", got)
	}
	assertStudyReviewCacheStats(t, warm.editor, len(directions.Directions), 0, 0, 0)
	if !bytes.Equal(cold.recordJSON, warm.recordJSON) || cold.recordHash != warm.recordHash {
		t.Fatalf("cold/warm normalized Study differs: %s / %s", cold.recordHash, warm.recordHash)
	}

	bundleB := mutateStudyReviewCacheSource(t, bundleA, 0, 2)
	beforeB := provider.calls.Load()
	runStudyReviewCacheFixture(t, provider, bundleB, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "one-fragment-b",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	if got := provider.calls.Load() - beforeB; got != 1 {
		t.Fatalf("one changed review fragment HTTP calls = %d, want 1", got)
	}

	var returnToALiveFactories atomic.Int64
	returnToA := runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:     runsDir,
		runName:     "return-to-a-without-api-key",
		endpoint:    provider.server.URL,
		model:       "study-cache-model",
		withoutLive: true,
		mutateEditor: func(editor *studyReviewCachingEditor) {
			editor.newLive = func() (semanticDiscoveryEditor, error) {
				returnToALiveFactories.Add(1)
				return nil, errors.New("A-B-A replay constructed a live provider")
			}
		},
	})
	if got := returnToALiveFactories.Load(); got != 0 {
		t.Fatalf("A-B-A live provider factories = %d, want 0", got)
	}
	if !bytes.Equal(cold.recordJSON, returnToA.recordJSON) || cold.recordHash != returnToA.recordHash {
		t.Fatalf("A-B-A normalized Study differs: %s / %s", cold.recordHash, returnToA.recordHash)
	}
}

func TestStudyReviewCacheIdentityDriftMisses(t *testing.T) {
	provider := newStudyReviewCacheHTTPFixture(t)
	defer provider.server.Close()

	runsDir := t.TempDir()
	bundle, directions := studyReviewCacheIndependentFixture(t)
	base := studyReviewCacheRunOptions{
		runsDir:  runsDir,
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	}
	runStudyReviewCacheFixture(t, provider, bundle, directions, withStudyReviewCacheRunName(base, "identity-base"))
	wantPerRun := int64(len(directions.Directions))
	if got := provider.calls.Load(); got != wantPerRun {
		t.Fatalf("identity seed calls = %d, want %d", got, wantPerRun)
	}

	tests := []struct {
		name   string
		change func(*studyReviewCacheRunOptions)
	}{
		{
			name: "endpoint",
			change: func(options *studyReviewCacheRunOptions) {
				options.endpoint += "/endpoint-drift"
			},
		},
		{
			name: "model",
			change: func(options *studyReviewCacheRunOptions) {
				options.model = "study-cache-other-model"
			},
		},
		{
			name: "contract",
			change: func(options *studyReviewCacheRunOptions) {
				options.mutateEditor = func(editor *studyReviewCachingEditor) {
					editor.stageContractVersion += "-drift"
				}
			},
		},
	}
	for _, test := range tests {
		before := provider.calls.Load()
		options := base
		options.runName = "identity-" + test.name
		test.change(&options)
		runStudyReviewCacheFixture(t, provider, bundle, directions, options)
		if got := provider.calls.Load() - before; got != wantPerRun {
			t.Errorf("%s identity drift calls = %d, want %d", test.name, got, wantPerRun)
		}
	}
}

func TestStudyReviewCacheNoCacheBypassesReadAndWrite(t *testing.T) {
	provider := newStudyReviewCacheHTTPFixture(t)
	defer provider.server.Close()

	runsDir := t.TempDir()
	bundle, directions := studyReviewCacheIndependentFixture(t)
	runStudyReviewCacheFixture(t, provider, bundle, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "no-cache-seed",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	beforeFiles := studyReviewCacheSnapshot(t, runsDir)
	beforeCalls := provider.calls.Load()
	result := runStudyReviewCacheFixture(t, provider, bundle, directions, studyReviewCacheRunOptions{
		runsDir:      runsDir,
		runName:      "no-cache-run",
		endpoint:     provider.server.URL,
		model:        "study-cache-model",
		disableCache: true,
	})
	if got, want := provider.calls.Load()-beforeCalls, int64(len(directions.Directions)); got != want {
		t.Fatalf("--no-cache review HTTP calls = %d, want %d", got, want)
	}
	assertStudyReviewCacheStats(t, result.editor, 0, 0, len(directions.Directions), 0)
	afterFiles := studyReviewCacheSnapshot(t, runsDir)
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Fatal("--no-cache changed the persistent review cache")
	}
}

func TestStudyReviewCacheRejectsCorruptWrongIDAndWrongSourceEntries(t *testing.T) {
	provider := newStudyReviewCacheHTTPFixture(t)
	defer provider.server.Close()

	runsDir := t.TempDir()
	bundleA, directions := studyReviewCacheIndependentFixture(t)
	seed := runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "invalid-seed",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	files := studyReviewCacheFiles(t, runsDir)
	if len(files) != len(directions.Directions) {
		t.Fatalf("seed cache files = %d, want %d", len(files), len(directions.Directions))
	}

	if err := os.WriteFile(files[0], []byte("{truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeCorrupt := provider.calls.Load()
	corrupt := runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "invalid-corrupt",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	if got := provider.calls.Load() - beforeCorrupt; got != 1 {
		t.Fatalf("corrupt cache recompute calls = %d, want 1", got)
	}
	if !bytes.Equal(seed.recordJSON, corrupt.recordJSON) || seed.recordHash != corrupt.recordHash {
		t.Fatal("corrupt cache recompute changed normalized Study")
	}

	files = studyReviewCacheFiles(t, runsDir)
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var record studyReviewCacheRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	proposal, err := studymap.DecodeReviewProposal(record.Response)
	if err != nil {
		t.Fatal(err)
	}
	proposal.DirectionID = "direction-wrong"
	record.Response, err = json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	record.ResponseSHA256 = studyReviewCacheTestSHA256(record.Response)
	wrongIDRecord, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], append(wrongIDRecord, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeWrongID := provider.calls.Load()
	runStudyReviewCacheFixture(t, provider, bundleA, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "invalid-wrong-id",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	if got := provider.calls.Load() - beforeWrongID; got != 1 {
		t.Fatalf("wrong direction ID recompute calls = %d, want 1", got)
	}

	bundleB := mutateStudyReviewCacheSource(t, bundleA, 0, 2)
	identityEditor := newStudyReviewCachingEditor(
		studyReviewCacheClient(provider.server.URL, "study-cache-model", ""),
		nil,
		runsDir,
		false,
		io.Discard,
	)
	keyA := studyReviewCacheKeyFor(t, identityEditor, bundleA, directions.Directions[0])
	keyB := studyReviewCacheKeyFor(t, identityEditor, bundleB, directions.Directions[0])
	if keyA == keyB {
		t.Fatal("source fragment change did not change review cache identity")
	}
	rawA, err := os.ReadFile(studyReviewCachePath(runsDir, keyA))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(studyReviewCachePath(runsDir, keyB), rawA, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeWrongSource := provider.calls.Load()
	runStudyReviewCacheFixture(t, provider, bundleB, directions, studyReviewCacheRunOptions{
		runsDir:  runsDir,
		runName:  "invalid-wrong-source",
		endpoint: provider.server.URL,
		model:    "study-cache-model",
	})
	if got := provider.calls.Load() - beforeWrongSource; got != 1 {
		t.Fatalf("wrong source identity recompute calls = %d, want 1", got)
	}
}

func TestStudyReviewCacheDoesNotPersistSecretResponse(t *testing.T) {
	runsDir := t.TempDir()
	bundle, directions := studyReviewCacheIndependentFixture(t)
	editor := newStudyReviewCachingEditor(
		studyReviewCacheClient("https://provider.example.test/chat", "study-cache-model", ""),
		nil,
		runsDir,
		false,
		io.Discard,
	)
	prompt, request, reviewBundle := studyReviewCachePlanFor(
		t,
		editor,
		bundle,
		directions.Directions[0],
	)
	proposal := studyReviewCacheProposal(reviewBundle)
	proposal.Reviews[0].SupportedObservation = "API_KEY=actual-secret-value"
	response, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	editor.storeStudyReview(prompt, request, bundle, directions.Directions[0], response)
	if files := studyReviewCacheFiles(t, runsDir); len(files) != 0 {
		t.Fatalf("secret-bearing response produced %d cache files, want 0", len(files))
	}
}

func newStudyReviewCacheHTTPFixture(t *testing.T) *studyReviewCacheHTTPFixture {
	t.Helper()
	fixture := &studyReviewCacheHTTPFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.calls.Add(1)
		if got, want := request.Header.Get("Authorization"), "Bearer "+studyReviewCacheTestAPIKey; got != want {
			t.Errorf("provider Authorization = %q, want bearer fixture credential", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read review request: %v", err)
			return
		}
		var envelope struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode review request: %v", err)
			return
		}
		var user string
		for _, message := range envelope.Messages {
			if message.Role == "user" {
				user = message.Content
			}
		}
		marker := strings.LastIndex(user, studyMapReviewBundleMarker)
		if marker < 0 {
			t.Error("review request omitted its bounded bundle")
			return
		}
		bundle, err := studymap.DecodeReviewBundle(
			[]byte(user[marker+len(studyMapReviewBundleMarker):]),
		)
		if err != nil {
			t.Errorf("decode bounded review bundle: %v", err)
			return
		}
		content, err := json.Marshal(studyReviewCacheProposal(bundle))
		if err != nil {
			t.Errorf("encode review proposal: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": string(content)},
			}},
			"usage": map[string]any{"prompt_tokens": 17, "completion_tokens": 11},
		}); err != nil {
			t.Errorf("encode provider envelope: %v", err)
		}
	}))
	return fixture
}

func runStudyReviewCacheFixture(
	t *testing.T,
	provider *studyReviewCacheHTTPFixture,
	bundle studymap.Bundle,
	directions studymap.DirectionProposal,
	options studyReviewCacheRunOptions,
) studyReviewCacheRunResult {
	t.Helper()
	if options.runName == "" || options.runsDir == "" || options.endpoint == "" || options.model == "" {
		t.Fatal("incomplete Study review cache test options")
	}
	runDir := filepath.Join(options.runsDir, options.runName)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	promptClient := studyReviewCacheClient(options.endpoint, options.model, "")
	newLive := func() (semanticDiscoveryEditor, error) {
		if options.withoutLive {
			return nil, errors.New("live provider is unavailable")
		}
		return studyReviewCacheClient(options.endpoint, options.model, studyReviewCacheTestAPIKey), nil
	}
	editor := newStudyReviewCachingEditor(
		promptClient,
		newLive,
		options.runsDir,
		options.disableCache,
		io.Discard,
	)
	if options.mutateEditor != nil {
		options.mutateEditor(editor)
	}
	bundleHash, err := studymap.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	reviews, summaries, stages, issues, err := reviewStudyMapDirections(
		context.Background(),
		runDir,
		bundle,
		directions,
		bundleHash,
		editor,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != len(directions.Directions) ||
		len(summaries) != len(directions.Directions) ||
		len(stages) != len(directions.Directions) || len(issues) != 0 {
		t.Fatalf(
			"reviews/summaries/stages/issues = %d/%d/%d/%d, want %d/%d/%d/0",
			len(reviews), len(summaries), len(stages), len(issues),
			len(directions.Directions), len(directions.Directions), len(directions.Directions),
		)
	}
	brief := studyReviewCacheBrief(bundle)
	record, _, err := studymap.BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return studyReviewCacheRunResult{
		recordJSON: recordJSON,
		recordHash: studyReviewCacheTestSHA256(recordJSON),
		editor:     editor,
	}
}

func studyReviewCacheIndependentFixture(
	t *testing.T,
) (studymap.Bundle, studymap.DirectionProposal) {
	t.Helper()
	bundle, all := studyMapV32SingleFileReviewFixture(t)
	selected := studymap.DirectionProposal{Version: all.Version}
	for _, index := range []int{0, 3, 6, 9} {
		selected.Directions = append(selected.Directions, all.Directions[index])
	}
	return bundle, selected
}

func mutateStudyReviewCacheSource(
	t *testing.T,
	bundle studymap.Bundle,
	anchorIndex,
	returnValue int,
) studymap.Bundle {
	t.Helper()
	copyBundle := bundle
	copyBundle.Anchors = append([]studymap.Anchor(nil), bundle.Anchors...)
	anchor := copyBundle.Anchors[anchorIndex]
	window, err := sourcewindowfacts.NewWindow(
		"cache-mutated-"+anchor.Symbol,
		anchor.Path,
		anchor.Function.StartLine,
		[]string{
			"func " + anchor.Symbol + "() int {",
			"\treturn " + string(rune('0'+returnValue)),
			"}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, anchor.Symbol)
	if err != nil {
		t.Fatal(err)
	}
	copyBundle.Anchors[anchorIndex].Function = function
	copyBundle.Anchors[anchorIndex].Line = function.StartLine
	return copyBundle
}

func studyReviewCacheBrief(bundle studymap.Bundle) studymap.BriefShapeProposal {
	support := []string{bundle.Anchors[0].ID}
	statement := func(text string) studymap.BriefStatement {
		return studymap.BriefStatement{Text: text, SupportIDs: append([]string(nil), support...)}
	}
	return studymap.BriefShapeProposal{
		Version:        studymap.BriefShapeProposalVersion,
		RepositoryType: studymap.RepositoryLibrary,
		Brief: studymap.Brief{
			WhatItIs:              statement("This fixture is a bounded source library."),
			Problem:               statement("It exercises deterministic Study review caching."),
			MainInput:             statement("Its input is a bounded source bundle."),
			CentralResponsibility: statement("It preserves complete reviewed directions."),
			ObservableResult:      statement("It produces a validated repository Study."),
		},
		ShapeAreaIDs: []string{bundle.Areas[0].ID},
	}
}

func studyReviewCacheClient(endpoint, model, apiKey string) *deepseek.Client {
	return &deepseek.Client{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     apiKey,
		Model:      model,
		MaxTokens:  4096,
		Endpoint:   endpoint,
		Auth:       "bearer",
	}
}

func studyReviewCacheProposal(bundle studymap.ReviewBundle) studymap.ReviewProposal {
	roles := []studymap.ReadingRole{
		studymap.ReadingRolePublicOrCLIEntry,
		studymap.ReadingRoleCoreOrchestration,
		studymap.ReadingRoleEffectOrIntegrationBoundary,
	}
	proposal := studymap.ReviewProposal{
		Version:     studymap.ReviewProposalVersion,
		DirectionID: bundle.DirectionID,
	}
	for index, anchor := range bundle.Anchors {
		proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
			AnchorID:             anchor.AnchorID,
			Fit:                  studymap.AnchorFitDirect,
			SupportedObservation: "This fragment defines the selected function.",
			Role:                 roles[index%len(roles)],
			OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
		})
	}
	return proposal
}

func studyReviewCachePlanFor(
	t *testing.T,
	editor *studyReviewCachingEditor,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) (semanticdiscovery.Prompt, []byte, studymap.ReviewBundle) {
	t.Helper()
	reviewBundle, err := studymap.BuildReviewBundle(bundle, direction)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(reviewBundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt := semanticdiscovery.Prompt{
		Version:         semanticdiscovery.ReadingPackReviewPromptVersion,
		System:          studyMapReviewSystemPrompt,
		User:            studyMapReviewTask + string(raw),
		ThinkingProfile: semanticdiscovery.ThinkingHigh,
		ProgressLabel:   "reading pack review " + reviewBundle.DirectionID,
	}
	request, err := editor.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	return prompt, request, reviewBundle
}

func studyReviewCacheKeyFor(
	t *testing.T,
	editor *studyReviewCachingEditor,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) string {
	t.Helper()
	prompt, request, _ := studyReviewCachePlanFor(t, editor, bundle, direction)
	_, key, _, err := editor.reviewCacheIdentity(prompt, request, bundle, direction)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func studyReviewCacheFiles(t *testing.T, runsDir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(
		runsDir,
		studyReviewCacheParentDirectory,
		studyReviewCacheVersionDirectory,
		"*.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func studyReviewCacheSnapshot(t *testing.T, runsDir string) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte)
	for _, path := range studyReviewCacheFiles(t, runsDir) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[filepath.Base(path)] = raw
	}
	return result
}

func assertStudyReviewCacheStats(
	t *testing.T,
	editor *studyReviewCachingEditor,
	hits,
	misses,
	bypassed,
	corrupt int,
) {
	t.Helper()
	editor.mu.Lock()
	stats := editor.stats
	editor.mu.Unlock()
	if stats.Hits != hits || stats.Misses != misses || stats.Bypassed != bypassed ||
		stats.Corrupt != corrupt || stats.WriteFailures != 0 {
		t.Fatalf(
			"cache stats = %#v, want hits=%d misses=%d bypassed=%d corrupt=%d write_failures=0",
			stats,
			hits,
			misses,
			bypassed,
			corrupt,
		)
	}
}

func withStudyReviewCacheRunName(
	options studyReviewCacheRunOptions,
	name string,
) studyReviewCacheRunOptions {
	options.runName = name
	return options
}

func studyReviewCacheTestSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
