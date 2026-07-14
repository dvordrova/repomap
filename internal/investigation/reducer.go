package investigation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

// Reduce applies one delivered fact or lifecycle event and returns requested
// side effects as data. It performs no I/O and never executes an Action.
func Reduce(session Session, event Event) (Session, []Action, error) {
	original := session
	if err := event.validateShape(); err != nil {
		return original, nil, err
	}
	if event.Kind == EventStarted {
		if session.Version != 0 || session.State != "" {
			return original, nil, fmt.Errorf("investigation: session already started")
		}
		next, err := startSession(*event.Start)
		if err != nil {
			return original, nil, err
		}
		return finishReduction(original, next)
	}
	if err := session.Validate(); err != nil {
		return original, nil, fmt.Errorf("investigation: invalid current session: %w", err)
	}

	switch event.Kind {
	case EventRepositoryChanged:
		if event.Revision == session.Repository.Revision {
			return original, nil, fmt.Errorf("investigation: repository revision did not change")
		}
		next := resetDerived(session)
		next.Origin = nil
		next.Repository.Revision = event.Revision
		issueResolve(&next, "repository revision changed; resolve the focus again before reusing evidence")
		return finishReduction(original, next)
	case EventFactContextChanged:
		next := resetDerived(session)
		issueResolve(&next, fmt.Sprintf("saved facts are stale (%s); resolve the focus again before reusing evidence", event.Message))
		return finishReduction(original, next)
	case EventClaimContextChanged:
		if session.Assessment == nil || session.SourceReport == nil {
			return original, nil, fmt.Errorf("investigation: no model claims exist to invalidate")
		}
		next := session
		next.SourceReport = nil
		next.Tests = nil
		next.Next = nil
		next.Stop = nil
		issue(&next, Action{
			Kind:         ActionAssessSource,
			Reason:       fmt.Sprintf("saved model claims are stale (%s); reassess the bounded source evidence", event.Message),
			AssessSource: next.Assessment,
		}, StateAssessingSource)
		return finishReduction(original, next)
	case EventRedirected:
		if event.Redirect.Revision != "" && event.Redirect.Revision != session.Repository.Revision {
			return original, nil, fmt.Errorf("investigation: repository revision changes require repository_changed event")
		}
		next := resetDerived(session)
		next.Goal = event.Redirect.Goal
		next.Focus = event.Redirect.Focus
		next.Focus.EvidenceID = ""
		if err := validateStartFields(next.Goal, next.Repository, next.Focus); err != nil {
			return original, nil, err
		}
		issueResolve(&next, "the user redirected the investigation; resolve the new focus")
		return finishReduction(original, next)
	case EventCanceled:
		if terminal(session.State) {
			return original, nil, fmt.Errorf("investigation: terminal session cannot be canceled again")
		}
		if event.ActionID != "" {
			if err := requirePending(session, event.ActionID); err != nil {
				return original, nil, err
			}
		}
		next := session
		next.State = StateCanceled
		next.Next = nil
		next.Stop = &Stop{Kind: StopCanceled, Message: event.Message}
		return finishReduction(original, next)
	case EventBudgetExhausted:
		if terminal(session.State) {
			return original, nil, fmt.Errorf("investigation: terminal session cannot exhaust budget")
		}
		next := session
		next.State = StateBlocked
		next.Next = nil
		next.Stop = &Stop{Kind: StopBudgetExhausted, Message: event.Message}
		return finishReduction(original, next)
	case EventActionFailed:
		if err := requirePending(session, event.ActionID); err != nil {
			return original, nil, err
		}
		next := session
		next.State = StateBlocked
		next.Next = nil
		next.Stop = &Stop{Kind: StopActionFailed, Message: event.Message}
		return finishReduction(original, next)
	}

	if err := requirePending(session, event.ActionID); err != nil {
		return original, nil, err
	}
	var (
		next Session
		err  error
	)
	switch event.Kind {
	case EventSymbolResolved:
		next, err = acceptSymbol(session, *event.Symbol)
	case EventSourceRead:
		next, err = acceptSource(session, *event.Source)
	case EventSourceAssessed:
		next, err = acceptSourceReport(session, *event.SourceReport)
	case EventTestReferencesFound:
		if session.State == StateFindingTestReferences {
			next, err = acceptTargetTestReferences(session, *event.Tests)
		} else {
			next, err = acceptTestReferences(session, *event.Tests)
		}
	case EventFinished:
		if session.State != StateWaitingUser {
			err = fmt.Errorf("investigation: finish is only valid while waiting for the user")
			break
		}
		next = session
		next.State = StateCompleted
		next.Next = nil
		next.Stop = &Stop{Kind: StopFinished, Message: event.Message}
	default:
		err = fmt.Errorf("investigation: event %q is not valid in state %q", event.Kind, session.State)
	}
	if err != nil {
		return original, nil, err
	}
	return finishReduction(original, next)
}

