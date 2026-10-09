package run

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// Owner decision 2026-09-26: a default comparison answer naming an unknown
// ref, or none, does not end the run. Target selection keeps every target and
// leaves the default unresolved; nothing is promoted in its place.
func TestRepositoryTargetPlanKeepsAnUnresolvedDefault(t *testing.T) {
	repository, goSource, project := repositoryTargetRuntimeInlineInputs(t)
	refs := make([]corpus.FileID, 0, 5)
	for _, path := range []string{
		"package.json", "native/runtime.py", "cmd/api/main.go", "cmd/worker/main.go", "pkg/client/client.go",
	} {
		refs = append(refs, repositoryTargetRuntimeFileRef(t, repository, path))
	}
	// The one answer serves the classification, which names no default, and
	// the default comparison it then needs, which names no ref and is refused.
	response, err := json.Marshal(map[string]any{"default_file_ref": nil, "target_file_refs": refs})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
	portfolioProvider := &targetPortfolioClientStub{response: response}
	providerCalls, discoveryCalls := 0, 0
	options := repositoryTargetRuntimeTestOptions(
		t, repository, &goSource, project, "", &providerCalls,
		&discoveryCalls, readmeProvider, portfolioProvider,
	)
	var console bytes.Buffer
	options.Output = newRunOutput(&console)

	plan, err := selectRepositoryTargetPlanForRun(context.Background(), options)
	if err != nil {
		t.Fatalf("a refused default comparison ended target selection: %v", err)
	}
	if portfolioProvider.calls != 2 {
		t.Fatalf("portfolio calls = %d, want the classification and one default comparison", portfolioProvider.calls)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate plan with an unresolved default: %v", err)
	}
	if _, found := plan.DefaultTarget(); found || plan.Default != (repositoryTargetKey{}) || plan.Outcome.SelectedRef != "" {
		t.Fatalf("default = %#v / %q, want it unresolved", plan.Default, plan.Outcome.SelectedRef)
	}
	if len(plan.Targets) != 5 || plan.Outcome.SelectedTargets != 5 {
		t.Fatalf("targets = %d, want all five", len(plan.Targets))
	}
	ordered, err := repositoryTargetExecutionOrder(plan)
	if err != nil {
		t.Fatalf("order targets without a default: %v", err)
	}
	for position := range plan.Targets {
		if ordered[position].Key != plan.Targets[position].Key {
			t.Fatalf("execution order promoted %s over plan order", ordered[0].Key)
		}
	}
	if !strings.Contains(console.String(), "default: unresolved") {
		t.Fatalf("console does not say the default is unresolved:\n%s", console.String())
	}
}

