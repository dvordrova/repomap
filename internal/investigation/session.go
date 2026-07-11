// Package investigation coordinates validated capability results through a
// replayable state machine. Reduce is pure; Runner is the explicit application
// boundary that executes requested capabilities.
package investigation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const SessionVersion = 1

type State string

const (
	StateResolvingSymbol State = "resolving_symbol"
	StateReadingSource   State = "reading_source"
	StateAssessingSource State = "assessing_source"
	StateFindingTests    State = "finding_tests"
	StateWaitingUser     State = "waiting_user"
	StateCompleted       State = "completed"
	StateCanceled        State = "canceled"
	StateBlocked         State = "blocked"
)

type FocusKind string

const FocusSymbol FocusKind = "symbol"

type StopKind string

const (
	StopFinished        StopKind = "finished"
	StopCanceled        StopKind = "canceled"
	StopBudgetExhausted StopKind = "budget_exhausted"
	StopActionFailed    StopKind = "action_failed"
)

type Goal struct {
	Text string `json:"text"`
}

type Repository struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}

type Focus struct {
	Kind       FocusKind `json:"kind"`
	Symbol     string    `json:"symbol"`
	EvidenceID string    `json:"evidence_id,omitempty"`
}

type Stop struct {
	Kind    StopKind `json:"kind"`
	Message string   `json:"message,omitempty"`
}

// Session stores the exact validated cube outputs for the first Go vertical
// slice. A generic fact/claim ledger is deferred until a second playbook forces
// a real shared representation.
type Session struct {
	Version      int                   `json:"version"`
	Goal         Goal                  `json:"goal"`
	Repository   Repository            `json:"repository"`
	Focus        Focus                 `json:"focus"`
	State        State                 `json:"state"`
	Sequence     uint64                `json:"sequence"`
	Symbol       *symbol.Bundle        `json:"symbol,omitempty"`
	Source       *sourcecard.Card      `json:"source,omitempty"`
	Assessment   *sourceexplain.Bundle `json:"assessment,omitempty"`
	SourceReport *sourceexplain.Report `json:"source_report,omitempty"`
	Tests        *testevidence.Bundle  `json:"tests,omitempty"`
	Next         []Action              `json:"next"`
	Stop         *Stop                 `json:"stop,omitempty"`
}

func (s Session) Validate() error {
	if s.Version != SessionVersion {
		return fmt.Errorf("investigation: unsupported session version %d", s.Version)
	}
	if strings.TrimSpace(s.Goal.Text) == "" || strings.TrimSpace(s.Repository.Path) == "" || strings.TrimSpace(s.Repository.Revision) == "" {
		return fmt.Errorf("investigation: goal, repository path, and revision are required")
	}
	if s.Focus.Kind != FocusSymbol || strings.TrimSpace(s.Focus.Symbol) == "" {
		return fmt.Errorf("investigation: one symbol focus is required")
	}
	if s.Sequence == 0 {
		return fmt.Errorf("investigation: action sequence has not started")
	}
	if err := validateArtifactPrefix(s); err != nil {
		return err
	}
	if err := validateStateShape(s); err != nil {
		return err
	}
	return nil
}

func validateArtifactPrefix(s Session) error {
	if s.Symbol != nil {
		if err := s.Symbol.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid symbol result: %w", err)
		}
		if s.Symbol.Query != s.Focus.Symbol || s.Symbol.Target.Entity.Name != s.Focus.Symbol {
			return fmt.Errorf("investigation: symbol result does not match focus")
		}
		if s.Focus.EvidenceID != s.Symbol.Target.EvidenceID {
			return fmt.Errorf("investigation: focus evidence does not match symbol result")
		}
	} else if s.Focus.EvidenceID != "" {
		return fmt.Errorf("investigation: unresolved focus has evidence id")
	}
	if s.Source != nil {
		if s.Symbol == nil {
			return fmt.Errorf("investigation: source exists without symbol evidence")
		}
		if err := s.Source.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid source result: %w", err)
		}
	}
	if s.Assessment != nil {
		if s.Source == nil {
			return fmt.Errorf("investigation: assessment exists without source")
		}
		if err := s.Assessment.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid source assessment: %w", err)
		}
		rebuilt, err := sourceexplain.Build(*s.Symbol, *s.Source)
		if err != nil || !reflect.DeepEqual(rebuilt, *s.Assessment) {
			return fmt.Errorf("investigation: assessment was not built from stored symbol and source")
		}
	} else if s.Source != nil {
		return fmt.Errorf("investigation: source exists without its deterministic assessment bundle")
	}
	if s.SourceReport != nil {
		if s.Assessment == nil {
			return fmt.Errorf("investigation: source report exists without assessment")
		}
		if err := sourceexplain.ValidateReport(*s.Assessment, *s.SourceReport); err != nil {
			return fmt.Errorf("investigation: invalid source report: %w", err)
		}
	}
	if s.Tests != nil {
		if s.SourceReport == nil {
			return fmt.Errorf("investigation: test references exist without source report")
		}
		if err := s.Tests.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid test references: %w", err)
		}
		if s.Tests.TargetName != s.Symbol.Target.Entity.Name {
			return fmt.Errorf("investigation: test references do not match focus")
		}
		if err := validateTestReferences(s, *s.Tests); err != nil {
			return err
		}
	}
	return nil
}

