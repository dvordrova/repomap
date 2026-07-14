package sourceexplain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fixedAssessor struct {
	response []byte
	err      error
	requests [][]byte
}

func (a *fixedAssessor) AssessSource(_ context.Context, bundleJSON []byte) ([]byte, error) {
	a.requests = append(a.requests, append([]byte{}, bundleJSON...))
	return append([]byte{}, a.response...), a.err
}

func TestServiceExplainsWithFixedAssessor(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	assessor := &fixedAssessor{response: []byte(validSourceResponse)}
	explanation, err := NewService(assessor).Explain(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if explanation.Evaluation.Score != 100 {
		t.Fatalf("score = %d, warnings = %#v", explanation.Evaluation.Score, explanation.Parsed.Warnings)
	}
	if len(assessor.requests) != 1 || !json.Valid(assessor.requests[0]) {
		t.Fatalf("requests = %#v", assessor.requests)
	}
}

func TestServiceReturnsAssessorError(t *testing.T) {
	t.Parallel()

	want := errors.New("provider failed")
	_, err := NewService(&fixedAssessor{err: want}).Explain(context.Background(), sourceBundleFixture(t))
	if !errors.Is(err, want) {
		t.Fatalf("Explain() error = %v, want %v", err, want)
	}
}

func TestServiceRetainsRawResponseWhenParsingFails(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"assessments":`)
	explanation, err := NewService(&fixedAssessor{response: raw}).Explain(context.Background(), sourceBundleFixture(t))
	if err == nil {
		t.Fatal("Explain() error = nil")
	}
	if string(explanation.Raw) != string(raw) {
		t.Fatalf("raw = %q, want %q", explanation.Raw, raw)
	}
}

func TestServiceRequiresAssessor(t *testing.T) {
	t.Parallel()

	if _, err := NewService(nil).Explain(context.Background(), sourceBundleFixture(t)); err == nil {
		t.Fatal("Explain() error = nil")
	}
}

func TestEvaluateSeparatesRecoveredContractFromValidFixture(t *testing.T) {
	t.Parallel()

	result, err := ParseReport(sourceBundleFixture(t), []byte(`{"assessments":[],"unknowns":[],"next_action_id":"bad"}`))
	if err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(result)
	if evaluation.Score >= evaluation.MaxScore {
		t.Fatalf("score = %d/%d, warnings = %#v", evaluation.Score, evaluation.MaxScore, result.Warnings)
	}
	if !hasMandatoryUnknowns(result.Report) {
		t.Fatalf("mandatory unknowns missing: %#v", result.Report.Unknowns)
	}
}