func startSession(input StartInput) (Session, error) {
	if err := validateStartFields(input.Goal, input.Repository, input.Focus); err != nil {
		return Session{}, err
	}
	session := Session{
		Version:    SessionVersion,
		Goal:       input.Goal,
		Repository: input.Repository,
		Focus:      input.Focus,
		Origin:     input.Origin,
		Next:       []Action{},
	}
	issueResolve(&session, "resolve the exact symbol before collecting focused evidence")
	return session, nil
}

func validateStartFields(goal Goal, repository Repository, focus Focus) error {
	if strings.TrimSpace(goal.Text) == "" || strings.TrimSpace(repository.Path) == "" || strings.TrimSpace(repository.Revision) == "" {
		return fmt.Errorf("investigation: start requires goal, repository path, and revision")
	}
	if focus.Kind != FocusSymbol || strings.TrimSpace(focus.Symbol) == "" || focus.EvidenceID != "" {
		return fmt.Errorf("investigation: start requires one unresolved symbol focus")
	}
	if err := validateFocusEntity(focus.Symbol, focus.Entity); err != nil {
		return err
	}
	return nil
}

func acceptSymbol(session Session, bundle symbol.Bundle) (Session, error) {
	if session.State != StateResolvingSymbol {
		return session, fmt.Errorf("investigation: symbol result is invalid in state %q", session.State)
	}
	if err := bundle.Validate(); err != nil {
		return session, fmt.Errorf("investigation: invalid symbol result: %w", err)
	}
	if bundle.Query != session.Focus.Symbol || bundle.Target.Entity.Name != session.Focus.Symbol {
		return session, fmt.Errorf("investigation: symbol result does not match focus")
	}
	if session.Focus.Entity != nil && !reflect.DeepEqual(bundle.Target.Entity, *session.Focus.Entity) {
		return session, fmt.Errorf("investigation: symbol result does not match exact focus entity")
	}
	stored, err := cloneValue(bundle)
	if err != nil {
		return session, err
	}
	next := session
	next.Symbol = &stored
	next.Focus.EvidenceID = stored.Target.EvidenceID
	issue(&next, Action{
		Kind:   ActionReadSource,
		Reason: "the exact symbol is resolved but behavior is still unsupported by source evidence",
		ReadSource: &ReadSourceInput{
			RepoPath:         next.Repository.Path,
			TargetEvidenceID: stored.Target.EvidenceID,
			Target:           stored.Target.Entity,
		},
	}, StateReadingSource)
	return next, nil
}

func acceptSource(session Session, card sourcecard.Card) (Session, error) {
	if session.State != StateReadingSource {
		return session, fmt.Errorf("investigation: source result is invalid in state %q", session.State)
	}
	if err := card.Validate(); err != nil {
		return session, fmt.Errorf("investigation: invalid source result: %w", err)
	}
	storedCard, err := cloneValue(card)
	if err != nil {
		return session, err
	}
	next := session
	next.Source = &storedCard
	if isLocalComponentInvestigation(session) {
		issue(&next, Action{
			Kind:   ActionFindTestReferences,
			Reason: "collect target-only test references without inventing source questions or model claims",
			FindTestReferences: &FindTestReferencesInput{
				RepoPath:   next.Repository.Path,
				Structural: *next.Symbol,
			},
		}, StateFindingTestReferences)
		return next, nil
	}
	assessment, err := sourceexplain.Build(*session.Symbol, card)
	if err != nil {
		return session, fmt.Errorf("investigation: source does not match symbol result: %w", err)
	}
	storedAssessment, err := cloneValue(assessment)
	if err != nil {
		return session, err
	}
	next.Assessment = &storedAssessment
	issue(&next, Action{
		Kind:         ActionAssessSource,
		Reason:       "interpret the bounded source questions without promoting callee names to behavior",
		AssessSource: &storedAssessment,
	}, StateAssessingSource)
	return next, nil
}

