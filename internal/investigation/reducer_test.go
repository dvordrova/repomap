package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

func TestReduceReplaysSourceGroundedSymbolJourney(t *testing.T) {
	t.Parallel()

	session := startFixture(t)
	assertPending(t, session, ActionResolveSymbol, 1)

	structural := structuralFixture(t)
	session = reduceFixture(t, session, Event{
		Kind:     EventSymbolResolved,
		ActionID: session.Next[0].ID,
		Symbol:   &structural,
	})
	assertPending(t, session, ActionReadSource, 2)

	card := sourceCardFixture(t)
	session = reduceFixture(t, session, Event{
		Kind:     EventSourceRead,
		ActionID: session.Next[0].ID,
		Source:   &card,
	})
	assertPending(t, session, ActionAssessSource, 3)
	if session.Assessment == nil || len(session.Assessment.Questions) != 4 {
		t.Fatalf("assessment = %#v", session.Assessment)
	}

	parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.Report
	session = reduceFixture(t, session, Event{
		Kind:         EventSourceAssessed,
		ActionID:     session.Next[0].ID,
		SourceReport: &report,
	})
	assertPending(t, session, ActionFindTests, 4)

	tests, err := testevidence.Collect(
		context.Background(),
		fixtureReferenceFinder{},
		"/repo",
		*session.Symbol,
		*session.Assessment,
		*session.SourceReport,
		testevidence.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	session = reduceFixture(t, session, Event{
		Kind:     EventTestReferencesFound,
		ActionID: session.Next[0].ID,
		Tests:    &tests,
	})
	assertPending(t, session, ActionAwaitUser, 5)
	if session.State != StateWaitingUser || session.Tests == nil || len(session.Tests.References) == 0 {
		t.Fatalf("waiting session = %#v", session)
	}
	if !hasUnknown(session.SourceReport.Unknowns, sourceexplain.UnknownTestCoverage) {
		t.Fatalf("test references incorrectly resolved test support: %#v", session.SourceReport.Unknowns)
	}

	session = reduceFixture(t, session, Event{
		Kind:     EventFinished,
		ActionID: session.Next[0].ID,
		Message:  "enough for now",
	})
	if session.State != StateCompleted || session.Stop == nil || session.Stop.Kind != StopFinished || len(session.Next) != 0 {
		t.Fatalf("completed session = %#v", session)
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var replayed Session
	if err := json.Unmarshal(data, &replayed); err != nil {
		t.Fatal(err)
	}
	if err := replayed.Validate(); err != nil {
		t.Fatalf("replayed Validate() error = %v", err)
	}
}

func TestReduceRejectsStaleAndOutOfOrderEventsWithoutMutation(t *testing.T) {
	t.Parallel()

	session := startFixture(t)
	original := cloneFixture(t, session)
	structural := structuralFixture(t)
	tests := []Event{
		{Kind: EventSymbolResolved, ActionID: "step-999-resolve_symbol", Symbol: &structural},
		{Kind: EventSourceRead, ActionID: session.Next[0].ID, Source: pointer(sourceCardFixture(t))},
		{Kind: EventFinished, ActionID: session.Next[0].ID},
	}
	for _, event := range tests {
		next, actions, err := Reduce(session, event)
		if err == nil {
			t.Fatalf("Reduce(%s) error = nil", event.Kind)
		}
		if len(actions) != 0 || !reflect.DeepEqual(next, original) || !reflect.DeepEqual(session, original) {
			t.Fatalf("invalid event %s mutated session", event.Kind)
		}
	}
}

func TestReduceRepositoryChangeAndRedirectInvalidateDerivedResults(t *testing.T) {
	t.Parallel()

	session := startFixture(t)
	structural := structuralFixture(t)
	session = reduceFixture(t, session, Event{Kind: EventSymbolResolved, ActionID: session.Next[0].ID, Symbol: &structural})
	oldActionID := session.Next[0].ID
	session = reduceFixture(t, session, Event{Kind: EventRepositoryChanged, Revision: "revision-2"})
	assertPending(t, session, ActionResolveSymbol, 3)
	if session.Symbol != nil || session.Focus.EvidenceID != "" || session.Repository.Revision != "revision-2" {
		t.Fatalf("repository reset = %#v", session)
	}
	if _, _, err := Reduce(session, Event{Kind: EventSourceRead, ActionID: oldActionID, Source: pointer(sourceCardFixture(t))}); err == nil {
		t.Fatal("late result from old revision was accepted")
	}
	if _, _, err := Reduce(session, Event{Kind: EventCanceled, ActionID: oldActionID, Message: "old command canceled"}); err == nil {
		t.Fatal("late cancellation from old revision was accepted")
	}

	session = reduceFixture(t, session, Event{
		Kind: EventRedirected,
		Redirect: &RedirectInput{
			Goal:  Goal{Text: "understand delete"},
			Focus: Focus{Kind: FocusSymbol, Symbol: "kvServer.DeleteRange"},
		},
	})
	assertPending(t, session, ActionResolveSymbol, 4)
	if session.Focus.Symbol != "kvServer.DeleteRange" || session.Next[0].ResolveSymbol.Query != "kvServer.DeleteRange" {
		t.Fatalf("redirected session = %#v", session)
	}
}

func TestReduceFactContextChangeInvalidatesFactsButPreservesGoal(t *testing.T) {
	t.Parallel()

	session := advanceToAssessing(t)
	originalGoal := session.Goal
	originalRevision := session.Repository.Revision
	oldActionID := session.Next[0].ID

	session = reduceFixture(t, session, Event{
		Kind:    EventFactContextChanged,
		Message: "gopls version changed",
	})

	assertPending(t, session, ActionResolveSymbol, 4)
	if session.Goal != originalGoal || session.Repository.Revision != originalRevision {
		t.Fatalf("fact-context reset changed goal or repository: %#v", session)
	}
	if session.Symbol != nil || session.Source != nil || session.Assessment != nil || session.SourceReport != nil || session.Tests != nil ||
		session.Focus.EvidenceID != "" {
		t.Fatalf("fact-context reset retained derived evidence: %#v", session)
	}
	if !strings.Contains(session.Next[0].Reason, "gopls version changed") {
		t.Fatalf("reset reason = %q", session.Next[0].Reason)
	}
	if _, _, err := Reduce(session, Event{Kind: EventActionFailed, ActionID: oldActionID, Message: "late result"}); err == nil {
		t.Fatal("late action from stale fact context was accepted")
	}
}

func TestReduceClaimContextChangeKeepsFactsAndReassessesSource(t *testing.T) {
	t.Parallel()

	session := advanceToFindingTests(t)
	originalSymbol := cloneFixture(t, *session.Symbol)
	originalSource := cloneFixture(t, *session.Source)
	originalAssessment := cloneFixture(t, *session.Assessment)

	session = reduceFixture(t, session, Event{
		Kind:    EventClaimContextChanged,
		Message: "source prompt version changed",
	})

	assertPending(t, session, ActionAssessSource, 5)
	if !reflect.DeepEqual(*session.Symbol, originalSymbol) ||
		!reflect.DeepEqual(*session.Source, originalSource) ||
		!reflect.DeepEqual(*session.Assessment, originalAssessment) {
		t.Fatalf("claim-context reset discarded deterministic facts: %#v", session)
	}
	if session.SourceReport != nil || session.Tests != nil {
		t.Fatalf("claim-context reset retained derived claims: %#v", session)
	}
	if !strings.Contains(session.Next[0].Reason, "source prompt version changed") {
		t.Fatalf("reassessment reason = %q", session.Next[0].Reason)
	}
}

func TestReduceContextChangesRequireReasonAndApplicableEvidence(t *testing.T) {
	t.Parallel()

	session := startFixture(t)
	for _, event := range []Event{
		{Kind: EventFactContextChanged},
		{Kind: EventClaimContextChanged},
		{Kind: EventClaimContextChanged, Message: "prompt changed"},
	} {
		if _, _, err := Reduce(session, event); err == nil {
			t.Fatalf("Reduce(%s) error = nil", event.Kind)
		}
	}
}

func TestReduceLifecycleStopsAreExplicit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		event     func(Session) Event
		wantState State
		wantStop  StopKind
	}{
		{name: "cancel", event: func(Session) Event { return Event{Kind: EventCanceled, Message: "user canceled"} }, wantState: StateCanceled, wantStop: StopCanceled},
		{name: "budget", event: func(Session) Event { return Event{Kind: EventBudgetExhausted, Message: "step limit"} }, wantState: StateBlocked, wantStop: StopBudgetExhausted},
		{name: "failure", event: func(session Session) Event {
			return Event{Kind: EventActionFailed, ActionID: session.Next[0].ID, Message: "gopls failed"}
		}, wantState: StateBlocked, wantStop: StopActionFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := startFixture(t)
			session = reduceFixture(t, session, test.event(session))
			if session.State != test.wantState || session.Stop == nil || session.Stop.Kind != test.wantStop || len(session.Next) != 0 {
				t.Fatalf("session = %#v", session)
			}
		})
	}
}

