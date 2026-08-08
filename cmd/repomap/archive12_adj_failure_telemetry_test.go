package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// Archive 12 P0 (owner directive): a provider failure in Theme Adjudication
// must preserve every known telemetry metric — attempts, latency, tokens and
// response bytes — so the incident is investigable from the outcome alone.
// The stage records these BEFORE branching on the transport error.
func TestThemeAdjudicationFailurePreservesKnownTelemetry(t *testing.T) {
	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	output := newRunOutput(discardSink{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	policy := modelresearch.DefaultPolicy()
	repository := modelresearch.RepositoryContext{Identity: "test", Revision: "revision", Scenario: "go-default"}

	// Minimal Scout request/result/expansion fixture (like compile_test).
	vocabulary := themestudy.BuildFileVocabulary(
		[]string{"a.go", "b.go"}, 0, func(path string) bool { return true },
	)
	seedSpecs := []themestudy.SeedSpec{
		{Ref: "a1", Path: "a.go", Line: 1, Symbol: "A", Provenance: "test", Kind: "focused"},
		{Ref: "a2", Path: "b.go", Line: 1, Symbol: "B", Provenance: "test", Kind: "focused"},
	}
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, 0, 0, 0,
		func(path string, start, end int) ([]string, error) { return []string{"line1"}, nil },
		func(path string) (int, error) { return 1, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	scoutRequest, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themestudy.ScoutContext{RepositoryName: "test"}, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	mockScout, err := themestudy.MockScoutResponse(scoutRequest)
	if err != nil {
		t.Fatal(err)
	}
	candidates, scoutStatus, err := themestudy.ValidateScout(
		mockScout, scoutRequest.AnchorRefs(), scoutRequest.FileRefs(), scoutRequest.CatalogSHA256,
	)
	if err != nil || len(candidates) == 0 {
		t.Fatalf("mock scout must produce accepted candidates: %v (%#v)", err, scoutStatus)
	}
	themestudy.AssignCandidateRefs(candidates)
	scoutResult := themestudy.ScoutResult{
		Version: themestudy.ScoutResultVersion, State: scoutStatus.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: themestudy.LanguageEnglish,
		CatalogSHA256: scoutRequest.CatalogSHA256, WireSHA256: scoutRequest.WireSHA256,
		Candidates: candidates, Status: scoutStatus,
	}
	expansion, err := themestudy.ExpandFiles(
		vocabulary.Files,
		func(path string, start, end int) ([]string, error) { return []string{"line1"}, nil },
		func(path string) (int, error) { return 1, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	expansion.Requested = themestudy.RefsForExpansion(candidates)

	// Adjudication client that FAILS after reporting measured metrics.
	failingClient := &failingThemeAdjClient{
		err: errors.New("transport timeout"), content: []byte(`{"partial":true}`), responseBytes: 321,
	}
	clients := func(requireCredentials bool) (themeStudyClient, error) { return failingClient, nil }

	stage := runThemeAdjudicationStage(
		ctx, runDir, filepath.Join(runDir, "runs"), repository, policy, true,
		scoutRequest, scoutResult, expansion,
		map[string]themestudy.AnchorInfo{
			"a1": {Path: "a.go", Symbol: "A", Line: 1},
			"a2": {Path: "b.go", Symbol: "B", Line: 1},
		},
		themestudy.LanguageEnglish, writer, output, clients,
	)
	// Decision 235 (v11) 1D chatto: an ordinary provider/validation failure
	// is NOT a terminal error — the stage writes the durable failed status
	// and returns err=nil so the run continues to the report. The failure
	// is observable through the outcome state/failure code.
	if stage.err != nil {
		t.Fatalf("ordinary provider failure must not terminate the stage: %v", stage.err)
	}
	o := stage.outcome
	if o.State != atlasstudy.ProductStateFailed {
		t.Errorf("expected failed state, got %s", o.State)
	}
	if o.FailureCode != atlasstudy.FailureProvider {
		t.Errorf("expected provider failure code, got %s", o.FailureCode)
	}
	if o.TransportAttempts != 2 {
		t.Errorf("expected 2 transport attempts preserved on failure, got %d", o.TransportAttempts)
	}
	if o.InputTokens != 111 || o.OutputTokens != 22 {
		t.Errorf("expected measured tokens preserved on failure, got in=%d out=%d", o.InputTokens, o.OutputTokens)
	}
	if o.ResponseBytes != 321 {
		t.Errorf("expected measured response bytes preserved on failure, got %d", o.ResponseBytes)
	}
	if o.SemanticCalls != 1 || o.RequestBytes != len(`{"ok":true}`) {
		t.Errorf("expected one exact live request, got calls=%d bytes=%d", o.SemanticCalls, o.RequestBytes)
	}
	entries := readSemanticJournalEntries(t, runDir)
	partial := []byte(`{"partial":true}`)
	if len(entries) != 1 ||
		entries[0].record.State != debugdump.SemanticStateProviderFailed ||
		entries[0].record.SemanticCalls != 1 ||
		entries[0].record.TransportAttempts != 2 ||
		entries[0].record.Response.Storage != "raw_content" ||
		entries[0].record.Response.OriginalBytes != len(partial) ||
		!bytes.Equal(entries[0].response, partial) {
		t.Fatalf("adjudication failure exchange = %#v", entries)
	}

	// When the provider exposes only the response byte count, the unavailable
	// marker carries that count — never the unrelated transport-attempt count.
	emptyRunDir := t.TempDir()
	emptyWriter, err := debugdump.OpenWriter(emptyRunDir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer emptyWriter.Close()
	emptyClient := &failingThemeAdjClient{
		err: errors.New("transport timeout"), responseBytes: 321,
	}
	emptyStage := runThemeAdjudicationStage(
		ctx, emptyRunDir, filepath.Join(emptyRunDir, "runs"), repository, policy, true,
		scoutRequest, scoutResult, expansion,
		map[string]themestudy.AnchorInfo{
			"a1": {Path: "a.go", Symbol: "A", Line: 1},
			"a2": {Path: "b.go", Symbol: "B", Line: 1},
		},
		themestudy.LanguageEnglish, emptyWriter, output,
		func(bool) (themeStudyClient, error) { return emptyClient, nil },
	)
	if emptyStage.err != nil || emptyStage.outcome.ResponseBytes != 321 {
		t.Fatalf("empty provider failure = %#v, err = %v", emptyStage.outcome, emptyStage.err)
	}
	emptyEntries := readSemanticJournalEntries(t, emptyRunDir)
	if len(emptyEntries) != 1 ||
		emptyEntries[0].record.Response.Storage != "raw_unavailable" ||
		emptyEntries[0].record.Response.UnavailableCode != debugdump.SemanticUnavailableNoContent ||
		emptyEntries[0].record.Response.OriginalBytes != 321 {
		t.Fatalf("empty adjudication failure exchange = %#v", emptyEntries)
	}
}

type failingThemeAdjClient struct {
	err           error
	content       []byte
	responseBytes int
}

func (c *failingThemeAdjClient) ThemeScoutPromptJSON(themestudy.ScoutPrompt, int) ([]byte, error) {
	return nil, errors.New("not used")
}
func (c *failingThemeAdjClient) ThemeScoutMeasured(context.Context, themestudy.ScoutPrompt, int) (modelresearch.ProviderResult, error) {
	return modelresearch.ProviderResult{}, errors.New("not used")
}
func (c *failingThemeAdjClient) ThemeAdjudicationPromptJSON(themestudy.AdjudicationPrompt, int) ([]byte, error) {
	return []byte(`{"ok":true}`), nil
}
func (c *failingThemeAdjClient) ThemeAdjudicationMeasured(context.Context, themestudy.AdjudicationPrompt, int) (modelresearch.ProviderResult, error) {
	return modelresearch.ProviderResult{
		Content: c.content, Attempts: 2, ResponseBytes: c.responseBytes,
		InputTokens: 111, OutputTokens: 22,
	}, c.err
}
func (c *failingThemeAdjClient) EffectiveConfig() deepseek.EffectiveConfig {
	return deepseek.EffectiveConfig{Endpoint: "https://api.deepseek.com", MaxTokens: 4096}
}

type discardSink struct{}

func (discardSink) Write(p []byte) (int, error) { return len(p), nil }