func validateStateShape(s Session) error {
	terminal := s.State == StateCompleted || s.State == StateCanceled || s.State == StateBlocked
	if terminal {
		if len(s.Next) != 0 || s.Stop == nil {
			return fmt.Errorf("investigation: terminal state requires one stop and no next action")
		}
		switch s.State {
		case StateCompleted:
			if s.Stop.Kind != StopFinished {
				return fmt.Errorf("investigation: completed state has invalid stop %q", s.Stop.Kind)
			}
			if s.SourceReport == nil {
				return fmt.Errorf("investigation: completed state has no assessed source result")
			}
			if err := validateOutcomeArtifacts(s); err != nil {
				return err
			}
		case StateCanceled:
			if s.Stop.Kind != StopCanceled {
				return fmt.Errorf("investigation: canceled state has invalid stop %q", s.Stop.Kind)
			}
		case StateBlocked:
			if s.Stop.Kind != StopBudgetExhausted && s.Stop.Kind != StopActionFailed {
				return fmt.Errorf("investigation: blocked state has invalid stop %q", s.Stop.Kind)
			}
		}
		return nil
	}
	if s.Stop != nil || len(s.Next) != 1 {
		return fmt.Errorf("investigation: active state requires exactly one next action and no stop")
	}
	if err := s.Next[0].Validate(); err != nil {
		return err
	}
	wantActionID := fmt.Sprintf("step-%03d-%s", s.Sequence, s.Next[0].Kind)
	if s.Next[0].ID != wantActionID {
		return fmt.Errorf("investigation: pending action id %q does not match sequence", s.Next[0].ID)
	}
	switch s.State {
	case StateResolvingSymbol:
		if s.Symbol != nil || s.Next[0].Kind != ActionResolveSymbol {
			return fmt.Errorf("investigation: resolving state has invalid artifacts or action")
		}
	case StateReadingSource:
		if s.Symbol == nil || s.Source != nil || s.Next[0].Kind != ActionReadSource {
			return fmt.Errorf("investigation: reading state has invalid artifacts or action")
		}
	case StateAssessingSource:
		if s.Assessment == nil || s.SourceReport != nil || s.Next[0].Kind != ActionAssessSource {
			return fmt.Errorf("investigation: assessing state has invalid artifacts or action")
		}
	case StateFindingTests:
		if s.SourceReport == nil || s.Tests != nil || s.Next[0].Kind != ActionFindTests {
			return fmt.Errorf("investigation: find-tests state has invalid artifacts or action")
		}
	case StateWaitingUser:
		if s.SourceReport == nil || s.Next[0].Kind != ActionAwaitUser {
			return fmt.Errorf("investigation: waiting state has invalid artifacts or action")
		}
	default:
		return fmt.Errorf("investigation: invalid state %q", s.State)
	}
	return validatePendingPayload(s)
}

func validatePendingPayload(s Session) error {
	action := s.Next[0]
	switch action.Kind {
	case ActionResolveSymbol:
		if action.ResolveSymbol.RepoPath != s.Repository.Path || action.ResolveSymbol.Query != s.Focus.Symbol {
			return fmt.Errorf("investigation: resolve action does not match session focus")
		}
	case ActionReadSource:
		if action.ReadSource.RepoPath != s.Repository.Path ||
			action.ReadSource.TargetEvidenceID != s.Symbol.Target.EvidenceID ||
			!reflect.DeepEqual(action.ReadSource.Target, s.Symbol.Target.Entity) {
			return fmt.Errorf("investigation: read-source action does not match symbol evidence")
		}
	case ActionAssessSource:
		if !reflect.DeepEqual(*action.AssessSource, *s.Assessment) {
			return fmt.Errorf("investigation: assess-source action does not match stored bundle")
		}
	case ActionFindTests:
		if action.FindTests.RepoPath != s.Repository.Path ||
			!reflect.DeepEqual(action.FindTests.Structural, *s.Symbol) ||
			!reflect.DeepEqual(action.FindTests.Assessment, *s.Assessment) ||
			!reflect.DeepEqual(action.FindTests.Report, *s.SourceReport) {
			return fmt.Errorf("investigation: find-tests action does not match stored evidence")
		}
	case ActionAwaitUser:
		return validateAwaitAction(s, action)
	}
	return nil
}

func validateOutcomeArtifacts(s Session) error {
	switch s.SourceReport.NextAction.Operation {
	case sourceexplain.OperationFindTests:
		if s.Tests == nil {
			return fmt.Errorf("investigation: find-tests outcome has no test-reference result")
		}
	case sourceexplain.OperationReadCallee:
		if s.Tests != nil {
			return fmt.Errorf("investigation: read-callee outcome contains unrelated test references")
		}
	default:
		return fmt.Errorf("investigation: outcome has unsupported source action %q", s.SourceReport.NextAction.Operation)
	}
	return nil
}

func validateAwaitAction(s Session, action Action) error {
	if err := validateOutcomeArtifacts(s); err != nil {
		return err
	}
	switch s.SourceReport.NextAction.Operation {
	case sourceexplain.OperationFindTests:
		if action.AwaitUser.Question != inspectTestsQuestion ||
			!reflect.DeepEqual(action.AwaitUser.Choices, []UserChoice{ChoiceInspectTests, ChoiceFinish}) ||
			action.AwaitUser.AnchorEvidenceID != s.Focus.EvidenceID {
			return fmt.Errorf("investigation: await-user action does not match find-tests outcome")
		}
	case sourceexplain.OperationReadCallee:
		if action.AwaitUser.Question != readCalleeQuestion ||
			!reflect.DeepEqual(action.AwaitUser.Choices, []UserChoice{ChoiceReadCallee, ChoiceFinish}) ||
			action.AwaitUser.AnchorEvidenceID != s.SourceReport.NextAction.AnchorEvidenceID {
			return fmt.Errorf("investigation: await-user action does not match read-callee outcome")
		}
	}
	return nil
}