func TestSessionValidateRejectsForgedEmptyCompletion(t *testing.T) {
	t.Parallel()

	session := startFixture(t)
	session.State = StateCompleted
	session.Next = nil
	session.Stop = &Stop{Kind: StopFinished}
	if err := session.Validate(); err == nil {
		t.Fatal("Validate() accepted completion without assessed source")
	}
}

func TestSessionValidateRequiresTestsForFindTestsOutcome(t *testing.T) {
	t.Parallel()

	session := advanceToFindingTests(t)
	tests, err := testevidence.Collect(context.Background(), fixtureReferenceFinder{}, "/repo", *session.Symbol, *session.Assessment, *session.SourceReport, testevidence.Options{})
	if err != nil {
		t.Fatal(err)
	}
	session = reduceFixture(t, session, Event{Kind: EventTestReferencesFound, ActionID: session.Next[0].ID, Tests: &tests})
	session.Tests = nil
	if err := session.Validate(); err == nil {
		t.Fatal("Validate() accepted waiting find-tests outcome without tests")
	}
}

func TestReduceMakesReadCalleeDecisionVisible(t *testing.T) {
	t.Parallel()

	session := advanceToAssessing(t)
	parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.Report
	allowed := session.Assessment.AllowedActions[1]
	report.NextAction = sourceexplain.Action{
		ID:               allowed.ID,
		Operation:        allowed.Operation,
		AnchorEvidenceID: allowed.AnchorEvidenceID,
		Origin:           sourceexplain.ActionOriginModel,
	}
	session = reduceFixture(t, session, Event{Kind: EventSourceAssessed, ActionID: session.Next[0].ID, SourceReport: &report})
	assertPending(t, session, ActionAwaitUser, 4)
	if session.Next[0].AwaitUser.AnchorEvidenceID != allowed.AnchorEvidenceID || !containsChoice(session.Next[0].AwaitUser.Choices, ChoiceReadCallee) {
		t.Fatalf("await action = %#v", session.Next[0])
	}
}

