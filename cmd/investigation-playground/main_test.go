package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

func TestPrepareSessionHandsOffSelectedOrientationFlow(t *testing.T) {
	repoPath, revision := newGitRepository(t)
	reportPath := filepath.Join(t.TempDir(), "orientation.json")
	report := fmt.Sprintf(`{
  "repo_name": %q,
  "explained_flows": [{"flow_seed":{"id":"put-flow","name":"HTTP/gRPC Put"}}]
}`, filepath.Base(repoPath))
	writeTestFile(t, reportPath, []byte(report))

	session, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{
		repoPath:        repoPath,
		symbolQuery:     "kvServer.Put",
		orientationJSON: reportPath,
		flowID:          "put-flow",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped || preserveArtifacts || session.State != investigation.StateResolvingSymbol {
		t.Fatalf("stopped = %v, preserve = %v, state = %q", stopped, preserveArtifacts, session.State)
	}
	if session.Origin == nil || session.Origin.FlowID != "put-flow" ||
		session.Origin.FlowName != "HTTP/gRPC Put" || session.Origin.AcceptedRevision != revision {
		t.Fatalf("origin = %#v", session.Origin)
	}
	if session.Goal.Text != `investigate flow "HTTP/gRPC Put" through kvServer.Put` {
		t.Fatalf("goal = %q", session.Goal.Text)
	}
}

func TestPrepareSessionRejectsOrientationRepositoryMismatch(t *testing.T) {
	repoPath, _ := newGitRepository(t)
	reportPath := filepath.Join(t.TempDir(), "orientation.json")
	writeTestFile(t, reportPath, []byte(`{
  "repo_name":"another-repository",
  "explained_flows":[{"flow_seed":{"id":"put-flow","name":"Put"}}]
}`))

	_, _, _, err := prepareSession(context.Background(), config{
		repoPath:        repoPath,
		symbolQuery:     "kvServer.Put",
		orientationJSON: reportPath,
		flowID:          "put-flow",
	})
	if err == nil || !strings.Contains(err.Error(), "another-repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareSessionStoresCanonicalRepositoryRoot(t *testing.T) {
	repoPath, _ := newGitRepository(t)
	subdirectory := filepath.Join(repoPath, "nested")
	if err := os.Mkdir(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}

	session, _, _, err := prepareSession(context.Background(), config{
		repoPath:    subdirectory,
		symbolQuery: "kvServer.Put",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if session.Repository.Path != wantRoot || !filepath.IsAbs(session.Repository.Path) {
		t.Fatalf("repository path = %q, want %q", session.Repository.Path, wantRoot)
	}
}

func TestPrepareResumedSessionRejectsExplicitRepositoryOverride(t *testing.T) {
	_, _, _, err := prepareSession(context.Background(), config{
		resumePath:   "does-not-need-to-exist.json",
		repoExplicit: true,
	})
	if err == nil || !strings.Contains(err.Error(), "--repo cannot be combined") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareResumedSessionAppliesOnlyExplicitSafeChoices(t *testing.T) {
	repoPath, revision := newGitRepository(t)
	origin := &investigation.Origin{
		Kind:             investigation.OriginOrientationFlow,
		Status:           investigation.OriginCandidate,
		ReportSHA256:     strings.Repeat("a", 64),
		RepoName:         filepath.Base(repoPath),
		FlowID:           "put-flow",
		FlowName:         "HTTP/gRPC Put",
		AcceptedRevision: revision,
	}
	waiting := waitingSessionFixture(t, repoPath, revision, origin)
	sessionPath := filepath.Join(t.TempDir(), "investigation_session.json")
	writeSessionFixture(t, sessionPath, waiting)

	t.Run("no choice preserves pending question", func(t *testing.T) {
		got, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{resumePath: sessionPath})
		if err != nil {
			t.Fatal(err)
		}
		if !stopped || !preserveArtifacts || !reflect.DeepEqual(got, waiting) {
			t.Fatalf("stopped = %v, preserve = %v, session changed", stopped, preserveArtifacts)
		}
	})

	t.Run("continue rejects presentation action", func(t *testing.T) {
		_, _, _, err := prepareSession(context.Background(), config{
			resumePath:  sessionPath,
			continueRun: true,
		})
		if err == nil || !strings.Contains(err.Error(), "cannot execute pending action") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("finish", func(t *testing.T) {
		got, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{resumePath: sessionPath, finish: true})
		if err != nil {
			t.Fatal(err)
		}
		if stopped || !preserveArtifacts || got.State != investigation.StateCompleted || got.Stop == nil ||
			got.Stop.Kind != investigation.StopFinished || got.Origin == nil {
			t.Fatalf("stopped = %v, preserve = %v, session = %#v", stopped, preserveArtifacts, got)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		got, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{
			resumePath:  sessionPath,
			symbolQuery: "kvServer.DeleteRange",
		})
		if err != nil {
			t.Fatal(err)
		}
		if stopped || preserveArtifacts || got.State != investigation.StateResolvingSymbol || got.Focus.Symbol != "kvServer.DeleteRange" ||
			got.Origin == nil || got.Origin.FlowID != origin.FlowID || got.Symbol != nil || got.Source != nil {
			t.Fatalf("stopped = %v, preserve = %v, session = %#v", stopped, preserveArtifacts, got)
		}
	})
}

func TestPrepareResumedSessionContinuesOnlyExecutableCapability(t *testing.T) {
	repoPath, revision := newGitRepository(t)
	assessing := assessingSessionFixture(t, repoPath, revision, nil)
	sessionPath := filepath.Join(t.TempDir(), "investigation_session.json")
	writeSessionFixture(t, sessionPath, assessing)

	_, _, _, err := prepareSession(context.Background(), config{
		resumePath:  sessionPath,
		continueRun: true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires --deepseek") {
		t.Fatalf("error = %v", err)
	}

	got, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{
		resumePath:   sessionPath,
		continueRun:  true,
		callDeepSeek: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped || !preserveArtifacts || !reflect.DeepEqual(got, assessing) {
		t.Fatalf("stopped = %v, preserve = %v, session changed", stopped, preserveArtifacts)
	}
}

func TestPrepareResumedSessionMakesRepositoryChangeVisibleBeforeChoice(t *testing.T) {
	repoPath, revision := newGitRepository(t)
	origin := &investigation.Origin{
		Kind:             investigation.OriginOrientationFlow,
		Status:           investigation.OriginCandidate,
		ReportSHA256:     strings.Repeat("b", 64),
		RepoName:         filepath.Base(repoPath),
		FlowID:           "put-flow",
		FlowName:         "Put flow",
		AcceptedRevision: revision,
	}
	waiting := waitingSessionFixture(t, repoPath, revision, origin)
	sessionPath := filepath.Join(t.TempDir(), "investigation_session.json")
	writeSessionFixture(t, sessionPath, waiting)
	writeTestFile(t, filepath.Join(repoPath, "main.go"), []byte("package fixture\n\n// changed\n"))

	got, stopped, preserveArtifacts, err := prepareSession(context.Background(), config{
		resumePath: sessionPath,
		finish:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stopped || preserveArtifacts || got.State != investigation.StateResolvingSymbol || got.Repository.Revision == revision ||
		got.Origin != nil || got.Symbol != nil || got.Source != nil || got.SourceReport != nil {
		t.Fatalf("stopped = %v, preserve = %v, session = %#v", stopped, preserveArtifacts, got)
	}
}

func TestWriteRunRemovesOnlyStaleGeneratedArtifacts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range allGeneratedArtifactNames() {
		writeTestFile(t, filepath.Join(dir, name), []byte("stale"))
	}
	writeTestFile(t, filepath.Join(dir, "keep-me.txt"), []byte("user file"))

	if err := writeRun(dir, investigation.Session{Version: investigation.SessionVersion}, runArtifacts{}, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range allGeneratedArtifactNames() {
		_, err := os.Stat(filepath.Join(dir, name))
		if name == "investigation_session.json" {
			if err != nil {
				t.Fatalf("current session artifact: %v", err)
			}
			continue
		}
		if !os.IsNotExist(err) {
			t.Fatalf("stale artifact %s still exists or stat failed: %v", name, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "keep-me.txt")); err != nil || string(data) != "user file" {
		t.Fatalf("unrelated file changed: data=%q err=%v", data, err)
	}
}

func TestWriteRunPreservesPriorRunArtifactsForSameEvidenceLineage(t *testing.T) {
	dir := t.TempDir()
	repoPath, revision := newGitRepository(t)
	session := waitingSessionFixture(t, repoPath, revision, nil)
	artifacts := runArtifacts{
		graphJSON:           []byte(`{"graph":"a"}`),
		rawSource:           []byte(`{"response":"a"}`),
		evaluationJSON:      []byte(`{"score":100}`),
		parseWarnings:       []byte(`[]`),
		deepseekRequest:     []byte(`{"request":"a"}`),
		sourceProvider:      "deepseek",
		sourceModel:         "fixture-model",
		sourcePromptVersion: "source-fixture-v1",
	}
	if err := writeRun(dir, session, artifacts, false); err != nil {
		t.Fatal(err)
	}
	if err := writeRun(dir, session, runArtifacts{}, true); err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{
		"evidence_graph.json":                   artifactPayload(artifacts.graphJSON),
		"deepseek_source_request.redacted.json": artifactPayload(artifacts.deepseekRequest),
		"deepseek_source_response.raw.txt":      artifactPayload(artifacts.rawSource),
		"source_evaluation.json":                artifactPayload(artifacts.evaluationJSON),
		"source_parse_warnings.json":            artifactPayload(artifacts.parseWarnings),
	}
	for name, wantData := range want {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !reflect.DeepEqual(data, wantData) {
			t.Fatalf("run artifact %s changed: data=%q want=%q err=%v", name, data, wantData, err)
		}
	}
	manifest, err := readRunArtifactManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	var callGroup string
	for _, name := range sourceRunArtifactNames {
		entry := manifest.Artifacts[name]
		if entry.Provider != "deepseek" || entry.Model != "fixture-model" || entry.PromptVersion != "source-fixture-v1" ||
			entry.ParserVersion != sourceexplain.ParserVersion || entry.EvaluatorVersion != sourceexplain.EvaluationVersion ||
			len(entry.CallGroupSHA256) != 64 {
			t.Fatalf("model manifest entry %s = %#v", name, entry)
		}
		if callGroup == "" {
			callGroup = entry.CallGroupSHA256
		} else if entry.CallGroupSHA256 != callGroup {
			t.Fatalf("model artifacts have different call groups: %q != %q", entry.CallGroupSHA256, callGroup)
		}
	}
}

func TestWriteRunDoesNotReuseArtifactsAcrossEvidenceLineages(t *testing.T) {
	dir := t.TempDir()
	repoPath, revision := newGitRepository(t)
	session := waitingSessionFixture(t, repoPath, revision, nil)
	if err := writeRun(dir, session, runArtifacts{
		graphJSON:           []byte(`{"graph":"a"}`),
		rawSource:           []byte(`{"response":"a"}`),
		evaluationJSON:      []byte(`{"score":100}`),
		parseWarnings:       []byte(`[]`),
		deepseekRequest:     []byte(`{"request":"a"}`),
		sourceProvider:      "deepseek",
		sourceModel:         "fixture-model",
		sourcePromptVersion: "source-fixture-v1",
	}, false); err != nil {
		t.Fatal(err)
	}
	session.Goal.Text = "a different investigation lineage"
	if err := writeRun(dir, session, runArtifacts{}, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range append(append([]string{}, runArtifactNames...), runArtifactManifestName) {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("foreign run artifact %s still exists: %v", name, err)
		}
	}
}

func TestWriteRunDoesNotReuseAcceptedModelOutcomeForPendingSession(t *testing.T) {
	dir := t.TempDir()
	repoPath, revision := newGitRepository(t)
	accepted := waitingSessionFixture(t, repoPath, revision, nil)
	pending := assessingSessionFixture(t, repoPath, revision, nil)
	artifacts := runArtifacts{
		graphJSON:           []byte(`{"graph":"a"}`),
		rawSource:           []byte(`{"response":"a"}`),
		evaluationJSON:      []byte(`{"score":100}`),
		parseWarnings:       []byte(`[]`),
		deepseekRequest:     []byte(`{"request":"a"}`),
		sourceProvider:      "deepseek",
		sourceModel:         "fixture-model",
		sourcePromptVersion: "source-fixture-v1",
	}
	if err := writeRun(dir, accepted, artifacts, false); err != nil {
		t.Fatal(err)
	}
	if err := writeRun(dir, pending, runArtifacts{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence_graph.json")); err != nil {
		t.Fatalf("shared structural artifact was not retained: %v", err)
	}
	for _, name := range sourceRunArtifactNames {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("accepted model artifact %s leaked into pending session: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "source_report.json")); !os.IsNotExist(err) {
		t.Fatalf("accepted source report leaked into pending session: %v", err)
	}
}

func TestResumeOwnsRunArtifactsOnlyForOutputSessionFile(t *testing.T) {
	dir := t.TempDir()
	outputSession := filepath.Join(dir, "investigation_session.json")
	otherSession := filepath.Join(t.TempDir(), "investigation_session.json")
	writeTestFile(t, outputSession, []byte(`{"session":"output"}`))
	writeTestFile(t, otherSession, []byte(`{"session":"other"}`))

	if !resumeOwnsRunArtifacts(outputSession, dir) {
		t.Fatal("output session did not own its colocated run artifacts")
	}
	if resumeOwnsRunArtifacts(otherSession, dir) {
		t.Fatal("foreign session was allowed to reuse output run artifacts")
	}
}

func allGeneratedArtifactNames() []string {
	names := append([]string{}, sessionArtifactNames...)
	names = append(names, runArtifactNames...)
	return append(names, runArtifactManifestName)
}

func waitingSessionFixture(t *testing.T, repoPath, revision string, origin *investigation.Origin) investigation.Session {
	t.Helper()
	session := assessingSessionFixture(t, repoPath, revision, origin)
	parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.Report
	for _, allowed := range session.Assessment.AllowedActions {
		if allowed.Operation == sourceexplain.OperationReadCallee {
			report.NextAction = sourceexplain.Action{
				ID:               allowed.ID,
				Operation:        allowed.Operation,
				AnchorEvidenceID: allowed.AnchorEvidenceID,
				Origin:           sourceexplain.ActionOriginModel,
			}
			break
		}
	}
	session = reduceTestEvent(t, session, investigation.Event{
		Kind:         investigation.EventSourceAssessed,
		ActionID:     session.Next[0].ID,
		SourceReport: &report,
	})
	if session.State != investigation.StateWaitingUser {
		t.Fatalf("fixture state = %q", session.State)
	}
	return session
}

func assessingSessionFixture(t *testing.T, repoPath, revision string, origin *investigation.Origin) investigation.Session {
	t.Helper()
	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "investigate Put flow"},
			Repository: investigation.Repository{Path: repoPath, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: "kvServer.Put"},
			Origin:     origin,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var card sourcecard.Card
	if err := json.Unmarshal(deepseektest.SourceCardJSON, &card); err != nil {
		t.Fatal(err)
	}
	structural := structuralBundleFixture(card)
	session = reduceTestEvent(t, session, investigation.Event{
		Kind:     investigation.EventSymbolResolved,
		ActionID: session.Next[0].ID,
		Symbol:   &structural,
	})
	session = reduceTestEvent(t, session, investigation.Event{
		Kind:     investigation.EventSourceRead,
		ActionID: session.Next[0].ID,
		Source:   &card,
	})
	if session.State != investigation.StateAssessingSource {
		t.Fatalf("fixture state = %q", session.State)
	}
	return session
}

func structuralBundleFixture(card sourcecard.Card) symbol.Bundle {
	target := evidence.Entity{
		ID:       card.Target.EntityID,
		Kind:     card.Target.Kind,
		Name:     card.Target.Name,
		Language: "go",
		Location: &evidence.Location{Path: card.Target.Path, Line: card.Target.Line, Column: card.Target.Column},
	}
	provenance := func(line int) []evidence.Provenance {
		return []evidence.Provenance{{
			Provider:  "gopls",
			Version:   "fixture",
			Operation: "call_hierarchy",
			Location:  &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
		}}
	}
	call := func(id, name string, line int) symbol.CallFact {
		return symbol.CallFact{
			EvidenceID: id,
			Caller:     target,
			Callee: evidence.Entity{
				ID:       "callee-" + id,
				Kind:     evidence.EntityFunction,
				Name:     name,
				Language: "go",
				Location: &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
			},
			Callsite:   &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
			Certainty:  evidence.CertaintyStatic,
			Provenance: provenance(line),
			Scenarios:  []string{"gopls-active-build"},
		}
	}
	return symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: card.RepoName,
		Query:    card.Target.Name,
		Target: symbol.Fact{
			EvidenceID: card.Target.EvidenceID,
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
			Provenance: provenance(card.Target.Line),
			Scenarios:  []string{"gopls-active-build"},
		},
		OutgoingCalls: []symbol.CallFact{
			call("call-out-001", "fill", 100),
			call("call-out-002", "checkPutRequest", 91),
			call("call-out-003", "togRPCError", 97),
			call("call-out-004", "Put", 95),
		},
		Scenarios:    []symbol.Scenario{{ID: "gopls-active-build"}},
		AllowedPaths: []string{card.Target.Path},
		Warnings:     []string{},
		Truncated:    map[string]int{},
	}
}

func reduceTestEvent(t *testing.T, session investigation.Session, event investigation.Event) investigation.Session {
	t.Helper()
	next, _, err := investigation.Reduce(session, event)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func newGitRepository(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	gitTestCommand(t, dir, "init", "--quiet")
	writeTestFile(t, filepath.Join(dir, "main.go"), []byte("package fixture\n"))
	gitTestCommand(t, dir, "add", "main.go")
	gitTestCommand(t, dir, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "fixture")
	revision, err := repositoryRevision(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, revision
}

func gitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func writeSessionFixture(t *testing.T, path string, session investigation.Session) {
	t.Helper()
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, data)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
