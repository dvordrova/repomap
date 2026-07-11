package investigation

import (
	"context"
	"encoding/json"
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

type fixtureAnalyzer struct {
	graph evidence.Graph
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
