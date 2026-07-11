package investigation

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

type EventKind string

const (
	EventStarted             EventKind = "started"
	EventSymbolResolved      EventKind = "symbol_resolved"
	EventSourceRead          EventKind = "source_read"
	EventSourceAssessed      EventKind = "source_assessed"
	EventTestReferencesFound EventKind = "test_references_found"
	EventRepositoryChanged   EventKind = "repository_changed"
	EventRedirected          EventKind = "redirected"
	EventCanceled            EventKind = "canceled"
	EventBudgetExhausted     EventKind = "budget_exhausted"
	EventActionFailed        EventKind = "action_failed"
	EventFinished            EventKind = "finished"
)

type Event struct {
	Kind         EventKind             `json:"kind"`
	ActionID     string                `json:"action_id,omitempty"`
	Start        *StartInput           `json:"start,omitempty"`
	Symbol       *symbol.Bundle        `json:"symbol,omitempty"`
	Source       *sourcecard.Card      `json:"source,omitempty"`
	SourceReport *sourceexplain.Report `json:"source_report,omitempty"`
	Tests        *testevidence.Bundle  `json:"tests,omitempty"`
	Revision     string                `json:"revision,omitempty"`
	Redirect     *RedirectInput        `json:"redirect,omitempty"`
	Message      string                `json:"message,omitempty"`
}

type StartInput struct {
	Goal       Goal       `json:"goal"`
	Repository Repository `json:"repository"`
	Focus      Focus      `json:"focus"`
	Origin     *Origin    `json:"origin,omitempty"`
}

type RedirectInput struct {
	Goal     Goal   `json:"goal"`
	Focus    Focus  `json:"focus"`
	Revision string `json:"revision,omitempty"`
}

func (e Event) validateShape() error {
	payloads := 0
	for _, present := range []bool{e.Start != nil, e.Symbol != nil, e.Source != nil, e.SourceReport != nil, e.Tests != nil, e.Redirect != nil} {
		if present {
			payloads++
		}
	}
	switch e.Kind {
	case EventStarted:
		if e.Start == nil || payloads != 1 || e.ActionID != "" {
			return fmt.Errorf("investigation: started event has invalid payload")
		}
	case EventSymbolResolved:
		if e.Symbol == nil || payloads != 1 || e.ActionID == "" {
			return fmt.Errorf("investigation: symbol-resolved event has invalid payload")
		}
	case EventSourceRead:
		if e.Source == nil || payloads != 1 || e.ActionID == "" {
			return fmt.Errorf("investigation: source-read event has invalid payload")
		}
	case EventSourceAssessed:
		if e.SourceReport == nil || payloads != 1 || e.ActionID == "" {
			return fmt.Errorf("investigation: source-assessed event has invalid payload")
		}
	case EventTestReferencesFound:
		if e.Tests == nil || payloads != 1 || e.ActionID == "" {
			return fmt.Errorf("investigation: test-references event has invalid payload")
		}
	case EventRepositoryChanged:
		if payloads != 0 || e.ActionID != "" || strings.TrimSpace(e.Revision) == "" {
			return fmt.Errorf("investigation: repository-changed event has invalid payload")
		}
	case EventRedirected:
		if e.Redirect == nil || payloads != 1 || e.ActionID != "" {
			return fmt.Errorf("investigation: redirected event has invalid payload")
		}
	case EventCanceled:
		if payloads != 0 {
			return fmt.Errorf("investigation: %s event has invalid payload", e.Kind)
		}
	case EventBudgetExhausted:
		if payloads != 0 || e.ActionID != "" {
			return fmt.Errorf("investigation: %s event has invalid payload", e.Kind)
		}
	case EventActionFailed:
		if payloads != 0 || e.ActionID == "" || strings.TrimSpace(e.Message) == "" {
			return fmt.Errorf("investigation: action-failed event has invalid payload")
		}
	case EventFinished:
		if payloads != 0 || e.ActionID == "" {
			return fmt.Errorf("investigation: finished event has invalid payload")
		}
	default:
		return fmt.Errorf("investigation: invalid event kind %q", e.Kind)
	}
	return nil
}
