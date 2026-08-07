package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/report"
)

// Decision 235 (v11): the three mechanical whole-rejections from the v10
// corpus must now normalize or item-local reject. Provider-free replay of the
// saved raw responses through the CURRENT decoder/validator.
func TestV11HardCasesReplay(t *testing.T) {
	cases := []struct {
		name        string
		runDir      string
		wantOutcome componentmap.ValidationOutcome
		wantDiag    string // expected counted normalization diagnostic, if any
	}{
		{
			name:   "soft-serve",
			runDir: "/Users/dvordrova/git/go-corpus-repomap/service__github__charmbracelet__soft-serve/runs/20260806-231132-soft-serve-4312522ddac4",
			// 14 components normalized (missing anchor_refs → []) and
			// accepted; 50/51 members covered — accepted_partial is the
			// honest outcome, never a whole fallback.
			wantOutcome: componentmap.ValidationAcceptedPartial,
			wantDiag:    "proposal.normalized_missing_anchor_refs",
		},
		{
			name:        "goargs",
			runDir:      "/Users/dvordrova/git/go-corpus-repomap/cli__gitlab__gitlab-org__language-tools__go__linters__goargs/runs/20260806-231411-goargs-dcfcc477f141",
			wantOutcome: componentmap.ValidationAcceptedPartial,
		},
		{
			name: "gotify",
			// The saved gotify run carries a pre-existing Scout-seed
			// binding defect (v10 era) unrelated to Architecture; the
			// replay strips Study artifacts so the Architecture response
			// can be re-evaluated in isolation.
			runDir:      "/Users/dvordrova/git/go-corpus-repomap/service__github__gotify__server/runs/20260806-231044-server-2f0a6dbd338b",
			wantOutcome: componentmap.ValidationAccepted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.runDir); err != nil {
				t.Skipf("run dir unavailable: %v", err)
			}
			// Decision 235 (v11): version bumps make saved v1 Scout/
			// Adjudication artifacts fail closed at read time; the
			// Architecture replay only needs the local facts, so stage
			// a copy without the Study artifact set (archive9 pattern).
			readDir := t.TempDir()
			if err := copyRunDirForReplay(tc.runDir, readDir); err != nil {
				t.Fatalf("stage run dir: %v", err)
			}
			for _, artifact := range []string{
				"theme_scout_request.v1.json", "theme_scout_result.v1.json",
				"theme_scout_status.v1.json",
				"theme_adjudication_request.v1.json",
				"theme_adjudication_result.v1.json",
				"theme_adjudication_status.v1.json",
				"study_themes.v1.json", "theme_source_expansion.v1.json",
			} {
				_ = os.Remove(filepath.Join(readDir, artifact))
			}
			data, err := report.ReadRunDir(readDir)
			if err != nil {
				t.Fatalf("read run dir: %v", err)
			}
			bundle, err := report.BuildArchitectureCanvasInput(data)
			if err != nil {
				t.Fatalf("build input: %v", err)
			}
			saved, err := savedArchitectureResponseBytes(t, tc.runDir)
			if err != nil {
				t.Fatalf("saved response: %v", err)
			}
			result, err := componentmap.RecordSynthesisResponse(bundle.CandidateBundle, "v11-hardcase-"+tc.name, "test", "test", time.Millisecond, saved)
			if err != nil {
				t.Fatalf("record: %v", err)
			}
			if result.Landscape.Fallback {
				diags := ""
				for _, d := range result.Landscape.Diagnostics {
					diags += d.Code + ";"
				}
				t.Fatalf("whole fallback: diags=%s", diags)
			}
			if result.Landscape.ValidationOutcome != tc.wantOutcome {
				t.Fatalf("outcome=%s want %s", result.Landscape.ValidationOutcome, tc.wantOutcome)
			}
			if tc.wantDiag != "" && !hasLandscapeDiagnosticCode(result.Landscape.Diagnostics, tc.wantDiag) {
				t.Fatalf("missing diagnostic %q in %v", tc.wantDiag, result.Landscape.Diagnostics)
			}
		})
	}
}

func savedArchitectureResponseBytes(t *testing.T, runDir string) ([]byte, error) {
	t.Helper()
	exchanges := filepath.Join(runDir, "semantic_exchanges")
	entries, err := os.ReadDir(exchanges)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(exchanges, entry.Name(), "exchange.v1.json"))
		if err != nil {
			continue
		}
		var ex struct {
			Kind  string `json:"kind"`
			Stage string `json:"stage"`
		}
		if json.Unmarshal(raw, &ex) == nil && (ex.Kind == "architecture_synthesis" || ex.Stage == "architecture_synthesis") {
			dir := filepath.Join(exchanges, entry.Name())
			saved, err := os.ReadFile(filepath.Join(dir, "response.json"))
			if err != nil {
				// Some saved runs store the raw provider bytes as
				// response.txt (gotify corpus run).
				saved, err = os.ReadFile(filepath.Join(dir, "response.txt"))
				if err != nil {
					return nil, err
				}
				return saved, nil
			}
			var envelope struct {
				OriginalBytes int    `json:"original_bytes"`
				Storage       string `json:"storage"`
			}
			if json.Unmarshal(saved, &envelope) == nil && envelope.Storage == "raw_content" && envelope.OriginalBytes > 0 {
				var candidate json.RawMessage
				if json.Unmarshal(saved, &candidate) == nil {
					return candidate, nil
				}
			}
			return saved, nil
		}
	}
	return nil, os.ErrNotExist
}

func hasLandscapeDiagnosticCode(diags []componentmap.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
