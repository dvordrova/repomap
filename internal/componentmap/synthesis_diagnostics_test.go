package componentmap

import (
	"errors"
	"testing"
)

func TestInspectSynthesisResponseShapeRetainsOnlyBoundedCardinalities(t *testing.T) {
	t.Parallel()

	shape := InspectSynthesisResponseShape([]byte(`{
  "subsystems": [
    {"name":"one","description":"x","components":[
      {"name":"a","description":"x","member_refs":["p1","p2"],"anchor_refs":[]},
      {"name":"b","description":"x","member_refs":["p3"]},
      {"name":"c","description":"x","unit_refs":["u1"],"anchor_refs":null},
      {"name":"d","description":"x","member_refs":["p4"],"anchor_refs":["a1","a2"]}
    ]}
  ]
}`))
	if !shape.JSONValid || shape.Grammar != "nested" || shape.SubsystemCount != 1 ||
		shape.ComponentCount != 4 || shape.MemberRefCount != 4 || shape.UnitRefCount != 1 ||
		shape.AnchorRefCount != 2 || shape.MissingAnchorRefsCount != 1 ||
		shape.EmptyAnchorRefsCount != 1 || shape.NullAnchorRefsCount != 1 {
		t.Fatalf("response shape = %#v", shape)
	}
}

func TestDiagnoseSynthesisFailureUsesClosedStableCause(t *testing.T) {
	t.Parallel()

	diagnostic := DiagnoseSynthesisFailure(errors.Join(
		errors.New("outer runtime context"),
		ErrPartialModelStateInconsistent,
	))
	if diagnostic.Stage != SynthesisFailureStageLandscapeValidation ||
		diagnostic.Code != SynthesisFailureCodePartialModelInconsistent || diagnostic.Detail == "" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	fallback := DiagnoseSynthesisFailure(errors.New("unbounded internal text must not be copied"))
	if fallback.Stage != SynthesisFailureStageResponseValidation ||
		fallback.Code != SynthesisFailureCodeResponseEvaluationFailed || fallback.Detail != "" {
		t.Fatalf("fallback diagnostic = %#v", fallback)
	}
}

func TestSynthesisResponseStateForDiagnosticsDistinguishesEmpty(t *testing.T) {
	t.Parallel()

	if got := SynthesisResponseStateForDiagnostics(nil); got != ResponseEmpty {
		t.Fatalf("empty diagnostic response state = %q, want %q", got, ResponseEmpty)
	}
	if got := SynthesisResponseStateForDiagnostics([]byte(`{}`)); got != ResponseCaptured {
		t.Fatalf("captured diagnostic response state = %q, want %q", got, ResponseCaptured)
	}
}
