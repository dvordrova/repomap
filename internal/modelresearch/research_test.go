package modelresearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
)

type savedProvider struct {
	response              []byte
	err                   error
	calls                 int
	inputTokens           int
	outputTokens          int
	promptCacheHitTokens  int
	promptCacheMissTokens int
}

func TestPlanTargetedRoundsHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := PlanTargetedRounds(ctx, PlanningInput{Policy: DefaultPolicy()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PlanTargetedRounds() error = %v, want context.Canceled", err)
	}
}

func (p *savedProvider) BuildResearchRequest(prompt Prompt) ([]byte, error) {
	return json.Marshal(prompt)
}

func (p *savedProvider) Research(context.Context, Prompt) (ProviderResult, error) {
	p.calls++
	return ProviderResult{
		Content: append([]byte(nil), p.response...), Attempts: 1,
		InputTokens: p.inputTokens, OutputTokens: p.outputTokens,
		PromptCacheHitTokens:  p.promptCacheHitTokens,
		PromptCacheMissTokens: p.promptCacheMissTokens,
	}, p.err
}

func TestPlanTargetedRoundsSelectsOnlyFocusedExactEvidence(t *testing.T) {
	repo := researchFixtureRepo(t)
	policy := DefaultPolicy()
	result, err := PlanTargetedRounds(context.Background(), PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "q-backup", Purpose: "understand backup responsibility",
			Question: "How does backup reach repository behavior?", CandidateIDs: []string{"candidate-backup"},
		}},
		Candidates: []FileCandidate{
			{ID: "candidate-backup", Path: "cmd/backup.go", Score: 100},
			{ID: "candidate-unrelated", Path: "internal/unrelated.go", Score: 99},
		},
		InitialProviderPaths: []string{"cmd/backup.go"},
		Universe: LocalRepositoryUniverse{AuthorizedPaths: []string{
			"cmd/backup.go", "internal/repository/repository.go", "internal/unrelated.go",
		}},
		Policy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 1 {
		t.Fatalf("selected rounds = %d, want 1; skipped=%#v", len(result.Selected), result.Skipped)
	}
	bundle := result.Selected[0].Bundle
	if containsString(bundle.ProviderAllowedPaths, "internal/unrelated.go") {
		t.Fatalf("targeted provider paths include unselected unrelated file: %v", bundle.ProviderAllowedPaths)
	}
	if !containsString(bundle.ProviderAllowedPaths, "cmd/backup.go") {
		t.Fatalf("targeted provider paths = %v, want selected backup file", bundle.ProviderAllowedPaths)
	}
	for _, item := range bundle.Evidence {
		if item.Location != nil && item.Location.Path == "internal/unrelated.go" {
			t.Fatalf("targeted evidence includes unselected file: %#v", item)
		}
	}
}

func TestPlanTargetedRoundsSkipsWithoutNewExactEvidence(t *testing.T) {
	input := basicPlanningInput(t)
	first, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Selected) != 1 {
		t.Fatalf("first selected rounds = %d, want 1", len(first.Selected))
	}
	for _, item := range first.Selected[0].Bundle.Evidence {
		input.PreviousEvidenceIDs = append(input.PreviousEvidenceIDs, item.ID)
	}
	second, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Selected) != 0 || len(second.Skipped) != 1 || second.Skipped[0].Gate.Reason != "no_new_exact_evidence" {
		t.Fatalf("second plan = %#v, want no-new-evidence skip", second)
	}
}

func TestPlanTargetedRoundsSkipsRuntimeOnlyFrontier(t *testing.T) {
	input := basicPlanningInput(t)
	input.Questions[0].Question = "This requires runtime observation only: which production backend is chosen?"
	result, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Gate.Reason != "runtime_only_frontier" {
		t.Fatalf("runtime-only plan = %#v", result)
	}
}