func TestReduceRejectsForgedSourceAndTestResults(t *testing.T) {
	t.Parallel()

	t.Run("source report", func(t *testing.T) {
		session := advanceToAssessing(t)
		parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
		if err != nil {
			t.Fatal(err)
		}
		report := parsed.Report
		report.Claims[0].SourceEvidenceIDs = []string{"source-999"}
		if _, _, err := Reduce(session, Event{Kind: EventSourceAssessed, ActionID: session.Next[0].ID, SourceReport: &report}); err == nil {
			t.Fatal("forged source report was accepted")
		}
	})

	t.Run("test search", func(t *testing.T) {
		session := advanceToFindingTests(t)
		tests, err := testevidence.Collect(context.Background(), fixtureReferenceFinder{}, "/repo", *session.Symbol, *session.Assessment, *session.SourceReport, testevidence.Options{})
		if err != nil {
			t.Fatal(err)
		}
		forged := cloneFixture(t, tests)
		forged.Searches[1].SourceEvidenceIDs = []string{"source-999"}
		if _, _, err := Reduce(session, Event{Kind: EventTestReferencesFound, ActionID: session.Next[0].ID, Tests: &forged}); err == nil {
			t.Fatal("forged test search was accepted")
		}
		forged = cloneFixture(t, tests)
		forged.Searches[1].SymbolName = "other.Symbol"
		if _, _, err := Reduce(session, Event{Kind: EventTestReferencesFound, ActionID: session.Next[0].ID, Tests: &forged}); err == nil {
			t.Fatal("test search for the wrong symbol was accepted")
		}
		forged = cloneFixture(t, tests)
		forged.Searches = forged.Searches[1:]
		filtered := forged.References[:0]
		for _, reference := range forged.References {
			if reference.SearchAnchorID != session.Focus.EvidenceID {
				filtered = append(filtered, reference)
			}
		}
		forged.References = filtered
		for index := range forged.References {
			forged.References[index].EvidenceID = "test-ref-" + fmt.Sprintf("%03d", index+1)
		}
		if _, _, err := Reduce(session, Event{Kind: EventTestReferencesFound, ActionID: session.Next[0].ID, Tests: &forged}); err == nil {
			t.Fatal("test evidence without focused-symbol search was accepted")
		}

		validSession := reduceFixture(t, session, Event{Kind: EventTestReferencesFound, ActionID: session.Next[0].ID, Tests: &tests})
		validSession.Tests.Searches[1].SymbolName = "other.Symbol"
		if err := validSession.Validate(); err == nil {
			t.Fatal("persisted session accepted forged test search")
		}
	})
}