func acceptTargetTestReferences(session Session, tests testevidence.Bundle) (Session, error) {
	if session.State != StateFindingTestReferences {
		return session, fmt.Errorf("investigation: target test references are invalid in state %q", session.State)
	}
	if err := tests.Validate(); err != nil {
		return session, fmt.Errorf("investigation: invalid target test-reference result: %w", err)
	}
	if err := validateTargetTestReferences(session, tests); err != nil {
		return session, err
	}
	stored, err := cloneValue(tests)
	if err != nil {
		return session, err
	}
	next := session
	next.Tests = &stored
	next.State = StateCompleted
	next.Next = nil
	next.Stop = &Stop{Kind: StopFinished, Message: "local source and target test references collected"}
	return next, nil
}

func validateTargetTestReferences(session Session, tests testevidence.Bundle) error {
	if !isLocalComponentInvestigation(session) || session.Symbol == nil || session.Source == nil {
		return fmt.Errorf("investigation: target-only test references require local component source evidence")
	}
	target := session.Symbol.Target
	if target.Entity.Location == nil || tests.TargetName != target.Entity.Name || len(tests.Searches) != 1 {
		return fmt.Errorf("investigation: target-only test references do not match focused symbol")
	}
	search := tests.Searches[0]
	if search.AnchorEvidenceID != target.EvidenceID || search.SymbolName != target.Entity.Name ||
		!reflect.DeepEqual(search.Location, *target.Entity.Location) || search.Predicate != "" || len(search.SourceEvidenceIDs) != 0 {
		return fmt.Errorf("investigation: target-only test search is not grounded in focused symbol")
	}
	return nil
}

func acceptSourceReport(session Session, report sourceexplain.Report) (Session, error) {
	if session.State != StateAssessingSource {
		return session, fmt.Errorf("investigation: source assessment is invalid in state %q", session.State)
	}
	if err := sourceexplain.ValidateReport(*session.Assessment, report); err != nil {
		return session, fmt.Errorf("investigation: invalid source assessment result: %w", err)
	}
	stored, err := cloneValue(report)
	if err != nil {
		return session, err
	}
	next := session
	next.SourceReport = &stored
	switch stored.NextAction.Operation {
	case sourceexplain.OperationFindTests:
		input := FindTestsInput{
			RepoPath:   next.Repository.Path,
			Structural: *next.Symbol,
			Assessment: *next.Assessment,
			Report:     stored,
		}
		issue(&next, Action{
			Kind:      ActionFindTests,
			Reason:    "test coverage remains unknown; collect related references without claiming what tests assert",
			FindTests: &input,
		}, StateFindingTests)
	case sourceexplain.OperationReadCallee:
		issue(&next, Action{
			Kind:   ActionAwaitUser,
			Reason: "the source assessment selected a callee branch that is not silently auto-expanded",
			AwaitUser: &AwaitUserInput{
				Question:         readCalleeQuestion,
				Choices:          []UserChoice{ChoiceReadCallee, ChoiceFinish},
				AnchorEvidenceID: stored.NextAction.AnchorEvidenceID,
			},
		}, StateWaitingUser)
	default:
		return session, fmt.Errorf("investigation: unsupported validated source action %q", stored.NextAction.Operation)
	}
	return next, nil
}

func acceptTestReferences(session Session, tests testevidence.Bundle) (Session, error) {
	if session.State != StateFindingTests {
		return session, fmt.Errorf("investigation: test references are invalid in state %q", session.State)
	}
	if err := tests.Validate(); err != nil {
		return session, fmt.Errorf("investigation: invalid test-reference result: %w", err)
	}
	if err := validateTestReferences(session, tests); err != nil {
		return session, err
	}
	stored, err := cloneValue(tests)
	if err != nil {
		return session, err
	}
	next := session
	next.Tests = &stored
	issue(&next, Action{
		Kind:   ActionAwaitUser,
		Reason: "related tests are navigation evidence only; choose whether to inspect bounded test source",
		AwaitUser: &AwaitUserInput{
			Question:         inspectTestsQuestion,
			Choices:          []UserChoice{ChoiceInspectTests, ChoiceFinish},
			AnchorEvidenceID: next.Focus.EvidenceID,
		},
	}, StateWaitingUser)
	return next, nil
}

