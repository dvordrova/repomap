package investigation

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

// Runner is deliberately concrete and exhaustive. Adding a capability requires
// adding one ActionKind branch rather than registering an opaque plugin.
type Runner struct {
	Analyzer        analysis.Provider
	SourceAssessor  sourceexplain.Assessor
	ReferenceFinder testevidence.ReferenceFinder
	SymbolOptions   symbol.Options
	SourceLimits    sourcecard.Limits
	TestOptions     testevidence.Options
}

// Execution keeps diagnostic artifacts out of Session while delivering one
// typed Event back to Reduce.
type Execution struct {
	Event             Event                      `json:"event"`
	Graph             *evidence.Graph            `json:"graph,omitempty"`
	SourceExplanation *sourceexplain.Explanation `json:"source_explanation,omitempty"`
	DiagnosticError   error                      `json:"-"`
}

func (r Runner) Execute(ctx context.Context, session Session, action Action) (Execution, error) {
	if err := session.Validate(); err != nil {
		return Execution{}, fmt.Errorf("investigation runner: invalid session: %w", err)
	}
	if err := action.Validate(); err != nil {
		return Execution{}, fmt.Errorf("investigation runner: invalid action: %w", err)
	}
	if len(session.Next) != 1 || !reflect.DeepEqual(session.Next[0], action) {
		return Execution{}, fmt.Errorf("investigation runner: action is not the pending session action")
	}
	if err := ctx.Err(); err != nil {
		return canceledExecution(action.ID, err), nil
	}

	switch action.Kind {
	case ActionResolveSymbol:
		if r.Analyzer == nil {
			return failedExecution(action, fmt.Errorf("symbol analyzer is not configured")), nil
		}
		graph, err := r.Analyzer.Analyze(ctx, analysis.Request{
			RepoPath: action.ResolveSymbol.RepoPath,
			Query:    action.ResolveSymbol.Query,
		})
		if err != nil {
			return executionError(ctx, action, err), nil
		}
		bundle, err := symbol.Build(graph, r.SymbolOptions)
		if err != nil {
			return failedExecution(action, err), nil
		}
		storedGraph, err := cloneValue(graph)
		if err != nil {
			return Execution{}, err
		}
		return Execution{
			Event: Event{Kind: EventSymbolResolved, ActionID: action.ID, Symbol: &bundle},
			Graph: &storedGraph,
		}, nil

	case ActionReadSource:
		card, err := sourcecard.Read(sourcecard.Request{
			RepoPath:         action.ReadSource.RepoPath,
			TargetEvidenceID: action.ReadSource.TargetEvidenceID,
			Target:           action.ReadSource.Target,
		}, r.SourceLimits)
		if err != nil {
			return executionError(ctx, action, err), nil
		}
		if err := sourcecard.ValidateForRemote(card); err != nil {
			return failedExecution(action, err), nil
		}
		return Execution{Event: Event{Kind: EventSourceRead, ActionID: action.ID, Source: &card}}, nil

	case ActionAssessSource:
		if r.SourceAssessor == nil {
			return failedExecution(action, fmt.Errorf("source assessor is not configured")), nil
		}
		explanation, err := sourceexplain.NewService(r.SourceAssessor).Explain(ctx, *action.AssessSource)
		execution := Execution{SourceExplanation: &explanation}
		if err != nil {
			failed := executionError(ctx, action, err)
			execution.Event = failed.Event
			execution.DiagnosticError = failed.DiagnosticError
			return execution, nil
		}
		report := explanation.Parsed.Report
		execution.Event = Event{Kind: EventSourceAssessed, ActionID: action.ID, SourceReport: &report}
		return execution, nil

	case ActionFindTests:
		if r.ReferenceFinder == nil {
			return failedExecution(action, fmt.Errorf("reference finder is not configured")), nil
		}
		bundle, err := testevidence.Collect(
			ctx,
			r.ReferenceFinder,
			action.FindTests.RepoPath,
			action.FindTests.Structural,
			action.FindTests.Assessment,
			action.FindTests.Report,
			r.TestOptions,
		)
		if err != nil {
			return executionError(ctx, action, err), nil
		}
		return Execution{Event: Event{Kind: EventTestReferencesFound, ActionID: action.ID, Tests: &bundle}}, nil

	case ActionAwaitUser:
		return Execution{}, fmt.Errorf("investigation runner: await_user belongs to presentation, not capability execution")
	default:
		return Execution{}, fmt.Errorf("investigation runner: unsupported action %q", action.Kind)
	}
}

func executionError(ctx context.Context, action Action, err error) Execution {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return canceledExecution(action.ID, ctx.Err())
	}
	return failedExecution(action, err)
}

func canceledExecution(actionID string, err error) Execution {
	return Execution{
		Event:           Event{Kind: EventCanceled, ActionID: actionID, Message: "capability action canceled"},
		DiagnosticError: err,
	}
}

func failedExecution(action Action, err error) Execution {
	return Execution{
		Event:           Event{Kind: EventActionFailed, ActionID: action.ID, Message: string(action.Kind) + " failed"},
		DiagnosticError: err,
	}
}
