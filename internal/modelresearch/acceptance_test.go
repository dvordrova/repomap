package modelresearch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestResticSavedResponseExpandsBeyondInitialProviderBundle(t *testing.T) {
	repo := t.TempDir()
	writeResearchFile(t, repo, "cmd/restic/cmd_backup.go", "package main\n\nfunc runBackup() {\n\tarchive()\n\topenRepository()\n}\n")
	writeResearchFile(t, repo, "internal/archiver/archiver.go", "package archiver\n\nfunc Archive() {}\n")
	writeResearchFile(t, repo, "internal/repository/repository.go", "package repository\n\nfunc Open() {}\n")
	trace := gofacts.CommandTrace{
		Version: gofacts.CommandTraceVersion, Framework: "cobra",
		EntrypointPackage: "github.com/restic/restic/cmd/restic", Command: "backup",
		Steps: []gofacts.CommandTraceStep{{
			Symbol: "runBackup", Relation: "callback",
			TargetLocation: evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 3},
		}},
		HandlerCalls: []gofacts.CommandTraceCall{
			{Symbol: "archive", Path: "cmd/restic/cmd_backup.go", Line: 12, Relation: "calls", Resolved: true, TargetPath: "internal/archiver/archiver.go", TargetLine: 21},
			{Symbol: "openRepository", Path: "cmd/restic/cmd_backup.go", Line: 13, Relation: "calls", Resolved: true, TargetPath: "internal/repository/repository.go", TargetLine: 31},
		},
	}
	policy := DefaultPolicy()
	plan, err := PlanTargetedRounds(context.Background(), PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "restic-backup", Purpose: "ground the backup architecture boundary",
			Question:     "How does the backup command reach repository and archiver behavior?",
			CandidateIDs: []string{"restic-backup-file"},
		}},
		Candidates:           []FileCandidate{{ID: "restic-backup-file", Path: "cmd/restic/cmd_backup.go", Score: 100}},
		InitialProviderPaths: []string{"cmd/restic/cmd_backup.go"},
		Universe: LocalRepositoryUniverse{
			AuthorizedPaths: []string{"cmd/restic/cmd_backup.go", "internal/archiver/archiver.go", "internal/repository/repository.go"},
			CommandTraces:   []gofacts.CommandTrace{trace}, ScenarioID: "go-default",
		},
		Policy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 1 {
		t.Fatalf("selected rounds = %#v", plan)
	}
	bundle := plan.Selected[0].Bundle
	for _, path := range []string{"internal/archiver/archiver.go", "internal/repository/repository.go"} {
		if !containsString(plan.Selected[0].Scope.LocallyInspected, path) || !containsString(bundle.ProviderAllowedPaths, path) {
			t.Fatalf("focused Restic expansion did not reach %q: scope=%v provider=%v", path, plan.Selected[0].Scope.LocallyInspected, bundle.ProviderAllowedPaths)
		}
	}
	provider := &savedProvider{response: readSavedResearchResponse(t, "restic_backup_response.json")}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan.Selected[0], Policy: policy,
		Repository: RepositoryContext{Identity: repo, Revision: "restic-fixture", Scenario: "go-default"},
		RunsDir:    t.TempDir(), Profile: "saved", Model: "saved-response", Provider: provider,
		ProviderEndpointSHA256: modelResearchTestEndpointSHA256(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != RoundCompleted || len(round.ValidatedFindings) != 1 || round.NewGroundedFactsCount == 0 {
		t.Fatalf("Restic round = %#v", round)
	}
	if provider.calls != 1 || round.RequestBytes > policy.Targeted.MaxRequestBytes {
		t.Fatalf("Restic calls/request bytes = %d/%d", provider.calls, round.RequestBytes)
	}
}

func TestCaddySavedResponsePreservesZeroSurfaceHonesty(t *testing.T) {
	repo := t.TempDir()
	writeResearchFile(t, repo, "caddyconfig/http.go", "package caddyconfig\n\nfunc LoadConfig() {}\n")
	writeResearchFile(t, repo, "admin.go", "package caddy\n\nfunc Admin() {}\n")
	writeResearchFile(t, repo, "modules/caddyhttp/app.go", "package caddyhttp\n\nfunc Start() {}\n")
	policy := DefaultPolicy()
	plan, err := PlanTargetedRounds(context.Background(), PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{
			{ID: "config", Purpose: "configuration boundary", Question: "How does configuration enter running Caddy state?", CandidateIDs: []string{"config-file"}},
			{ID: "admin", Purpose: "admin control plane", Question: "How does an admin request reach configuration application?", CandidateIDs: []string{"admin-file"}},
			{ID: "served", Purpose: "served site", Question: "How does served-site handling reach the HTTP application boundary?", CandidateIDs: []string{"http-file"}},
		},
		Candidates: []FileCandidate{
			{ID: "config-file", Path: "caddyconfig/http.go", Score: 100},
			{ID: "admin-file", Path: "admin.go", Score: 90},
			{ID: "http-file", Path: "modules/caddyhttp/app.go", Score: 80},
		},
		InitialProviderPaths: []string{"caddyconfig/http.go", "admin.go", "modules/caddyhttp/app.go"},
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: []string{"caddyconfig/http.go", "admin.go", "modules/caddyhttp/app.go"}},
		Policy:               policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 2 || len(plan.Skipped) != 1 {
		t.Fatalf("Caddy planning = %#v, want two bounded rounds and one round-limit skip", plan)
	}
	var configPlan PlannedRound
	for _, candidate := range plan.Selected {
		if candidate.Question.ID == "config" {
			configPlan = candidate
		}
	}
	if configPlan.Question.ID == "" {
		t.Fatalf("Caddy config question was not selected: %#v", plan.Selected)
	}
	provider := &savedProvider{response: readSavedResearchResponse(t, "caddy_config_response.json")}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: configPlan, Policy: policy,
		Repository: RepositoryContext{Identity: repo, Revision: "caddy-fixture", Scenario: "go-default"},
		Model:      "saved-response", Provider: provider,
		ProviderEndpointSHA256: modelResearchTestEndpointSHA256(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(round.ValidatedFindings) != 1 || round.ValidatedFindings[0].ID != "caddy-config-ingress" {
		t.Fatalf("Caddy round = %#v", round)
	}
	// The research contract has no surface/route output field; zero discovered
	// terminal surfaces therefore remains an honest local result.
	encoded, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"surfaces"`) || strings.Contains(string(encoded), `"routes"`) {
		t.Fatalf("Caddy model research invented a structural catalog: %s", encoded)
	}
}

func readSavedResearchResponse(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