func validateTestReferences(session Session, tests testevidence.Bundle) error {
	if tests.TargetName != session.Symbol.Target.Entity.Name {
		return fmt.Errorf("investigation: test references do not match focused symbol")
	}
	if session.Symbol.Target.Entity.Location == nil {
		return fmt.Errorf("investigation: focused symbol has no location")
	}
	type claimKey struct {
		anchor    string
		predicate sourceexplain.Predicate
	}
	claims := make(map[claimKey][]string, len(session.SourceReport.Claims))
	for _, claim := range session.SourceReport.Claims {
		claims[claimKey{anchor: claim.StructuralEvidenceIDs[0], predicate: claim.Predicate}] = claim.SourceEvidenceIDs
	}
	calls := make(map[string]symbol.CallFact, len(session.Symbol.OutgoingCalls))
	for _, call := range session.Symbol.OutgoingCalls {
		calls[call.EvidenceID] = call
	}
	sawTarget := false
	for _, search := range tests.Searches {
		if search.AnchorEvidenceID == session.Focus.EvidenceID {
			sawTarget = true
			if search.Predicate != "" || len(search.SourceEvidenceIDs) != 0 {
				return fmt.Errorf("investigation: target test search contains claim support")
			}
			if search.SymbolName != session.Symbol.Target.Entity.Name || !reflect.DeepEqual(search.Location, *session.Symbol.Target.Entity.Location) {
				return fmt.Errorf("investigation: target test search does not match focused symbol location")
			}
			continue
		}
		want, ok := claims[claimKey{anchor: search.AnchorEvidenceID, predicate: search.Predicate}]
		if !ok || !reflect.DeepEqual(want, search.SourceEvidenceIDs) {
			return fmt.Errorf("investigation: test search is not grounded in a stored source claim")
		}
		call, ok := calls[search.AnchorEvidenceID]
		if !ok || call.Callee.Location == nil || search.SymbolName != call.Callee.Name || !reflect.DeepEqual(search.Location, *call.Callee.Location) {
			return fmt.Errorf("investigation: claim test search does not match its structural callee")
		}
	}
	if !sawTarget {
		return fmt.Errorf("investigation: test references omit the focused-symbol search")
	}
	return nil
}

func issueResolve(session *Session, reason string) {
	issue(session, Action{
		Kind:   ActionResolveSymbol,
		Reason: reason,
		ResolveSymbol: &ResolveSymbolInput{
			RepoPath: session.Repository.Path,
			Query:    session.Focus.Symbol,
			Expected: session.Focus.Entity,
		},
	}, StateResolvingSymbol)
}

func issue(session *Session, action Action, state State) {
	session.Sequence++
	action.ID = fmt.Sprintf("step-%03d-%s", session.Sequence, action.Kind)
	session.State = state
	session.Stop = nil
	session.Next = []Action{action}
}

func requirePending(session Session, actionID string) error {
	if len(session.Next) != 1 || actionID == "" || actionID != session.Next[0].ID {
		return fmt.Errorf("investigation: event action %q does not match pending action", actionID)
	}
	return nil
}

func resetDerived(session Session) Session {
	next := session
	next.Focus.EvidenceID = ""
	next.Symbol = nil
	next.Source = nil
	next.Assessment = nil
	next.SourceReport = nil
	next.Tests = nil
	next.Next = nil
	next.Stop = nil
	return next
}

func finishReduction(original, next Session) (Session, []Action, error) {
	stored, err := cloneValue(next)
	if err != nil {
		return original, nil, err
	}
	if err := stored.Validate(); err != nil {
		return original, nil, fmt.Errorf("investigation: invalid reduced session: %w", err)
	}
	actions, err := cloneValue(stored.Next)
	if err != nil {
		return original, nil, err
	}
	return stored, actions, nil
}

func terminal(state State) bool {
	return state == StateCompleted || state == StateCanceled || state == StateBlocked
}

func cloneValue[T any](value T) (T, error) {
	var result T
	data, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("investigation: clone value: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("investigation: clone value: %w", err)
	}
	return result, nil
}