func startFixture(t *testing.T) Session {
	t.Helper()
	session, actions, err := Reduce(Session{}, Event{
		Kind: EventStarted,
		Start: &StartInput{
			Goal:       Goal{Text: "understand kvServer.Put"},
			Repository: Repository{Path: "/repo", Revision: "revision-1"},
			Focus:      Focus{Kind: FocusSymbol, Symbol: "kvServer.Put"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || !reflect.DeepEqual(actions[0], session.Next[0]) {
		t.Fatalf("actions = %#v session.next = %#v", actions, session.Next)
	}
	return session
}

func advanceToAssessing(t *testing.T) Session {
	t.Helper()
	session := startFixture(t)
	structural := structuralFixture(t)
	session = reduceFixture(t, session, Event{Kind: EventSymbolResolved, ActionID: session.Next[0].ID, Symbol: &structural})
	card := sourceCardFixture(t)
	return reduceFixture(t, session, Event{Kind: EventSourceRead, ActionID: session.Next[0].ID, Source: &card})
}

func advanceToFindingTests(t *testing.T) Session {
	t.Helper()
	session := advanceToAssessing(t)
	parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.Report
	return reduceFixture(t, session, Event{Kind: EventSourceAssessed, ActionID: session.Next[0].ID, SourceReport: &report})
}

func reduceFixture(t *testing.T, session Session, event Event) Session {
	t.Helper()
	next, actions, err := Reduce(session, event)
	if err != nil {
		t.Fatalf("Reduce(%s) error = %v", event.Kind, err)
	}
	if !reflect.DeepEqual(actions, next.Next) {
		t.Fatalf("actions = %#v next = %#v", actions, next.Next)
	}
	return next
}

func assertPending(t *testing.T, session Session, kind ActionKind, sequence uint64) {
	t.Helper()
	if len(session.Next) != 1 || session.Next[0].Kind != kind || session.Sequence != sequence {
		t.Fatalf("state=%s sequence=%d next=%#v", session.State, session.Sequence, session.Next)
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func structuralFixture(t *testing.T) symbol.Bundle {
	t.Helper()
	card := sourceCardFixture(t)
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

func sourceCardFixture(t *testing.T) sourcecard.Card {
	t.Helper()
	var card sourcecard.Card
	if err := json.Unmarshal(deepseektest.SourceCardJSON, &card); err != nil {
		t.Fatal(err)
	}
	return card
}

type fixtureReferenceFinder struct{}

func (fixtureReferenceFinder) References(_ context.Context, _ string, location evidence.Location) (evidence.LocationSet, error) {
	return evidence.LocationSet{
		Locations: []evidence.Location{{Path: "server/etcdserver/api/v3rpc/key_test.go", Line: location.Line + 100, Column: 2}},
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider:  "gopls",
			Version:   "fixture",
			Operation: "references",
		}},
		Scenarios: []evidence.Scenario{{ID: "gopls-active-build", Name: "fixture active build"}},
	}, nil
}

func hasUnknown(values []sourceexplain.Unknown, kind sourceexplain.UnknownKind) bool {
	for _, value := range values {
		if value.Kind == kind {
			return true
		}
	}
	return false
}

func containsChoice(values []UserChoice, want UserChoice) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func cloneFixture[T any](t *testing.T, value T) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned T
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func pointer[T any](value T) *T {
	return &value
}
