package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestRunnerExecutesRequestedCubesUntilUserBoundary(t *testing.T) {
	t.Parallel()

	repo := runnerRepo(t)
	runner := Runner{
		Analyzer:        fixtureAnalyzer{graph: runnerGraph(repo)},
		SourceAssessor:  fixtureAssessor{},
		ReferenceFinder: fixtureReferenceFinder{},
	}
	session, _, err := Reduce(Session{}, Event{
		Kind: EventStarted,
		Start: &StartInput{
			Goal:       Goal{Text: "understand server.Work"},
			Repository: Repository{Path: repo, Revision: "fixture-revision"},
			Focus:      Focus{Kind: FocusSymbol, Symbol: "server.Work"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	seenGraph := false
	seenExplanation := false
	for session.Next[0].Kind != ActionAwaitUser {
		execution, err := runner.Execute(context.Background(), session, session.Next[0])
		if err != nil {
			t.Fatal(err)
		}
		seenGraph = seenGraph || execution.Graph != nil
		seenExplanation = seenExplanation || execution.SourceExplanation != nil
		session, _, err = Reduce(session, execution.Event)
		if err != nil {
			t.Fatalf("Reduce(%s) error = %v", execution.Event.Kind, err)
		}
	}
	if !seenGraph || !seenExplanation {
		t.Fatalf("diagnostic artifacts: graph=%v explanation=%v", seenGraph, seenExplanation)
	}
	if session.State != StateWaitingUser || session.Tests == nil || len(session.Tests.References) == 0 {
		t.Fatalf("session = %#v", session)
	}
	if _, err := runner.Execute(context.Background(), session, session.Next[0]); err == nil {
		t.Fatal("Runner.Execute(await_user) error = nil")
	}
}

func TestRunnerTurnsCapabilityFailureAndCancellationIntoEvents(t *testing.T) {
	t.Parallel()

	session := advanceToAssessing(t)
	execution, err := (Runner{}).Execute(context.Background(), session, session.Next[0])
	if err != nil {
		t.Fatal(err)
	}
	if execution.Event.Kind != EventActionFailed || execution.Event.ActionID != session.Next[0].ID {
		t.Fatalf("failure event = %#v", execution.Event)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session = startFixture(t)
	execution, err = (Runner{}).Execute(ctx, session, session.Next[0])
	if err != nil {
		t.Fatal(err)
	}
	if execution.Event.Kind != EventCanceled || execution.Event.ActionID != session.Next[0].ID {
		t.Fatalf("canceled event = %#v", execution.Event)
	}
}

func TestRunnerUsesLocationPinnedAnalyzerForExactFocus(t *testing.T) {
	t.Parallel()

	repo := runnerRepo(t)
	graph := runnerGraph(repo)
	var target evidence.Entity
	for _, entity := range graph.Entities {
		if entity.ID == "target" {
			target = entity
			break
		}
	}
	exact := &fixtureExactAnalyzer{graph: graph}
	runner := Runner{ExactAnalyzer: exact}
	session, _, err := Reduce(Session{}, Event{
		Kind: EventStarted,
		Start: &StartInput{
			Goal:       Goal{Text: "understand selected server.Work declaration"},
			Repository: Repository{Path: repo, Revision: "fixture-revision"},
			Focus:      Focus{Kind: FocusSymbol, Symbol: target.Name, Entity: &target},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := runner.Execute(context.Background(), session, session.Next[0])
	if err != nil {
		t.Fatal(err)
	}
	if exact.calls != 1 || execution.Event.Kind != EventSymbolResolved || execution.Event.Symbol == nil {
		t.Fatalf("exact calls=%d event=%#v", exact.calls, execution.Event)
	}
}

func TestRunnerCompletesComponentSourceWithTargetOnlyTestReferences(t *testing.T) {
	t.Parallel()

	structural := structuralFixture(t)
	structural.OutgoingCalls = nil
	revision := "revision-1"
	target := structural.Target.Entity
	session, _, err := Reduce(Session{}, Event{
		Kind: EventStarted,
		Start: &StartInput{
			Goal:       Goal{Text: "inspect selected component symbol"},
			Repository: Repository{Path: "/repo", Revision: revision},
			Focus:      Focus{Kind: FocusSymbol, Symbol: target.Name, Entity: &target},
			Origin: &Origin{
				Kind:             OriginOrientationComponent,
				Status:           OriginCandidate,
				ReportSHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				RepoName:         "etcd",
				ComponentID:      "component-kv",
				AnchorID:         "anchor-put",
				AcceptedRevision: revision,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session = reduceFixture(t, session, Event{Kind: EventSymbolResolved, ActionID: session.Next[0].ID, Symbol: &structural})
	card := sourceCardFixture(t)
	session = reduceFixture(t, session, Event{Kind: EventSourceRead, ActionID: session.Next[0].ID, Source: &card})
	if session.State != StateFindingTestReferences || session.Next[0].Kind != ActionFindTestReferences ||
		session.Assessment != nil || session.SourceReport != nil {
		t.Fatalf("source-ready session = %#v", session)
	}

	execution, err := (Runner{ReferenceFinder: fixtureReferenceFinder{}}).Execute(context.Background(), session, session.Next[0])
	if err != nil || execution.DiagnosticError != nil {
		t.Fatalf("Execute(find_test_references) = %#v, %v", execution, err)
	}
	session = reduceFixture(t, session, execution.Event)
	if session.State != StateCompleted || session.Stop == nil || session.Stop.Kind != StopFinished ||
		session.Tests == nil || len(session.Tests.Searches) != 1 || session.Tests.Searches[0].AnchorEvidenceID != session.Focus.EvidenceID ||
		session.Assessment != nil || session.SourceReport != nil {
		t.Fatalf("completed local checkpoint = %#v", session)
	}
}

type fixtureAnalyzer struct {
	graph evidence.Graph
}

type fixtureExactAnalyzer struct {
	graph evidence.Graph
	calls int
}

func (f *fixtureExactAnalyzer) AnalyzeExactSymbol(_ context.Context, request analysis.ExactSymbolRequest) (evidence.Graph, error) {
	f.calls++
	if request.Symbol.Location == nil || request.Symbol.Location.Path != "pkg/work.go" {
		return evidence.Graph{}, fmt.Errorf("unexpected exact symbol: %#v", request.Symbol)
	}
	return f.graph, nil
}

func (f fixtureAnalyzer) Analyze(_ context.Context, _ analysis.Request) (evidence.Graph, error) {
	return f.graph, nil
}

type fixtureAssessor struct{}

func (fixtureAssessor) AssessSource(_ context.Context, bundleJSON []byte) ([]byte, error) {
	var bundle sourceexplain.Bundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return nil, err
	}
	type assessment struct {
		QuestionID        string   `json:"question_id"`
		Verdict           string   `json:"verdict"`
		SourceEvidenceIDs []string `json:"source_evidence_ids"`
	}
	assessments := make([]assessment, 0, len(bundle.Questions))
	for _, question := range bundle.Questions {
		assessments = append(assessments, assessment{
			QuestionID:        question.ID,
			Verdict:           "shown",
			SourceEvidenceIDs: append([]string{}, question.CandidateSourceEvidenceIDs...),
		})
	}
	return json.Marshal(struct {
		Assessments  []assessment            `json:"assessments"`
		Unknowns     []sourceexplain.Unknown `json:"unknowns"`
		NextActionID string                  `json:"next_action_id"`
	}{
		Assessments: assessments,
		Unknowns: []sourceexplain.Unknown{
			{Kind: sourceexplain.UnknownTestCoverage, AnchorEvidenceID: bundle.Target.EvidenceID},
			{Kind: sourceexplain.UnknownRuntimeReachability, AnchorEvidenceID: bundle.Target.EvidenceID},
		},
		NextActionID: "action-find-tests",
	})
}

func runnerRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	source := `package fixture

func (s *server) Work(v string) (*Response, error) {
	if err := checkInput(v); err != nil {
		return nil, err
	}

	response, err := s.inner.Work(v)
	if err != nil {
		return nil, toError(err)
	}

	s.header.fill(response)
	return response, nil
}
`
	if err := os.WriteFile(filepath.Join(repo, "pkg", "work.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}

func runnerGraph(repo string) evidence.Graph {
	graph := evidence.NewGraph(repo, "server.Work")
	graph.Scenarios = []evidence.Scenario{{ID: "active-build", Name: "fixture build"}}
	query := evidence.Entity{ID: "query", Kind: evidence.EntityQuery, Name: "server.Work"}
	target := evidence.Entity{
		ID:       "target",
		Kind:     evidence.EntityMethod,
		Name:     "server.Work",
		Language: "go",
		Location: &evidence.Location{Path: "pkg/work.go", Line: 3, Column: 18},
	}
	graph.AddEntity(query)
	graph.AddEntity(target)
	provenance := func(operation string, line int) []evidence.Provenance {
		return []evidence.Provenance{{
			Provider:  "fixture",
			Version:   "v1",
			Operation: operation,
			Location:  &evidence.Location{Path: "pkg/work.go", Line: line, Column: 2},
		}}
	}
	graph.AddRelation(evidence.Relation{
		From:       query.ID,
		To:         target.ID,
		Kind:       evidence.RelationMatchesQuery,
		Certainty:  evidence.CertaintyPossible,
		Provenance: provenance("workspace_symbol", 3),
		Scenarios:  []string{"active-build"},
	})
	graph.AddRelation(evidence.Relation{
		From:       query.ID,
		To:         target.ID,
		Kind:       evidence.RelationResolvesTo,
		Certainty:  evidence.CertaintyStatic,
		Provenance: provenance("workspace_symbol", 3),
		Scenarios:  []string{"active-build"},
	})
	callees := []struct {
		id   string
		name string
		line int
	}{
		{id: "callee-check", name: "checkInput", line: 4},
		{id: "callee-fill", name: "fill", line: 13},
		{id: "callee-to-error", name: "toError", line: 10},
		{id: "callee-work", name: "Work", line: 8},
	}
	for _, item := range callees {
		callee := evidence.Entity{
			ID:       item.id,
			Kind:     evidence.EntityFunction,
			Name:     item.name,
			Language: "go",
			Location: &evidence.Location{Path: "pkg/work.go", Line: item.line, Column: 2},
		}
		graph.AddEntity(callee)
		graph.AddRelation(evidence.Relation{
			From:       target.ID,
			To:         callee.ID,
			Kind:       evidence.RelationCalls,
			Certainty:  evidence.CertaintyStatic,
			Provenance: provenance("call_hierarchy", item.line),
			Scenarios:  []string{"active-build"},
		})
	}
	graph.Sort()
	return graph
}