// The dispatcher publishes a plan whose default is unresolved: target pages
// run in plan order, the first published page owns the one report, the
// outcome portfolio records no default, and a new process reads and serves
// the saved report. The plan is the explicit one with its default removed,
// exactly as target selection leaves an unresolved default
// (TestRepositoryTargetPlanKeepsAnUnresolvedDefault); its explicit default,
// the worker, is deliberately not first in plan order.
func TestUnresolvedDefaultPublishesTheFirstPageInPlanOrder(t *testing.T) {
	ctx := context.Background()
	repositoryRoot := ordinaryGraphCumulativeRepository(t)
	debugDir := t.TempDir()
	const selectors = "python:.:guard:src/acme/worker,python:.:guard:src/acme/api"
	const runID = "unresolved-default"
	repository, err := corpus.Open(ctx, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	state, err := freshness.CaptureRepository(ctx, repositoryRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := selectRepositoryTargetPlanForRun(ctx, repositoryTargetRuntimeOptions{
		RepoName: "fixture", Repository: repository, DiscoverPython: true, DiscoverJSTS: true,
		TargetOverride: selectors, NoModel: true, ScoutJSTSFn: jstsproject.ScoutTargets,
	})
	if err != nil {
		t.Fatal(err)
	}
	if explicit, found := plan.DefaultTarget(); !found || explicit.Selector != "python:.:guard:src/acme/worker" ||
		len(plan.Targets) != 2 || plan.Targets[0].Selector != "python:.:guard:src/acme/api" {
		t.Fatalf("fixture plan = %+v; want the worker default after the api", plan.Targets)
	}
	plan = plan.withoutGuidance()
	plan.Default, plan.Outcome.SelectedRef = repositoryTargetKey{}, ""

	var served []report.RunReceipt
	reportPath, err := dispatchRepositoryTargetPlan(ctx, repositoryTargetDispatchOptions{
		Repo:      repositoryRoot,
		ExtraArgs: []string{"--no-model", "--target", selectors, "--no-open", "--debug-dir", debugDir},
		Deps: defaultRunDeps{
			ctx: ctx, stdout: io.Discard, stderr: io.Discard,
			llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
			serveReport: func(context.Context, reportserver.Options) error { return nil },
			openReport:  func(string) error { return nil },
		},
		Corpus: repository, RepositoryState: state, Plan: plan,
		RunID: runID, DebugDir: debugDir, NoModel: true, NoOpen: true,
		Output: newRunOutput(io.Discard), FirstLayer: debugdump.NewSemanticObserver(nil),
		DiscoverJSTSFn:   jstsproject.DiscoverSelected,
		VerifiedRunsSink: func(receipts []report.RunReceipt) { served = receipts },
	})
	if err != nil {
		t.Fatalf("an unresolved default ended the run: %v", err)
	}
	ownerDir := filepath.Join(debugDir, runID)
	if filepath.Dir(reportPath) != ownerDir || len(served) != 1 {
		t.Fatalf("report %s, %d receipts; want one report in %s", reportPath, len(served), ownerDir)
	}
	if published := ordinaryGraphRunDirs(t, debugDir); len(published) != 1 || published[0] != ownerDir {
		t.Fatalf("published run directories = %v", published)
	}

	raw, err := os.ReadFile(filepath.Join(ownerDir, targetoutcome.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := targetoutcome.Decode(raw)
	if err != nil {
		t.Fatalf("saved outcome portfolio does not validate: %v", err)
	}
	if outcomes.DefaultSelectedTargetID != "" || len(outcomes.Outcomes) != 2 ||
		outcomes.Outcomes[0].SelectedTarget.ID != "t1" ||
		outcomes.Outcomes[0].SelectedTarget.Selector != "python:.:guard:src/acme/api" ||
		outcomes.Outcomes[0].State != targetoutcome.StateAnalyzed ||
		outcomes.Outcomes[1].State != targetoutcome.StateAnalyzed {
		t.Fatalf("saved outcome portfolio = %+v", outcomes)
	}

	// A new process reads the saved report and serves it.
	restored, err := report.ReadRunReceipt(ownerDir)
	if err != nil {
		t.Fatalf("saved report does not read: %v", err)
	}
	data := restored.Data()
	if data.TargetOutcomePortfolio == nil || data.TargetOutcomePortfolio.DefaultSelectedTargetID != "" ||
		data.ProgramPortfolio == nil || data.ProgramPortfolio.DefaultTargetID != "t1" ||
		data.ProgramPortfolio.Len() != 2 {
		t.Fatalf("saved report does not keep the unresolved default beside its owner page")
	}
	if _, err := report.RenderSavedHTML(ownerDir); err != nil {
		t.Fatalf("saved report does not render again: %v", err)
	}
	const capability = "unresolved-default-test"
	handler, err := reportserver.NewHandler(reportserver.Options{
		RunsDir: debugDir, InitialRunID: runID, Capability: capability,
		OpenFile: func(context.Context, string, int, int) error { return nil },
	})
	if err != nil {
		t.Fatalf("server cannot load the saved report: %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/_repomap/"+capability+"/runs/"+runID+"/"+restored.HTMLFilename(), nil,
	))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<html") {
		t.Fatalf("served report = %d", response.Code)
	}
}
