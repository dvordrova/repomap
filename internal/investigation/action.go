package investigation

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

type ActionKind string

const (
	ActionResolveSymbol      ActionKind = "resolve_symbol"
	ActionReadSource         ActionKind = "read_source"
	ActionAssessSource       ActionKind = "assess_source"
	ActionFindTests          ActionKind = "find_tests"
	ActionFindTestReferences ActionKind = "find_test_references"
	ActionAwaitUser          ActionKind = "await_user"
)

type UserChoice string

const (
	ChoiceInspectTests UserChoice = "inspect_test_references"
	ChoiceReadCallee   UserChoice = "read_callee"
	ChoiceFinish       UserChoice = "finish"
)

const (
	readCalleeQuestion   = "Read the selected callee next, or finish this bounded investigation?"
	inspectTestsQuestion = "Inspect a related test reference, or finish this symbol overview?"
)

type Action struct {
	ID                 string                   `json:"id"`
	Kind               ActionKind               `json:"kind"`
	Reason             string                   `json:"reason"`
	ResolveSymbol      *ResolveSymbolInput      `json:"resolve_symbol,omitempty"`
	ReadSource         *ReadSourceInput         `json:"read_source,omitempty"`
	AssessSource       *sourceexplain.Bundle    `json:"assess_source,omitempty"`
	FindTests          *FindTestsInput          `json:"find_tests,omitempty"`
	FindTestReferences *FindTestReferencesInput `json:"find_test_references,omitempty"`
	AwaitUser          *AwaitUserInput          `json:"await_user,omitempty"`
}

type ResolveSymbolInput struct {
	RepoPath string           `json:"repo_path"`
	Query    string           `json:"query"`
	Expected *evidence.Entity `json:"expected,omitempty"`
}

// ReadSourceInput is the durable investigation-owned equivalent of
// sourcecard.Request. The application runner performs that small conversion.
type ReadSourceInput struct {
	RepoPath         string          `json:"repo_path"`
	TargetEvidenceID string          `json:"target_evidence_id"`
	Target           evidence.Entity `json:"target"`
}

type FindTestsInput struct {
	RepoPath   string               `json:"repo_path"`
	Structural symbol.Bundle        `json:"structural"`
	Assessment sourceexplain.Bundle `json:"assessment"`
	Report     sourceexplain.Report `json:"report"`
}

type FindTestReferencesInput struct {
	RepoPath   string        `json:"repo_path"`
	Structural symbol.Bundle `json:"structural"`
}

type AwaitUserInput struct {
	Question         string       `json:"question"`
	Choices          []UserChoice `json:"choices"`
	AnchorEvidenceID string       `json:"anchor_evidence_id,omitempty"`
}

func (a Action) Validate() error {
	if a.ID == "" || !strings.HasPrefix(a.ID, "step-") || !strings.HasSuffix(a.ID, "-"+string(a.Kind)) {
		return fmt.Errorf("investigation: action has invalid deterministic id %q", a.ID)
	}
	if strings.TrimSpace(a.Reason) == "" {
		return fmt.Errorf("investigation: action %q has no reason", a.ID)
	}
	payloads := 0
	for _, present := range []bool{
		a.ResolveSymbol != nil,
		a.ReadSource != nil,
		a.AssessSource != nil,
		a.FindTests != nil,
		a.FindTestReferences != nil,
		a.AwaitUser != nil,
	} {
		if present {
			payloads++
		}
	}
	if payloads != 1 {
		return fmt.Errorf("investigation: action %q must have exactly one payload", a.ID)
	}
	switch a.Kind {
	case ActionResolveSymbol:
		if a.ResolveSymbol == nil || strings.TrimSpace(a.ResolveSymbol.RepoPath) == "" || strings.TrimSpace(a.ResolveSymbol.Query) == "" {
			return fmt.Errorf("investigation: resolve-symbol action is incomplete")
		}
		if err := validateFocusEntity(a.ResolveSymbol.Query, a.ResolveSymbol.Expected); err != nil {
			return fmt.Errorf("investigation: resolve-symbol exact entity: %w", err)
		}
	case ActionReadSource:
		if a.ReadSource == nil || strings.TrimSpace(a.ReadSource.RepoPath) == "" || a.ReadSource.TargetEvidenceID == "" ||
			a.ReadSource.Target.Location == nil || a.ReadSource.Target.Location.Path == "" || a.ReadSource.Target.Location.Line <= 0 {
			return fmt.Errorf("investigation: read-source action is incomplete")
		}
	case ActionAssessSource:
		if a.AssessSource == nil {
			return fmt.Errorf("investigation: assess-source action has no bundle")
		}
		if err := a.AssessSource.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid assess-source action: %w", err)
		}
	case ActionFindTests:
		if a.FindTests == nil || strings.TrimSpace(a.FindTests.RepoPath) == "" {
			return fmt.Errorf("investigation: find-tests action is incomplete")
		}
		if err := a.FindTests.Structural.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid find-tests structural input: %w", err)
		}
		if err := sourceexplain.ValidateReport(a.FindTests.Assessment, a.FindTests.Report); err != nil {
			return fmt.Errorf("investigation: invalid find-tests assessment input: %w", err)
		}
	case ActionFindTestReferences:
		if a.FindTestReferences == nil || strings.TrimSpace(a.FindTestReferences.RepoPath) == "" {
			return fmt.Errorf("investigation: find-test-references action is incomplete")
		}
		if err := a.FindTestReferences.Structural.Validate(); err != nil {
			return fmt.Errorf("investigation: invalid find-test-references structural input: %w", err)
		}
	case ActionAwaitUser:
		if a.AwaitUser == nil || strings.TrimSpace(a.AwaitUser.Question) == "" || len(a.AwaitUser.Choices) == 0 {
			return fmt.Errorf("investigation: await-user action is incomplete")
		}
		seen := make(map[UserChoice]struct{}, len(a.AwaitUser.Choices))
		for _, choice := range a.AwaitUser.Choices {
			if !choice.valid() {
				return fmt.Errorf("investigation: await-user action has invalid choice %q", choice)
			}
			if _, exists := seen[choice]; exists {
				return fmt.Errorf("investigation: await-user action repeats choice %q", choice)
			}
			seen[choice] = struct{}{}
		}
	default:
		return fmt.Errorf("investigation: invalid action kind %q", a.Kind)
	}
	return nil
}

func (c UserChoice) valid() bool {
	return c == ChoiceInspectTests || c == ChoiceReadCallee || c == ChoiceFinish
}