func TestReadSourceWindowCentersExactFocusAndContainsFunctionBoundaries(t *testing.T) {
	repo := t.TempDir()
	var source strings.Builder
	source.WriteString("package focused\n\n")
	for line := 3; line < 31; line++ {
		fmt.Fprintf(&source, "// preamble %d\n", line)
	}
	source.WriteString("func focused() {\n")
	for line := 32; line < 70; line++ {
		fmt.Fprintf(&source, "\t_ = %d\n", line)
	}
	source.WriteString("}\n")
	for line := 71; line <= 120; line++ {
		fmt.Fprintf(&source, "// trailing %d\n", line)
	}
	writeResearchFile(t, repo, "focused.go", source.String())
	reader, err := reporead.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	window, ok := readSourceWindow(reader, "focused.go", 50)
	if !ok || !window.CodeBearing {
		t.Fatalf("focused source window = %#v, ok=%t", window, ok)
	}
	if window.StartLine > 31 || window.EndLine < 70 {
		t.Fatalf("window %d-%d does not contain declaration 31-70", window.StartLine, window.EndLine)
	}
	if 50-window.StartLine < 20 || window.EndLine-50 < 20 {
		t.Fatalf("exact focus line 50 is not centered in %d-%d", window.StartLine, window.EndLine)
	}
}

func TestReadSourceWindowWithoutFocusStartsAtCodeInsteadOfLineOne(t *testing.T) {
	repo := t.TempDir()
	source := "package focused\n\n" + strings.Repeat("// license header\n", 70) +
		"func run() {\n\twork()\n}\n" + strings.Repeat("// trailing context\n", 50)
	writeResearchFile(t, repo, "focused.go", source)
	reader, err := reporead.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	window, ok := readSourceWindow(reader, "focused.go", 0)
	if !ok || window.StartLine == 1 || window.EndLine < 75 {
		t.Fatalf("missing-focus source window = %#v, ok=%t", window, ok)
	}
}

func TestPlanTargetedRoundsRetainsDistantWindowsInOneFile(t *testing.T) {
	repo := t.TempDir()
	var source strings.Builder
	source.WriteString("package wal\n\n")
	for line := 3; line <= 600; line++ {
		switch line {
		case 100:
			source.WriteString("func Close() { syncFile() }\n")
		case 500:
			source.WriteString("func writeBatch() { syncFile() }\n")
		default:
			fmt.Fprintf(&source, "var line%d = %d\n", line, line)
		}
	}
	writeResearchFile(t, repo, "wal.go", source.String())
	input := PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "durability", Purpose: "understand durability",
			Question: "How does writeBatch synchronize writes?", CandidateIDs: []string{"wal"},
		}},
		Candidates: []FileCandidate{{
			ID: "wal", Path: "wal.go", Score: 100,
			FocusLocations: []evidence.Location{
				{Path: "wal.go", Line: 100},
				{Path: "wal.go", Line: 300},
				{Path: "wal.go", Line: 500},
			},
		}},
		InitialProviderPaths: []string{"wal.go"},
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: []string{"wal.go"}},
		Policy:               DefaultPolicy(),
	}
	plan, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 1 {
		t.Fatalf("selected rounds = %#v", plan)
	}
	var windows []SourceWindow
	for _, item := range plan.Selected[0].Bundle.Evidence {
		if item.Kind == EvidenceSource && item.Window != nil {
			windows = append(windows, *item.Window)
		}
	}
	if len(windows) != 2 {
		t.Fatalf("source windows = %#v, want two bounded distant windows", windows)
	}
	foundClose, foundWriteBatch := false, false
	for _, window := range windows {
		for _, line := range window.Lines {
			foundClose = foundClose || strings.Contains(line, "func Close")
			foundWriteBatch = foundWriteBatch || strings.Contains(line, "func writeBatch")
		}
	}
	if !foundClose || !foundWriteBatch {
		t.Fatalf("distant focus coverage: Close=%t writeBatch=%t windows=%#v", foundClose, foundWriteBatch, windows)
	}
	prompt, err := BuildPrompt(plan.Selected[0].Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.User, "Missing text cannot prove") ||
		!strings.Contains(prompt.User, "unresolved frontier") {
		t.Fatalf("targeted research prompt lacks bounded absence contract:\n%s", prompt.User)
	}
}

