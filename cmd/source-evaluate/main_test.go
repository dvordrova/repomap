package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestRunEvaluatesSavedSourceResponse(t *testing.T) {
	t.Parallel()

	temporaryDir := t.TempDir()
	bundlePath := filepath.Join(temporaryDir, "bundle.json")
	responsePath := filepath.Join(temporaryDir, "response.txt")
	outDir := filepath.Join(temporaryDir, "out")
	if err := os.WriteFile(bundlePath, deepseektest.SourceBundleJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(responsePath, deepseektest.SourceResponseJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{
		"--bundle", bundlePath,
		"--response", responsePath,
		"--out-dir", outDir,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stdout.String() != "100/100\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}

	evaluationJSON, err := os.ReadFile(filepath.Join(outDir, "source_evaluation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evaluation sourceexplain.Evaluation
	if err := json.Unmarshal(evaluationJSON, &evaluation); err != nil {
		t.Fatal(err)
	}
	if evaluation.Score != 100 {
		t.Fatalf("score = %d, want 100", evaluation.Score)
	}
}

func TestRunRequiresPaths(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := run(nil, &output, &output); err == nil {
		t.Fatal("run() error = nil")
	}
}
