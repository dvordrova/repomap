package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

func TestPublicationFailureShowsTranslationReasonAndExistingExchange(t *testing.T) {
	root := t.TempDir()
	const runID = "successful-sibling-owner"
	runDir := filepath.Join(root, runID)
	writer, err := debugdump.NewWriter(root, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	provider := &targetPortfolioClientStub{response: []byte(`{"translations":[]}`)}
	executor := debugdump.BindStage(llm.Executor{
		RootDir: root, Enabled: true, Observer: debugdump.NewSemanticObserver(writer),
	}, debugdump.SemanticStageReportTranslation)
	_, err = llm.ExecuteJSON(t.Context(), executor, provider, llm.Call[struct{}]{
		State:  []byte(`{"stage":"report_translation","test":true}`),
		Prompt: llm.Prompt{System: "Translate the supplied display text.", User: `{"ref":"t2","text":"Saved explanation."}`, ResponseFormatJSON: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
		DecodeValidate: func([]byte) (struct{}, error) {
			return struct{}{}, errors.New("display translation: missing t2")
		},
	})
	if err == nil {
		t.Fatal("invalid translation was accepted")
	}
	failure := fmt.Errorf("report translation: %w", err)
	var console bytes.Buffer
	targets := []targetPageConsoleContext{
		{DisplayPath: "api", Scope: "library", RunID: runID, Role: "default"},
		{DisplayPath: "worker", Scope: "executable", RunID: "worker-run", Role: "sibling"},
	}
	reportAnalyzedTargetPagePublicationFailure(newRunOutput(&console), targets, runDir, failure)
	for _, want := range []string{
		"Report publication:", "state: failed", "analyzed target pages: 2",
		"reason: report translation: llm: reject response: display translation: missing t2",
		"artifacts: " + runDir,
		"rejections: " + filepath.Join(runDir, modeldiag.Filename),
		"model request/response journals: " + filepath.Join(runDir, debugdump.SemanticExchangesDir),
	} {
		if !strings.Contains(console.String(), want) {
			t.Fatalf("publication failure omitted %q:\n%s", want, console.String())
		}
	}
	if strings.Count(console.String(), "state: analyzed") != 2 || strings.Count(console.String(), "state: failed") != 1 {
		t.Fatalf("publication failure changed completed target states:\n%s", console.String())
	}
	rows, err := modeldiag.Read(runDir)
	if err != nil || len(rows) != 1 || rows[0].ResponseRef == "" {
		t.Fatalf("reported rejection path cannot locate its exchange: %+v / %v", rows, err)
	}
	metadataPath := filepath.Join(runDir, filepath.FromSlash(rows[0].ResponseRef))
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var exchange debugdump.SemanticExchangeRecord
	if err := json.Unmarshal(raw, &exchange); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []struct {
		file string
		want string
	}{
		{exchange.Request.File, "Saved explanation."},
		{exchange.Response.File, string(provider.response)},
	} {
		content, err := os.ReadFile(filepath.Join(filepath.Dir(metadataPath), payload.file))
		if err != nil || !strings.Contains(string(content), payload.want) {
			t.Fatalf("reported exchange lost its original payload %q: %v", payload.file, err)
		}
	}
}

func TestPublicationFailureDoesNotAdvertiseMissingDiagnosticFiles(t *testing.T) {
	runDir := t.TempDir()
	for _, failure := range []error{
		errors.New("report translation: model provider is unavailable"),
		&os.PathError{Op: "write", Path: filepath.Join(runDir, "report.json"), Err: os.ErrPermission},
	} {
		var console bytes.Buffer
		reportAnalyzedTargetPagePublicationFailure(newRunOutput(&console), nil, runDir, failure)
		if !strings.Contains(console.String(), "reason: "+failure.Error()) || !strings.Contains(console.String(), "artifacts: "+runDir) {
			t.Fatalf("local publication cause was obscured:\n%s", console.String())
		}
		if strings.Contains(console.String(), "rejections:") || strings.Contains(console.String(), "model request/response journals:") {
			t.Fatalf("failure advertises nonexistent model diagnostics:\n%s", console.String())
		}
	}
	reportAnalyzedTargetPagePublicationFailure(nil, nil, runDir, errors.New("silent test output"))
}