func TestPlanTargetedRoundsSkipsHeaderOnlyGoWithoutProviderCall(t *testing.T) {
	repo := t.TempDir()
	writeResearchFile(t, repo, "header.go", "// package header\npackage header\n\nimport \"fmt\"\n")
	input := PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "header", Purpose: "inspect startup", Question: "How does startup work?",
			CandidateIDs: []string{"header-file"},
		}},
		Candidates: []FileCandidate{{
			ID: "header-file", Path: "header.go", Score: 100,
			FocusLocations: []evidence.Location{{Path: "header.go", Line: 4}},
		}},
		InitialProviderPaths: []string{"header.go"},
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: []string{"header.go"}},
		Policy:               DefaultPolicy(),
	}
	plan, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 0 || len(plan.Skipped) != 1 ||
		plan.Skipped[0].Gate.Reason != noCodeBearingBoundedWindow {
		t.Fatalf("header-only plan = %#v", plan)
	}
	provider := &savedProvider{response: []byte(`{"findings":[],"unresolved_frontiers":[]}`)}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: plan.Skipped[0], Policy: input.Policy, Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || round.Status != RoundSkipped || round.StopReason != noCodeBearingBoundedWindow {
		t.Fatalf("header-only execution = %#v, provider calls=%d", round, provider.calls)
	}
}

func TestFocusedScopeMarksEvidenceNeverSentToProvider(t *testing.T) {
	input := basicPlanningInput(t)
	input.Policy.Targeted.MaxEvidenceItems = 1
	result, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 1 {
		t.Fatalf("selected rounds = %#v", result)
	}
	plan := result.Selected[0]
	if len(plan.Scope.LocalEvidence) <= len(plan.Bundle.Evidence) {
		t.Fatalf("local/provider evidence = %d/%d, want locally retained omission", len(plan.Scope.LocalEvidence), len(plan.Bundle.Evidence))
	}
	foundNeverSent := false
	for _, item := range plan.Scope.LocalEvidence {
		for _, visibility := range item.Visibility {
			if visibility == VisibilityNeverProvider {
				foundNeverSent = true
			}
		}
	}
	if !foundNeverSent {
		t.Fatalf("focused local evidence lacks never-sent provenance: %#v", plan.Scope.LocalEvidence)
	}
}

