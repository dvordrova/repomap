package main

import (
	"os"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/report"
)

// Decision 235 (v11) 1D caddy: an overlarge candidate bundle must not fail
// whole-bundle — facts are capped deterministically in the builder
// (MaxFactsPerCandidate), valid siblings survive, and the omission is
// recorded as a diagnostic. Provider-free replay of the saved caddy run.
func TestV11CaddyCandidateFactCap(t *testing.T) {
	const runDir = "/Users/dvordrova/git/go-corpus-repomap/service__github__caddyserver__caddy/runs/20260806-231149-caddy-2e29abfe52c0"
	if _, err := os.Stat(runDir); err != nil {
		t.Skipf("run dir unavailable: %v", err)
	}
	readDir := t.TempDir()
	if err := copyRunDirForReplay(runDir, readDir); err != nil {
		t.Fatalf("stage run dir: %v", err)
	}
	data, err := report.ReadRunDir(readDir)
	if err != nil {
		t.Fatalf("read run dir: %v", err)
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatalf("build input: %v", err)
	}
	if len(input.CandidateBundle.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	for _, candidate := range input.CandidateBundle.Candidates {
		if len(candidate.Facts) > componentmap.MaxFactsPerCandidate {
			t.Fatalf("candidate %s carries %d facts, cap is %d", candidate.ID.Value, len(candidate.Facts), componentmap.MaxFactsPerCandidate)
		}
	}
	// The bundle must validate without whole-bundle failure.
	if err := input.CandidateBundle.Validate(); err != nil {
		t.Fatalf("candidate bundle validation: %v", err)
	}
	// The omission is recorded on the landscape diagnostics, never silent.
	found := false
	for _, diagnostic := range input.Landscape.Diagnostics {
		if diagnostic.Code == "builder.candidate_facts_capped" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("candidate facts cap omission diagnostic not recorded")
	}
}