func TestPlanTargetedRoundsHardCapsTwoRounds(t *testing.T) {
	input := basicPlanningInput(t)
	input.Questions = []ProposedQuestion{
		{ID: "q1", Purpose: "backup", Question: "How does backup run?", CandidateIDs: []string{"candidate-backup"}},
		{ID: "q2", Purpose: "config", Question: "How does config load?", CandidateIDs: []string{"candidate-config"}},
		{ID: "q3", Purpose: "admin", Question: "How does admin control work?", CandidateIDs: []string{"candidate-admin"}},
	}
	input.Candidates = append(input.Candidates,
		FileCandidate{ID: "candidate-config", Path: "internal/config/config.go", Score: 90},
		FileCandidate{ID: "candidate-admin", Path: "internal/admin/admin.go", Score: 80},
	)
	input.Universe.AuthorizedPaths = append(input.Universe.AuthorizedPaths,
		"internal/config/config.go", "internal/admin/admin.go",
	)
	result, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 2 {
		t.Fatalf("selected rounds = %d, want hard cap 2", len(result.Selected))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Gate.Reason != "targeted_round_limit" {
		t.Fatalf("skipped rounds = %#v, want targeted round limit", result.Skipped)
	}
}

func TestPlanTargetedRoundsDiversifiesSecondRound(t *testing.T) {
	repo := t.TempDir()
	writeResearchFile(t, repo, "beets/__main__.py", "from .ui import main\n\nmain([])\n")
	writeResearchFile(t, repo, "beets/ui/commands/__init__.py", "def dispatch():\n    return True\n")
	writeResearchFile(t, repo, "beets/ui/commands/import_/__init__.py", "def import_files():\n    return True\n")
	writeResearchFile(t, repo, "beets/importer/__init__.py", "class ImportSession:\n    pass\n")
	writeResearchFile(t, repo, "beets/plugins.py", "class BeetsPlugin:\n    pass\n")

	questions := []ProposedQuestion{
		{
			ID: "cli-dispatch", Purpose: "Clarify how argparse or similar is wired to subcommand modules, enabling trace of any CLI flow.",
			Question:     "How does beets/ui/commands/__init__.py register subcommands, and what is the dispatch mechanism from beets/__main__.py?",
			CandidateIDs: []string{"main", "commands"},
		},
		{
			ID: "plugin-api", Purpose: "Understand the plugin interface (base class, hooks) to know how plugins integrate.",
			Question:     "What base class or protocol do plugins in beetsplug/ follow, and how are they loaded by beets/plugins.py?",
			CandidateIDs: []string{"plugins"},
		},
		{
			ID: "import-pipeline-start", Purpose: "Trace the first steps of import to map the flow from CLI command to importer session.",
			Question:     "How does beets/ui/commands/import_/__init__.py invoke the importer subsystem (beets/importer), and what is the initial call?",
			CandidateIDs: []string{"import-command", "importer"},
		},
	}
	candidates := []FileCandidate{
		{ID: "main", Path: "beets/__main__.py", Score: 180},
		{ID: "commands", Path: "beets/ui/commands/__init__.py", Score: 50},
		{ID: "plugins", Path: "beets/plugins.py", Score: 50},
		{ID: "import-command", Path: "beets/ui/commands/import_/__init__.py", Score: 50},
		{ID: "importer", Path: "beets/importer/__init__.py", Score: 50},
	}
	authorized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		authorized = append(authorized, candidate.Path)
	}

	result, err := PlanTargetedRounds(context.Background(), PlanningInput{
		RepoPath: repo, Questions: questions, Candidates: candidates,
		InitialProviderPaths: authorized,
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: authorized},
		Policy:               DefaultPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(plannedQuestionIDs(result.Selected), ","); got != "cli-dispatch,plugin-api" {
		t.Fatalf("selected rounds = %v, want diverse CLI and plugin rounds", result.Selected)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Question.ID != "import-pipeline-start" ||
		result.Skipped[0].Gate.Reason != "targeted_round_limit" {
		t.Fatalf("skipped rounds = %#v, want overlapping import round at targeted limit", result.Skipped)
	}
	scores := make(map[string]int, len(result.Selected)+len(result.Skipped))
	for _, plan := range append(append([]PlannedRound(nil), result.Selected...), result.Skipped...) {
		scores[plan.Question.ID] = plan.Score
	}
	if scores["cli-dispatch"] != 8 || scores["import-pipeline-start"] != 8 || scores["plugin-api"] != 4 {
		t.Fatalf("planner scores = %v, want cli=8 import=8 plugin=4", scores)
	}
}

func TestExecuteRoundRejectsUnknownModelEvidenceID(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{response: []byte(`{
  "findings":[{"id":"invented","interpretation":"repository responsibility","hypothesis_assessment":"supported","evidence_ids":["unknown-evidence"]}],
  "unresolved_frontiers":[]
}`)}
	round, err := ExecuteRound(context.Background(), ExecuteInput{
		Plan: planResult.Selected[0], Policy: input.Policy, Repository: RepositoryContext{
			Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default",
		}, RunsDir: t.TempDir(), Profile: "test", Model: "saved", Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if round.Status != RoundRejected || len(round.ValidatedFindings) != 0 ||
		len(round.RejectedFindings) != 1 || round.RejectedFindings[0].Reason != "unknown_evidence_id" {
		t.Fatalf("round validation = %#v", round)
	}
}

func TestExecuteRoundReplaysIdenticalCachedInput(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := planResult.Selected[0].Bundle.Evidence[0].ID
	response, err := json.Marshal(researchResponse{Findings: []RawFinding{{
		ID: "finding-1", Interpretation: "backup is coordinated here",
		HypothesisAssessment: "supported", EvidenceIDs: []string{evidenceID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{
		response: response, inputTokens: 120, outputTokens: 17,
		promptCacheHitTokens: 96, promptCacheMissTokens: 24,
	}
	runsDir := t.TempDir()
	repository := RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"}
	execute := func() ResearchRound {
		round, executeErr := ExecuteRound(context.Background(), ExecuteInput{
			Plan: planResult.Selected[0], Policy: input.Policy,
			Repository: repository,
			RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
		})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		return round
	}
	first := execute()
	second := execute() // A report-template/layout change is intentionally absent from the fingerprint.
	if first.Status != RoundCompleted || second.Status != RoundCached || !second.Cached {
		t.Fatalf("round statuses = %q/%q cached=%t", first.Status, second.Status, second.Cached)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one call plus cache replay", provider.calls)
	}
	for _, round := range []ResearchRound{first, second} {
		if round.InputTokens != 120 || round.OutputTokens != 17 ||
			round.PromptCacheHitTokens != 96 || round.PromptCacheMissTokens != 24 {
			t.Fatalf("round token usage = %#v", round)
		}
	}
}

func TestExecuteRoundWithoutRunsDirDoesNotReuseOrPopulateCache(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := planResult.Selected[0].Bundle.Evidence[0].ID
	response, err := json.Marshal(researchResponse{Findings: []RawFinding{{
		ID: "finding-1", Interpretation: "backup is coordinated here",
		HypothesisAssessment: "supported", EvidenceIDs: []string{evidenceID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{response: response}
	repository := RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"}
	for range 2 {
		round, executeErr := ExecuteRound(context.Background(), ExecuteInput{
			Plan: planResult.Selected[0], Policy: input.Policy, Repository: repository,
			Profile: "test", Model: "saved", Provider: provider,
		})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		if round.Cached || round.Status != RoundCompleted {
			t.Fatalf("uncached round = %#v", round)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want one call per round", provider.calls)
	}
}

func TestExecuteRoundRefetchesInvalidCachedInput(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := planResult.Selected[0].Bundle.Evidence[0].ID
	response, err := json.Marshal(researchResponse{Findings: []RawFinding{{
		ID: "finding-1", Interpretation: "backup is coordinated here",
		HypothesisAssessment: "supported", EvidenceIDs: []string{evidenceID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{response: response}
	runsDir := t.TempDir()
	execute := func() ResearchRound {
		round, executeErr := ExecuteRound(context.Background(), ExecuteInput{
			Plan: planResult.Selected[0], Policy: input.Policy,
			Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
			RunsDir:    runsDir, Profile: "test", Model: "saved", Provider: provider,
		})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		return round
	}
	if round := execute(); round.Status != RoundCompleted {
		t.Fatalf("first round = %#v", round)
	}
	cacheFiles, err := filepath.Glob(filepath.Join(runsDir, cacheDirectory, "*.json"))
	if err != nil || len(cacheFiles) != 1 {
		t.Fatalf("cache files = %v, err = %v", cacheFiles, err)
	}
	if err := os.WriteFile(cacheFiles[0], []byte(`{"version":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if round := execute(); round.Status != RoundCompleted || round.Cached {
		t.Fatalf("refetched round = %#v", round)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want invalid cache to trigger a refetch", provider.calls)
	}
}

func TestStageResponseCachePreservesPromptCacheTokens(t *testing.T) {
	t.Parallel()

	runsDir := t.TempDir()
	request := []byte(`{"model":"fixture"}`)
	bundleHash := SHA256([]byte("bounded evidence"))
	cacheInput := StageCacheInput{
		RunsDir: runsDir,
		Fingerprint: FingerprintInput{
			Repository: RepositoryContext{Identity: "fixture", Revision: "abc", Scenario: "go-default"},
			Stage:      "guided_tour_leaf", PromptVersion: "prompt-v1", Profile: "test",
			Model: "deepseek-v4-flash", EvidenceBundleHash: bundleHash, PolicyVersion: PolicyVersion,
		},
		Request: request, EvidenceBundleHash: bundleHash,
	}
	_, err := SaveStageResponse(cacheInput, StageResponse{
		Content:     []byte(`{"status":"ok"}`),
		InputTokens: 120, OutputTokens: 17,
		PromptCacheHitTokens: 96, PromptCacheMissTokens: 24,
	})
	if err != nil {
		t.Fatalf("SaveStageResponse() error = %v", err)
	}
	response, found, err := LoadStageResponse(cacheInput)
	if err != nil {
		t.Fatalf("LoadStageResponse() error = %v", err)
	}
	if !found || !response.Cached {
		t.Fatalf("LoadStageResponse() found/cached = %t/%t", found, response.Cached)
	}
	if response.InputTokens != 120 || response.OutputTokens != 17 ||
		response.PromptCacheHitTokens != 96 || response.PromptCacheMissTokens != 24 {
		t.Fatalf("cached token usage = %#v", response)
	}
}

func TestStageResponseCacheReplaysLegacyRecordWithoutPromptCacheTokens(t *testing.T) {
	t.Parallel()

	runsDir := t.TempDir()
	request := []byte(`{"model":"legacy"}`)
	responseContent := []byte(`{"status":"ok"}`)
	bundleHash := SHA256([]byte("legacy evidence"))
	fingerprint := FingerprintInput{
		Repository: RepositoryContext{Identity: "legacy", Revision: "abc", Scenario: "go-default"},
		Stage:      "guided_tour_leaf", PromptVersion: "prompt-v1", Profile: "test",
		Model: "deepseek-v4-flash", EvidenceBundleHash: bundleHash, PolicyVersion: PolicyVersion,
	}
	cacheKey, err := CacheKey(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCache(runsDir, cacheRecord{
		Version: cacheRecordVersion, CacheKey: cacheKey,
		RequestSHA256: requestHash(request), BundleSHA256: bundleHash,
		ResponseSHA256: requestHash(responseContent), Response: responseContent,
		RequestBytes: len(request), ResponseBytes: len(responseContent),
		InputTokens: 41, OutputTokens: 7,
	}); err != nil {
		t.Fatalf("save legacy cache record: %v", err)
	}

	response, found, err := LoadStageResponse(StageCacheInput{
		RunsDir: runsDir, Fingerprint: fingerprint, Request: request, EvidenceBundleHash: bundleHash,
	})
	if err != nil {
		t.Fatalf("LoadStageResponse() error = %v", err)
	}
	if !found || response.PromptCacheHitTokens != 0 || response.PromptCacheMissTokens != 0 {
		t.Fatalf("legacy cache replay = found %t, response %#v", found, response)
	}
}

func TestFailedRoundPreservesGroundedLocalEvidence(t *testing.T) {
	input := basicPlanningInput(t)
	planResult, err := PlanTargetedRounds(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	provider := &savedProvider{err: errors.New("saved provider failure")}
	round, callErr := ExecuteRound(context.Background(), ExecuteInput{
		Plan: planResult.Selected[0], Policy: input.Policy,
		Repository: RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"},
		Model:      "saved", Provider: provider,
	})
	if callErr == nil || round.Status != RoundFailed {
		t.Fatalf("failed round = %#v, err=%v", round, callErr)
	}
	state := NewState(input.Policy, RepositoryContext{Identity: repoIdentity(t), Revision: "abc", Scenario: "go-default"})
	ApplyRound(&state, planResult.Selected[0], round)
	if len(state.Theory.GroundedFacts) == 0 {
		t.Fatal("failed targeted call discarded local evidence")
	}
}

func basicPlanningInput(t *testing.T) PlanningInput {
	t.Helper()
	repo := researchFixtureRepo(t)
	return PlanningInput{
		RepoPath: repo,
		Questions: []ProposedQuestion{{
			ID: "q-backup", Purpose: "backup architecture", Question: "How does backup run?",
			CandidateIDs: []string{"candidate-backup"},
		}},
		Candidates:           []FileCandidate{{ID: "candidate-backup", Path: "cmd/backup.go", Score: 100}},
		InitialProviderPaths: []string{"cmd/backup.go"},
		Universe:             LocalRepositoryUniverse{AuthorizedPaths: []string{"cmd/backup.go", "internal/repository/repository.go"}},
		Policy:               DefaultPolicy(),
	}
}

func researchFixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeResearchFile(t, repo, "cmd/backup.go", "package cmd\n\nfunc runBackup() { executeBackup() }\n")
	writeResearchFile(t, repo, "internal/repository/repository.go", "package repository\n\nfunc Save() error { return nil }\n")
	writeResearchFile(t, repo, "internal/unrelated.go", "package internal\n\nfunc Unrelated() {}\n")
	writeResearchFile(t, repo, "internal/config/config.go", "package config\n\nfunc Load() {}\n")
	writeResearchFile(t, repo, "internal/admin/admin.go", "package admin\n\nfunc Apply() {}\n")
	return repo
}

func writeResearchFile(t *testing.T, repo, path, content string) {
	t.Helper()
	absolute := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func plannedQuestionIDs(plans []PlannedRound) []string {
	ids := make([]string, 0, len(plans))
	for _, plan := range plans {
		ids = append(ids, plan.Question.ID)
	}
	return ids
}

func repoIdentity(t *testing.T) string {
	t.Helper()
	identity, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
